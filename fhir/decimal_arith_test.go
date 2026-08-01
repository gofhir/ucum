package fhir_test

import (
	"math/big"
	"testing"

	"github.com/gofhir/ucum/v3/fhir"
)

func mustDec(t *testing.T, s string) fhir.Decimal {
	t.Helper()
	d, err := fhir.ParseDecimal(s)
	if err != nil {
		t.Fatalf("ParseDecimal(%q): %v", s, err)
	}
	return d
}

// Multiplication and division carry the significant figures of the least precise
// operand: a product is no better known than its worst-known factor.
func TestDecimalMulDivPrecision(t *testing.T) {
	tests := []struct {
		a, b    string
		mul     string
		mulFigs int
	}{
		{"1.5", "2.0", "3.0", 2},
		{"1.50", "2.0", "3.0", 2}, // the weaker operand governs
		{"1.50", "2.00", "3.00", 3},
		{"2.5", "4.0", "10", 2}, // 10.0 at 2 figures is "10"
		{"1.234", "5.6", "6.9", 2},
		{"0.010", "3.0", "0.030", 2},
	}
	for _, tt := range tests {
		got := mustDec(t, tt.a).Mul(mustDec(t, tt.b))
		if got.SignificantFigures() != tt.mulFigs {
			t.Errorf("%s * %s has %d significant figures, want %d",
				tt.a, tt.b, got.SignificantFigures(), tt.mulFigs)
		}
		if got.String() != tt.mul {
			t.Errorf("%s * %s = %q, want %q", tt.a, tt.b, got.String(), tt.mul)
		}
	}
}

// Addition and subtraction carry decimal places, not significant figures: a sum
// is only as resolved as its coarsest addend. This is the rule the Java
// reference's own TODO says it does not implement.
func TestDecimalAddSubPrecision(t *testing.T) {
	tests := []struct {
		a, b, want string
	}{
		{"1.23", "4.5", "5.7"},      // one decimal place, not three figures
		{"1.5", "2.25", "3.8"},      // one decimal place
		{"100.0", "1.234", "101.2"}, // one decimal place
		{"0.10", "0.005", "0.11"},   // two decimal places
	}
	for _, tt := range tests {
		got := mustDec(t, tt.a).Add(mustDec(t, tt.b))
		if got.String() != tt.want {
			t.Errorf("%s + %s = %q, want %q", tt.a, tt.b, got.String(), tt.want)
		}
	}
	sub := mustDec(t, "5.75").Sub(mustDec(t, "1.2"))
	if sub.String() != "4.6" {
		t.Errorf(`5.75 - 1.2 = %q, want "4.6"`, sub.String())
	}
}

// An exactly known operand does not limit the result: multiplying by a count or
// an exact conversion factor neither adds nor removes precision.
func TestDecimalExactOperandDoesNotLimit(t *testing.T) {
	measured := mustDec(t, "1.50") // 3 significant figures
	exact := mustDec(t, "3")       // written without a point: unlimited

	got := measured.Mul(exact)
	if got.SignificantFigures() != 3 {
		t.Errorf("1.50 * 3 has %d significant figures, want 3", got.SignificantFigures())
	}
	if got.String() != "4.50" {
		t.Errorf(`1.50 * 3 = %q, want "4.50"`, got.String())
	}

	sum := measured.Add(exact)
	if sum.String() != "4.50" {
		t.Errorf(`1.50 + 3 = %q, want "4.50"`, sum.String())
	}
}

// The value underneath stays exact whatever the reported precision.
func TestDecimalArithmeticIsExactUnderneath(t *testing.T) {
	a := mustDec(t, "1.0")
	b := mustDec(t, "3.0")
	q, err := a.Div(b)
	if err != nil {
		t.Fatal(err)
	}
	// One third has no finite decimal form; the rational holds it exactly.
	if q.Rat().RatString() != "1/3" {
		t.Errorf("1.0 / 3.0 = %s, want 1/3 exactly", q.Rat().RatString())
	}
	if q.String() != "0.33" {
		t.Errorf(`1.0 / 3.0 renders as %q, want "0.33" at two significant figures`, q.String())
	}
}

func TestDecimalDivByZero(t *testing.T) {
	if _, err := mustDec(t, "1.0").Div(mustDec(t, "0.0")); err == nil {
		t.Error("Div by zero = nil error, want an error")
	}
}

// A precision coarser than the integer part cannot be written in plain decimal
// notation — "150" says nothing about whether the trailing zero is significant —
// so it is rendered in scientific notation, which can.
func TestDecimalScientificWhenPlainCannotExpress(t *testing.T) {
	tests := []struct {
		value   string
		sigFigs int
		want    string
	}{
		{"150", 2, "1.5e2"},
		{"150", 1, "2e2"}, // rounds to one figure
		{"150", 3, "150"}, // plain notation suffices
		{"1500", 2, "1.5e3"},
		{"12345", 3, "1.23e4"},
		{"0.00123", 2, "0.0012"}, // small values need no exponent
		{"-1500", 2, "-1.5e3"},
	}
	for _, tt := range tests {
		v, ok := new(big.Rat).SetString(tt.value)
		if !ok {
			t.Fatalf("bad literal %q", tt.value)
		}
		got := fhir.NewDecimal(v, tt.sigFigs).String()
		if got != tt.want {
			t.Errorf("%s at %d significant figures = %q, want %q",
				tt.value, tt.sigFigs, got, tt.want)
		}
	}
}

// Whatever the rendering, the declared precision is still readable, and a
// rendered value parses back to the same precision.
func TestDecimalScientificRoundTrip(t *testing.T) {
	v, _ := new(big.Rat).SetString("150")
	d := fhir.NewDecimal(v, 2)
	if d.SignificantFigures() != 2 {
		t.Errorf("SignificantFigures() = %d, want 2", d.SignificantFigures())
	}
	back, err := fhir.ParseDecimal(d.String())
	if err != nil {
		t.Fatalf("ParseDecimal(%q): %v", d.String(), err)
	}
	if back.SignificantFigures() != 2 {
		t.Errorf("re-parsed %q has %d significant figures, want 2", d.String(), back.SignificantFigures())
	}
	if back.Rat().Cmp(v) != 0 {
		t.Errorf("re-parsed %q holds %s, want 150", d.String(), back.Rat().RatString())
	}
}

// Rounding the mantissa can carry it to 10, which belongs in the exponent.
func TestDecimalScientificCarry(t *testing.T) {
	for _, tt := range []struct {
		value   string
		sigFigs int
		want    string
	}{
		{"996", 2, "1.0e3"},
		{"9996", 3, "1.00e4"},
		{"999", 1, "1e3"},
	} {
		v, _ := new(big.Rat).SetString(tt.value)
		if got := fhir.NewDecimal(v, tt.sigFigs).String(); got != tt.want {
			t.Errorf("%s at %d figures = %q, want %q", tt.value, tt.sigFigs, got, tt.want)
		}
	}
}

// Unlimited precision propagates as "no constraint" through every operation.
func TestDecimalUnlimitedPrecisionPropagation(t *testing.T) {
	exactA := mustDec(t, "6")
	exactB := mustDec(t, "7")
	measured := mustDec(t, "2.5")

	// Two exact operands stay exact.
	if got := exactA.Mul(exactB); got.SignificantFigures() != 0 || got.String() != "42" {
		t.Errorf("6 * 7 = %q with %d figures, want \"42\" and unlimited",
			got.String(), got.SignificantFigures())
	}
	if got := exactA.Add(exactB); got.SignificantFigures() != 0 || got.String() != "13" {
		t.Errorf("6 + 7 = %q with %d figures, want \"13\" and unlimited",
			got.String(), got.SignificantFigures())
	}
	// One exact operand does not constrain the other, in either family.
	if got := exactA.Mul(measured); got.SignificantFigures() != 2 {
		t.Errorf("6 * 2.5 has %d figures, want 2", got.SignificantFigures())
	}
	if got := exactA.Sub(measured); got.String() != "3.5" {
		t.Errorf(`6 - 2.5 = %q, want "3.5"`, got.String())
	}
	// Division by an exact operand likewise.
	q, err := measured.Div(exactA)
	if err != nil {
		t.Fatal(err)
	}
	if q.SignificantFigures() != 2 {
		t.Errorf("2.5 / 6 has %d figures, want 2", q.SignificantFigures())
	}
}

// A difference can be coarser than its own leading digit, where one significant
// figure is the least that can honestly be reported.
func TestDecimalSubtractionLosingPrecision(t *testing.T) {
	got := mustDec(t, "0.4").Sub(mustDec(t, "0.35"))
	if got.SignificantFigures() < 1 {
		t.Errorf("0.4 - 0.35 reports %d significant figures, want at least 1",
			got.SignificantFigures())
	}
	if got.Rat().RatString() != "1/20" {
		t.Errorf("0.4 - 0.35 = %s, want 1/20 exactly", got.Rat().RatString())
	}
}
