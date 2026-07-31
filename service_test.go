package ucum

import (
	"errors"
	"math"
	"math/big"
	"testing"
)

func newTestService(t *testing.T) Service {
	t.Helper()
	svc, err := New()
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestServiceValidateValid(t *testing.T) {
	svc := newTestService(t)

	valid := []string{
		"m", "kg", "cm", "km", "mg", "g", "L", "mL", "dL",
		"m/s", "mg/dL", "kg.m/s2", "10*3/uL", "mm[Hg]",
		"[lb_av]", "mol/L", "%", "1", "m2", "m-2",
		"Cel", "[degF]", "K",
	}
	for _, code := range valid {
		if err := svc.Validate(code); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", code, err)
		}
	}
}

func TestServiceValidateInvalid(t *testing.T) {
	svc := newTestService(t)

	invalid := []string{"xyz", "invalid_unit", "", "m/"}
	for _, code := range invalid {
		err := svc.Validate(code)
		if err == nil {
			t.Errorf("Validate(%q) = nil, want error", code)
		}
		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Errorf("Validate(%q) error type = %T, want *ValidationError", code, err)
		}
	}
}

func TestServiceConvertMetric(t *testing.T) {
	svc := newTestService(t)

	tests := []struct {
		value    float64
		from, to string
		want     float64
		tol      float64
	}{
		{1, "m", "cm", 100, 1e-9},
		{1, "km", "m", 1000, 1e-9},
		{1, "[lb_av]", "g", 453.59237, 1e-4},
		{1000, "mg", "g", 1, 1e-9},
		{1, "L", "mL", 1000, 1e-9},
		{1, "kg", "g", 1000, 1e-9},
	}

	for _, tc := range tests {
		got, err := svc.Convert(tc.value, tc.from, tc.to)
		if err != nil {
			t.Errorf("Convert(%g, %q, %q) error: %v", tc.value, tc.from, tc.to, err)
			continue
		}
		if math.Abs(got-tc.want) > tc.tol {
			t.Errorf("Convert(%g, %q, %q) = %g, want %g", tc.value, tc.from, tc.to, got, tc.want)
		}
	}
}

func TestServiceConvertSpecialUnits(t *testing.T) {
	svc := newTestService(t)

	tests := []struct {
		value    float64
		from, to string
		want     float64
		tol      float64
	}{
		// Celsius to Fahrenheit: 37C = 98.6F
		{37, "Cel", "[degF]", 98.6, 0.1},
		// Celsius to Kelvin: 100C = 373.15 K
		{100, "Cel", "K", 373.15, 0.01},
		// Fahrenheit to Celsius: 212F = 100C
		{212, "[degF]", "Cel", 100, 0.1},
		// Kelvin to Celsius: 273.15 K = 0C
		{273.15, "K", "Cel", 0, 0.01},
		// Freezing point: 0C = 32F
		{0, "Cel", "[degF]", 32, 0.1},
	}

	for _, tc := range tests {
		got, err := svc.Convert(tc.value, tc.from, tc.to)
		if err != nil {
			t.Errorf("Convert(%g, %q, %q) error: %v", tc.value, tc.from, tc.to, err)
			continue
		}
		if math.Abs(got-tc.want) > tc.tol {
			t.Errorf("Convert(%g, %q, %q) = %g, want %g", tc.value, tc.from, tc.to, got, tc.want)
		}
	}
}

func TestServiceConvertIncompatible(t *testing.T) {
	svc := newTestService(t)

	_, err := svc.Convert(1, "m", "kg")
	if err == nil {
		t.Error("Convert(m, kg) should fail: incompatible units")
	}
	var ce *ConversionError
	if !errors.As(err, &ce) {
		t.Errorf("Convert(m, kg) error type = %T, want *ConversionError", err)
	}
}

func TestServiceIsComparable(t *testing.T) {
	svc := newTestService(t)

	tests := []struct {
		code1, code2 string
		want         bool
	}{
		{"mg", "g", true},
		{"km", "m", true},
		{"mg", "mL", false},
		{"m", "kg", false},
		{"Cel", "K", true},
		{"Cel", "[degF]", true},
	}

	for _, tc := range tests {
		got, err := svc.IsComparable(tc.code1, tc.code2)
		if err != nil {
			t.Errorf("IsComparable(%q, %q) error: %v", tc.code1, tc.code2, err)
			continue
		}
		if got != tc.want {
			t.Errorf("IsComparable(%q, %q) = %v, want %v", tc.code1, tc.code2, got, tc.want)
		}
	}
}

func TestServiceAnalyze(t *testing.T) {
	svc := newTestService(t)

	tests := []struct {
		code string
		want string
	}{
		{"m", "meter"},
		{"km", "kilometer"},
		{"m/s", "meter/second"},
		{"kg", "kilogram"},
	}

	for _, tc := range tests {
		got, err := svc.Analyze(tc.code)
		if err != nil {
			t.Errorf("Analyze(%q) error: %v", tc.code, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Analyze(%q) = %q, want %q", tc.code, got, tc.want)
		}
	}
}

func TestServiceCanonical(t *testing.T) {
	svc := newTestService(t)

	// 1 km in canonical form should be 1000 m.
	p, err := svc.Canonical(1, "km")
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(p.Value-1000) > 1e-9 {
		t.Errorf("Canonical(1, km).Value = %g, want 1000", p.Value)
	}
	if p.Code != "m" {
		t.Errorf("Canonical(1, km).Code = %q, want %q", p.Code, "m")
	}
}

func TestServiceMultiply(t *testing.T) {
	svc := newTestService(t)

	// 2 m * 3 m = 6 m2.
	result, err := svc.Multiply(Pair{Value: 2, Code: "m"}, Pair{Value: 3, Code: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(result.Value-6) > 1e-9 {
		t.Errorf("Multiply value = %g, want 6", result.Value)
	}
	if result.Code != "m2" {
		t.Errorf("Multiply code = %q, want %q", result.Code, "m2")
	}
}

// TestNoPanicOnZeroDivisor covers codes that are syntactically valid but divide
// by a zero factor. They must return an error, never panic.
func TestNoPanicOnZeroDivisor(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatal(err)
	}
	codes := []string{"m/0", "0/0", "1/0", "m/0.s", "kg/(0.m)"}
	for _, code := range codes {
		t.Run(code, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Canonical(1, %q) panicked: %v", code, r)
				}
			}()
			if verr := svc.Validate(code); verr != nil {
				t.Skipf("Validate(%q) rejects it up front: %v", code, verr)
			}
			if _, err := svc.Canonical(1, code); err == nil {
				t.Errorf("Canonical(1, %q) = nil error, want an error", code)
			}
		})
	}
}

func TestNoPanicOnZeroDivisorConvert(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Convert panicked: %v", r)
		}
	}()
	if _, err := svc.Convert(1, "1", "0"); err == nil {
		t.Error(`Convert(1, "1", "0") = nil error, want an error`)
	}
	if _, err := svc.Convert(1, "m/0", "m"); err == nil {
		t.Error(`Convert(1, "m/0", "m") = nil error, want an error`)
	}
}

func TestNoPanicOnZeroDivisorMultiply(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Multiply panicked: %v", r)
		}
	}()
	if _, err := svc.Multiply(Pair{1, "m/0"}, Pair{1, "m"}); err == nil {
		t.Error(`Multiply(1 "m/0", 1 m) = nil error, want an error`)
	}
}

// TestConvertExactWhenRepresentable pins the property that Convert rounds once.
// Special units are excluded: their handler applies an offset, so the result is
// not a plain ratio of canonical factors.
//
// its result must be the float64 nearest to the exact rational conversion, not
// the quotient of two already-rounded canonical factors.
func TestConvertExactWhenRepresentable(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatal(err)
	}
	s := svc.(*service)

	pairs := [][2]string{
		{"L", "mL"}, {"mol/L", "mmol/L"}, {"kg", "g"}, {"m", "cm"},
		{"h", "s"}, {"d", "h"}, {"g/L", "mg/dL"}, {"L", "m3"},
		{"km", "m"}, {"mL", "uL"}, {"min", "s"}, {"[in_i]", "cm"},
		{"ug/L", "ng/mL"}, {"kPa", "mm[Hg]"}, {"mm[Hg]", "Pa"},
		{"cm3", "mL"}, {"ueq/L", "meq/L"},
	}
	for _, p := range pairs {
		got, err := svc.Convert(1, p[0], p[1])
		if err != nil {
			t.Fatalf("Convert(1, %q, %q): %v", p[0], p[1], err)
		}
		src, err := s.getCanonical(p[0])
		if err != nil {
			t.Fatal(err)
		}
		dst, err := s.getCanonical(p[1])
		if err != nil {
			t.Fatal(err)
		}
		want, _ := new(big.Rat).Quo(src.value.rat(), dst.value.rat()).Float64()
		if got != want {
			t.Errorf("Convert(1, %q, %q) = %.20g, want %.20g (exact, rounded once)",
				p[0], p[1], got, want)
		}
	}
}

// TestConvertIntegerResults spells out the cases whose exact result is an
// integer, so a regression is readable without recomputing rationals.
func TestConvertIntegerResults(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		from, to string
		want     float64
	}{
		{"L", "mL", 1000},
		{"mol/L", "mmol/L", 1000},
		{"kg", "g", 1000},
		{"m", "cm", 100},
		{"h", "s", 3600},
		{"d", "h", 24},
		{"g/L", "mg/dL", 100},
		{"km", "m", 1000},
		{"mL", "uL", 1000},
		{"min", "s", 60},
	}
	for _, tt := range tests {
		got, err := svc.Convert(1, tt.from, tt.to)
		if err != nil {
			t.Fatalf("Convert(1, %q, %q): %v", tt.from, tt.to, err)
		}
		if got != tt.want {
			t.Errorf("Convert(1, %q, %q) = %.20g, want exactly %v", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestCanonicalExactWhenRepresentable(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatal(err)
	}
	s := svc.(*service)
	codes := []string{"L", "mL", "mg/dL", "km", "[in_i]", "mm[Hg]", "ug"}
	values := []float64{1, 2, 3, 7, 0.1, 12.5, 1e6, -3}
	for _, code := range codes {
		can, err := s.getCanonical(code)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range values {
			p, err := svc.Canonical(v, code)
			if err != nil {
				t.Fatalf("Canonical(%v, %q): %v", v, code, err)
			}
			want, _ := new(big.Rat).Mul(ratFromFloat(v), can.value.rat()).Float64()
			if p.Value != want {
				t.Errorf("Canonical(%v, %q) = %.20g, want %.20g (exact, rounded once)",
					v, code, p.Value, want)
			}
		}
	}
}

func TestMultiplyExactWhenRepresentable(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatal(err)
	}
	s := svc.(*service)
	tests := []struct {
		v1, v2 Pair
	}{
		{Pair{1, "L"}, Pair{1, "L"}},
		{Pair{3, "mL"}, Pair{7, "mL"}},
		{Pair{2.5, "mg/dL"}, Pair{4, "L"}},
		{Pair{1, "km"}, Pair{1, "mm[Hg]"}},
	}
	for _, tt := range tests {
		can1, err := s.getCanonical(tt.v1.Code)
		if err != nil {
			t.Fatal(err)
		}
		can2, err := s.getCanonical(tt.v2.Code)
		if err != nil {
			t.Fatal(err)
		}
		p, err := svc.Multiply(tt.v1, tt.v2)
		if err != nil {
			t.Fatalf("Multiply(%v, %v): %v", tt.v1, tt.v2, err)
		}
		exact := new(big.Rat).Mul(can1.value.rat(), can2.value.rat())
		exact.Mul(exact, ratFromFloat(tt.v1.Value*tt.v2.Value))
		want, _ := exact.Float64()
		if p.Value != want {
			t.Errorf("Multiply(%v, %v) = %.20g, want %.20g (exact, rounded once)",
				tt.v1, tt.v2, p.Value, want)
		}
	}
}

// TestConvertNonFiniteUnchanged guards the fallback: NaN and infinities have no
// rational representation and must keep propagating as before.
func TestConvertNonFiniteUnchanged(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatal(err)
	}
	nan, err := svc.Convert(math.NaN(), "L", "mL")
	if err != nil {
		t.Fatal(err)
	}
	if nan == nan { //nolint:staticcheck // NaN != NaN is the assertion
		t.Errorf("Convert(NaN, L, mL) = %v, want NaN", nan)
	}
	inf, err := svc.Convert(math.Inf(1), "L", "mL")
	if err != nil {
		t.Fatal(err)
	}
	if !math.IsInf(inf, 1) {
		t.Errorf("Convert(+Inf, L, mL) = %v, want +Inf", inf)
	}
}
