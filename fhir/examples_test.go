package fhir_test

import (
	"errors"
	"math/big"
	"os"
	"path/filepath"
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
		})
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
