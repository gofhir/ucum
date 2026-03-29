package ucum

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
		d, err := decimalFromString(tt.input)
		if err != nil {
			t.Errorf("decimalFromString(%q) error: %v", tt.input, err)
			continue
		}
		if got := d.float64(); got != tt.float64 {
			t.Errorf("decimalFromString(%q).float64() = %v, want %v", tt.input, got, tt.float64)
		}
	}
}

func TestDecimalExactDivision(t *testing.T) {
	one := decimalFromInt(1)
	three := decimalFromInt(3)
	result := one.div(three).mul(three)
	if !result.equal(one) {
		t.Errorf("1/3*3 = %v, want exactly 1", result.float64())
	}
}

func TestDecimalExactChain(t *testing.T) {
	// Simulate Celsius->Fahrenheit round-trip factor: 5/9 * 9/5 = 1
	five := decimalFromInt(5)
	nine := decimalFromInt(9)
	factor := five.div(nine).mul(nine).div(five)
	if !factor.equal(decimalFromInt(1)) {
		t.Errorf("5/9 * 9/5 = %v, want exactly 1", factor.float64())
	}
}

func TestDecimalPow(t *testing.T) {
	two := decimalFromInt(2)
	if got := two.pow(10).float64(); got != 1024 {
		t.Errorf("2^10 = %v, want 1024", got)
	}
	if got := two.pow(-3).float64(); got != 0.125 {
		t.Errorf("2^-3 = %v, want 0.125", got)
	}
	if got := two.pow(0).float64(); got != 1 {
		t.Errorf("2^0 = %v, want 1", got)
	}
}

func TestDecimalPrefixRange(t *testing.T) {
	// Verify yocto * yotta = 1
	yocto, _ := decimalFromString("1e-24")
	yotta, _ := decimalFromString("1e24")
	result := yocto.mul(yotta)
	if !result.equal(decimalFromInt(1)) {
		t.Errorf("1e-24 * 1e24 = %v, want exactly 1", result.float64())
	}
}

func TestDecimalArithmetic(t *testing.T) {
	a := decimalFromInt(10)
	b := decimalFromInt(3)
	if got := a.add(b).float64(); got != 13 {
		t.Errorf("10+3 = %v", got)
	}
	if got := a.sub(b).float64(); got != 7 {
		t.Errorf("10-3 = %v", got)
	}
	if got := a.mul(b).float64(); got != 30 {
		t.Errorf("10*3 = %v", got)
	}
	if !decimalFromInt(0).isZero() {
		t.Error("0 should be zero")
	}
}
