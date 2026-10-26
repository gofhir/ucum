package fhir_test

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/gofhir/ucum/v4"
	"github.com/gofhir/ucum/v4/fhir"
)

// Plan 001, test 7: cantidades UCUM de los ejemplos oficiales de HL7 en R4,
// R4B y R5. Los packages se descargan con scripts/fetch-packages.sh, que
// verifica su SHA-256 contra scripts/packages.sha256; make check-packages
// corre este test con UCUM_PACKAGES apuntando a esa carpeta.

var examplePackages = []struct{ version, file string }{
	{"R4", "hl7.fhir.r4.examples-4.0.1.tgz"},
	{"R4B", "hl7.fhir.r4b.examples-4.3.0.tgz"},
	{"R5", "hl7.fhir.r5.examples-5.0.0.tgz"},
}

type exampleQuantity struct{ value, code string }

// exampleQuantities lee cada recurso JSON del package y junta los pares
// (valor, código) distintos de todo objeto con system UCUM. El valor se guarda
// como el texto del JSON (json.Number), que es el decimal exacto.
func exampleQuantities(t *testing.T, path string) []exampleQuantity {
	t.Helper()
	skipped := 0
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[exampleQuantity]bool{}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		base := filepath.Base(h.Name)
		if !strings.HasSuffix(base, ".json") || base == "package.json" || strings.HasPrefix(base, ".") {
			continue
		}
		dec := json.NewDecoder(tr)
		dec.UseNumber()
		var doc any
		if err := dec.Decode(&doc); err != nil {
			skipped++
			continue
		}
		walkQuantities(doc, seen)
	}
	t.Logf("%s: %d JSON files that do not decode skipped", filepath.Base(path), skipped)
	out := make([]exampleQuantity, 0, len(seen))
	for q := range seen {
		out = append(out, q)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].code != out[j].code {
			return out[i].code < out[j].code
		}
		return out[i].value < out[j].value
	})
	return out
}

func walkQuantities(x any, seen map[exampleQuantity]bool) {
	switch v := x.(type) {
	case map[string]any:
		if v["system"] == fhir.UCUMSystem {
			num, okNum := v["value"].(json.Number)
			code, okCode := v["code"].(string)
			if okNum && okCode {
				seen[exampleQuantity{num.String(), code}] = true
			}
		}
		for _, e := range v {
			walkQuantities(e, seen)
		}
	case []any:
		for _, e := range v {
			walkQuantities(e, seen)
		}
	}
}

func TestOfficialExamples(t *testing.T) {
	dir := os.Getenv("UCUM_PACKAGES")
	if dir == "" {
		t.Skip("UCUM_PACKAGES not set; run make check-packages")
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

	for _, pkg := range examplePackages {
		t.Run(pkg.version, func(t *testing.T) {
			qs := exampleQuantities(t, filepath.Join(dir, pkg.file))
			if len(qs) < 200 {
				t.Fatalf("%d distinct UCUM quantities, want at least 200", len(qs))
			}
			invalid, special := map[string]bool{}, map[string]bool{}
			scaled, paired, fails := 0, 0, 0
			for _, q := range qs {
				v, ok := new(big.Rat).SetString(q.value)
				if !ok {
					t.Errorf("value %q is not a decimal", q.value)
					continue
				}
				// Test 8: con solo Value, el resultado es el de v4.3.0.
				f, _ := v.Float64()
				want, err := svc.Canonical(f, q.code)
				if err != nil {
					invalid[q.code] = true
					continue
				}
				code, got, err := c.CanonicalKey(fhir.Quantity{Value: f, Code: q.code})
				if err != nil || code != want.Code || got != want.Value {
					t.Errorf("CanonicalKey(Value=%v %s) = (%q, %v, %v), want (%q, %v) as in v4.3.0",
						f, q.code, code, got, err, want.Code, want.Value)
				}

				_, err = svc.ConversionFactor(q.code, q.code)
				if errors.Is(err, ucum.ErrNotLinear) {
					special[q.code] = true
					continue
				}
				if err != nil {
					t.Errorf("ConversionFactor(%q): %v", q.code, err)
					continue
				}
				ok = checkScale(t, c, v, q.code)
				scaled++
				// Hasta tres unidades comunes con el mismo canónico.
				partners := 0
				for _, u2 := range byCanonical[want.Code] {
					if partners == 3 {
						break
					}
					if u2 == q.code {
						continue
					}
					ok = checkPair(t, c, svc, v, q.code, u2) && ok
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
