package decimal

import (
	"math"
	"math/big"
	"testing"
)

func TestDecimalFromString(t *testing.T) {
	tests := []struct {
		input   string
		float64 float64
	}{
		{"1", 1},
		{"0.001", 0.001},
		{"1e3", 1000},
		{"1e-24", 1e-24},
		{"2.54", 2.54},
		{"1e24", 1e24},
		{"5", 5},
		{"0.0254", 0.0254},
	}
	for _, tt := range tests {
		d, err := FromString(tt.input)
		if err != nil {
			t.Errorf("FromString(%q) error: %v", tt.input, err)
			continue
		}
		if got := d.Float64(); got != tt.float64 {
			t.Errorf("FromString(%q).Float64() = %v, want %v", tt.input, got, tt.float64)
		}
	}
}

func TestDecimalExactDivision(t *testing.T) {
	one := FromInt(1)
	three := FromInt(3)
	result := one.Div(three).Mul(three)
	if !result.Equal(one) {
		t.Errorf("1/3*3 = %v, want exactly 1", result.Float64())
	}
}

func TestDecimalExactChain(t *testing.T) {
	// Simulate Celsius->Fahrenheit round-trip factor: 5/9 * 9/5 = 1
	five := FromInt(5)
	nine := FromInt(9)
	factor := five.Div(nine).Mul(nine).Div(five)
	if !factor.Equal(FromInt(1)) {
		t.Errorf("5/9 * 9/5 = %v, want exactly 1", factor.Float64())
	}
}

func TestDecimalPow(t *testing.T) {
	two := FromInt(2)
	if got := two.Pow(10).Float64(); got != 1024 {
		t.Errorf("2^10 = %v, want 1024", got)
	}
	if got := two.Pow(-3).Float64(); got != 0.125 {
		t.Errorf("2^-3 = %v, want 0.125", got)
	}
	if got := two.Pow(0).Float64(); got != 1 {
		t.Errorf("2^0 = %v, want 1", got)
	}
}

func TestDecimalPrefixRange(t *testing.T) {
	// Verify yocto * yotta = 1
	yocto, _ := FromString("1e-24")
	yotta, _ := FromString("1e24")
	result := yocto.Mul(yotta)
	if !result.Equal(FromInt(1)) {
		t.Errorf("1e-24 * 1e24 = %v, want exactly 1", result.Float64())
	}
}

func TestDecimalArithmetic(t *testing.T) {
	a := FromInt(10)
	b := FromInt(3)
	if got := a.Add(b).Float64(); got != 13 {
		t.Errorf("10+3 = %v", got)
	}
	if got := a.Sub(b).Float64(); got != 7 {
		t.Errorf("10-3 = %v", got)
	}
	if got := a.Mul(b).Float64(); got != 30 {
		t.Errorf("10*3 = %v", got)
	}
	if !FromInt(0).IsZero() {
		t.Error("0 should be zero")
	}
}

// TestLog10RatPowersOfTen: UCUM define sus unidades con decimales, y el log10 de un
// prefijo redondeado movería el último dígito de una conversión logarítmica
// (1000 W en B[kW] es 0).
func TestLog10RatPowersOfTen(t *testing.T) {
	ten := big.NewInt(10)
	for k := int64(-1000); k <= 1000; k++ {
		p := new(big.Int).Exp(ten, big.NewInt(max(k, -k)), nil)
		r := new(big.Rat).SetInt(p)
		if k < 0 {
			r.Inv(r)
		}
		if got := Log10Rat(r); got != float64(k) {
			t.Errorf("Log10Rat(10^%d) = %v, want %d", k, got, k)
		}
	}
	if got := Log10Rat(new(big.Rat)); !math.IsInf(got, -1) {
		t.Errorf("Log10Rat(0) = %v, want -Inf", got)
	}
}

// TestLog10AbsSubnormals: log10(2^-k) = -k·log10(2), y log10(10^-k) = -k, en
// toda plataforma; en amd64 math.Log10 no los da (golang/go#56600).
func TestLog10AbsSubnormals(t *testing.T) {
	for k := 1; k <= 1074; k++ {
		v := math.Ldexp(1, -k)
		want := -float64(k) * math.Log10(2)
		for _, s := range []float64{1, -1} {
			if got := Log10Abs(s * v); math.Abs(got-want) > 1e-15*math.Abs(want) {
				t.Errorf("Log10Abs(%g·2^-%d) = %v, want %v", s, k, got, want)
			}
		}
	}
	for _, k := range []int{308, 310, 315, 320, 323} {
		v, _ := new(big.Rat).SetFrac(big.NewInt(1), new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(k)), nil)).Float64()
		// Un subnormal tiene menos cifras, así que v no es 10^-k: el esperado
		// es log10 del valor exacto de v, m·2^-1074 con m entero.
		m := math.Ldexp(v, 1074)
		want := math.Log10(m) - 1074*math.Log10(2)
		if got := Log10Abs(v); math.Abs(got-want) > 1e-15*math.Abs(want) {
			t.Errorf("Log10Abs(1e-%d) = %v, want %v", k, got, want)
		}
	}
	if got := Log10Abs(0); !math.IsInf(got, -1) {
		t.Errorf("Log10Abs(0) = %v, want -Inf", got)
	}
}

// TestLog10RatPowersOfTwo: log10(2^k) = k·log10(2); sin potencias de 10 que
// contar, Log10Rat pasa por la mantisa y el exponente binarios.
func TestLog10RatPowersOfTwo(t *testing.T) {
	two := big.NewInt(2)
	for k := int64(-1000); k <= 1000; k++ {
		r := new(big.Rat).SetInt(new(big.Int).Exp(two, big.NewInt(max(k, -k)), nil))
		if k < 0 {
			r.Inv(r)
		}
		want := float64(k) * math.Log10(2)
		if got := Log10Rat(r); math.Abs(got-want) > 1e-15*max(1, math.Abs(want)) {
			t.Errorf("Log10Rat(2^%d) = %v, want %v", k, got, want)
		}
	}
}
