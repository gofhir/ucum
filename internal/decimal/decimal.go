// Package decimal holds the exact rational arithmetic the definitions are read
// into and the canonicalizer runs on.
//
// UCUM conversion factors are ratios of decimal literals, so they are rational
// and can be carried exactly. Rounding happens once, when a result is handed back
// as a float64, rather than at every step of an expansion.
package decimal

import (
	"fmt"
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
