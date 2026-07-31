package ucum

import (
	"math"
	"testing"
)

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
		h := specialHandlers[code]
		for _, v := range []float64{-2, 0, 0.5, 1, 3} {
			if got := h.fromCanonical(h.toCanonical(v)); math.Abs(got-v) > 1e-9 {
				t.Errorf("%s round-trip(%v) = %v", code, v, got)
			}
		}
	}
}
