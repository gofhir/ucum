// Package decimal holds the exact rational arithmetic the definitions are read
// into and the canonicalizer runs on.
//
// UCUM conversion factors are ratios of decimal literals, so they are rational
// and can be carried exactly. Rounding happens once, when a result is handed back
// as a float64, rather than at every step of an expansion.
package decimal

import (
	"fmt"
	"math"
	"math/big"
	"strings"
)

// Decimal is an exact rational number. The zero value is not usable; build one
// with FromInt or FromString.
type Decimal struct{ val *big.Rat }

// FromInt returns n as an exact value.
func FromInt(n int64) Decimal {
	return Decimal{new(big.Rat).SetInt64(n)}
}

// FromString parses a decimal literal, with or without an exponent. An empty
// string is 1, which is what an omitted value means in the definitions.
func FromString(s string) (Decimal, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return FromInt(1), nil
	}
	if idx := strings.IndexAny(s, "eE"); idx >= 0 {
		base, exp := s[:idx], s[idx+1:]
		r := new(big.Rat)
		if _, ok := r.SetString(base); !ok {
			return Decimal{}, fmt.Errorf("invalid decimal %q", s)
		}
		e := new(big.Int)
		if _, ok := e.SetString(exp, 10); !ok {
			return Decimal{}, fmt.Errorf("invalid exponent in %q", s)
		}
		ten := big.NewInt(10)
		if e.Sign() >= 0 {
			factor := new(big.Int).Exp(ten, e, nil)
			r.Mul(r, new(big.Rat).SetInt(factor))
		} else {
			e.Neg(e)
			factor := new(big.Int).Exp(ten, e, nil)
			r.Quo(r, new(big.Rat).SetInt(factor))
		}
		return Decimal{r}, nil
	}
	r := new(big.Rat)
	if _, ok := r.SetString(s); !ok {
		return Decimal{}, fmt.Errorf("invalid decimal %q", s)
	}
	return Decimal{r}, nil
}

// Add returns d + o.
func (d Decimal) Add(o Decimal) Decimal { return Decimal{new(big.Rat).Add(d.val, o.val)} }

// Sub returns d - o.
func (d Decimal) Sub(o Decimal) Decimal { return Decimal{new(big.Rat).Sub(d.val, o.val)} }

// Mul returns d * o.
func (d Decimal) Mul(o Decimal) Decimal { return Decimal{new(big.Rat).Mul(d.val, o.val)} }

// Div returns d / o. The caller must have checked that o is not zero.
func (d Decimal) Div(o Decimal) Decimal { return Decimal{new(big.Rat).Quo(d.val, o.val)} }

// Pow raises d to an integer power by square-and-multiply, in about log2(n)
// multiplications rather than n of them.
//
// The difference is not academic here: prefixed units raise powers of ten, so
// the iterative version made canonicalizing "m1000000" take 312ms and scale
// linearly from there. The exponent is bounded at parse time as well — see
// MaxExponent — because the *result* still grows with n however few
// multiplications produce it.
func (d Decimal) Pow(n int) Decimal {
	if n == 0 {
		return FromInt(1)
	}
	neg := n < 0
	if neg {
		n = -n
	}

	result := FromInt(1)
	base := d
	for n > 0 {
		if n&1 == 1 {
			result = result.Mul(base)
		}
		n >>= 1
		if n > 0 {
			base = base.Mul(base)
		}
	}

	if neg {
		result = FromInt(1).Div(result)
	}
	return result
}

// Float64 returns the nearest float64, rounding once.
func (d Decimal) Float64() float64 {
	f, _ := d.val.Float64()
	return f
}

// Rat returns a copy of the exact rational value. Callers get a copy so they
// cannot mutate the definition it came from.
func (d Decimal) Rat() *big.Rat {
	return new(big.Rat).Set(d.val)
}

// RatFromFloat converts v to an exact rational. It returns nil for NaN and the
// infinities, which have no rational representation.
func RatFromFloat(v float64) *big.Rat {
	return new(big.Rat).SetFloat64(v)
}

// MulExact multiplies value by an exact rational factor and rounds once, so a
// representable result comes out exact. Non-finite values keep the float64
// path and propagate as before.
func MulExact(value float64, factor *big.Rat) float64 {
	rv := RatFromFloat(value)
	if rv == nil {
		f, _ := factor.Float64()
		return value * f
	}
	out, _ := new(big.Rat).Mul(rv, factor).Float64()
	return out
}

// Log10Rat returns log10 |r| without rounding r to float64, so the factor of
// Ym13 (10^312) has a log10 although no float64 holds it: it is log10 of the
// binary mantissa plus the exponent times log10(2). It returns -Inf for 0.
//
// The powers of ten in the numerator and the denominator are counted apart, so
// a power of ten gives its exponent exactly on every platform: math.Log is not
// correctly rounded, and on amd64 log10 of the mantissa of 10^-920 plus its
// exponent comes out as -920.0000000000001.
func Log10Rat(r *big.Rat) float64 {
	if r.Sign() == 0 {
		return math.Inf(-1)
	}
	num, nk := stripTens(new(big.Int).Abs(r.Num()))
	den, dk := stripTens(new(big.Int).Set(r.Denom()))
	tens := float64(nk - dk)
	if num.Cmp(den) == 0 {
		return tens
	}
	f := new(big.Float).SetPrec(64).SetRat(new(big.Rat).SetFrac(num, den))
	mant := new(big.Float)
	exp := f.MantExp(mant)
	m, _ := mant.Float64()
	return tens + (math.Log10(m) + float64(exp)*math.Log10(2))
}

// stripTens divide n por 10 mientras se pueda y dice cuántas veces lo hizo.
func stripTens(n *big.Int) (rest *big.Int, tens int) {
	ten := big.NewInt(10)
	q, m := new(big.Int), new(big.Int)
	for n.Sign() != 0 {
		q.QuoRem(n, ten, m)
		if m.Sign() != 0 {
			break
		}
		n, q = q, n
		tens++
	}
	return n, tens
}

// Log10Abs returns log10 |v|, also for a subnormal v.
//
// On amd64, math.Log reads the exponent of a subnormal as if it were normal
// (golang/go#56600): math.Log10(5e-324) is -307.95 there, not -323.31. A
// subnormal is therefore scaled by 2^54, which is exact and makes it normal,
// before taking the logarithm.
func Log10Abs(v float64) float64 {
	a := math.Abs(v)
	if a != 0 && a < smallestNormal {
		return math.Log10(a*0x1p54) - 54*math.Log10(2)
	}
	return math.Log10(a)
}

// smallestNormal es 2^-1022, el menor float64 con todas sus cifras.
const smallestNormal = 0x1p-1022

// Equal reports whether two values are exactly equal.
func (d Decimal) Equal(o Decimal) bool { return d.val.Cmp(o.val) == 0 }

// IsZero reports whether the value is zero.
func (d Decimal) IsZero() bool { return d.val.Sign() == 0 }

func (d Decimal) String() string {
	if d.val.IsInt() {
		return d.val.Num().String()
	}
	return d.val.FloatString(10)
}
