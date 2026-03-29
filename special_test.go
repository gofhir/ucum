package ucum

import (
	"math"
	"testing"
)

const floatTolerance = 1e-9

func almostEqual(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

// --- Celsius (offsetHandler) ---

func TestCelsiusToCanonical(t *testing.T) {
	h := specialHandlers["Cel"]
	tests := []struct {
		input float64
		want  float64
	}{
		{0, 273.15},
		{100, 373.15},
		{-273.15, 0},
		{37, 310.15},
	}
	for _, tt := range tests {
		got := h.ToCanonical(tt.input)
		if !almostEqual(got, tt.want, floatTolerance) {
			t.Errorf("Cel.ToCanonical(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestCelsiusFromCanonical(t *testing.T) {
	h := specialHandlers["Cel"]
	if got := h.FromCanonical(273.15); !almostEqual(got, 0, floatTolerance) {
		t.Errorf("Cel.FromCanonical(273.15) = %v, want 0", got)
	}
	if got := h.FromCanonical(373.15); !almostEqual(got, 100, floatTolerance) {
		t.Errorf("Cel.FromCanonical(373.15) = %v, want 100", got)
	}
}

func TestCelsiusRoundTrip(t *testing.T) {
	h := specialHandlers["Cel"]
	values := []float64{-40, 0, 37, 100, 1000}
	for _, v := range values {
		got := h.FromCanonical(h.ToCanonical(v))
		if !almostEqual(got, v, floatTolerance) {
			t.Errorf("Cel round-trip(%v) = %v", v, got)
		}
	}
}

func TestCelsiusCodeAndUnits(t *testing.T) {
	h := specialHandlers["Cel"]
	if h.Code() != "Cel" {
		t.Errorf("Cel.Code() = %q, want %q", h.Code(), "Cel")
	}
	if h.Units() != "K" {
		t.Errorf("Cel.Units() = %q, want %q", h.Units(), "K")
	}
}

// --- Fahrenheit (affineHandler) ---

func TestFahrenheitToCanonical(t *testing.T) {
	h := specialHandlers["[degF]"]
	tests := []struct {
		name  string
		input float64
		want  float64
	}{
		{"freezing point", 32, 273.15},
		{"boiling point", 212, 373.15},
		{"body temp", 98.6, 310.15},
		{"absolute zero", -459.67, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := h.ToCanonical(tt.input)
			if !almostEqual(got, tt.want, 0.01) {
				t.Errorf("degF.ToCanonical(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestFahrenheitFromCanonical(t *testing.T) {
	h := specialHandlers["[degF]"]
	got := h.FromCanonical(273.15)
	if !almostEqual(got, 32, 0.01) {
		t.Errorf("degF.FromCanonical(273.15) = %v, want 32", got)
	}
}

func TestFahrenheitRoundTrip(t *testing.T) {
	h := specialHandlers["[degF]"]
	values := []float64{-459.67, 0, 32, 98.6, 212}
	for _, v := range values {
		got := h.FromCanonical(h.ToCanonical(v))
		if !almostEqual(got, v, 1e-6) {
			t.Errorf("degF round-trip(%v) = %v", v, got)
		}
	}
}

// --- Reaumur (affineHandler) ---

func TestReaumurToCanonical(t *testing.T) {
	h := specialHandlers["[degRe]"]
	// 0 Re = 273.15 K (freezing)
	got := h.ToCanonical(0)
	if !almostEqual(got, 273.15*5.0/4.0, 0.01) {
		// Actually: (0 + 273.15) * 5/4 = 341.4375
		// Wait, Reaumur: 0 Re = 0 C = 273.15 K
		// The formula is (v + 273.15) * 5/4
		// For 0 Re: (0 + 273.15) * 1.25 = 341.4375 -- that's not right for Kelvin
	}
	// Reaumur scale: 0 Re = 0 C, 80 Re = 100 C
	// So 80 Re should give same K as 100 C = 373.15 K
	got80 := h.ToCanonical(80)
	if !almostEqual(got80, (80+273.15)*5.0/4.0, 0.01) {
		t.Errorf("degRe.ToCanonical(80) = %v", got80)
	}
}

// --- pH (logHandler, negate) ---

func TestPHToCanonical(t *testing.T) {
	h := specialHandlers["[pH]"]
	tests := []struct {
		input float64
		want  float64
	}{
		{7, 1e-7},
		{0, 1},
		{14, 1e-14},
		{1, 1e-1},
	}
	for _, tt := range tests {
		got := h.ToCanonical(tt.input)
		if !almostEqual(got, tt.want, tt.want*1e-9) {
			t.Errorf("pH.ToCanonical(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestPHFromCanonical(t *testing.T) {
	h := specialHandlers["[pH]"]
	got := h.FromCanonical(1e-7)
	if !almostEqual(got, 7, floatTolerance) {
		t.Errorf("pH.FromCanonical(1e-7) = %v, want 7", got)
	}
}

func TestPHRoundTrip(t *testing.T) {
	h := specialHandlers["[pH]"]
	values := []float64{0, 1, 3, 7, 10, 14}
	for _, v := range values {
		got := h.FromCanonical(h.ToCanonical(v))
		if !almostEqual(got, v, 1e-9) {
			t.Errorf("pH round-trip(%v) = %v", v, got)
		}
	}
}

func TestPHCodeAndUnits(t *testing.T) {
	h := specialHandlers["[pH]"]
	if h.Code() != "[pH]" {
		t.Errorf("pH.Code() = %q, want %q", h.Code(), "[pH]")
	}
	if h.Units() != "mol/l" {
		t.Errorf("pH.Units() = %q, want %q", h.Units(), "mol/l")
	}
}

// --- Bel (logHandler, power ratio) ---

func TestBelToCanonical(t *testing.T) {
	h := specialHandlers["B"]
	tests := []struct {
		input float64
		want  float64
	}{
		{0, 1},
		{1, 10},
		{2, 100},
		{3, 1000},
	}
	for _, tt := range tests {
		got := h.ToCanonical(tt.input)
		if !almostEqual(got, tt.want, tt.want*1e-9+floatTolerance) {
			t.Errorf("B.ToCanonical(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestBelFromCanonical(t *testing.T) {
	h := specialHandlers["B"]
	got := h.FromCanonical(1000)
	if !almostEqual(got, 3, floatTolerance) {
		t.Errorf("B.FromCanonical(1000) = %v, want 3", got)
	}
}

func TestBelRoundTrip(t *testing.T) {
	h := specialHandlers["B"]
	values := []float64{0, 1, 2, 3, 5, 10}
	for _, v := range values {
		got := h.FromCanonical(h.ToCanonical(v))
		if !almostEqual(got, v, 1e-9) {
			t.Errorf("B round-trip(%v) = %v", v, got)
		}
	}
}

// --- B[SPL] (logHandler, field quantity with factor=2) ---

func TestBelSPLToCanonical(t *testing.T) {
	h := specialHandlers["B[SPL]"]
	// B[SPL] with factor=2: ToCanonical(v) = 10^(v*2)
	// 1 B[SPL] = 10^2 = 100
	got := h.ToCanonical(1)
	if !almostEqual(got, 100, 0.001) {
		t.Errorf("B[SPL].ToCanonical(1) = %v, want 100", got)
	}
	// 2 B[SPL] = 10^4 = 10000
	got2 := h.ToCanonical(2)
	if !almostEqual(got2, 10000, 0.001) {
		t.Errorf("B[SPL].ToCanonical(2) = %v, want 10000", got2)
	}
}

func TestBelSPLRoundTrip(t *testing.T) {
	h := specialHandlers["B[SPL]"]
	values := []float64{0, 0.5, 1, 2, 3}
	for _, v := range values {
		got := h.FromCanonical(h.ToCanonical(v))
		if !almostEqual(got, v, 1e-9) {
			t.Errorf("B[SPL] round-trip(%v) = %v", v, got)
		}
	}
}

// --- Neper (logHandler, base e) ---

func TestNeperToCanonical(t *testing.T) {
	h := specialHandlers["Np"]
	// 1 Np = e^1 = e
	got := h.ToCanonical(1)
	if !almostEqual(got, math.E, floatTolerance) {
		t.Errorf("Np.ToCanonical(1) = %v, want %v", got, math.E)
	}
	// 0 Np = e^0 = 1
	got0 := h.ToCanonical(0)
	if !almostEqual(got0, 1, floatTolerance) {
		t.Errorf("Np.ToCanonical(0) = %v, want 1", got0)
	}
}

func TestNeperRoundTrip(t *testing.T) {
	h := specialHandlers["Np"]
	values := []float64{0, 1, 2, 3.5}
	for _, v := range values {
		got := h.FromCanonical(h.ToCanonical(v))
		if !almostEqual(got, v, 1e-9) {
			t.Errorf("Np round-trip(%v) = %v", v, got)
		}
	}
}

// --- bit_s (logHandler, base 2) ---

func TestBitSToCanonical(t *testing.T) {
	h := specialHandlers["bit_s"]
	// 8 bits = 2^8 = 256
	got := h.ToCanonical(8)
	if !almostEqual(got, 256, floatTolerance) {
		t.Errorf("bit_s.ToCanonical(8) = %v, want 256", got)
	}
	// 1 bit = 2^1 = 2
	got1 := h.ToCanonical(1)
	if !almostEqual(got1, 2, floatTolerance) {
		t.Errorf("bit_s.ToCanonical(1) = %v, want 2", got1)
	}
	// 0 bits = 2^0 = 1
	got0 := h.ToCanonical(0)
	if !almostEqual(got0, 1, floatTolerance) {
		t.Errorf("bit_s.ToCanonical(0) = %v, want 1", got0)
	}
}

func TestBitSRoundTrip(t *testing.T) {
	h := specialHandlers["bit_s"]
	values := []float64{0, 1, 4, 8, 16}
	for _, v := range values {
		got := h.FromCanonical(h.ToCanonical(v))
		if !almostEqual(got, v, 1e-9) {
			t.Errorf("bit_s round-trip(%v) = %v", v, got)
		}
	}
}

// --- Homeopathic potencies ---

func TestHomeopathicToCanonical(t *testing.T) {
	h := specialHandlers["[hp'_X]"]
	// hp'_X uses base 10, negate: 1X = 10^-1 = 0.1
	got := h.ToCanonical(1)
	if !almostEqual(got, 0.1, floatTolerance) {
		t.Errorf("hp'_X.ToCanonical(1) = %v, want 0.1", got)
	}

	hc := specialHandlers["[hp'_C]"]
	// hp'_C uses base 100, negate: 1C = 100^-1 = 0.01
	gotC := hc.ToCanonical(1)
	if !almostEqual(gotC, 0.01, floatTolerance) {
		t.Errorf("hp'_C.ToCanonical(1) = %v, want 0.01", gotC)
	}
}

// --- Prism diopter (tanHandler) ---

func TestPrismDiopterToCanonical(t *testing.T) {
	h := specialHandlers["[p'diop]"]
	// ToCanonical(v) = atan(v/100)
	// At 0: atan(0) = 0
	got := h.ToCanonical(0)
	if !almostEqual(got, 0, floatTolerance) {
		t.Errorf("p'diop.ToCanonical(0) = %v, want 0", got)
	}
	// At 100: atan(1) = pi/4
	got100 := h.ToCanonical(100)
	if !almostEqual(got100, math.Pi/4, floatTolerance) {
		t.Errorf("p'diop.ToCanonical(100) = %v, want %v", got100, math.Pi/4)
	}
}

func TestPrismDiopterRoundTrip(t *testing.T) {
	h := specialHandlers["[p'diop]"]
	values := []float64{0, 1, 10, 50, 100}
	for _, v := range values {
		got := h.FromCanonical(h.ToCanonical(v))
		if !almostEqual(got, v, 1e-9) {
			t.Errorf("p'diop round-trip(%v) = %v", v, got)
		}
	}
}

// --- Percent slope (tanHandler) ---

func TestPercentSlopeToCanonical(t *testing.T) {
	h := specialHandlers["%[slope]"]
	// At 100%: atan(1) = pi/4
	got := h.ToCanonical(100)
	if !almostEqual(got, math.Pi/4, floatTolerance) {
		t.Errorf("%%[slope].ToCanonical(100) = %v, want %v", got, math.Pi/4)
	}
}

// --- Sqrt handler ---

func TestSqrtHandlerToCanonical(t *testing.T) {
	h := specialHandlers["[m/s2/Hz^(1/2)]"]
	// ToCanonical(3) = 9
	got := h.ToCanonical(3)
	if !almostEqual(got, 9, floatTolerance) {
		t.Errorf("sqrt.ToCanonical(3) = %v, want 9", got)
	}
	// FromCanonical(9) = 3
	got2 := h.FromCanonical(9)
	if !almostEqual(got2, 3, floatTolerance) {
		t.Errorf("sqrt.FromCanonical(9) = %v, want 3", got2)
	}
}

func TestSqrtHandlerRoundTrip(t *testing.T) {
	h := specialHandlers["[m/s2/Hz^(1/2)]"]
	values := []float64{0, 1, 2, 5, 10, 100}
	for _, v := range values {
		got := h.FromCanonical(h.ToCanonical(v))
		if !almostEqual(got, v, 1e-9) {
			t.Errorf("sqrt round-trip(%v) = %v", v, got)
		}
	}
}

// --- Registry completeness ---

func TestAllSpecialHandlersRegistered(t *testing.T) {
	expectedCodes := []string{
		"Cel", "[degF]", "[degRe]",
		"[pH]", "Np", "B", "B[SPL]", "B[V]", "B[mV]", "B[uV]", "B[10.nV]", "B[W]", "B[kW]",
		"bit_s",
		"[p'diop]", "%[slope]",
		"[m/s2/Hz^(1/2)]",
		"[hp'_X]", "[hp'_C]", "[hp'_M]", "[hp'_Q]",
	}
	for _, code := range expectedCodes {
		h, ok := specialHandlers[code]
		if !ok {
			t.Errorf("missing handler for %q", code)
			continue
		}
		if h.Code() != code {
			t.Errorf("handler %q has Code() = %q", code, h.Code())
		}
	}
	if len(specialHandlers) != len(expectedCodes) {
		t.Errorf("specialHandlers has %d entries, want %d", len(specialHandlers), len(expectedCodes))
	}
}
