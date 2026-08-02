package engine

import (
	"errors"
	"math"
	"math/big"
	"testing"

	"github.com/gofhir/ucum/v4/internal/decimal"
	"github.com/gofhir/ucum/v4/internal/ucumerr"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	svc, err := New(nil, false)
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
		var ve *ucumerr.ValidationError
		if !errors.As(err, &ve) {
			t.Errorf("Validate(%q) error type = %T, want *ucumerr.ValidationError", code, err)
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
	var ce *ucumerr.ConversionError
	if !errors.As(err, &ce) {
		t.Errorf("Convert(m, kg) error type = %T, want *ucumerr.ConversionError", err)
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
		{"m", "(meter)"},
		{"km", "(kilometer)"},
		{"m/s", "(meter) / (second)"},
		{"kg", "(kilogram)"},
		{"", "(unity)"},
		{"cm2", "(centimeter ^ 2)"},
		{"kg.m/s2", "(kilogram) * (meter) / (second ^ 2)"},
		{"mm[Hg]", "(millimeter of mercury column)"},
		{"10*3/uL", "(the number ten for arbitrary powers ^ 3) / (microliter)"},
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
	svc, err := New(nil, false)
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
	svc, err := New(nil, false)
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
	svc, err := New(nil, false)
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

// TestConvertExactWhenRepresentable pins the property that Convert rounds once:
// its result must be the float64 nearest to the exact rational conversion, not
// the quotient of two already-rounded canonical factors.
// Special units are excluded, since their handler applies an offset and the
// result is not a plain ratio of canonical factors.
func TestConvertExactWhenRepresentable(t *testing.T) {
	svc, err := New(nil, false)
	if err != nil {
		t.Fatal(err)
	}

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
		src, err := svc.getCanonical(p[0])
		if err != nil {
			t.Fatal(err)
		}
		dst, err := svc.getCanonical(p[1])
		if err != nil {
			t.Fatal(err)
		}
		want, _ := new(big.Rat).Quo(src.Value.Rat(), dst.Value.Rat()).Float64()
		if got != want {
			t.Errorf("Convert(1, %q, %q) = %.20g, want %.20g (exact, rounded once)",
				p[0], p[1], got, want)
		}
	}
}

// TestConvertIntegerResults spells out the cases whose exact result is an
// integer, so a regression is readable without recomputing rationals.
func TestConvertIntegerResults(t *testing.T) {
	svc, err := New(nil, false)
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
	svc, err := New(nil, false)
	if err != nil {
		t.Fatal(err)
	}
	codes := []string{"L", "mL", "mg/dL", "km", "[in_i]", "mm[Hg]", "ug"}
	values := []float64{1, 2, 3, 7, 0.1, 12.5, 1e6, -3}
	for _, code := range codes {
		can, err := svc.getCanonical(code)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range values {
			p, err := svc.Canonical(v, code)
			if err != nil {
				t.Fatalf("Canonical(%v, %q): %v", v, code, err)
			}
			want, _ := new(big.Rat).Mul(decimal.RatFromFloat(v), can.Value.Rat()).Float64()
			if p.Value != want {
				t.Errorf("Canonical(%v, %q) = %.20g, want %.20g (exact, rounded once)",
					v, code, p.Value, want)
			}
		}
	}
}

func TestMultiplyExactWhenRepresentable(t *testing.T) {
	svc, err := New(nil, false)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		v1, v2 Pair
	}{
		{Pair{1, "L"}, Pair{1, "L"}},
		{Pair{3, "mL"}, Pair{7, "mL"}},
		{Pair{2.5, "mg/dL"}, Pair{4, "L"}},
		{Pair{1, "km"}, Pair{1, "mm[Hg]"}},
	}
	for _, tt := range tests {
		can1, err := svc.getCanonical(tt.v1.Code)
		if err != nil {
			t.Fatal(err)
		}
		can2, err := svc.getCanonical(tt.v2.Code)
		if err != nil {
			t.Fatal(err)
		}
		p, err := svc.Multiply(tt.v1, tt.v2)
		if err != nil {
			t.Fatalf("Multiply(%v, %v): %v", tt.v1, tt.v2, err)
		}
		exact := new(big.Rat).Mul(can1.Value.Rat(), can2.Value.Rat())
		exact.Mul(exact, decimal.RatFromFloat(tt.v1.Value*tt.v2.Value))
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
	svc, err := New(nil, false)
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

// TestCanonicalExactForAffineScales pins Canonical to a single rounding for the
// rational special scales, the same guarantee Convert gives.
func TestCanonicalExactForAffineScales(t *testing.T) {
	svc, err := New(nil, false)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct{ value, code string }{
		{"1", "[degRe]"}, {"0", "[degRe]"}, {"80", "[degRe]"},
		{"1", "Cel"}, {"37", "Cel"}, {"0.1", "Cel"},
		{"1", "[degF]"}, {"98.6", "[degF]"}, {"100", "[degF]"},
	}
	for _, tt := range tests {
		v, _ := new(big.Rat).SetString(tt.value)
		exact, err := svc.CanonicalRat(v, tt.code)
		if err != nil {
			t.Fatal(err)
		}
		want, _ := exact.Value.Float64()
		f, _ := v.Float64()
		p, err := svc.Canonical(f, tt.code)
		if err != nil {
			t.Fatal(err)
		}
		if p.Value != want {
			t.Errorf("Canonical(%s, %q) = %.20g, want %.20g (single rounding of %s)",
				tt.value, tt.code, p.Value, want, exact.Value.RatString())
		}
	}
}

func TestMultiplyExactForAffineScales(t *testing.T) {
	svc, err := New(nil, false)
	if err != nil {
		t.Fatal(err)
	}
	// 1 [degRe] canonicalizes to 274.4 K exactly; times 1 (dimensionless) it
	// must stay the single rounding of 1372/5.
	want, _ := new(big.Rat).SetFrac64(1372, 5).Float64()
	p, err := svc.Multiply(Pair{1, "[degRe]"}, Pair{1, "1"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Value != want {
		t.Errorf("Multiply(1 [degRe], 1) = %.20g, want %.20g", p.Value, want)
	}
}

func TestDivide(t *testing.T) {
	svc := newTestService(t)
	tests := []struct {
		v1, v2   Pair
		wantVal  float64
		wantCode string
	}{
		{Pair{1.5, "g"}, Pair{2, "m"}, 0.75, "g.m-1"},
		{Pair{1, "L"}, Pair{1, "mL"}, 1000, "1"},
		{Pair{1, "m"}, Pair{1, "s"}, 1, "m.s-1"},
		{Pair{10, "km"}, Pair{2, "h"}, 10000.0 / 7200.0, "m.s-1"},
		{Pair{1, "mol/L"}, Pair{1, "mmol/L"}, 1000, "1"},
	}
	for _, tt := range tests {
		got, err := svc.Divide(tt.v1, tt.v2)
		if err != nil {
			t.Fatalf("Divide(%v, %v): %v", tt.v1, tt.v2, err)
		}
		if math.Abs(got.Value-tt.wantVal) > 1e-9 {
			t.Errorf("Divide(%v, %v).Value = %v, want %v", tt.v1, tt.v2, got.Value, tt.wantVal)
		}
		if got.Code != tt.wantCode {
			t.Errorf("Divide(%v, %v).Code = %q, want %q", tt.v1, tt.v2, got.Code, tt.wantCode)
		}
	}
}

// TestDivideExact holds Divide to the same single-rounding property as Convert.
func TestDivideExact(t *testing.T) {
	svc := newTestService(t)
	tests := []struct{ v1, v2 Pair }{
		{Pair{1, "L"}, Pair{1, "mL"}},
		{Pair{3, "mL"}, Pair{7, "uL"}},
		{Pair{1, "mg/dL"}, Pair{1, "g/L"}},
	}
	for _, tt := range tests {
		can1, err := svc.getCanonical(tt.v1.Code)
		if err != nil {
			t.Fatal(err)
		}
		can2, err := svc.getCanonical(tt.v2.Code)
		if err != nil {
			t.Fatal(err)
		}
		exact := new(big.Rat).Quo(can1.Value.Rat(), can2.Value.Rat())
		exact.Mul(exact, new(big.Rat).Quo(decimal.RatFromFloat(tt.v1.Value), decimal.RatFromFloat(tt.v2.Value)))
		want, _ := exact.Float64()

		got, err := svc.Divide(tt.v1, tt.v2)
		if err != nil {
			t.Fatalf("Divide(%v, %v): %v", tt.v1, tt.v2, err)
		}
		if got.Value != want {
			t.Errorf("Divide(%v, %v) = %.20g, want %.20g (exact, rounded once)",
				tt.v1, tt.v2, got.Value, want)
		}
	}
}

func TestDivideByZero(t *testing.T) {
	svc := newTestService(t)
	cases := []struct {
		name   string
		v1, v2 Pair
	}{
		{"zero value", Pair{1, "m"}, Pair{0, "s"}},
		{"zero factor", Pair{1, "m"}, Pair{1, "0"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Divide panicked: %v", r)
				}
			}()
			if _, err := svc.Divide(tc.v1, tc.v2); !errors.Is(err, ucumerr.ErrDivisionByZero) {
				t.Errorf("Divide(%v, %v) error = %v, want ucumerr.ErrDivisionByZero", tc.v1, tc.v2, err)
			}
		})
	}
}

// TestDivideNearAbsoluteZero documents that a special operand close to the
// bottom of its scale does not divide by zero: -273.15 is not exactly absolute
// zero in float64, so the mapped operand is tiny but non-zero and the result is
// large but finite. The zero guard in Divide covers the exact case.
func TestDivideNearAbsoluteZero(t *testing.T) {
	svc := newTestService(t)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Divide panicked: %v", r)
		}
	}()
	got, err := svc.Divide(Pair{1, "m"}, Pair{-273.15, "Cel"})
	if err != nil {
		t.Fatalf("Divide(1 m, -273.15 Cel): %v", err)
	}
	if math.IsInf(got.Value, 0) || math.IsNaN(got.Value) {
		t.Errorf("Divide(1 m, -273.15 Cel) = %v, want a finite value", got.Value)
	}
	if got.Code != "K-1.m" {
		t.Errorf("Divide(1 m, -273.15 Cel).Code = %q, want %q", got.Code, "K-1.m")
	}
}

// TestDivideNonRationalScale checks the float64 fallback still works for the
// scales that have no exact rational form.
func TestDivideNonRationalScale(t *testing.T) {
	svc := newTestService(t)
	// 1 pH = 0.1 mol/L; dividing by 1 mol/L gives 0.1 dimensionless.
	got, err := svc.Divide(Pair{1, "[pH]"}, Pair{1, "mol/L"})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got.Value-0.1) > 1e-9 {
		t.Errorf("Divide(1 [pH], 1 mol/L).Value = %v, want 0.1", got.Value)
	}
	if got.Code != "1" {
		t.Errorf("Divide(1 [pH], 1 mol/L).Code = %q, want %q", got.Code, "1")
	}
}
func TestValidateInPropertyAccepts(t *testing.T) {
	svc := newTestService(t)
	cases := [][2]string{
		// atomic units, matched against their declared property
		{"m", "length"},
		{"kg", "mass"},
		{"g", "mass"},
		{"s", "time"},
		{"K", "temperature"},
		{"Cel", "temperature"},
		{"N", "force"},
		{"L", "volume"},
		{"mol", "amount of substance"},
		{"Pa", "pressure"},
		{"mm[Hg]", "pressure"},
		{"[IU]", "arbitrary"},
		{"J", "energy"},
		{"Hz", "frequency"},
		// case-insensitive, as before
		{"m", "LENGTH"},
		{"N", "Force"},
		// compound expressions have no declared property, so these resolve
		// dimensionally against the units that do declare it
		{"m/s2", "acceleration"},
		{"m/s", "velocity"},
		{"m2", "area"},
		{"m3", "volume"},
		{"kg.m/s2", "force"},
		{"g/L", "mass concentration"},
	}
	for _, c := range cases {
		if err := svc.ValidateInProperty(c[0], c[1]); err != nil {
			t.Errorf("ValidateInProperty(%q, %q) = %v, want nil", c[0], c[1], err)
		}
	}
}

func TestValidateInPropertyRejects(t *testing.T) {
	svc := newTestService(t)
	cases := [][2]string{
		{"m", "mass"},
		{"L", "length"},
		{"N", "pressure"},
		{"s", "length"},
		{"m2", "length"},
		{"kg", "force"},
		// unknown property
		{"m", "no such property"},
		// invalid code
		{"not-a-unit", "length"},
	}
	for _, c := range cases {
		if err := svc.ValidateInProperty(c[0], c[1]); err == nil {
			t.Errorf("ValidateInProperty(%q, %q) = nil, want an error", c[0], c[1])
		}
	}
}

// TestValidateInPropertyErrorMessage: the message must name a property, not a
// canonical unit string, which was the shape of the old bug.
func TestValidateInPropertyErrorMessage(t *testing.T) {
	svc := newTestService(t)
	err := svc.ValidateInProperty("m", "mass")
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := err.Error(); !contains(got, "length") || !contains(got, "mass") {
		t.Errorf("error = %q, want it to name both the actual property (length) and the expected one (mass)", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestValidateInPropertyAtomicIsStrict: an atomic unit is judged by its declared
// property alone. Falling back to a dimensional match would accept these, since
// mol and 1 share a canonical form and so do m and [hand].
func TestValidateInPropertyAtomicIsStrict(t *testing.T) {
	svc := newTestService(t)
	cases := [][2]string{
		{"mol", "fraction"},
		{"mol", "amount of information"},
		{"m", "height of horses"},
		{"m", "depth of water"},
	}
	for _, c := range cases {
		if err := svc.ValidateInProperty(c[0], c[1]); err == nil {
			t.Errorf("ValidateInProperty(%q, %q) = nil, want an error: %q is declared for a different property",
				c[0], c[1], c[0])
		}
	}
	// The declared property still resolves.
	if err := svc.ValidateInProperty("mol", "amount of substance"); err != nil {
		t.Errorf(`ValidateInProperty("mol", "amount of substance") = %v, want nil`, err)
	}
	if err := svc.ValidateInProperty("[hd_i]", "height of horses"); err != nil {
		t.Errorf(`ValidateInProperty("[hd_i]", "height of horses") = %v, want nil`, err)
	}
}

// TestMultiplyNonRationalScale covers the float64 fallback of Multiply, the
// symmetric case of TestDivideNonRationalScale. Without it the branch is
// verified on the division side only.
func TestMultiplyNonRationalScale(t *testing.T) {
	svc := newTestService(t)
	// 1 pH = 0.1 mol/L; times 2 (dimensionless) gives 0.2 mol/L in canonical
	// form, which is 0.2 * 6.02214076e26 m-3.
	got, err := svc.Multiply(Pair{1, "[pH]"}, Pair{2, "1"})
	if err != nil {
		t.Fatal(err)
	}
	want := 0.2 * 6.02214076e26
	if math.Abs(got.Value-want) > want*1e-12 {
		t.Errorf("Multiply(1 [pH], 2).Value = %v, want %v", got.Value, want)
	}
	if got.Code != "m-3" {
		t.Errorf("Multiply(1 [pH], 2).Code = %q, want %q", got.Code, "m-3")
	}
}

// TestMultiplyNonFinite covers the other fallback trigger: an operand with no
// rational representation.
func TestMultiplyNonFinite(t *testing.T) {
	svc := newTestService(t)
	got, err := svc.Multiply(Pair{math.Inf(1), "m"}, Pair{2, "m"})
	if err != nil {
		t.Fatal(err)
	}
	if !math.IsInf(got.Value, 1) {
		t.Errorf("Multiply(+Inf m, 2 m).Value = %v, want +Inf", got.Value)
	}
	if got.Code != "m2" {
		t.Errorf("Multiply(+Inf m, 2 m).Code = %q, want %q", got.Code, "m2")
	}
}

// TestAnalyzeEmptyVersusValidate documents the deliberate disagreement: the
// official suite requires Analyze("") to describe the unity, while "" is not a
// valid code.
func TestAnalyzeEmptyVersusValidate(t *testing.T) {
	svc := newTestService(t)
	got, err := svc.Analyze("")
	if err != nil {
		t.Fatalf(`Analyze("") = %v, want no error`, err)
	}
	if got != "(unity)" {
		t.Errorf(`Analyze("") = %q, want "(unity)"`, got)
	}
	if err := svc.Validate(""); err == nil {
		t.Error(`Validate("") = nil, want an error: the empty string is not a valid code`)
	}
}

// TestModelValuesAreExact checks that the definitions are held exactly rather
// than as rounded floats, which is what the exact API depends on. The model is
// unexported, so this reaches the values the way the package itself does.
func TestModelValuesAreExact(t *testing.T) {
	svc := newTestService(t)

	prefixes := map[string]string{
		"m":  "1/1000",
		"k":  "1000",
		"u":  "1/1000000",
		"Ki": "1024",
	}
	for code, want := range prefixes {
		p := svc.model.LookupPrefix(code)
		if p == nil {
			t.Errorf("prefix %q not found", code)
			continue
		}
		if got := p.Value.Rat().RatString(); got != want {
			t.Errorf("prefix %q = %s, want %s", code, got, want)
		}
		// String is lossy, which is why nothing reads a definition through it.
		if code == "m" && p.Value.String() == want {
			t.Error("decimal.String() unexpectedly produced an exact fraction")
		}
	}

	// Multipliers are relative to the definition's own unit, not to a base unit:
	// [in_i] is declared as 254e-2 cm, so the multiplier is 127/50 over "cm".
	units := map[string]struct{ value, unit string }{
		"[in_i]": {"127/50", "cm"},
		"min":    {"60", "s"},
		"L":      {"1", "l"},
	}
	for code, want := range units {
		u := svc.model.Lookup(code)
		if u == nil || u.Value == nil {
			t.Errorf("unit %q has no definition", code)
			continue
		}
		if got := u.Value.Value.Rat().RatString(); got != want.value {
			t.Errorf("unit %q multiplier = %s, want %s", code, got, want.value)
		}
		if u.Value.Unit != want.unit {
			t.Errorf("unit %q is defined over %q, want %q", code, u.Value.Unit, want.unit)
		}
	}

	// decimal.rat returns a copy, so a caller cannot mutate a definition.
	p := svc.model.LookupPrefix("k")
	r := p.Value.Rat()
	r.Mul(r, big.NewRat(7, 1))
	if again := p.Value.Rat().RatString(); again != "1000" {
		t.Errorf("mutating a copy changed the definition: prefix k reads %s", again)
	}
}

// TestAnalyzeKeepsGroups is the same rule in the public Analyze, whose output is
// read by a person. A description of "mL/(kg.min)" that reads as "mL/kg.min"
// names a different unit; the brackets are square so they cannot be confused
// with the parentheses Analyze already puts around every unit name.
func TestAnalyzeKeepsGroups(t *testing.T) {
	svc := newTestService(t)

	tests := []struct {
		code string
		want string
	}{
		{"mL/(kg.min)", "(milliliter) / [(kilogram) * (minute)]"},
		{"kg/(s.m2)", "(kilogram) / [(second) * (meter ^ 2)]"},
		{"(kg/s).m2", "(kilogram) / (second) * (meter ^ 2)"},
		{"kg.m/s2", "(kilogram) * (meter) / (second ^ 2)"},
		{"(m)", "(meter)"},
	}
	for _, tt := range tests {
		got, err := svc.Analyze(tt.code)
		if err != nil {
			t.Errorf("Analyze(%q): %v", tt.code, err)
			continue
		}
		if got != tt.want {
			t.Errorf("Analyze(%q) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

// Issue #4: lgTimes2 units must compute 10^(v/2), not 10^(2v).
func TestLgTimes2Units(t *testing.T) {
	svc := newTestService(t)
	tests := []struct {
		value    float64
		from, to string
		want     float64
	}{
		{0, "B[V]", "V", 1},
		{1, "B[V]", "V", math.Sqrt(10)},
		{2, "B[V]", "V", 10},
		{10, "B[V]", "V", 1e5},
		{1, "B[mV]", "mV", math.Sqrt(10)},
		{2, "B[uV]", "uV", 10},
		// plain lg / ln / ld units must be unaffected
		{1, "B", "1", 10},
		{1, "B[W]", "W", 10},
		{2, "B[kW]", "kW", 100},
		{1, "Np", "1", math.E},
		{8, "bit_s", "1", 256},
		{1, "[pH]", "mol/L", 0.1},
		{1, "[hp'_X]", "1", 0.1},
	}
	for _, tt := range tests {
		got, err := svc.Convert(tt.value, tt.from, tt.to)
		if err != nil {
			t.Fatalf("Convert(%v, %q, %q): %v", tt.value, tt.from, tt.to, err)
		}
		if math.Abs(got-tt.want) > math.Abs(tt.want)*1e-12+1e-15 {
			t.Errorf("Convert(%v, %q, %q) = %v, want %v", tt.value, tt.from, tt.to, got, tt.want)
		}
	}
}

// Issue #5: B[SPL] reference level is 2x10^-5 Pa.
func TestBelSPLReferenceLevel(t *testing.T) {
	svc := newTestService(t)
	// 0 B[SPL] is the reference level itself: 20 uPa.
	got, err := svc.Convert(0, "B[SPL]", "Pa")
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got-2e-5) > 1e-20 {
		t.Errorf("Convert(0, B[SPL], Pa) = %v, want 2e-05", got)
	}
	// 1 B[SPL] = 2e-5 * 10^0.5
	got, err = svc.Convert(1, "B[SPL]", "Pa")
	if err != nil {
		t.Fatal(err)
	}
	want := 2e-5 * math.Sqrt(10)
	if math.Abs(got-want) > want*1e-12 {
		t.Errorf("Convert(1, B[SPL], Pa) = %v, want %v", got, want)
	}
}

// Issue #6: %[slope] must express its result in its declared unit (deg).
func TestPercentSlope(t *testing.T) {
	svc := newTestService(t)
	tests := []struct {
		value    float64
		from, to string
		want     float64
	}{
		{100, "%[slope]", "deg", 45},
		{100, "%[slope]", "rad", math.Pi / 4},
		{0, "%[slope]", "deg", 0},
		// [p'diop] is declared against rad and must keep working.
		{100, "[p'diop]", "rad", math.Pi / 4},
		{0, "[p'diop]", "rad", 0},
	}
	for _, tt := range tests {
		got, err := svc.Convert(tt.value, tt.from, tt.to)
		if err != nil {
			t.Fatalf("Convert(%v, %q, %q): %v", tt.value, tt.from, tt.to, err)
		}
		if math.Abs(got-tt.want) > math.Abs(tt.want)*1e-12+1e-15 {
			t.Errorf("Convert(%v, %q, %q) = %v, want %v", tt.value, tt.from, tt.to, got, tt.want)
		}
	}
}

// Issue #7: arbitrary units are commensurable with nothing but themselves.
func TestArbitraryUnitsNotCommensurable(t *testing.T) {
	svc := newTestService(t)
	incompatible := [][2]string{
		{"[IU]", "[iU]"},
		{"[IU]", "1"},
		{"[IU]", "mol"},
		{"[arb'U]", "[IU]"},
		{"[USP'U]", "[IU]"},
		{"[PFU]", "[CFU]"},
	}
	for _, p := range incompatible {
		ok, err := svc.IsComparable(p[0], p[1])
		if err != nil {
			t.Fatalf("IsComparable(%q, %q): %v", p[0], p[1], err)
		}
		if ok {
			t.Errorf("IsComparable(%q, %q) = true, want false", p[0], p[1])
		}
		if _, err := svc.Convert(1, p[0], p[1]); err == nil {
			t.Errorf("Convert(1, %q, %q) = nil error, want an error", p[0], p[1])
		}
	}
}

// TestArbitraryUnitsSelfConsistent: an arbitrary unit is its own dimension, so
// it converts to itself and scales with the rest of a compound expression.
func TestArbitraryUnitsSelfConsistent(t *testing.T) {
	svc := newTestService(t)
	tests := []struct {
		value    float64
		from, to string
		want     float64
	}{
		{1, "[IU]", "[IU]", 1},
		{5, "[IU]/L", "[IU]/mL", 0.005},
		{1, "[IU]/mL", "[IU]/L", 1000},
		{2, "k[IU]/L", "[IU]/L", 2000},
	}
	for _, tt := range tests {
		got, err := svc.Convert(tt.value, tt.from, tt.to)
		if err != nil {
			t.Fatalf("Convert(%v, %q, %q): %v", tt.value, tt.from, tt.to, err)
		}
		if math.Abs(got-tt.want) > math.Abs(tt.want)*1e-12+1e-15 {
			t.Errorf("Convert(%v, %q, %q) = %v, want %v", tt.value, tt.from, tt.to, got, tt.want)
		}
	}
	// Canonical keeps the arbitrary unit rather than reducing it to a number.
	p, err := svc.Canonical(1, "[IU]")
	if err != nil {
		t.Fatal(err)
	}
	if p.Code != "[IU]" {
		t.Errorf("Canonical(1, [IU]).Code = %q, want %q", p.Code, "[IU]")
	}
}

// A special unit inside an algebraic term denotes a difference, not a point on
// its scale: the offset cancels but the scale does not. 1 degF of difference is
// 5/9 K, so a gradient of 1 [degF]/min is 5/9 K/min.
func TestSpecialInAlgebraicTermKeepsItsScale(t *testing.T) {
	svc := newTestService(t)
	tests := []struct {
		value    float64
		from, to string
		want     float64
		why      string
	}{
		{1, "Cel/min", "K/min", 1, "a Celsius degree and a kelvin are the same size"},
		{1, "[degF]/min", "K/min", 5.0 / 9.0, "a Fahrenheit degree is 5/9 of a kelvin"},
		{9, "[degF]/h", "Cel/h", 5, "9 degF of difference is 5 Cel"},
		{1, "[degRe]/min", "K/min", 1.25, "a Reaumur degree is 5/4 of a kelvin"},
		{1, "Cel/min", "[degF]/min", 9.0 / 5.0, "between two special units, both scales apply"},
		{100, "Cel.m", "K.m", 100, "the scale is 1, so the value carries over"},
		{1, "[degF].m", "K.m", 5.0 / 9.0, "and here it does not"},
	}
	for _, tt := range tests {
		got, err := svc.Convert(tt.value, tt.from, tt.to)
		if err != nil {
			t.Fatalf("Convert(%v, %q, %q): %v", tt.value, tt.from, tt.to, err)
		}
		if math.Abs(got-tt.want) > math.Abs(tt.want)*1e-12 {
			t.Errorf("Convert(%v, %q, %q) = %v, want %v (%s)",
				tt.value, tt.from, tt.to, got, tt.want, tt.why)
		}
	}
}

// A standalone special unit keeps meaning a point on its scale, offset included.
func TestStandaloneSpecialStillUsesTheOffset(t *testing.T) {
	svc := newTestService(t)
	tests := []struct {
		value    float64
		from, to string
		want     float64
	}{
		{0, "Cel", "K", 273.15},
		{32, "[degF]", "K", 273.15},
		{80, "[degRe]", "Cel", 100},
		{100, "[degF]", "Cel", 37.77777777777778},
	}
	for _, tt := range tests {
		got, err := svc.Convert(tt.value, tt.from, tt.to)
		if err != nil {
			t.Fatalf("Convert(%v, %q, %q): %v", tt.value, tt.from, tt.to, err)
		}
		if math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("Convert(%v, %q, %q) = %v, want %v", tt.value, tt.from, tt.to, got, tt.want)
		}
	}
	p, err := svc.Canonical(1, "Cel")
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(p.Value-274.15) > 1e-9 {
		t.Errorf("Canonical(1, Cel) = %v, want 274.15", p.Value)
	}
}

// An exponent on a special unit has no meaning: a squared temperature is not a
// difference, so no scale rescues it.
func TestSpecialWithExponentIsRejected(t *testing.T) {
	svc := newTestService(t)
	for _, code := range []string{"Cel2", "[degF]2", "Cel-1", "[degRe]3"} {
		if _, err := svc.Canonical(1, code); err == nil {
			t.Errorf("Canonical(1, %q) = nil error, want an error", code)
		}
	}
}

// A non-linear special unit has no multiplicative scale to fall back on: 10^-pH
// does not decompose into a factor times a value, so it cannot appear in an
// algebraic term at all.
func TestNonLinearSpecialInAlgebraicTermIsRejected(t *testing.T) {
	svc := newTestService(t)
	for _, code := range []string{"[pH]/L", "B.m", "B[V]/s", "[p'diop]/m", "Np.s"} {
		if _, err := svc.Canonical(1, code); err == nil {
			t.Errorf("Canonical(1, %q) = nil error, want an error", code)
		}
	}
	// Standalone they still work.
	for _, code := range []string{"[pH]", "B", "B[V]", "[p'diop]", "Np"} {
		if _, err := svc.Canonical(1, code); err != nil {
			t.Errorf("Canonical(1, %q) = %v, want nil", code, err)
		}
	}
}

// TestPrefixRequiresMetricAtom covers UCUM §11 ■1, "Only metric unit atoms may
// be combined with a prefix".
func TestPrefixRequiresMetricAtom(t *testing.T) {
	svc := newTestService(t)

	// Non-metric atoms: a prefixed form is not a valid code.
	// Every atom below is declared isMetric="no" in the definitions.
	rejected := []string{
		"k[ft_i]", "m[lb_av]", "k[in_i]", "c[pi]", "k[oz_av]", "m[yd_i]",
	}
	for _, code := range rejected {
		if err := svc.Validate(code); err == nil {
			t.Errorf("Validate(%q) = nil, want an error: the atom is not metric", code)
		}
	}

	// Metric atoms, base units, and bracket units that are metric: still fine.
	accepted := []string{
		"mm", "kg", "kL", "cm3", "us", "nmol",
		"k[IU]", "m[IU]", "k[iU]", // [IU] and [iU] are isMetric="yes"
		"mCel",                        // §22 ■3 allows a prefix on a special unit
		"m[H2O]", "cm[H2O]", "mm[Hg]", // metric bracket units
		"[ft_i]", "[lb_av]", "[in_i]", "[pi]", // unprefixed, always valid
	}
	for _, code := range accepted {
		if err := svc.Validate(code); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", code, err)
		}
	}
}

// TestPrefixedNonMetricDoesNotConvert: the codes above must not sneak through
// into conversion either, which is what makes this more than a validation nicety.
func TestPrefixedNonMetricDoesNotConvert(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.Convert(1, "k[ft_i]", "m"); err == nil {
		t.Error(`Convert(1, "k[ft_i]", "m") = nil error, want an error`)
	}
	if _, err := svc.Canonical(1, "k[ft_i]"); err == nil {
		t.Error(`Canonical(1, "k[ft_i]") = nil error, want an error`)
	}
}

// TestIntegerFactorTakesNoExponent pins a rule where the specification's prose
// and its conformance suite disagree, and the suite wins.
//
// The prose of §9 says "the plus sign on positive exponents can be used to
// delimit exponents from integer numbers used as simple units. Thus, 2+10 means
// 2^10 = 1024", which reads as though an integer factor could be raised to a
// power. The official suite says otherwise, and says why:
//
//	id=1-107  unit="10*+3/ul"  valid=true
//	id=1-108  unit="10+3/ul"   valid=false   reason="10 is not a valid unit"
//
// An integer is a number, not a unit atom, so nothing can be raised. Powers of
// ten are written with the unit atom "10*", which is what 1-107 shows. This test
// exists because implementing the prose reading breaks case 1-108, which is how
// the contradiction surfaced.
func TestIntegerFactorTakesNoExponent(t *testing.T) {
	svc := newTestService(t)

	// Rejected: an exponent on a bare integer.
	for _, code := range []string{"10+3/ul", "2+10", "2-10", "3+2.m", "m.2+3"} {
		if err := svc.Validate(code); err == nil {
			t.Errorf("Validate(%q) = nil, want an error: an integer is not a unit atom", code)
		}
	}

	// Accepted: the same magnitudes written with the 10* unit atom.
	for _, code := range []string{"10*+3/ul", "10*3", "10*-3", "10*3/uL"} {
		if err := svc.Validate(code); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", code, err)
		}
	}

	// And a bare integer, or a product of integers, is still fine.
	for _, code := range []string{"123", "2.5", "2"} {
		if err := svc.Validate(code); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", code, err)
		}
	}
	// The period is multiplication, so "2.5" is 2 x 5.
	if got, err := svc.Convert(1, "2.5", "1"); err != nil || got != 10 {
		t.Errorf(`Convert(1, "2.5", "1") = %v (err %v), want 10`, got, err)
	}
	if got, err := svc.Convert(1, "10*+3", "1"); err != nil || got != 1000 {
		t.Errorf(`Convert(1, "10*+3", "1") = %v (err %v), want 1000`, got, err)
	}
}

// TestBelReferenceLevels pins the reference level of every unit defined with a
// logarithmic function, which is what value=N over model.Unit=X in the XML sets.
// Reading the reported unit alone is not enough: B[SPL] is 2x10^-5 Pa and
// B[10.nV] is 10 nV, while the rest are one of their reported unit.
func TestBelReferenceLevels(t *testing.T) {
	svc := newTestService(t)
	// At 0 bel the value is exactly the reference level.
	tests := []struct {
		code, in string
		want     float64
	}{
		{"B[V]", "V", 1},
		{"B[mV]", "mV", 1},
		{"B[uV]", "uV", 1},
		{"B[10.nV]", "nV", 10},  // value="10" Unit="nV"
		{"B[SPL]", "Pa", 2e-05}, // value="2" Unit="10*-5.Pa"
		{"B[W]", "W", 1},
		{"B[kW]", "kW", 1},
	}
	for _, tt := range tests {
		got, err := svc.Convert(0, tt.code, tt.in)
		if err != nil {
			t.Fatalf("Convert(0, %q, %q): %v", tt.code, tt.in, err)
		}
		if !almostEqual(got, tt.want, math.Abs(tt.want)*1e-12) {
			t.Errorf("Convert(0, %q, %q) = %v, want %v (the reference level)",
				tt.code, tt.in, got, tt.want)
		}
	}
}

// TestAllSpecialHandlersAgainstSpec audits every special unit against the
// normative definition table of the UCUM specification, so the class is pinned
// rather than the instances that happened to be reported.
//
// Four bugs in this family were found one at a time by following individual
// reports (the [degRe] offset, the lgTimes2 exponent, the B[SPL] reference and
// the B[10.nV] reference). Each time, the defect was present in a neighbor that
// nobody had looked at. The table below is derived from UCUM's Tables 18, 19, 20
// and 21 instead, and every expectation is an independently checkable physical
// reference point rather than a restatement of the handler's own formula.
func TestAllSpecialHandlersAgainstSpec(t *testing.T) {
	svc := newTestService(t)

	tests := []struct {
		code string // the special unit
		spec string // its definition in the UCUM specification
		in   float64
		to   string
		want float64
		why  string
	}{
		// Table 20, Levels. "ln", "lg" and "2lg" are the natural logarithm, the
		// decadic logarithm, and the decadic logarithm times two, with their
		// respective inverse functions.
		{"Np", "ln(1 1)", 1, "1", math.E, "1 Np is a ratio of e"},
		{"B", "lg(1 1)", 1, "1", 10, "1 B is a ratio of 10"},
		{"B", "lg(1 1)", 3, "1", 1000, "3 B is a ratio of 1000"},
		{"B[W]", "lg(1 W)", 1, "W", 10, "reference is 1 W"},
		{"B[kW]", "lg(1 kW)", 1, "kW", 10, "reference is 1 kW"},
		{"B[V]", "2lg(1 V)", 0, "V", 1, "reference is 1 V"},
		{"B[V]", "2lg(1 V)", 2, "V", 10, "2lg means the inverse is 10^(v/2)"},
		{"B[mV]", "2lg(1 mV)", 2, "mV", 10, "reference is 1 mV"},
		{"B[uV]", "2lg(1 uV)", 2, "uV", 10, "reference is 1 uV"},
		{"B[10.nV]", "2lg(10 nV)", 0, "nV", 10, "reference is 10 nV, not 1 nV"},
		{"B[SPL]", "2lg(2 10*-5.Pa)", 0, "Pa", 2e-5, "reference is 20 uPa, the hearing threshold"},
		{"B[SPL]", "2lg(2 10*-5.Pa)", 2, "Pa", 2e-4, "two bels above the reference is ten times it"},

		// Table 19. f_pH^-1(x) = 10^-x converts a pH value back to moles per liter.
		{"[pH]", "pH(1 mol/l)", 7, "mol/L", 1e-7, "pH 7 is 1e-7 mol/L, neutral water"},
		{"[pH]", "pH(1 mol/l)", 0, "mol/L", 1, "pH 0 is 1 mol/L"},

		// Table 22. "ld" is the binary logarithm.
		{"bit_s", "ld(1 1)", 8, "1", 256, "8 bits distinguish 256 states"},

		// Table 18. f_PD^-1(x) = arctan(x/100) converts a prism diopter or slope
		// percent value back to a plane angle.
		{"[p'diop]", "100tan(1 rad)", 100, "rad", math.Pi / 4, "100 PD is a 45 degree deflection"},
		{"%[slope]", "100tan(1 rad)", 100, "deg", 45, "a 100% slope is a 45 degree incline"},
		{"%[slope]", "100tan(1 rad)", 100, "rad", math.Pi / 4, "the same angle in radians"},

		// Table 18, retired homeopathic series. f_hpX^-1(x) = 10^-x, f_hpC^-1(x)
		// = 100^-x, and analogous functions with bases 1,000 and 50,000.
		{"[hp'_X]", "hpX(1 1)", 1, "1", 0.1, "1X is a one in ten dilution"},
		{"[hp'_X]", "hpX(1 1)", 3, "1", 1e-3, "3X is one in a thousand"},
		{"[hp'_C]", "hpC(1 1)", 1, "1", 0.01, "1C is a one in a hundred dilution"},
		{"[hp'_M]", "hpM(1 1)", 1, "1", 1e-3, "1M is a one in a thousand dilution"},
		{"[hp'_Q]", "hpQ(1 1)", 1, "1", 1.0 / 50000, "1Q is a one in fifty thousand dilution"},

		// Table 21. "sqrt" is the square root with the square as its inverse.
		{"[m/s2/Hz^(1/2)]", "sqrt(1 m2/s4/Hz)", 3, "m2/s4/Hz", 9, "the inverse of the square root is the square"},

		// Temperature, Table 6. Reference points are physical, not formulaic.
		{"Cel", "cel(1 K)", 0, "K", 273.15, "the freezing point of water"},
		{"[degF]", "degf(5 K/9)", 32, "K", 273.15, "32 F is the freezing point"},
		{"[degF]", "degf(5 K/9)", 212, "K", 373.15, "212 F is the boiling point"},
		{"[degRe]", "degre(5 K/4)", 80, "K", 373.15, "80 Re is the boiling point"},
	}

	for _, tt := range tests {
		t.Run(tt.code+"_"+tt.to, func(t *testing.T) {
			got, err := svc.Convert(tt.in, tt.code, tt.to)
			if err != nil {
				t.Fatalf("Convert(%v, %q, %q): %v", tt.in, tt.code, tt.to, err)
			}
			tol := math.Abs(tt.want) * 1e-12
			if tol == 0 {
				tol = 1e-12
			}
			if math.Abs(got-tt.want) > tol {
				t.Errorf("Convert(%v, %q, %q) = %v, want %v\n  spec: %s\n  why:  %s",
					tt.in, tt.code, tt.to, got, tt.want, tt.spec, tt.why)
			}
		})
	}
}

// TestEverySpecialUnitHasAHandler guards against a special unit being added to
// the definitions without a handler, which would surface only when someone
// converted it.
func TestEverySpecialUnitHasAHandler(t *testing.T) {
	svc := newTestService(t)
	handlers := svc.handlers

	var specials int
	for _, du := range svc.model.DefinedUnits {
		if !du.IsSpecial {
			continue
		}
		specials++
		if _, ok := handlers[du.Code]; !ok {
			t.Errorf("special unit %q has no handler", du.Code)
		}
		if _, err := svc.Canonical(1, du.Code); err != nil {
			t.Errorf("Canonical(1, %q): %v", du.Code, err)
		}
	}
	if specials != len(handlers) {
		t.Errorf("%d special units in the definitions but %d handlers registered",
			specials, len(handlers))
	}
	if specials != 21 {
		t.Errorf("found %d special units, want 21 for this version of the definitions", specials)
	}
}

func almostEqual(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}
