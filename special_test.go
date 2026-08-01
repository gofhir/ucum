package ucum

import (
	"math"
	"testing"
)

const floatTolerance = 1e-9

func almostEqual(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

// handlerFor returns the conversion built for a special unit from its definition.
// The handlers are per-service now, since which function a unit performs is data
// in ucum-essence.xml rather than a fact about this package.
func handlerFor(t *testing.T, code string) specialHandler {
	t.Helper()
	svc, err := New()
	if err != nil {
		t.Fatal(err)
	}
	s, ok := svc.(*service)
	if !ok {
		t.Fatal("unexpected service type")
	}
	h, ok := s.handlers[code]
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
func handlersOf(t *testing.T) map[string]specialHandler {
	t.Helper()
	svc, err := New()
	if err != nil {
		t.Fatal(err)
	}
	s, ok := svc.(*service)
	if !ok {
		t.Fatal("unexpected service type")
	}
	return s.handlers
}

// referenceOf returns the reference expression a special unit's definition gives.
func referenceOf(t *testing.T, code string) string {
	t.Helper()
	svc, err := New()
	if err != nil {
		t.Fatal(err)
	}
	s, ok := svc.(*service)
	if !ok {
		t.Fatal("unexpected service type")
	}
	u := s.model.getUnit(code)
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

// TestBelReferenceLevels pins the reference level of every unit defined with a
// logarithmic function, which is what value=N over Unit=X in the XML sets.
// Reading the reported unit alone is not enough: B[SPL] is 2x10^-5 Pa and
// B[10.nV] is 10 nV, while the rest are one of their reported unit.
func TestBelReferenceLevels(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatal(err)
	}
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
	svc, err := New()
	if err != nil {
		t.Fatal(err)
	}

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
	svc, err := New()
	if err != nil {
		t.Fatal(err)
	}
	s, ok := svc.(*service)
	if !ok {
		t.Fatal("unexpected service type")
	}
	var special int
	for _, du := range s.model.DefinedUnits {
		if !du.IsSpecial {
			continue
		}
		special++
		if _, ok := s.handlers[du.Code]; !ok {
			t.Errorf("special unit %q has no handler", du.Code)
		}
		if _, err := svc.Canonical(1, du.Code); err != nil {
			t.Errorf("Canonical(1, %q): %v", du.Code, err)
		}
	}
	if special != len(s.handlers) {
		t.Errorf("%d special units in the definitions but %d handlers registered",
			special, len(s.handlers))
	}
	if special != 21 {
		t.Errorf("found %d special units, want 21 for this version of the definitions", special)
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
