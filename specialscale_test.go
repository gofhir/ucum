package ucum

import (
	"math"
	"math/big"
	"testing"
)

// The rules under test come from UCUM §22, "Special Units on Non-Ratio Scales":
//
//	§22.3  "due to the requirement of the SI that does allow prefixes on the
//	        degree Celsius, special units may be scaled trough a prefix or an
//	        arbitrary numeric factor."
//	§22.4  "s = (u, f_s, f_s-1, α) ... x' = f_s(x) / α converts from x expressed
//	        in the corresponding proper unit to x' in terms of the special unit
//	        and x = f_s-1(α x') does the reverse."
//
// The scale factor is therefore applied to the *argument* of the conversion
// function, not to its result. Applying it to the result would move the origin
// of the scale with the prefix, which is what makes the distinction observable:
// 0 mCel has to be the same temperature as 0 Cel.

// TestPrefixScalesTheArgumentNotTheResult pins §22.4 for the affine scale.
func TestPrefixScalesTheArgumentNotTheResult(t *testing.T) {
	svc := newTestService(t)

	tests := []struct {
		value float64
		code  string
		want  float64 // kelvin
		why   string
	}{
		{0, "Cel", 273.15, "the origin of the Celsius scale"},
		{0, "mCel", 273.15, "a prefix does not move the origin: f(0.001*0) = f(0)"},
		{1, "mCel", 273.151, "f(0.001*1) = 0.001 + 273.15"},
		{1000, "mCel", 274.15, "1000 mCel is 1 Cel"},
		{1, "kCel", 1273.15, "f(1000*1) = 1000 + 273.15"},
		{0, "kCel", 273.15, "the origin again, unmoved"},
	}
	for _, tt := range tests {
		got, err := svc.Canonical(tt.value, tt.code)
		if err != nil {
			t.Errorf("Canonical(%v, %q) error: %v", tt.value, tt.code, err)
			continue
		}
		if math.Abs(got.Value-tt.want) > 1e-9 {
			t.Errorf("Canonical(%v, %q) = %v %s, want %v (%s)",
				tt.value, tt.code, got.Value, got.Code, tt.want, tt.why)
		}
	}
}

// TestPrefixedSpecialRoundTrips checks that the scaled scale is self-consistent,
// which is the property a moved origin breaks.
func TestPrefixedSpecialRoundTrips(t *testing.T) {
	svc := newTestService(t)

	tests := []struct {
		value    float64
		from, to string
		want     float64
	}{
		{1, "kCel", "Cel", 1000},
		{1000, "mCel", "Cel", 1},
		{1, "Cel", "mCel", 1000},
		{0, "Cel", "mCel", 0},
		{0, "mCel", "Cel", 0},
		{37, "Cel", "mCel", 37000},
	}
	for _, tt := range tests {
		got, err := svc.Convert(tt.value, tt.from, tt.to)
		if err != nil {
			t.Errorf("Convert(%v, %q, %q) error: %v", tt.value, tt.from, tt.to, err)
			continue
		}
		if math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("Convert(%v, %q, %q) = %v, want %v", tt.value, tt.from, tt.to, got, tt.want)
		}
	}
}

// TestDecibelIsTheLogarithmicCase is the same rule on a logarithmic scale, and
// the one most likely to be met in practice: a gain of 1 dB is a factor of
// 10^0.1, and one of 20 dB is a factor of 100. Scaling the result instead would
// make 1 dB a factor of 1, i.e. no gain at all.
func TestDecibelIsTheLogarithmicCase(t *testing.T) {
	svc := newTestService(t)

	tests := []struct {
		value float64
		code  string
		want  float64
	}{
		{1, "B", 10},                      // 1 bel is a factor of ten
		{1, "dB", math.Pow(10, 0.1)},      // ≈ 1.2589
		{10, "dB", 10},                    // 10 dB is 1 B
		{20, "dB", 100},                   // the textbook figure
		{-3, "dB", math.Pow(10, -0.3)},    // ≈ 0.5012, the half-power point
		{1, "cB", math.Pow(10, 0.01)},     // centibel, for completeness
		{1, "dNp", math.Pow(math.E, 0.1)}, // the neper takes prefixes too
	}
	for _, tt := range tests {
		got, err := svc.Convert(tt.value, tt.code, "1")
		if err != nil {
			t.Errorf("Convert(%v, %q, \"1\") error: %v", tt.value, tt.code, err)
			continue
		}
		if math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("Convert(%v, %q, \"1\") = %v, want %v", tt.value, tt.code, got, tt.want)
		}
	}
}

// TestPrefixedSpecialIsExact checks that the exact API applies the same rule,
// since a caller reaching for big.Rat is the one least willing to accept a
// silently different answer.
func TestPrefixedSpecialIsExact(t *testing.T) {
	ex, err := NewExact()
	if err != nil {
		t.Fatal(err)
	}

	// 1 mCel is exactly 273.151 K = 273151/1000.
	got, err := ex.CanonicalRat(big.NewRat(1, 1), "mCel")
	if err != nil {
		t.Fatalf("CanonicalRat(1, \"mCel\") error: %v", err)
	}
	want := new(big.Rat).SetFrac64(273151, 1000)
	if got.Value.Cmp(want) != 0 {
		t.Errorf("CanonicalRat(1, \"mCel\") = %v, want %v", got.Value, want)
	}

	// And the round trip is exact: 1 kCel is exactly 1000 Cel.
	conv, err := ex.ConvertRat(big.NewRat(1, 1), "kCel", "Cel")
	if err != nil {
		t.Fatalf("ConvertRat(1, \"kCel\", \"Cel\") error: %v", err)
	}
	if conv.Cmp(big.NewRat(1000, 1)) != 0 {
		t.Errorf("ConvertRat(1, \"kCel\", \"Cel\") = %v, want 1000", conv)
	}
}

// TestPrefixedSpecialAsDeltaScalesTheDifference covers the other reading. In an
// algebraic term a special unit denotes a difference, the offset cancels, and a
// prefix then scales that difference in the ordinary way: a gradient of 1000
// mCel/min is one of 1 Cel/min.
func TestPrefixedSpecialAsDeltaScalesTheDifference(t *testing.T) {
	svc := newTestService(t)

	got, err := svc.Convert(1000, "mCel/min", "Cel/min")
	if err != nil {
		t.Fatalf("Convert(1000, \"mCel/min\", \"Cel/min\") error: %v", err)
	}
	if math.Abs(got-1) > 1e-9 {
		t.Errorf("Convert(1000, \"mCel/min\", \"Cel/min\") = %v, want 1", got)
	}

	// The delta reading is unaffected by the origin, so a millidegree per minute
	// is a millikelvin per minute.
	got, err = svc.Convert(1, "mCel/min", "K/min")
	if err != nil {
		t.Fatalf("Convert(1, \"mCel/min\", \"K/min\") error: %v", err)
	}
	if math.Abs(got-0.001) > 1e-12 {
		t.Errorf("Convert(1, \"mCel/min\", \"K/min\") = %v, want 0.001", got)
	}
}

// TestRedundantParenthesesDoNotChangeMeaning pins that a parenthesised code is
// the same code. UCUM §22.1-2 says a special unit "cannot take part in any
// algebraic operations", and parentheses are not an operation — so "(Cel)" has
// to behave exactly like "Cel" rather than falling through to the difference
// reading, which would drop the offset and be wrong by 273.15.
func TestRedundantParenthesesDoNotChangeMeaning(t *testing.T) {
	svc := newTestService(t)

	for _, code := range []string{"Cel", "(Cel)", "((Cel))", "(((Cel)))"} {
		got, err := svc.Convert(1, code, "K")
		if err != nil {
			t.Errorf("Convert(1, %q, \"K\") error: %v", code, err)
			continue
		}
		if math.Abs(got-274.15) > 1e-9 {
			t.Errorf("Convert(1, %q, \"K\") = %v, want 274.15", code, got)
		}

		can, err := svc.Canonical(1, code)
		if err != nil {
			t.Errorf("Canonical(1, %q) error: %v", code, err)
			continue
		}
		if math.Abs(can.Value-274.15) > 1e-9 {
			t.Errorf("Canonical(1, %q) = %v, want 274.15", code, can.Value)
		}
	}

	// Prefixes and parentheses compose.
	got, err := svc.Convert(1, "(mCel)", "K")
	if err != nil {
		t.Fatalf("Convert(1, \"(mCel)\", \"K\") error: %v", err)
	}
	if math.Abs(got-273.151) > 1e-9 {
		t.Errorf("Convert(1, \"(mCel)\", \"K\") = %v, want 273.151", got)
	}

	// The non-rational scales too, which take the float64 path.
	got, err = svc.Convert(7, "([pH])", "mol/l")
	if err != nil {
		t.Fatalf("Convert(7, \"([pH])\", \"mol/l\") error: %v", err)
	}
	if math.Abs(got-1e-7) > 1e-16 {
		t.Errorf("Convert(7, \"([pH])\", \"mol/l\") = %v, want 1e-7", got)
	}

	// An exponent on a parenthesised special unit is still an exponent, and still
	// has to be refused.
	if _, err := svc.Convert(1, "(Cel)2", "K2"); err == nil {
		t.Error(`Convert(1, "(Cel)2", "K2") = nil error, want an error`)
	}
}

// TestRedundantParenthesesKeepTheDeclaredProperty is the same rule in
// ValidateInProperty. An atom is judged by the property UCUM declares for it,
// and a parenthesised atom is still an atom — otherwise it falls to the
// dimensional comparison, which cannot tell a dimensionless "amount of
// substance" from a "fraction".
func TestRedundantParenthesesKeepTheDeclaredProperty(t *testing.T) {
	svc := newTestService(t)

	for _, code := range []string{"mol", "(mol)", "((mol))"} {
		if err := svc.ValidateInProperty(code, "fraction"); err == nil {
			t.Errorf("ValidateInProperty(%q, \"fraction\") = nil, want an error: it measures amount of substance", code)
		}
		if err := svc.ValidateInProperty(code, "amount of substance"); err != nil {
			t.Errorf("ValidateInProperty(%q, \"amount of substance\") = %v, want nil", code, err)
		}
	}
}
