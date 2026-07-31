package ucum

import (
	"errors"
	"math/big"
	"testing"
)

func newExactTestService(t *testing.T) ExactService {
	t.Helper()
	svc, err := NewExact()
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestConversionFactorLinear(t *testing.T) {
	svc := newExactTestService(t)
	tests := []struct {
		from, to string
		want     string // RatString
	}{
		{"L", "mL", "1000"},
		{"mL", "L", "1/1000"},
		{"kg", "g", "1000"},
		{"m", "cm", "100"},
		{"h", "s", "3600"},
		{"mol/L", "mmol/L", "1000"},
		{"mg/dL", "g/L", "1/100"},
		{"[in_i]", "cm", "127/50"},
		{"L", "L", "1"},
	}
	for _, tt := range tests {
		got, err := svc.ConversionFactor(tt.from, tt.to)
		if err != nil {
			t.Fatalf("ConversionFactor(%q, %q): %v", tt.from, tt.to, err)
		}
		if got.RatString() != tt.want {
			t.Errorf("ConversionFactor(%q, %q) = %s, want %s", tt.from, tt.to, got.RatString(), tt.want)
		}
	}
}

// TestConversionFactorRejectsSpecial covers the design point: between Cel and
// [degF] the relation is affine, so no single factor describes it.
func TestConversionFactorRejectsSpecial(t *testing.T) {
	svc := newExactTestService(t)
	pairs := [][2]string{
		{"Cel", "K"}, {"K", "Cel"}, {"Cel", "[degF]"},
		{"[degF]", "K"}, {"[degRe]", "K"}, {"[pH]", "mol/L"}, {"B", "1"},
	}
	for _, p := range pairs {
		_, err := svc.ConversionFactor(p[0], p[1])
		if !errors.Is(err, ErrNotLinear) {
			t.Errorf("ConversionFactor(%q, %q) error = %v, want ErrNotLinear", p[0], p[1], err)
		}
	}
}

func TestConversionFactorRejectsIncommensurable(t *testing.T) {
	svc := newExactTestService(t)
	if _, err := svc.ConversionFactor("m", "s"); err == nil {
		t.Error(`ConversionFactor("m", "s") = nil error, want an error`)
	}
	if _, err := svc.ConversionFactor("cm2", "cm"); err == nil {
		t.Error(`ConversionFactor("cm2", "cm") = nil error, want an error`)
	}
	if _, err := svc.ConversionFactor("nope", "m"); err == nil {
		t.Error(`ConversionFactor("nope", "m") = nil error, want an error`)
	}
}

func TestConvertRatLinear(t *testing.T) {
	svc := newExactTestService(t)
	tests := []struct {
		value    string
		from, to string
		want     string
	}{
		{"1", "L", "mL", "1000"},
		{"1/3", "L", "mL", "1000/3"},
		{"2.5", "kg", "g", "2500"},
		{"1", "mg/dL", "g/L", "1/100"},
		{"0", "m", "cm", "0"},
	}
	for _, tt := range tests {
		v, ok := new(big.Rat).SetString(tt.value)
		if !ok {
			t.Fatalf("bad test literal %q", tt.value)
		}
		got, err := svc.ConvertRat(v, tt.from, tt.to)
		if err != nil {
			t.Fatalf("ConvertRat(%s, %q, %q): %v", tt.value, tt.from, tt.to, err)
		}
		if got.RatString() != tt.want {
			t.Errorf("ConvertRat(%s, %q, %q) = %s, want %s",
				tt.value, tt.from, tt.to, got.RatString(), tt.want)
		}
	}
}

// TestConvertRatAffine covers the second class: no single factor exists, but the
// mapping is rational, so the result is still exact.
func TestConvertRatAffine(t *testing.T) {
	svc := newExactTestService(t)
	tests := []struct {
		value    string
		from, to string
		want     string
	}{
		{"0", "Cel", "K", "5463/20"},      // 273.15
		{"1", "Cel", "K", "5483/20"},      // 274.15
		{"100", "Cel", "K", "7463/20"},    // 373.15
		{"32", "[degF]", "K", "5463/20"},  // 273.15
		{"100", "[degF]", "Cel", "340/9"}, // 37.777...
		{"212", "[degF]", "Cel", "100"},
		{"-40", "Cel", "[degF]", "-40"},
		{"80", "[degRe]", "Cel", "100"},
		{"0", "[degRe]", "K", "5463/20"},
		{"37", "Cel", "[degF]", "493/5"}, // 98.6
	}
	for _, tt := range tests {
		v, ok := new(big.Rat).SetString(tt.value)
		if !ok {
			t.Fatalf("bad test literal %q", tt.value)
		}
		got, err := svc.ConvertRat(v, tt.from, tt.to)
		if err != nil {
			t.Fatalf("ConvertRat(%s, %q, %q): %v", tt.value, tt.from, tt.to, err)
		}
		if got.RatString() != tt.want {
			t.Errorf("ConvertRat(%s, %q, %q) = %s, want %s",
				tt.value, tt.from, tt.to, got.RatString(), tt.want)
		}
	}
}

// TestConvertRatRejectsNonRational covers the third class: logarithmic,
// trigonometric and square-root scales have no exact rational result, so the
// API must refuse rather than return a rounded value typed as *big.Rat.
func TestConvertRatRejectsNonRational(t *testing.T) {
	svc := newExactTestService(t)
	pairs := [][2]string{
		{"[pH]", "mol/L"}, {"mol/L", "[pH]"},
		{"B", "1"}, {"Np", "1"}, {"bit_s", "1"},
		{"B[V]", "V"}, {"B[SPL]", "Pa"},
		{"[p'diop]", "rad"}, {"%[slope]", "deg"},
		{"[hp'_X]", "1"},
		{"[m/s2/Hz^(1/2)]", "m2/s4/Hz"},
	}
	for _, p := range pairs {
		_, err := svc.ConvertRat(big.NewRat(1, 1), p[0], p[1])
		if !errors.Is(err, ErrNotRational) {
			t.Errorf("ConvertRat(1, %q, %q) error = %v, want ErrNotRational", p[0], p[1], err)
		}
	}
}

func TestCanonicalRat(t *testing.T) {
	svc := newExactTestService(t)
	tests := []struct {
		value, code string
		wantVal     string
		wantCode    string
	}{
		{"1", "L", "1/1000", "m3"},
		{"1", "mL", "1/1000000", "m3"},
		{"1", "kg", "1000", "g"},
		{"3", "[in_i]", "381/5000", "m"},
		{"0", "Cel", "5463/20", "K"},
		{"1", "Cel", "5483/20", "K"},
		{"32", "[degF]", "5463/20", "K"},
		{"80", "[degRe]", "7463/20", "K"},
	}
	for _, tt := range tests {
		v, ok := new(big.Rat).SetString(tt.value)
		if !ok {
			t.Fatalf("bad test literal %q", tt.value)
		}
		got, err := svc.CanonicalRat(v, tt.code)
		if err != nil {
			t.Fatalf("CanonicalRat(%s, %q): %v", tt.value, tt.code, err)
		}
		if got.Value.RatString() != tt.wantVal {
			t.Errorf("CanonicalRat(%s, %q).Value = %s, want %s",
				tt.value, tt.code, got.Value.RatString(), tt.wantVal)
		}
		if got.Code != tt.wantCode {
			t.Errorf("CanonicalRat(%s, %q).Code = %q, want %q",
				tt.value, tt.code, got.Code, tt.wantCode)
		}
	}
}

func TestCanonicalRatRejectsNonRational(t *testing.T) {
	svc := newExactTestService(t)
	for _, code := range []string{"[pH]", "B", "B[V]", "[p'diop]", "Np"} {
		if _, err := svc.CanonicalRat(big.NewRat(1, 1), code); !errors.Is(err, ErrNotRational) {
			t.Errorf("CanonicalRat(1, %q) error = %v, want ErrNotRational", code, err)
		}
	}
}

func TestExactAPIRejectsNilValue(t *testing.T) {
	svc := newExactTestService(t)
	if _, err := svc.ConvertRat(nil, "L", "mL"); err == nil {
		t.Error("ConvertRat(nil, ...) = nil error, want an error")
	}
	if _, err := svc.CanonicalRat(nil, "L"); err == nil {
		t.Error("CanonicalRat(nil, ...) = nil error, want an error")
	}
}

// TestExactAPIDoesNotMutateInput guards against handing callers a *big.Rat that
// aliases a definition, or scribbling on their input.
func TestExactAPIDoesNotMutateInput(t *testing.T) {
	svc := newExactTestService(t)
	in := big.NewRat(1, 1)
	if _, err := svc.ConvertRat(in, "L", "mL"); err != nil {
		t.Fatal(err)
	}
	if in.RatString() != "1" {
		t.Errorf("ConvertRat mutated its input: %s", in.RatString())
	}

	// Two calls must not alias: mutating the first result cannot affect the
	// second.
	f1, err := svc.ConversionFactor("L", "mL")
	if err != nil {
		t.Fatal(err)
	}
	f1.Mul(f1, big.NewRat(7, 1))
	f2, err := svc.ConversionFactor("L", "mL")
	if err != nil {
		t.Fatal(err)
	}
	if f2.RatString() != "1000" {
		t.Errorf("ConversionFactor returned an aliased value: %s", f2.RatString())
	}
}

// TestNewIsExactService documents that the concrete service returned by New
// also satisfies ExactService, so callers do not have to switch constructors.
func TestNewIsExactService(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := svc.(ExactService); !ok {
		t.Error("New() result does not satisfy ExactService")
	}
}

// TestConvertMatchesConvertRat ties the float64 API to the exact one: Convert
// must return the single-rounding of the exact result whenever one exists.
func TestConvertMatchesConvertRat(t *testing.T) {
	svc := newExactTestService(t)
	tests := [][3]string{
		{"1", "L", "mL"},
		{"1", "mol/L", "mmol/L"},
		{"100", "[degF]", "Cel"},
		{"0", "Cel", "K"},
		{"37", "Cel", "[degF]"},
		{"80", "[degRe]", "Cel"},
		{"3", "[in_i]", "cm"},
	}
	for _, tt := range tests {
		v, ok := new(big.Rat).SetString(tt[0])
		if !ok {
			t.Fatalf("bad test literal %q", tt[0])
		}
		exact, err := svc.ConvertRat(v, tt[1], tt[2])
		if err != nil {
			t.Fatalf("ConvertRat(%s, %q, %q): %v", tt[0], tt[1], tt[2], err)
		}
		want, _ := exact.Float64()
		f, _ := v.Float64()
		got, err := svc.Convert(f, tt[1], tt[2])
		if err != nil {
			t.Fatalf("Convert(%v, %q, %q): %v", f, tt[1], tt[2], err)
		}
		if got != want {
			t.Errorf("Convert(%v, %q, %q) = %.20g, want %.20g (single rounding of %s)",
				f, tt[1], tt[2], got, want, exact.RatString())
		}
	}
}
