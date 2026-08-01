package fhir_test

import (
	"math/big"
	"testing"

	"github.com/gofhir/ucum/v4/fhir"
)

// TestParseDecimalPrecision pins the rule FHIR states: "0.010 is regarded as
// different to 0.01, and the original precision should be preserved".
func TestParseDecimalPrecision(t *testing.T) {
	tests := []struct {
		in      string
		sigFigs int
	}{
		{"0.01", 1},
		{"0.010", 2}, // the trailing zero is the whole point
		{"0.0100", 3},
		{"1.5", 2},
		{"1.50", 3},
		{"1.500", 4},
		{"12.34", 4},
		{"0.000123", 3},
		{"-1.50", 3},
		{"+2.5", 2},
		// The exponent carries magnitude, not precision.
		{"1.5e3", 2},
		{"1.50E-4", 3},
		// Written without a point, nothing is claimed.
		{"100", 0},
		{"42", 0},
		{"0", 0},
		// All zeros still state a resolution.
		{"0.00", 2},
		{"0.0", 1},
	}
	for _, tt := range tests {
		d, err := fhir.ParseDecimal(tt.in)
		if err != nil {
			t.Errorf("ParseDecimal(%q): %v", tt.in, err)
			continue
		}
		if got := d.SignificantFigures(); got != tt.sigFigs {
			t.Errorf("ParseDecimal(%q).SignificantFigures() = %d, want %d", tt.in, got, tt.sigFigs)
		}
	}
}

// TestDecimalDistinguishesPrecision is the FHIR requirement stated directly:
// 0.010 and 0.01 are the same number and different values.
func TestDecimalDistinguishesPrecision(t *testing.T) {
	a, err := fhir.ParseDecimal("0.010")
	if err != nil {
		t.Fatal(err)
	}
	b, err := fhir.ParseDecimal("0.01")
	if err != nil {
		t.Fatal(err)
	}
	if a.Rat().Cmp(b.Rat()) != 0 {
		t.Error("0.010 and 0.01 should hold the same number")
	}
	if a.SignificantFigures() == b.SignificantFigures() {
		t.Error("0.010 and 0.01 should differ in precision")
	}
	if a.String() == b.String() {
		t.Errorf("0.010 and 0.01 render the same (%q); the precision is lost for presentation", a.String())
	}
}

func TestDecimalString(t *testing.T) {
	tests := []struct{ in, want string }{
		{"0.01", "0.01"},
		{"0.010", "0.010"},
		{"1.5", "1.5"},
		{"1.50", "1.50"},
		{"12.34", "12.34"},
		{"-1.50", "-1.50"},
		{"0.000123", "0.000123"},
		{"100", "100"},
		{"0.0", "0.0"},
	}
	for _, tt := range tests {
		d, err := fhir.ParseDecimal(tt.in)
		if err != nil {
			t.Errorf("ParseDecimal(%q): %v", tt.in, err)
			continue
		}
		if got := d.String(); got != tt.want {
			t.Errorf("ParseDecimal(%q).String() = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestDecimalRoundingCarries covers the case where rounding to n figures grows
// the leading digit: 9.99 to three figures is 9.99, but 9.996 is 10.0.
func TestDecimalRoundingCarries(t *testing.T) {
	v, ok := new(big.Rat).SetString("9.996")
	if !ok {
		t.Fatal("bad literal")
	}
	d := fhir.NewDecimal(v, 3)
	if got := d.String(); got != "10.0" {
		t.Errorf("9.996 at 3 significant figures = %q, want %q", got, "10.0")
	}
}

func TestParseDecimalRejects(t *testing.T) {
	for _, in := range []string{"", "  ", "abc", "1.2.3", "1,5"} {
		if _, err := fhir.ParseDecimal(in); err == nil {
			t.Errorf("ParseDecimal(%q) = nil error, want an error", in)
		}
	}
}

// TestConvertDecimalPreservesPrecision is the point of the type: a conversion
// multiplies by an exactly known factor, so it neither adds nor removes
// significant figures.
func TestConvertDecimalPreservesPrecision(t *testing.T) {
	c, err := fhir.NewComparator()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		in       string
		from, to string
		wantVal  string
		wantSF   int
	}{
		{"1.50", "g/L", "mg/dL", "150", 3},
		// Two significant figures cannot be written as "150", which would claim
		// three, so these render in scientific notation.
		{"1.5", "g/L", "mg/dL", "1.5e2", 2},
		{"5.4", "mmol/L", "umol/L", "5.4e3", 2},
		{"0.010", "L", "mL", "10", 2},
		{"98.6", "[degF]", "Cel", "37.0", 3},
	}
	for _, tt := range tests {
		d, err := fhir.ParseDecimal(tt.in)
		if err != nil {
			t.Fatal(err)
		}
		out, err := c.ConvertDecimal(d, tt.from, tt.to)
		if err != nil {
			t.Fatalf("ConvertDecimal(%s, %q, %q): %v", tt.in, tt.from, tt.to, err)
		}
		if got := out.SignificantFigures(); got != tt.wantSF {
			t.Errorf("ConvertDecimal(%s, %q, %q) has %d significant figures, want %d",
				tt.in, tt.from, tt.to, got, tt.wantSF)
		}
		if got := out.String(); got != tt.wantVal {
			t.Errorf("ConvertDecimal(%s, %q, %q) = %q, want %q",
				tt.in, tt.from, tt.to, got, tt.wantVal)
		}
	}
}

// TestConvertDecimalIsExactUnderneath: the rendering is rounded to the declared
// precision, but the value carried forward is not.
func TestConvertDecimalIsExactUnderneath(t *testing.T) {
	c, err := fhir.NewComparator()
	if err != nil {
		t.Fatal(err)
	}
	d, err := fhir.ParseDecimal("1.0")
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.ConvertDecimal(d, "[degF]", "Cel")
	if err != nil {
		t.Fatal(err)
	}
	// 1 degF is exactly -155/9 Cel, a repeating decimal.
	want, _ := new(big.Rat).SetString("-155/9")
	if out.Rat().Cmp(want) != 0 {
		t.Errorf("ConvertDecimal(1.0, [degF], Cel) = %s, want %s exactly",
			out.Rat().RatString(), want.RatString())
	}
}

// TestConvertDecimalRejectsNonRational: claiming the input's precision for an
// approximation would be a lie, so the non-rational scales are refused.
func TestConvertDecimalRejectsNonRational(t *testing.T) {
	c, err := fhir.NewComparator()
	if err != nil {
		t.Fatal(err)
	}
	d, err := fhir.ParseDecimal("7.0")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ConvertDecimal(d, "[pH]", "mol/L"); err == nil {
		t.Error("ConvertDecimal(7.0, [pH], mol/L) = nil error, want an error")
	}
}
