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

// TestConvertExactWhenRepresentable pins the property that Convert rounds once:
// its result must be the float64 nearest to the exact rational conversion, not
// the quotient of two already-rounded canonical factors.
// Special units are excluded, since their handler applies an offset and the
// result is not a plain ratio of canonical factors.
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

// TestCanonicalExactForAffineScales pins Canonical to a single rounding for the
// rational special scales, the same guarantee Convert gives.
func TestCanonicalExactForAffineScales(t *testing.T) {
	svc, err := NewExact()
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
	svc, err := NewExact()
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
	s := svc.(*service)
	tests := []struct{ v1, v2 Pair }{
		{Pair{1, "L"}, Pair{1, "mL"}},
		{Pair{3, "mL"}, Pair{7, "uL"}},
		{Pair{1, "mg/dL"}, Pair{1, "g/L"}},
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
		exact := new(big.Rat).Quo(can1.value.rat(), can2.value.rat())
		exact.Mul(exact, new(big.Rat).Quo(ratFromFloat(tt.v1.Value), ratFromFloat(tt.v2.Value)))
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
			if _, err := svc.Divide(tc.v1, tc.v2); !errors.Is(err, errDivisionByZero) {
				t.Errorf("Divide(%v, %v) error = %v, want errDivisionByZero", tc.v1, tc.v2, err)
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
