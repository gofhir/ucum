package engine

import (
	"errors"
	"math"
	"math/big"
	"testing"

	"github.com/gofhir/ucum/v4/internal/decimal"
	"github.com/gofhir/ucum/v4/internal/expr"
	"github.com/gofhir/ucum/v4/internal/model"
	"github.com/gofhir/ucum/v4/internal/ucumerr"
)

// TestProductUnits pins the result unit of Multiply and Divide: the operands'
// own atoms, merged, rather than the canonical unit.
func TestProductUnits(t *testing.T) {
	svc := newTestService(t)
	tests := []struct {
		name     string
		div      bool
		v1, v2   Pair
		wantVal  float64
		wantCode string
	}{
		// The three examples of FHIRPath 2.0.0.
		{"cm times cm", false, Pair{12, "cm"}, Pair{3, "cm"}, 36, "cm2"},
		{"cm times cm2", false, Pair{3, "cm"}, Pair{12, "cm2"}, 36, "cm3"},
		{"cm2 over cm", true, Pair{12, "cm2"}, Pair{3, "cm"}, 4, "cm"},

		// The same atom in two prefixes is read in the smaller, in either order.
		{"cm times m", false, Pair{2, "cm"}, Pair{2, "m"}, 400, "cm2"},
		{"m times cm", false, Pair{2, "m"}, Pair{2, "cm"}, 400, "cm2"},
		{"km over m", true, Pair{1, "km"}, Pair{1, "m"}, 1000, "1"},
		{"mg/dL times L", false, Pair{2.5, "mg/dL"}, Pair{4, "L"}, 100, "mg"},
		{"g over mg", true, Pair{1, "g"}, Pair{1, "mg"}, 1000, "1"},

		// The unity contributes nothing.
		{"unity times cm", false, Pair{2, "1"}, Pair{2, "cm"}, 4, "cm"},
		{"unity over s", true, Pair{60, "1"}, Pair{1, "s"}, 60, "s-1"},
		{"unity times unity", false, Pair{2, "1"}, Pair{3, "1"}, 6, "1"},

		// Negative exponents are written as exponents, in order of appearance.
		{"g over m", true, Pair{1.5, "g"}, Pair{2, "m"}, 0.75, "g.m-1"},
		{"m over g", true, Pair{2, "m"}, Pair{1.5, "g"}, 2 / 1.5, "m.g-1"},
		{"kg times m/s2", false, Pair{2, "kg"}, Pair{3, "m/s2"}, 6, "kg.m.s-2"},
		{"m/s times s", false, Pair{3, "m/s"}, Pair{2, "s"}, 6, "m"},
		{"m over (s.kg)", true, Pair{1, "m"}, Pair{1, "s.kg"}, 1, "m.s-1.kg-1"},
		{"divisor in parentheses", true, Pair{1, "N"}, Pair{1, "m/(s.kg)"}, 1, "N.m-1.s.kg"},

		// Different atoms stay apart even when they share a dimension.
		{"L times dm3", false, Pair{1, "L"}, Pair{1, "dm3"}, 1, "L.dm3"},
		{"km over h", true, Pair{10, "km"}, Pair{2, "h"}, 5, "km.h-1"},

		// Annotations carry no meaning and are dropped.
		{"annotation", false, Pair{2, "mg{total}"}, Pair{3, "1"}, 6, "mg"},
		{"annotation alone", true, Pair{10, "{rbc}"}, Pair{2, "uL"}, 5, "uL-1"},

		// Numeric factors and the 10* atom.
		{"power of ten", false, Pair{2, "10*3/uL"}, Pair{3, "uL"}, 6, "10*3"},
		{"integer factor", false, Pair{2, "100.mL"}, Pair{3, "1"}, 6, "100.mL"},
		{"integer divisor", true, Pair{2, "m"}, Pair{1, "10.s"}, 2, "m.s-1/10"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := svc.Multiply
			if tt.div {
				op = svc.Divide
			}
			got, err := op(tt.v1, tt.v2)
			if err != nil {
				t.Fatalf("%v op %v: %v", tt.v1, tt.v2, err)
			}
			if got.Code != tt.wantCode {
				t.Errorf("%v op %v: code = %q, want %q", tt.v1, tt.v2, got.Code, tt.wantCode)
			}
			if math.Abs(got.Value-tt.wantVal) > math.Abs(tt.wantVal)*1e-15 {
				t.Errorf("%v op %v: value = %.20g, want %.20g", tt.v1, tt.v2, got.Value, tt.wantVal)
			}
		})
	}
}

// TestProductUnitsFallBack covers the operands the result unit cannot be built
// from. A special unit means a point on its scale when it stands alone and a
// difference inside an expression, so moving it between the two would change
// the quantity; an arbitrary unit is kept apart from everything; and a value
// with no rational form takes the float64 path. All three keep the canonical
// unit.
func TestProductUnitsFallBack(t *testing.T) {
	svc := newTestService(t)
	tests := []struct {
		name     string
		div      bool
		v1, v2   Pair
		wantVal  float64
		wantCode string
	}{
		{"Cel standalone", false, Pair{1, "Cel"}, Pair{1, "1"}, 274.15, "K"},
		{"Cel as a difference", false, Pair{1, "Cel/min"}, Pair{1, "min"}, 1, "K"},
		{"non-rational scale", true, Pair{1, "[pH]"}, Pair{1, "mol/L"}, 0.1, "1"},
		{"arbitrary unit", false, Pair{10, "[IU]/L"}, Pair{2, "mL"}, 0.02, "[IU]"},
		{"non-finite value", false, Pair{math.Inf(1), "cm"}, Pair{1, "cm"}, math.Inf(1), "m2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := svc.Multiply
			if tt.div {
				op = svc.Divide
			}
			got, err := op(tt.v1, tt.v2)
			if err != nil {
				t.Fatalf("%v op %v: %v", tt.v1, tt.v2, err)
			}
			if got.Code != tt.wantCode {
				t.Errorf("%v op %v: code = %q, want %q", tt.v1, tt.v2, got.Code, tt.wantCode)
			}
			if got.Value != tt.wantVal && math.Abs(got.Value-tt.wantVal) > math.Abs(tt.wantVal)*1e-12 {
				t.Errorf("%v op %v: value = %.20g, want %.20g", tt.v1, tt.v2, got.Value, tt.wantVal)
			}
		})
	}
}

// TestProductUnitsCaseInsensitive checks that a case-insensitive service writes
// the result in case-sensitive codes, as it writes canonical forms.
func TestProductUnitsCaseInsensitive(t *testing.T) {
	svc, err := New(nil, true)
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.Multiply(Pair{2, "CM"}, Pair{2, "M"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Code != "cm2" || got.Value != 400 {
		t.Errorf("Multiply(2 CM, 2 M) = %v %q, want 400 \"cm2\"", got.Value, got.Code)
	}
}

// productSeeds are the codes the product property starts from: prefixed and
// unprefixed forms of the same atom, compound codes, numeric factors,
// annotations, and the units that fall back.
var productSeeds = []string{
	"1", "m", "cm", "km", "m2", "cm2", "g", "mg", "kg", "s", "min", "h",
	"L", "mL", "dL", "dm3", "m/s", "km/h", "m/s2", "kg.m/s2", "N", "Pa",
	"mg/dL", "mmol/L", "g/L", "10*3/uL", "10*9/L", "100.mL", "m/(s.kg)",
	"mg{total}", "{rbc}", "/min", "[in_i]", "[lb_av]", "mm[Hg]", "%",
	"Cel", "[degF]", "Cel/min", "[pH]", "[IU]", "[IU]/L", "m-3", "s-1",
}

// FuzzProductUnits states the property the result unit must keep: whatever unit
// Multiply and Divide write the result in, it is the same quantity as the
// canonical result — commensurable with it, and equal in value once converted.
func FuzzProductUnits(f *testing.F) {
	for _, a := range productSeeds {
		for _, b := range productSeeds {
			f.Add(a, b, 3.5, 0.25)
		}
	}
	svc, err := New(nil, false)
	if err != nil {
		f.Fatal(err)
	}

	f.Fuzz(func(t *testing.T, code1, code2 string, v1, v2 float64) {
		r1, r2 := decimal.RatFromFloat(v1), decimal.RatFromFloat(v2)
		if r1 == nil || r2 == nil {
			return
		}
		c1, err := svc.CanonicalRat(r1, code1)
		if err != nil {
			return
		}
		c2, err := svc.CanonicalRat(r2, code2)
		if err != nil {
			return
		}
		for _, div := range []bool{false, true} {
			if div && c2.Value.Sign() == 0 {
				continue
			}
			want := new(big.Rat).Mul(c1.Value, c2.Value)
			op := svc.Multiply
			if div {
				want.Quo(c1.Value, c2.Value)
				op = svc.Divide
			}
			got, err := op(Pair{v1, code1}, Pair{v2, code2})
			if err != nil {
				if errors.Is(err, ErrNotRational) {
					continue
				}
				// Fuera de float64 solo si el resultado exacto en la unidad
				// canónica no cabe (#59).
				if f, _ := want.Float64(); errors.Is(err, ucumerr.ErrOutOfRange) &&
					(math.IsInf(f, 0) || f == 0 && want.Sign() != 0) {
					continue
				}
				t.Fatalf("%v %q op %v %q (div=%v): %v", v1, code1, v2, code2, div, err)
			}

			// The canonical unit of the old result, built as combine builds it.
			can1, err1 := svc.getCanonical(code1)
			can2, err2 := svc.getCanonical(code2)
			if err1 != nil || err2 != nil {
				t.Fatalf("getCanonical(%q, %q): %v, %v", code1, code2, err1, err2)
			}
			sign := 1
			if div {
				sign = -1
			}
			oldCode := expr.ComposeCanonicalUnits(&model.Canonical{Units: mergeUnitLists(can1.Units, can2.Units, sign)})

			// A canonical form can be past what the parser accepts — barn to the
			// 700th is m1400, beyond MaxExponent — and then there is no code to
			// convert to. The result is compared in canonical units directly,
			// which is what IsComparable and ConvertRat do underneath.
			if got.Code != oldCode && svc.Validate(oldCode) != nil {
				canGot, err := svc.getCanonical(got.Code)
				if err != nil || expr.ComposeCanonicalUnits(canGot) != oldCode {
					t.Fatalf("%v %q op %v %q (div=%v) = %v %q, not commensurable with %q (%v)",
						v1, code1, v2, code2, div, got.Value, got.Code, oldCode, err)
				}
				continue
			}
			ok, err := got.Code == oldCode, error(nil)
			if !ok {
				ok, err = svc.IsComparable(got.Code, oldCode)
			}
			if err != nil || !ok {
				t.Fatalf("%v %q op %v %q (div=%v) = %v %q, not comparable with %q (%v)",
					v1, code1, v2, code2, div, got.Value, got.Code, oldCode, err)
			}

			// A result past the normal float64 range has no full-precision value
			// to compare, in either unit.
			if w, _ := want.Float64(); want.Sign() != 0 && (math.IsInf(w, 0) || math.Abs(w) < smallestNormal) {
				continue
			}
			gotRat := decimal.RatFromFloat(got.Value)
			if gotRat == nil {
				t.Fatalf("%v %q op %v %q (div=%v) = %v %q, want a finite value",
					v1, code1, v2, code2, div, got.Value, got.Code)
			}
			back := gotRat
			if got.Code != oldCode {
				if back, err = svc.ConvertRat(gotRat, got.Code, oldCode); err != nil {
					t.Fatalf("ConvertRat(%v, %q, %q): %v", got.Value, got.Code, oldCode, err)
				}
			}
			if !closeRat(back, want) {
				b, _ := back.Float64()
				w, _ := want.Float64()
				t.Fatalf("%v %q op %v %q (div=%v) = %v %q, which is %.17g %s; want %.17g",
					v1, code1, v2, code2, div, got.Value, got.Code, b, oldCode, w)
			}
		}
	})
}

// closeRat reports whether got is within a float64 rounding of want: the result
// is rounded once to float64, so the two agree to about one part in 2^53.
func closeRat(got, want *big.Rat) bool {
	if want.Sign() == 0 {
		return got.Sign() == 0
	}
	diff := new(big.Rat).Sub(got, want)
	diff.Quo(diff, want)
	d, _ := diff.Float64()
	return math.Abs(d) <= 1e-15
}
