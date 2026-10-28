package fhir_test

import (
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"

	"github.com/gofhir/ucum/v4"
	"github.com/gofhir/ucum/v4/fhir"
	"github.com/gofhir/ucum/v4/internal/fhirexamples"
)

// Plan 001, test 7: cantidades UCUM de los ejemplos oficiales de HL7 en R4,
// R4B y R5. Los packages se descargan con scripts/fetch-packages.sh, que
// verifica su SHA-256 contra scripts/packages.sha256; make check-packages
// corre este test con UCUM_PACKAGES apuntando a esa carpeta.

// exampleQuantities lee las cantidades UCUM distintas del package; un JSON
// que no decodifica es un error.
func exampleQuantities(t *testing.T, path string) []fhirexamples.Quantity {
	t.Helper()
	qs, undecodable, err := fhirexamples.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if undecodable > 0 {
		t.Errorf("%s: %d JSON files do not decode", filepath.Base(path), undecodable)
	}
	return qs
}

func TestOfficialExamples(t *testing.T) {
	dir := os.Getenv("UCUM_PACKAGES")
	if dir == "" {
		t.Skip("UCUM_PACKAGES not set; run make check-packages")
	}
	if fhirexamples.System != fhir.UCUMSystem {
		t.Fatalf("fhirexamples.System = %q, want %q", fhirexamples.System, fhir.UCUMSystem)
	}
	c, svc := newComparator(t)
	linearCommon, _ := linearCodes(t, svc, fhir.CommonCodes())
	byCanonical := map[string][]string{}
	for _, u := range linearCommon {
		p, err := svc.CanonicalRat(big.NewRat(1, 1), u)
		if err != nil {
			t.Fatal(err)
		}
		byCanonical[p.Code] = append(byCanonical[p.Code], u)
	}

	for _, pkg := range fhirexamples.Packages {
		t.Run(pkg.Version, func(t *testing.T) {
			qs := exampleQuantities(t, filepath.Join(dir, pkg.File))
			if len(qs) < 200 {
				t.Fatalf("%d distinct UCUM quantities, want at least 200", len(qs))
			}
			invalid, special := map[string]bool{}, map[string]bool{}
			scaled, paired, fails := 0, 0, 0
			for _, q := range qs {
				v, ok := new(big.Rat).SetString(q.Value)
				if !ok {
					t.Errorf("value %q is not a decimal", q.Value)
					continue
				}
				// Test 8: con solo Value, el resultado es el de v4.3.0.
				f, _ := v.Float64()
				want, err := svc.Canonical(f, q.Code)
				if err != nil {
					invalid[q.Code] = true
					continue
				}
				code, got, err := c.CanonicalKey(fhir.Quantity{Value: f, Code: q.Code})
				if err != nil || code != want.Code || got != want.Value {
					t.Errorf("CanonicalKey(Value=%v %s) = (%q, %v, %v), want (%q, %v) as in v4.3.0",
						f, q.Code, code, got, err, want.Code, want.Value)
				}

				_, err = svc.ConversionFactor(q.Code, q.Code)
				if errors.Is(err, ucum.ErrNotLinear) {
					special[q.Code] = true
					continue
				}
				if err != nil {
					t.Errorf("ConversionFactor(%q): %v", q.Code, err)
					continue
				}
				ok = checkScale(t, c, v, q.Code)
				scaled++
				// Hasta tres unidades comunes con el mismo canónico.
				partners := 0
				for _, u2 := range byCanonical[want.Code] {
					if partners == 3 {
						break
					}
					if u2 == q.Code {
						continue
					}
					ok = checkPair(t, c, svc, v, q.Code, u2) && ok
					partners++
					paired++
				}
				if !ok {
					fails++
					if fails > stopAfter {
						t.Fatal("too many failures")
					}
				}
			}
			t.Logf("%d distinct quantities: %d scaled, %d cross-unit pairs, %d special codes skipped %v, %d invalid codes skipped %v",
				len(qs), scaled, paired, len(special), sortedKeys(special), len(invalid), sortedKeys(invalid))
			// Plan 004, test 5: los inválidos se fijan, para que una regresión del
			// lexer no pase como un código más saltado. El de R5 tiene un espacio
			// dentro de llaves, que UCUM §6■1 no admite.
			want, ok := wantInvalidExamples[pkg.Version]
			if !ok {
				t.Fatalf("no expected invalid codes for %s", pkg.Version)
			}
			if got := sortedKeys(invalid); !slices.Equal(got, want) {
				t.Errorf("invalid codes %q, want %q", got, want)
			}
		})
	}
}

// wantInvalidExamples son los códigos de los ejemplos que no son UCUM válido,
// por versión.
var wantInvalidExamples = map[string][]string{
	// Ninguno es un átomo de ucum-essence.xml, y Ucum-java 1.0.10 rechaza los
	// ocho por desconocidos (internal/engine/testdata/java, casos fhir:00038,
	// 00120, 00122, 00160, 00188, 00241, 00260 y 00266).
	"R4":  {"TAB", "drop", "ea", "mEq", "mcg", "ml/hr", "month", "tbl"},
	"R4B": {},
	"R5":  {"{delivered dose}"}, // Ingredient-example
}

// Plan 004, test 4: el ValueSet ucum-common de cada versión. El oráculo no es
// el lexer sino un escaneo de bytes. UCUM §6 da una dirección: un byte fuera
// de 33–126 dentro de llaves (§6■1) o una llave dentro de otra (§6■5) hacen
// inválido el código. La otra, que todo concepto sin eso es válido, es una
// propiedad de estos datos, y el test la verifica. Las cuentas son las del recurso publicado.
func TestCommonUnitsValueSetVersions(t *testing.T) {
	dir := os.Getenv("UCUM_PACKAGES")
	if dir == "" {
		t.Skip("UCUM_PACKAGES not set; run make check-packages")
	}
	svc, err := ucum.New()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]struct{ concepts, outside int }{
		"R4":  {1364, 11},
		"R4B": {1364, 11},
		"R5":  {848, 0},
	}
	for _, pkg := range fhirexamples.Packages {
		t.Run(pkg.Version, func(t *testing.T) {
			codes, err := fhirexamples.ValueSetCodes(filepath.Join(dir, pkg.File), "ValueSet-ucum-common.json")
			if err != nil {
				t.Fatal(err)
			}
			outside := 0
			for _, code := range codes {
				bad := annotationBreaksSection6(code)
				if bad {
					outside++
				}
				if err := svc.Validate(code); (err != nil) != bad {
					t.Errorf("Validate(%q) = %v; §6■1 says invalid = %t", code, err, bad)
				}
			}
			if w := want[pkg.Version]; len(codes) != w.concepts || outside != w.outside {
				t.Errorf("%d concepts, %d breaking UCUM §6 in an annotation; want %d, %d",
					len(codes), outside, w.concepts, w.outside)
			}
		})
	}
}

// annotationBreaksSection6 informa si una anotación rompe UCUM §6: un byte
// entre { y } fuera de 33–126 (§6■1), o una { dentro de otra (§6■5, plan
// 005). No usa el lexer: es el oráculo del test 4.
func annotationBreaksSection6(code string) bool {
	inside := false
	for i := 0; i < len(code); i++ {
		switch c := code[i]; {
		case c == '{' && inside:
			return true
		case c == '{':
			inside = true
		case c == '}':
			inside = false
		case inside && (c < 33 || c > 126):
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// El oráculo del test 4 también se prueba: un byte fuera de las llaves no
// cuenta.
func TestAnnotationBreaksSection6(t *testing.T) {
	for _, tt := range []struct {
		code string
		want bool
	}{
		{"{a b}", true},
		{"m{a\x7f}", true},
		{"m{ab}", false},
		{"m{a}.s b", false}, // el espacio está después de la anotación
		{"m b", false},
		{"m{a{b}", true},    // §6■5
		{"m{a}.{b}", false}, // dos anotaciones, ninguna dentro de otra
	} {
		if got := annotationBreaksSection6(tt.code); got != tt.want {
			t.Errorf("annotationBreaksSection6(%q) = %t, want %t", tt.code, got, tt.want)
		}
	}
}
