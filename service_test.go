package ucum

import (
	"math"
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

// ---------------------------------------------------------------------------
// Validate
// ---------------------------------------------------------------------------

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
		if _, ok := err.(*ValidationError); !ok {
			t.Errorf("Validate(%q) error type = %T, want *ValidationError", code, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Convert – simple metric
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Convert – special units (temperature)
// ---------------------------------------------------------------------------

func TestServiceConvertSpecialUnits(t *testing.T) {
	svc := newTestService(t)

	tests := []struct {
		value    float64
		from, to string
		want     float64
		tol      float64
	}{
		// Celsius to Fahrenheit: 37°C = 98.6°F
		{37, "Cel", "[degF]", 98.6, 0.1},
		// Celsius to Kelvin: 100°C = 373.15 K
		{100, "Cel", "K", 373.15, 0.01},
		// Fahrenheit to Celsius: 212°F = 100°C
		{212, "[degF]", "Cel", 100, 0.1},
		// Kelvin to Celsius: 273.15 K = 0°C
		{273.15, "K", "Cel", 0, 0.01},
		// Freezing point: 0°C = 32°F
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

// ---------------------------------------------------------------------------
// Convert – incompatible units
// ---------------------------------------------------------------------------

func TestServiceConvertIncompatible(t *testing.T) {
	svc := newTestService(t)

	_, err := svc.Convert(1, "m", "kg")
	if err == nil {
		t.Error("Convert(m, kg) should fail: incompatible units")
	}
	if _, ok := err.(*ConversionError); !ok {
		t.Errorf("Convert(m, kg) error type = %T, want *ConversionError", err)
	}
}

// ---------------------------------------------------------------------------
// IsComparable
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Analyse
// ---------------------------------------------------------------------------

func TestServiceAnalyse(t *testing.T) {
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
		got, err := svc.Analyse(tc.code)
		if err != nil {
			t.Errorf("Analyse(%q) error: %v", tc.code, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Analyse(%q) = %q, want %q", tc.code, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Canonical
// ---------------------------------------------------------------------------

func TestServiceCanonical(t *testing.T) {
	svc := newTestService(t)

	// 1 km in canonical form should be 1000 m
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

// ---------------------------------------------------------------------------
// Multiply
// ---------------------------------------------------------------------------

func TestServiceMultiply(t *testing.T) {
	svc := newTestService(t)

	// 2 m * 3 m = 6 m2
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
