package ucum

import (
	"encoding/xml"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
)

// XML structures matching UcumFunctionalTests.xml.

type ucumTests struct {
	XMLName        xml.Name              `xml:"ucumTests"`
	Validation     validationSection     `xml:"validation"`
	Conversion     conversionSection     `xml:"conversion"`
	Multiplication multiplicationSection `xml:"multiplication"`
	Division       divisionSection       `xml:"division"`
	DisplayName    displayNameSection    `xml:"displayNameGeneration"`
}

type validationSection struct {
	Cases []validationCase `xml:"case"`
}

type validationCase struct {
	ID    string `xml:"id,attr"`
	Unit  string `xml:"unit,attr"`
	Valid string `xml:"valid,attr"`
}

type conversionSection struct {
	Cases []conversionCase `xml:"case"`
}

type conversionCase struct {
	ID      string `xml:"id,attr"`
	Value   string `xml:"value,attr"`
	SrcUnit string `xml:"srcUnit,attr"`
	DstUnit string `xml:"dstUnit,attr"`
	Outcome string `xml:"outcome,attr"`
}

type multiplicationSection struct {
	Cases []multiplicationCase `xml:"case"`
}

type multiplicationCase struct {
	ID   string `xml:"id,attr"`
	V1   string `xml:"v1,attr"`
	U1   string `xml:"u1,attr"`
	V2   string `xml:"v2,attr"`
	U2   string `xml:"u2,attr"`
	VRes string `xml:"vRes,attr"`
	URes string `xml:"uRes,attr"`
}

type displayNameSection struct {
	Cases []displayNameCase `xml:"case"`
}

type displayNameCase struct {
	ID      string `xml:"id,attr"`
	Unit    string `xml:"unit,attr"`
	Display string `xml:"display,attr"`
}

type divisionSection struct {
	Cases []multiplicationCase `xml:"case"`
}

// countSigFigs returns the number of significant figures in a numeric string.
// For integers without a decimal point it returns 0 (unlimited precision).
func countSigFigs(s string) int {
	s = strings.TrimLeft(s, "-+")
	if !strings.Contains(s, ".") {
		return 0 // integer input, treat as unlimited precision
	}
	// Remove leading zeros and the decimal point to count significant digits.
	s = strings.TrimLeft(s, "0")
	count := 0
	for _, ch := range s {
		if ch >= '0' && ch <= '9' {
			count++
		}
	}
	return count
}

// roundToSigFigs rounds value to the given number of significant figures.
func roundToSigFigs(value float64, sigFigs int) float64 {
	if sigFigs <= 0 || value == 0 {
		return value
	}
	d := math.Ceil(math.Log10(math.Abs(value)))
	pow := math.Pow(10, float64(sigFigs)-d)
	return math.Round(value*pow) / pow
}

func loadTestSuite(t *testing.T) ucumTests {
	t.Helper()
	data, err := os.ReadFile("testdata/UcumFunctionalTests.xml")
	if err != nil {
		t.Fatalf("failed to read test XML: %v", err)
	}
	var suite ucumTests
	if err := xml.Unmarshal(data, &suite); err != nil {
		t.Fatalf("failed to parse test XML: %v", err)
	}
	return suite
}

// Validation tests.

func TestFunctionalValidation(t *testing.T) {
	suite := loadTestSuite(t)
	svc := newTestService(t)

	for _, tc := range suite.Validation.Cases {
		tc := tc
		t.Run(fmt.Sprintf("%s_%s", tc.ID, tc.Unit), func(t *testing.T) {
			err := svc.Validate(tc.Unit)
			expectValid := tc.Valid == "true"

			if expectValid && err != nil {
				t.Errorf("Validate(%q): expected valid but got error: %v", tc.Unit, err)
			}
			if !expectValid && err == nil {
				t.Errorf("Validate(%q): expected invalid but got nil error", tc.Unit)
			}
		})
	}
}

// Conversion tests.

func TestFunctionalConversion(t *testing.T) {
	suite := loadTestSuite(t)
	svc := newTestService(t)

	for _, tc := range suite.Conversion.Cases {
		tc := tc
		t.Run(fmt.Sprintf("%s_%s->%s", tc.ID, tc.SrcUnit, tc.DstUnit), func(t *testing.T) {
			value, err := strconv.ParseFloat(tc.Value, 64)
			if err != nil {
				t.Fatalf("bad test value %q: %v", tc.Value, err)
			}
			outcome, err := strconv.ParseFloat(tc.Outcome, 64)
			if err != nil {
				t.Fatalf("bad test outcome %q: %v", tc.Outcome, err)
			}

			got, err := svc.Convert(value, tc.SrcUnit, tc.DstUnit)
			if err != nil {
				t.Errorf("Convert(%v, %q, %q) error: %v",
					value, tc.SrcUnit, tc.DstUnit, err)
				return
			}

			// The Java UCUM library uses significant-figure-aware arithmetic.
			// When the input value has a decimal point (e.g. "6.3" = 2 sig figs),
			// round our exact result to match Java's sig-fig rounding.
			sigFigs := countSigFigs(tc.Value)
			if sigFigs > 0 {
				got = roundToSigFigs(got, sigFigs)
			}

			// Use relative tolerance of 1e-6, but fall back to absolute 1e-10
			// for values near zero.
			delta := math.Abs(outcome) * 1e-6
			if delta < 1e-10 {
				delta = 1e-10
			}
			if diff := math.Abs(got - outcome); diff > delta {
				t.Errorf("Convert(%v, %q, %q) = %v, want %v (diff=%v, tol=%v)",
					value, tc.SrcUnit, tc.DstUnit, got, outcome, diff, delta)
			}
		})
	}
}

// Multiplication tests.

func TestFunctionalMultiplication(t *testing.T) {
	suite := loadTestSuite(t)
	svc := newTestService(t)

	if len(suite.Multiplication.Cases) == 0 {
		t.Skip("no multiplication test cases found")
	}

	for _, tc := range suite.Multiplication.Cases {
		tc := tc
		t.Run(tc.ID, func(t *testing.T) {
			v1, err := strconv.ParseFloat(tc.V1, 64)
			if err != nil {
				t.Fatalf("bad v1 %q: %v", tc.V1, err)
			}
			v2, err := strconv.ParseFloat(tc.V2, 64)
			if err != nil {
				t.Fatalf("bad v2 %q: %v", tc.V2, err)
			}
			vRes, err := strconv.ParseFloat(tc.VRes, 64)
			if err != nil {
				t.Fatalf("bad vRes %q: %v", tc.VRes, err)
			}

			got, err := svc.Multiply(Pair{Value: v1, Code: tc.U1}, Pair{Value: v2, Code: tc.U2})
			if err != nil {
				t.Errorf("Multiply({%v,%q}, {%v,%q}) error: %v",
					v1, tc.U1, v2, tc.U2, err)
				return
			}

			// The result units may differ from expected, so convert the result
			// to the expected unit for comparison if they differ.
			gotValue := got.Value
			if tc.URes != "" && got.Code != tc.URes {
				converted, err := svc.Convert(got.Value, got.Code, tc.URes)
				if err != nil {
					t.Errorf("cannot convert result unit %q to expected %q: %v", got.Code, tc.URes, err)
					return
				}
				gotValue = converted
			}

			delta := math.Abs(vRes) * 1e-6
			if delta < 1e-10 {
				delta = 1e-10
			}
			if diff := math.Abs(gotValue - vRes); diff > delta {
				t.Errorf("Multiply({%v,%q}, {%v,%q}) = {%v,%q}, want value ~%v in unit %q (diff=%v)",
					v1, tc.U1, v2, tc.U2, got.Value, got.Code, vRes, tc.URes, diff)
			}
		})
	}
}

// Special units: conversions that Java (HAPI/HL7 validator) cannot handle.

func TestFunctionalSpecialUnitsJavaFails(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatal(err)
	}

	// These conversions throw UcumException in Java but work in our lib
	tests := []struct {
		value    float64
		from, to string
		want     float64
		delta    float64
	}{
		{0, "Cel", "K", 273.15, 0.01},
		{100, "Cel", "K", 373.15, 0.01},
		{37, "Cel", "[degF]", 98.6, 0.1},
		{32, "[degF]", "Cel", 0, 0.1},
		{212, "[degF]", "K", 373.15, 0.1},
		{-40, "Cel", "[degF]", -40, 0.1}, // -40 is same in both scales
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%v_%s->%s", tt.value, tt.from, tt.to), func(t *testing.T) {
			got, err := svc.Convert(tt.value, tt.from, tt.to)
			if err != nil {
				t.Fatalf("Convert(%v, %q, %q) error: %v", tt.value, tt.from, tt.to, err)
			}
			if diff := math.Abs(got - tt.want); diff > tt.delta {
				t.Errorf("Convert(%v, %q, %q) = %v, want %v (±%v)", tt.value, tt.from, tt.to, got, tt.want, tt.delta)
			}
		})
	}
}

func TestFunctionalReaumurConversion(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		value    float64
		from, to string
		want     float64
	}{
		{0, "[degRe]", "Cel", 0},
		{80, "[degRe]", "Cel", 100},
		{100, "Cel", "[degRe]", 80},
		{0, "[degRe]", "K", 273.15},
		{80, "[degRe]", "[degF]", 212},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%v_%s->%s", tt.value, tt.from, tt.to), func(t *testing.T) {
			got, err := svc.Convert(tt.value, tt.from, tt.to)
			if err != nil {
				t.Fatalf("Convert(%v, %q, %q) error: %v", tt.value, tt.from, tt.to, err)
			}
			if diff := math.Abs(got - tt.want); diff > 1e-9 {
				t.Errorf("Convert(%v, %q, %q) = %v, want %v", tt.value, tt.from, tt.to, got, tt.want)
			}
		})
	}
}
func TestFunctionalCanonicalSpecialUnits(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		value    float64
		code     string
		wantVal  float64
		wantCode string
		tol      float64
	}{
		{0, "Cel", 273.15, "K", 1e-9},
		{1, "Cel", 274.15, "K", 1e-9},
		{100, "Cel", 373.15, "K", 1e-9},
		{32, "[degF]", 273.15, "K", 1e-9},
		{212, "[degF]", 373.15, "K", 1e-9},
		{0, "[degRe]", 273.15, "K", 1e-9},
		{80, "[degRe]", 373.15, "K", 1e-9},
		// 1 pH = 10^-1 mol/L = 6.02214076e25 m-3
		{1, "[pH]", 6.02214076e25, "m-3", 1e18},
		// 1 B = 10^1 (dimensionless ratio)
		{1, "B", 10, "1", 1e-9},
		// non-special units must be unaffected
		{1, "L", 0.001, "m3", 1e-15},
		{1, "kg", 1000, "g", 1e-9},
	}
	for _, tt := range tests {
		p, err := svc.Canonical(tt.value, tt.code)
		if err != nil {
			t.Fatalf("Canonical(%v, %q): %v", tt.value, tt.code, err)
		}
		if math.Abs(p.Value-tt.wantVal) > tt.tol {
			t.Errorf("Canonical(%v, %q).Value = %v, want %v", tt.value, tt.code, p.Value, tt.wantVal)
		}
		if p.Code != tt.wantCode {
			t.Errorf("Canonical(%v, %q).Code = %q, want %q", tt.value, tt.code, p.Code, tt.wantCode)
		}
	}
}

// TestFunctionalCanonicalComparableTemperatures is the regression test for the consumer
// pattern: normalise via Canonical, then compare. Before the fix both sides
// canonicalized to their raw numeric value in K, so 100 [degF] compared as
// greater than 50 Cel.
func TestFunctionalCanonicalComparableTemperatures(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatal(err)
	}
	hot, err := svc.Canonical(100, "[degF]") // = 37.78 Cel = 310.93 K
	if err != nil {
		t.Fatal(err)
	}
	cold, err := svc.Canonical(50, "Cel") // = 323.15 K
	if err != nil {
		t.Fatal(err)
	}
	if !(hot.Value < cold.Value) {
		t.Errorf("Canonical(100,[degF]) = %v should be less than Canonical(50,Cel) = %v",
			hot.Value, cold.Value)
	}
}

func TestFunctionalMultiplySpecialUnits(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatal(err)
	}
	// 1 Cel * 1 (dimensionless) = 274.15 K
	p, err := svc.Multiply(Pair{1, "Cel"}, Pair{1, "1"})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(p.Value-274.15) > 1e-9 {
		t.Errorf("Multiply(1 Cel, 1).Value = %v, want 274.15", p.Value)
	}
	if p.Code != "K" {
		t.Errorf("Multiply(1 Cel, 1).Code = %q, want %q", p.Code, "K")
	}
}

// Division tests (official suite, <division> section).

func TestFunctionalDivision(t *testing.T) {
	suite := loadTestSuite(t)
	svc := newTestService(t)

	if len(suite.Division.Cases) == 0 {
		t.Fatal("no division test cases found in the official suite")
	}

	for _, tc := range suite.Division.Cases {
		t.Run(tc.ID, func(t *testing.T) {
			v1, err := strconv.ParseFloat(tc.V1, 64)
			if err != nil {
				t.Fatalf("bad v1 %q: %v", tc.V1, err)
			}
			v2, err := strconv.ParseFloat(tc.V2, 64)
			if err != nil {
				t.Fatalf("bad v2 %q: %v", tc.V2, err)
			}
			vRes, err := strconv.ParseFloat(tc.VRes, 64)
			if err != nil {
				t.Fatalf("bad vRes %q: %v", tc.VRes, err)
			}

			got, err := svc.Divide(Pair{Value: v1, Code: tc.U1}, Pair{Value: v2, Code: tc.U2})
			if err != nil {
				t.Errorf("Divide({%v,%q}, {%v,%q}) error: %v", v1, tc.U1, v2, tc.U2, err)
				return
			}

			// An empty uRes in the suite means dimensionless, which this package
			// spells "1".
			wantUnit := tc.URes
			if wantUnit == "" {
				wantUnit = "1"
			}
			gotValue := got.Value
			if got.Code != wantUnit {
				converted, cerr := svc.Convert(got.Value, got.Code, wantUnit)
				if cerr != nil {
					t.Errorf("cannot convert result unit %q to expected %q: %v", got.Code, wantUnit, cerr)
					return
				}
				gotValue = converted
			}

			// The suite records outcomes rounded to the significant figures of the
			// inputs, as the Java implementation produces them.
			if sf := countSigFigs(tc.VRes); sf > 0 {
				gotValue = roundToSigFigs(gotValue, sf)
			}

			delta := math.Abs(vRes) * 1e-6
			if delta < 1e-10 {
				delta = 1e-10
			}
			if diff := math.Abs(gotValue - vRes); diff > delta {
				t.Errorf("Divide({%v,%q}, {%v,%q}) = {%v,%q}, want value ~%v in unit %q (diff=%v)",
					v1, tc.U1, v2, tc.U2, got.Value, got.Code, vRes, wantUnit, diff)
			}
		})
	}
}

// Display name tests (official suite, <displayNameGeneration> section).

func TestFunctionalDisplayNameGeneration(t *testing.T) {
	suite := loadTestSuite(t)
	svc := newTestService(t)

	if len(suite.DisplayName.Cases) == 0 {
		t.Fatal("no displayNameGeneration test cases found in the official suite")
	}

	for _, tc := range suite.DisplayName.Cases {
		t.Run(fmt.Sprintf("%s_%s", tc.ID, tc.Unit), func(t *testing.T) {
			got, err := svc.Analyze(tc.Unit)
			if err != nil {
				t.Errorf("Analyze(%q) error: %v", tc.Unit, err)
				return
			}
			if got != tc.Display {
				t.Errorf("Analyze(%q) = %q, want %q", tc.Unit, got, tc.Display)
			}
		})
	}
}
