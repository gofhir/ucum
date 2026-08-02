package decimal

import "testing"

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
