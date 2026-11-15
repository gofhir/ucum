package fhir

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/gofhir/ucum/v4"
)

// Plan 022: con dos códigos, la sintaxis de los dos se mira antes de expandir
// ninguno. El orden de los errores es el de Convert: análisis del primero,
// análisis del segundo, expansión del primero, expansión del segundo. Se
// prueba por la identidad del error, sin reloj; los esperados salen de esa
// regla, no de correr el código.

type twoCodeCase struct {
	code   string
	expand bool // falla solo al expandirse: pasa Validate
	is     func(error) bool
}

func hasText(s string) func(error) bool {
	return func(err error) bool { return strings.Contains(err.Error(), s) }
}

var twoCodeCases = []twoCodeCase{
	{"m/", false, hasText("unexpected end of expression")},
	{"xyz", false, hasText(`unknown unit "xyz"`)},
	{"", false, hasText("UCUM expression is empty")},
	{"m{", false, hasText("unterminated annotation")},
	{"m/0", true, func(err error) bool { return errors.Is(err, ucum.ErrDivisionByZero) }},
	{"Cel2", true, hasText("cannot carry an exponent")},
	{"[pH]/m", true, hasText("is not on a linear scale")},
	{"[cy_a]", true, hasText("definition cycle")},
}

func twoCodeComparator(t *testing.T) *Comparator {
	t.Helper()
	raw, err := os.ReadFile("../internal/essence/ucum-essence.xml")
	if err != nil {
		t.Fatal(err)
	}
	const extra = ` <unit Code="[cy_a]" CODE="[CY_A]" isMetric="no" class="dimless">
  <name>a</name><printSymbol>a</printSymbol><property>test</property>
  <value Unit="[cy_b]" UNIT="[CY_B]" value="1">a</value>
 </unit>
 <unit Code="[cy_b]" CODE="[CY_B]" isMetric="no" class="dimless">
  <name>b</name><printSymbol>b</printSymbol><property>test</property>
  <value Unit="[cy_a]" UNIT="[CY_A]" value="1">b</value>
 </unit>
`
	xml := strings.Replace(string(raw), "</root>", extra+"</root>", 1)
	svc, err := ucum.NewFromReader(strings.NewReader(xml))
	if err != nil {
		t.Fatal(err)
	}
	ex, ok := svc.(ucum.ExactService)
	if !ok {
		t.Fatal("service is not an ExactService")
	}
	for _, c := range twoCodeCases {
		if c.expand != (svc.Validate(c.code) == nil) {
			t.Fatalf("%q: expand = %v but Validate = %v", c.code, c.expand, svc.Validate(c.code))
		}
	}
	return NewComparatorWith(ex)
}

func TestTwoCodeErrorOrder(t *testing.T) {
	c := twoCodeComparator(t)
	d, err := ParseDecimal("1")
	if err != nil {
		t.Fatal(err)
	}
	entries := []struct {
		name string
		call func(a, b string) error
		// swapped: el error sigue el orden de la conversión de b a a, la de
		// ConversionFactor(b, a) (Add y Sub).
		swapped bool
	}{
		{"Compare", func(a, b string) error {
			_, err := c.Compare(Quantity{Value: 1, Code: a}, Quantity{Value: 1, Code: b})
			return err
		}, false},
		{"Add", func(a, b string) error {
			_, err := c.Add(Quantity{Value: 1, Code: a}, Quantity{Value: 1, Code: b})
			return err
		}, true},
		{"Sub", func(a, b string) error {
			_, err := c.Sub(Quantity{Value: 1, Code: a}, Quantity{Value: 1, Code: b})
			return err
		}, true},
		{"ConvertDecimal", func(a, b string) error { _, err := c.ConvertDecimal(d, a, b); return err }, false},
		{"Comparable", func(a, b string) error {
			_, err := c.Comparable(Quantity{Value: 1, Code: a}, Quantity{Value: 1, Code: b})
			return err
		}, false},
	}
	for _, e := range entries {
		for _, a := range twoCodeCases {
			for _, b := range twoCodeCases {
				// Con el primero que falla solo al expandirse y el segundo ya al
				// analizarse, el error es el del segundo; en los demás, el del primero.
				first, second := a, b
				if e.swapped {
					first, second = b, a
				}
				want := first
				if first.expand && !second.expand {
					want = second
				}
				err := e.call(a.code, b.code)
				if err == nil {
					t.Errorf("%s(%q, %q) = nil, want the error of %q", e.name, a.code, b.code, want.code)
					continue
				}
				for _, other := range twoCodeCases {
					if got := other.is(err); got != (other.code == want.code) {
						t.Errorf("%s(%q, %q): error %q identifies %q = %v, want the error of %q",
							e.name, a.code, b.code, err, other.code, got, want.code)
					}
				}
			}
		}
	}
}

// Add y Sub deciden ErrNotLinear sin expandir el otro código.
func TestAddSubNotLinearWithoutExpanding(t *testing.T) {
	c := twoCodeComparator(t)
	for _, codes := range [][2]string{{"Cel", "m/0"}, {"m/0", "Cel"}} {
		a, b := Quantity{Value: 1, Code: codes[0]}, Quantity{Value: 1, Code: codes[1]}
		if _, err := c.Add(a, b); !errors.Is(err, ucum.ErrNotLinear) {
			t.Errorf("Add(%q, %q) = %v, want ErrNotLinear", codes[0], codes[1], err)
		}
		if _, err := c.Sub(a, b); !errors.Is(err, ucum.ErrNotLinear) {
			t.Errorf("Sub(%q, %q) = %v, want ErrNotLinear", codes[0], codes[1], err)
		}
	}
}
