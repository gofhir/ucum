package special

import (
	"math"
	"math/big"
	"testing"

	"github.com/gofhir/ucum/v4/internal/decimal"
)

// Plan 009, test 2: Use.ToCanonical con prefijos que la API no admite en
// estas unidades (k[pH] no parsea), para cubrir el signo de pH y hpC con
// α ≠ 1. El esperado es el float64 más cercano a base^n (UCUM §22: pH es
// 10^-x, hpC 100^-x), calculado con big.Rat.
func TestUseToCanonicalIntegerExponentWithPrefix(t *testing.T) {
	nearest := func(base int64, n int) float64 {
		m := n
		if m < 0 {
			m = -m
		}
		p := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(base), big.NewInt(int64(m)), nil))
		if n < 0 {
			p.Inv(p)
		}
		f, _ := p.Float64()
		return f
	}
	for _, tc := range []struct {
		code  string
		base  int64
		alpha string
	}{
		{"[pH]", 10, "0.1"},
		{"[pH]", 10, "1000"},
		{"[hp'_C]", 100, "0.01"},
		{"B[V]", 10, "0.1"}, // lgTimes2: 10^(αv/2)
	} {
		alpha, err := decimal.FromString(tc.alpha)
		if err != nil {
			t.Fatal(err)
		}
		u := Use{Handler: handlerFor(t, tc.code), Alpha: alpha}
		lh := u.Handler.(logHandler)
		off := 0
		for n := -300; n <= 300; n++ {
			want := nearest(tc.base, n)
			if math.Abs(want) < 0x1p-1022 || math.IsInf(want, 0) {
				continue
			}
			// e = ±α·v/expDivisor = n  ⇒  v = ±n·expDivisor/α.
			e := new(big.Rat).SetFrac64(int64(n)*int64(lh.expDivisor), 1)
			e.Quo(e, alpha.Rat())
			if lh.negate {
				e.Neg(e)
			}
			v, exact := e.Float64()
			if !exact {
				continue
			}
			if got := u.ToCanonical(v); got != want {
				off++
				if off <= 3 {
					t.Errorf("%s with α=%s: ToCanonical(%v) = %v, want %v, the nearest float64 of %d^%d", tc.code, tc.alpha, v, got, want, tc.base, n)
				}
			}
		}
		if off > 3 {
			t.Errorf("%s with α=%s: %d exponents off", tc.code, tc.alpha, off)
		}
	}
}
