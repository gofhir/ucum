package special

import (
	"math"
	"testing"

	"github.com/gofhir/ucum/v4/internal/essence"
)

const floatTolerance = 1e-9

func almostEqual(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

// handlerFor returns the conversion built for a special unit from its definition.
// The handlers are per-service now, since which function a unit performs is data
// in ucum-essence.xml rather than a fact about this package.
func handlerFor(t *testing.T, code string) Handler {
	t.Helper()
	m, err := essence.Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	handlers, err := BuildHandlers(m)
	if err != nil {
		t.Fatal(err)
	}
	h, ok := handlers[code]
	if !ok {
		t.Fatalf("no handler for special unit %q", code)
	}
	return h
}

// Celsius (offsetHandler) tests.

func TestCelsiusToCanonical(t *testing.T) {
	h := handlerFor(t, "Cel")
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
		got := h.toCanonical(tt.input)
		if !almostEqual(got, tt.want, floatTolerance) {
			t.Errorf("Cel.toCanonical(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestCelsiusFromCanonical(t *testing.T) {
	h := handlerFor(t, "Cel")
	if got := h.fromCanonical(273.15); !almostEqual(got, 0, floatTolerance) {
		t.Errorf("Cel.fromCanonical(273.15) = %v, want 0", got)
	}
	if got := h.fromCanonical(373.15); !almostEqual(got, 100, floatTolerance) {
		t.Errorf("Cel.fromCanonical(373.15) = %v, want 100", got)
	}
}

func TestCelsiusRoundTrip(t *testing.T) {
	h := handlerFor(t, "Cel")
	values := []float64{-40, 0, 37, 100, 1000}
	for _, v := range values {
		got := h.fromCanonical(h.toCanonical(v))
		if !almostEqual(got, v, floatTolerance) {
			t.Errorf("Cel round-trip(%v) = %v", v, got)
		}
	}
}

func TestCelsiusCodeAndReference(t *testing.T) {
	h := handlerFor(t, "Cel")
	if h.code() != "Cel" {
		t.Errorf("Cel.code() = %q, want %q", h.code(), "Cel")
	}
	// The reference quantity is data in the definitions, not a field on the
	// handler: Cel is declared cel(1 K).
	if got := referenceOf(t, "Cel"); got != "1.K" {
		t.Errorf("Cel reference = %q, want %q", got, "1.K")
	}
}

// Fahrenheit (affineHandler) tests.

// The handler moves the origin only, in the unit's own degrees: the size of a
// Fahrenheit degree is the reference quantity degf(5 K/9) and is applied by the
// canonicalizer. TestAllSpecialHandlersAgainstSpec covers the whole conversion.
func TestFahrenheitToCanonical(t *testing.T) {
	h := handlerFor(t, "[degF]")
	tests := []struct {
		name  string
		input float64
		want  float64
	}{
		{"freezing point", 32, 32 + 459.67},
		{"boiling point", 212, 212 + 459.67},
		{"body temp", 98.6, 98.6 + 459.67},
		{"absolute zero", -459.67, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := h.toCanonical(tt.input)
			if !almostEqual(got, tt.want, 0.01) {
				t.Errorf("degF.toCanonical(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestFahrenheitFromCanonical(t *testing.T) {
	h := handlerFor(t, "[degF]")
	// The canonicalizer has already divided by the scale, so the input is in
	// Fahrenheit degrees above absolute zero.
	got := h.fromCanonical(491.67)
	if !almostEqual(got, 32, 0.01) {
		t.Errorf("degF.fromCanonical(491.67) = %v, want 32", got)
	}
}

func TestFahrenheitRoundTrip(t *testing.T) {
	h := handlerFor(t, "[degF]")
	values := []float64{-459.67, 0, 32, 98.6, 212}
	for _, v := range values {
		got := h.fromCanonical(h.toCanonical(v))
		if !almostEqual(got, v, 1e-6) {
			t.Errorf("degF round-trip(%v) = %v", v, got)
		}
	}
}

// Reaumur (affineHandler) tests.

func TestReaumurToCanonical(t *testing.T) {
	h := handlerFor(t, "[degRe]")
	// Reaumur scale: 0 Re = 0 Cel, 80 Re = 100 Cel. Expected values come from
	// the physical definition, not from the handler's own formula.
	got := h.toCanonical(0)
	if !almostEqual(got, 218.52, 0.01) {
		t.Errorf("degRe.toCanonical(0) = %v, want 218.52 (Reaumur degrees above absolute zero)", got)
	}
	got80 := h.toCanonical(80)
	if !almostEqual(got80, 298.52, 0.01) {
		t.Errorf("degRe.toCanonical(80) = %v, want 298.52", got80)
	}
	// Absolute zero.
	if got0 := h.toCanonical(-218.52); !almostEqual(got0, 0, 1e-9) {
		t.Errorf("degRe.toCanonical(-218.52) = %v, want 0", got0)
	}
}

func TestReaumurFromCanonical(t *testing.T) {
	h := handlerFor(t, "[degRe]")
	if got := h.fromCanonical(218.52); !almostEqual(got, 0, 1e-9) {
		t.Errorf("degRe.fromCanonical(218.52) = %v, want 0", got)
	}
	if got := h.fromCanonical(298.52); !almostEqual(got, 80, 1e-9) {
		t.Errorf("degRe.fromCanonical(298.52) = %v, want 80", got)
	}
}

func TestReaumurRoundTrip(t *testing.T) {
	h := handlerFor(t, "[degRe]")
	for _, v := range []float64{-218.52, 0, 20, 80, 800} {
		if got := h.fromCanonical(h.toCanonical(v)); !almostEqual(got, v, 1e-6) {
			t.Errorf("degRe round-trip(%v) = %v", v, got)
		}
	}
}

// PH (logHandler, negate) tests.

func TestPHToCanonical(t *testing.T) {
	h := handlerFor(t, "[pH]")
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
		got := h.toCanonical(tt.input)
		if !almostEqual(got, tt.want, tt.want*1e-9) {
			t.Errorf("pH.toCanonical(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestPHFromCanonical(t *testing.T) {
	h := handlerFor(t, "[pH]")
	got := h.fromCanonical(1e-7)
	if !almostEqual(got, 7, floatTolerance) {
		t.Errorf("pH.fromCanonical(1e-7) = %v, want 7", got)
	}
}

func TestPHRoundTrip(t *testing.T) {
	h := handlerFor(t, "[pH]")
	values := []float64{0, 1, 3, 7, 10, 14}
	for _, v := range values {
		got := h.fromCanonical(h.toCanonical(v))
		if !almostEqual(got, v, 1e-9) {
			t.Errorf("pH round-trip(%v) = %v", v, got)
		}
	}
}

func TestPHCodeAndReference(t *testing.T) {
	h := handlerFor(t, "[pH]")
	if h.code() != "[pH]" {
		t.Errorf("pH.code() = %q, want %q", h.code(), "[pH]")
	}
	if got := referenceOf(t, "[pH]"); got != "1.mol/l" {
		t.Errorf("pH reference = %q, want %q", got, "1.mol/l")
	}
}

// TestSpecialReferencesComeFromDefinitions checks that the reference quantity of
// every special unit is read from the XML rather than restated in code. These are
// the figures that used to be hardcoded in special.go, including the two that
// were wrong there.
func TestSpecialReferencesComeFromDefinitions(t *testing.T) {
	want := map[string]string{
		"Cel":             "1.K",
		"[degF]":          "5.K/9",
		"[degRe]":         "5.K/4",
		"[pH]":            "1.mol/l",
		"B":               "1.1",
		"B[V]":            "1.V",
		"B[SPL]":          "2.10*-5.Pa",
		"B[10.nV]":        "10.nV",
		"[p'diop]":        "1.rad",
		"%[slope]":        "1.deg",
		"[m/s2/Hz^(1/2)]": "1.m2/s4/Hz",
		"[hp'_C]":         "1.1",
	}
	for code, ref := range want {
		if got := referenceOf(t, code); got != ref {
			t.Errorf("reference of %q = %q, want %q", code, got, ref)
		}
	}
}

// handlersOf returns every handler the service built from the definitions.
func handlersOf(t *testing.T) map[string]Handler {
	t.Helper()
	m, err := essence.Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	handlers, err := BuildHandlers(m)
	if err != nil {
		t.Fatal(err)
	}
	return handlers
}

// referenceOf returns the reference expression a special unit's definition gives.
func referenceOf(t *testing.T, code string) string {
	t.Helper()
	m, err := essence.Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	u := m.Lookup(code)
	if u == nil || u.Value == nil {
		t.Fatalf("no definition for %q", code)
	}
	return u.Value.Function.Reference()
}

// Bel (logHandler, power ratio) tests.

func TestBelToCanonical(t *testing.T) {
	h := handlerFor(t, "B")
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
		got := h.toCanonical(tt.input)
		if !almostEqual(got, tt.want, tt.want*1e-9+floatTolerance) {
			t.Errorf("B.toCanonical(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestBelFromCanonical(t *testing.T) {
	h := handlerFor(t, "B")
	got := h.fromCanonical(1000)
	if !almostEqual(got, 3, floatTolerance) {
		t.Errorf("B.fromCanonical(1000) = %v, want 3", got)
	}
}

func TestBelRoundTrip(t *testing.T) {
	h := handlerFor(t, "B")
	values := []float64{0, 1, 2, 3, 5, 10}
	for _, v := range values {
		got := h.fromCanonical(h.toCanonical(v))
		if !almostEqual(got, v, 1e-9) {
			t.Errorf("B round-trip(%v) = %v", v, got)
		}
	}
}

// B[SPL] (logHandler, field quantity defined with lgTimes2) tests.

func TestBelSPLToCanonical(t *testing.T) {
	h := handlerFor(t, "B[SPL]")
	// lgTimes2 means value = 2*lg(canonical), so toCanonical(v) = 10^(v/2),
	// expressed in multiples of the reference level.
	got := h.toCanonical(1)
	if !almostEqual(got, math.Sqrt(10), 1e-9) {
		t.Errorf("B[SPL].toCanonical(1) = %v, want %v", got, math.Sqrt(10))
	}
	// 2 B[SPL] = 10^1 = 10 times the reference level.
	got2 := h.toCanonical(2)
	if !almostEqual(got2, 10, 1e-9) {
		t.Errorf("B[SPL].toCanonical(2) = %v, want 10", got2)
	}
}

func TestBelSPLRoundTrip(t *testing.T) {
	h := handlerFor(t, "B[SPL]")
	values := []float64{0, 0.5, 1, 2, 3}
	for _, v := range values {
		got := h.fromCanonical(h.toCanonical(v))
		if !almostEqual(got, v, 1e-9) {
			t.Errorf("B[SPL] round-trip(%v) = %v", v, got)
		}
	}
}

// Neper (logHandler, base e) tests.

func TestNeperToCanonical(t *testing.T) {
	h := handlerFor(t, "Np")
	// 1 Np = e^1 = e
	got := h.toCanonical(1)
	if !almostEqual(got, math.E, floatTolerance) {
		t.Errorf("Np.toCanonical(1) = %v, want %v", got, math.E)
	}
	// 0 Np = e^0 = 1
	got0 := h.toCanonical(0)
	if !almostEqual(got0, 1, floatTolerance) {
		t.Errorf("Np.toCanonical(0) = %v, want 1", got0)
	}
}

func TestNeperRoundTrip(t *testing.T) {
	h := handlerFor(t, "Np")
	values := []float64{0, 1, 2, 3.5}
	for _, v := range values {
		got := h.fromCanonical(h.toCanonical(v))
		if !almostEqual(got, v, 1e-9) {
			t.Errorf("Np round-trip(%v) = %v", v, got)
		}
	}
}

// Bit_s (logHandler, base 2) tests.

func TestBitSToCanonical(t *testing.T) {
	h := handlerFor(t, "bit_s")
	// 8 bits = 2^8 = 256
	got := h.toCanonical(8)
	if !almostEqual(got, 256, floatTolerance) {
		t.Errorf("bit_s.toCanonical(8) = %v, want 256", got)
	}
	// 1 bit = 2^1 = 2
	got1 := h.toCanonical(1)
	if !almostEqual(got1, 2, floatTolerance) {
		t.Errorf("bit_s.toCanonical(1) = %v, want 2", got1)
	}
	// 0 bits = 2^0 = 1
	got0 := h.toCanonical(0)
	if !almostEqual(got0, 1, floatTolerance) {
		t.Errorf("bit_s.toCanonical(0) = %v, want 1", got0)
	}
}

func TestBitSRoundTrip(t *testing.T) {
	h := handlerFor(t, "bit_s")
	values := []float64{0, 1, 4, 8, 16}
	for _, v := range values {
		got := h.fromCanonical(h.toCanonical(v))
		if !almostEqual(got, v, 1e-9) {
			t.Errorf("bit_s round-trip(%v) = %v", v, got)
		}
	}
}

// Homeopathic potencies tests.

func TestHomeopathicToCanonical(t *testing.T) {
	h := handlerFor(t, "[hp'_X]")
	// hp'_X uses base 10, negate: 1X = 10^-1 = 0.1
	got := h.toCanonical(1)
	if !almostEqual(got, 0.1, floatTolerance) {
		t.Errorf("hp'_X.toCanonical(1) = %v, want 0.1", got)
	}

	hc := handlerFor(t, "[hp'_C]")
	// hp'_C uses base 100, negate: 1C = 100^-1 = 0.01
	gotC := hc.toCanonical(1)
	if !almostEqual(gotC, 0.01, floatTolerance) {
		t.Errorf("hp'_C.toCanonical(1) = %v, want 0.01", gotC)
	}
}

// Prism diopter (tanHandler) tests.

func TestPrismDiopterToCanonical(t *testing.T) {
	h := handlerFor(t, "[p'diop]")
	// ToCanonical(v) = atan(v/100)
	// At 0: atan(0) = 0
	got := h.toCanonical(0)
	if !almostEqual(got, 0, floatTolerance) {
		t.Errorf("p'diop.toCanonical(0) = %v, want 0", got)
	}
	// At 100: atan(1) = pi/4
	got100 := h.toCanonical(100)
	if !almostEqual(got100, math.Pi/4, floatTolerance) {
		t.Errorf("p'diop.toCanonical(100) = %v, want %v", got100, math.Pi/4)
	}
}

func TestPrismDiopterRoundTrip(t *testing.T) {
	h := handlerFor(t, "[p'diop]")
	values := []float64{0, 1, 10, 50, 100}
	for _, v := range values {
		got := h.fromCanonical(h.toCanonical(v))
		if !almostEqual(got, v, 1e-9) {
			t.Errorf("p'diop round-trip(%v) = %v", v, got)
		}
	}
}

// Percent slope (tanHandler) tests.

func TestPercentSlopeToCanonical(t *testing.T) {
	h := handlerFor(t, "%[slope]")
	// The handler yields radians, which is what atan produces; the conversion into
	// the declared deg is the canonicalizer's job. A 100% slope is a 45 degree
	// incline, so atan(1) is pi/4. TestPercentSlope covers the whole conversion.
	got := h.toCanonical(100)
	if !almostEqual(got, math.Pi/4, floatTolerance) {
		t.Errorf("%%[slope].toCanonical(100) = %v, want %v", got, math.Pi/4)
	}
	if got0 := h.toCanonical(0); !almostEqual(got0, 0, floatTolerance) {
		t.Errorf("%%[slope].toCanonical(0) = %v, want 0", got0)
	}
	if got50 := h.toCanonical(50); !almostEqual(got50, math.Atan(0.5), floatTolerance) {
		t.Errorf("%%[slope].toCanonical(50) = %v, want %v", got50, math.Atan(0.5))
	}
}

// Sqrt handler tests.

func TestSqrtHandlerToCanonical(t *testing.T) {
	h := handlerFor(t, "[m/s2/Hz^(1/2)]")
	// Squaring 3 should give 9.
	got := h.toCanonical(3)
	if !almostEqual(got, 9, floatTolerance) {
		t.Errorf("sqrt.toCanonical(3) = %v, want 9", got)
	}
	// Square root of 9 should give 3.
	got2 := h.fromCanonical(9)
	if !almostEqual(got2, 3, floatTolerance) {
		t.Errorf("sqrt.fromCanonical(9) = %v, want 3", got2)
	}
}

func TestSqrtHandlerRoundTrip(t *testing.T) {
	h := handlerFor(t, "[m/s2/Hz^(1/2)]")
	values := []float64{0, 1, 2, 5, 10, 100}
	for _, v := range values {
		got := h.fromCanonical(h.toCanonical(v))
		if !almostEqual(got, v, 1e-9) {
			t.Errorf("sqrt round-trip(%v) = %v", v, got)
		}
	}
}

// Registry completeness tests.

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
		h, ok := handlersOf(t)[code]
		if !ok {
			t.Errorf("missing handler for %q", code)
			continue
		}
		if h.code() != code {
			t.Errorf("handler %q has Code() = %q", code, h.code())
		}
	}
	if got := len(handlersOf(t)); got != len(expectedCodes) {
		t.Errorf("the service built %d handlers, want %d", got, len(expectedCodes))
	}
}

func TestLgTimes2RoundTrip(t *testing.T) {
	for _, code := range []string{"B[V]", "B[mV]", "B[uV]", "B[10.nV]", "B[SPL]", "B", "B[W]", "Np", "bit_s"} {
		h := handlerFor(t, code)
		for _, v := range []float64{-2, 0, 0.5, 1, 3} {
			if got := h.fromCanonical(h.toCanonical(v)); math.Abs(got-v) > 1e-9 {
				t.Errorf("%s round-trip(%v) = %v", code, v, got)
			}
		}
	}
}

func TestTanHandlerRoundTrip(t *testing.T) {
	for _, code := range []string{"[p'diop]", "%[slope]"} {
		h := handlerFor(t, code)
		for _, v := range []float64{-50, 0, 10, 100} {
			if got := h.fromCanonical(h.toCanonical(v)); math.Abs(got-v) > 1e-9 {
				t.Errorf("%s round-trip(%v) = %v", code, v, got)
			}
		}
	}
}
