package fhir_test

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/gofhir/ucum/v4"
	"github.com/gofhir/ucum/v4/fhir"
)

// Plan 018, 4.4: el exponente canónico que desborda int es un error también en
// la capa FHIR. UCUM §9 ■1 («The exponent is an integer number») no lo acota;
// §18 ■3-■4 lo definen por multiplicación y suma de los exponentes de la cadena; y
// §19 ■1 permite no representar alguna, no dar otra distinta. Las
// cadenas y sus esperados son los de expoverflow_test.go del paquete raíz:
// [ovF_i] = plantilla([ovF_{i+1}]), valor 1, hoja 1 m, [ovF_k] con 64-k niveles.
//
//	s: X2   [ovs_2] = m^(2^62), [ovs_1] = 2^63 (no cabe), [ovs_0] = 2^64 (daba 0)
//	d: X.X  lo mismo por suma
//	n: X-2  [ovn_1] = -2^63 = math.MinInt (cabe); su opuesto 2^63 no
//
// 2^62 = 4611686018427387904 y -2^63 = -9223372036854775808.

const (
	ovLevels = 64
	ovSystem = "http://unitsofmeasure.org"
	ovTwo62  = "4611686018427387904"
	ovMinInt = "-9223372036854775808"
)

func ovChain(family, tmpl string) string {
	var b strings.Builder
	for i := 0; i <= ovLevels; i++ {
		unit := "m"
		if i < ovLevels {
			unit = strings.ReplaceAll(tmpl, "X", fmt.Sprintf("[ov%s_%d]", family, i+1))
		}
		fmt.Fprintf(&b, ` <unit Code="[ov%[1]s_%[2]d]" CODE="[OV%[3]s_%[2]d]" isMetric="no" class="dimless">
  <name>ov</name><printSymbol>ov</printSymbol><property>test</property>
  <value Unit="%[4]s" UNIT="%[5]s" value="1">ov</value>
 </unit>
`, family, i, strings.ToUpper(family), unit, strings.ToUpper(unit))
	}
	return b.String()
}

var (
	ovOnce sync.Once
	ovCS   *fhir.Comparator
	ovCI   *fhir.Comparator
	errOv  error
)

func ovComparators(t testing.TB) (cs, ci *fhir.Comparator) {
	t.Helper()
	ovOnce.Do(func() {
		raw, err := os.ReadFile("../internal/essence/ucum-essence.xml")
		if err != nil {
			errOv = err
			return
		}
		extra := ovChain("s", "X2") + ovChain("d", "X.X") + ovChain("n", "X-2")
		xml := strings.Replace(string(raw), "</root>", extra+"</root>", 1)
		svc, err := ucum.NewFromReader(strings.NewReader(xml))
		if err != nil {
			errOv = err
			return
		}
		ovCS = fhir.NewComparatorWith(svc.(ucum.ExactService))
		svcCI, err := ucum.NewCaseInsensitiveFromReader(strings.NewReader(xml))
		if err != nil {
			errOv = err
			return
		}
		ovCI = fhir.NewComparatorWith(svcCI.(ucum.ExactService))
	})
	if errOv != nil {
		t.Fatalf("building the comparators: %v", errOv)
	}
	return ovCS, ovCI
}

type ovCase struct {
	name, code, want string // want == "" : ErrExponentTooLarge
}

func ovCases() []ovCase {
	return []ovCase{
		{"square 62 levels", "[ovs_2]", "m" + ovTwo62},
		{"square 63 levels", "[ovs_1]", ""},
		{"square 64 levels", "[ovs_0]", ""},
		{"product 62 levels", "[ovd_2]", "m" + ovTwo62},
		{"product 63 levels", "[ovd_1]", ""},
		{"product 64 levels", "[ovd_0]", ""},
		{"negative square 62 levels", "[ovn_2]", "m" + ovTwo62},
		{"negative square 63 levels reaches MinInt", "[ovn_1]", "m" + ovMinInt},
		{"negative square 64 levels", "[ovn_0]", ""},
		{"quotient negates MinInt", "/[ovn_1]", ""},
		{"caller squares 2^62", "[ovs_2]2", ""},
		{"caller multiplies 2^62 by itself", "[ovs_2].[ovs_2]", ""},
		{"caller divides 2^62 by m", "[ovs_2]/m", "m4611686018427387903"},
	}
}

func TestExpOverflowFHIR(t *testing.T) {
	cs, ci := ovComparators(t)
	one := fhir.Quantity{Value: 1, Code: "1", System: ovSystem}
	dec, err := fhir.ParseDecimal("1")
	if err != nil {
		t.Fatal(err)
	}
	for label, cmp := range map[string]*fhir.Comparator{"cs": cs, "ci": ci} {
		for _, c := range ovCases() {
			code := c.code
			if label == "ci" {
				code = strings.ToUpper(code)
			}
			t.Run(label+"/"+c.name, func(t *testing.T) {
				q := fhir.Quantity{Value: 1, Code: code, System: ovSystem}
				key, val, kerr := cmp.CanonicalKey(q)
				_, cerr := cmp.Compare(q, one)
				_, derr := cmp.ConvertDecimal(dec, code, "1")
				same, serr := cmp.Comparable(q, one)
				if c.want == "" {
					for api, e := range map[string]error{
						"CanonicalKey": kerr, "Compare": cerr, "ConvertDecimal": derr, "Comparable": serr,
					} {
						if !errors.Is(e, ucum.ErrExponentTooLarge) {
							t.Errorf("%s(%q) error = %v, want ErrExponentTooLarge", api, code, e)
						}
					}
					if same {
						t.Errorf("Comparable(%q, 1) = true, want false", code)
					}
					return
				}
				if kerr != nil {
					t.Fatalf("CanonicalKey(%q) error = %v", code, kerr)
				}
				if !strings.EqualFold(key, c.want) || val != 1 {
					t.Errorf("CanonicalKey(%q) = %q, %v, want %q, 1", code, key, val, c.want)
				}
				// Distinta dimensión que «1»: Compare dice «no comparable», no un
				// error de exponente.
				if errors.Is(cerr, ucum.ErrExponentTooLarge) || errors.Is(derr, ucum.ErrExponentTooLarge) {
					t.Errorf("%q: spurious ErrExponentTooLarge: %v / %v", code, cerr, derr)
				}
			})
		}
	}
}
