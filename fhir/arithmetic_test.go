package fhir

import (
	"errors"
	"math/big"
	"testing"

	"github.com/gofhir/ucum/v4"
)

// FHIRPath defines addition and subtraction over quantities, and says that
// "implementations that do support units shall do so as specified by [UCUM]".
// The unit of the result is the unit of the left operand; the right one is
// converted to it.

func TestAddAndSubConvertTheRightOperand(t *testing.T) {
	c, err := NewComparator()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		a, b     Quantity
		wantSum  string
		wantDiff string
		wantCode string
	}{
		{
			name:    "same unit",
			a:       Quantity{Value: 1, Code: "m"},
			b:       Quantity{Value: 2, Code: "m"},
			wantSum: "3", wantDiff: "-1", wantCode: "m",
		},
		{
			name:    "the right operand is converted",
			a:       Quantity{Value: 1, Code: "m"},
			b:       Quantity{Value: 50, Code: "cm"},
			wantSum: "3/2", wantDiff: "1/2", wantCode: "m",
		},
		{
			name:    "and the result keeps the left unit, not the canonical one",
			a:       Quantity{Value: 50, Code: "cm"},
			b:       Quantity{Value: 1, Code: "m"},
			wantSum: "150", wantDiff: "-50", wantCode: "cm",
		},
		{
			name:    "a clinical case, exactly",
			a:       Quantity{Value: 1, Code: "mg/dL"},
			b:       Quantity{Value: 1, Code: "g/L"},
			wantSum: "101", wantDiff: "-99", wantCode: "mg/dL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sum, err := c.Add(tt.a, tt.b)
			if err != nil {
				t.Fatalf("Add: %v", err)
			}
			if got := sum.Exact.RatString(); got != tt.wantSum {
				t.Errorf("Add = %s, want %s", got, tt.wantSum)
			}
			if sum.Code != tt.wantCode {
				t.Errorf("Add unit = %q, want %q", sum.Code, tt.wantCode)
			}

			diff, err := c.Sub(tt.a, tt.b)
			if err != nil {
				t.Fatalf("Sub: %v", err)
			}
			if got := diff.Exact.RatString(); got != tt.wantDiff {
				t.Errorf("Sub = %s, want %s", got, tt.wantDiff)
			}
			if diff.Code != tt.wantCode {
				t.Errorf("Sub unit = %q, want %q", diff.Code, tt.wantCode)
			}
		})
	}
}

// TestAddIsExact is the reason this goes through the rational API: 1/10 is not a
// binary fraction, so ten of them added as float64 do not make 1.
func TestAddIsExact(t *testing.T) {
	c, err := NewComparator()
	if err != nil {
		t.Fatal(err)
	}

	tenth, err := ParseDecimal("0.1")
	if err != nil {
		t.Fatal(err)
	}
	total := Quantity{Exact: new(big.Rat), Code: "L"}
	for i := 0; i < 10; i++ {
		total, err = c.Add(total, Quantity{Exact: tenth.Rat(), Code: "L"})
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := total.Exact.RatString(); got != "1" {
		t.Errorf("ten tenths of a liter = %s, want exactly 1", got)
	}
}

// TestAddRejectsNonRatioScales pins the refusal. Adding two points on an affine
// scale gives an answer that depends on the unit it is done in — 20 Cel + 20 Cel
// is 40 Cel, but the same sum carried out in kelvin is 313.15 Cel — so there is
// no right answer to return and the operation is refused instead.
func TestAddRejectsNonRatioScales(t *testing.T) {
	c, err := NewComparator()
	if err != nil {
		t.Fatal(err)
	}

	pairs := [][2]Quantity{
		{{Value: 20, Code: "Cel"}, {Value: 20, Code: "Cel"}},
		{{Value: 20, Code: "Cel"}, {Value: 293, Code: "K"}},
		{{Value: 20, Code: "K"}, {Value: 20, Code: "Cel"}},
		{{Value: 7, Code: "[pH]"}, {Value: 1, Code: "[pH]"}},
		{{Value: 3, Code: "B"}, {Value: 3, Code: "B"}},
	}
	for _, p := range pairs {
		if _, err := c.Add(p[0], p[1]); !errors.Is(err, ucum.ErrNotLinear) {
			t.Errorf("Add(%s, %s) = %v, want ErrNotLinear", p[0].Code, p[1].Code, err)
		}
		if _, err := c.Sub(p[0], p[1]); !errors.Is(err, ucum.ErrNotLinear) {
			t.Errorf("Sub(%s, %s) = %v, want ErrNotLinear", p[0].Code, p[1].Code, err)
		}
	}

	// A temperature *difference* is still available through Convert, which is
	// what the refusal points a caller at.
	svc, err := ucum.New()
	if err != nil {
		t.Fatal(err)
	}
	if got, err := svc.Convert(1, "Cel/min", "K/min"); err != nil || got != 1 {
		t.Errorf(`Convert(1, "Cel/min", "K/min") = %v, %v; want 1, nil`, got, err)
	}
}

func TestAddRejectsIncommensurableAndForeignSystems(t *testing.T) {
	c, err := NewComparator()
	if err != nil {
		t.Fatal(err)
	}

	if _, err := c.Add(Quantity{Value: 1, Code: "m"}, Quantity{Value: 1, Code: "s"}); err == nil {
		t.Error("Add(m, s) = nil error, want one: the units are not comparable")
	}
	if _, err := c.Add(Quantity{Value: 1, Code: "m"}, Quantity{Value: 1, Code: "nope"}); err == nil {
		t.Error("Add with an invalid code = nil error, want one")
	}

	snomed := Quantity{Value: 1, Code: "m", System: "http://snomed.info/sct"}
	if _, err := c.Add(snomed, Quantity{Value: 1, Code: "m"}); !errors.Is(err, ErrNotUCUMSystem) {
		t.Errorf("Add with a non-UCUM system = %v, want ErrNotUCUMSystem", err)
	}
}

// TestAddPrefersExactOverValue checks that the Exact field wins over Value, the
// same rule Compare follows, since that is where a FHIR decimal survives.
func TestAddPrefersExactOverValue(t *testing.T) {
	c, err := NewComparator()
	if err != nil {
		t.Fatal(err)
	}

	exact, _ := new(big.Rat).SetString("0.01")
	a := Quantity{Value: 999, Exact: exact, Code: "g/L"}
	b := Quantity{Value: 999, Exact: exact, Code: "g/L"}

	sum, err := c.Add(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if got := sum.Exact.RatString(); got != "1/50" {
		t.Errorf("Add = %s, want 1/50: Value must be ignored when Exact is set", got)
	}
}
