package ucum

import (
	"fmt"
	"math/big"
	"strings"
)

type decimal struct{ val *big.Rat }

func decimalFromInt(n int64) decimal {
	return decimal{new(big.Rat).SetInt64(n)}
}

func decimalFromString(s string) (decimal, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return decimalFromInt(1), nil
	}
	if idx := strings.IndexAny(s, "eE"); idx >= 0 {
		base, exp := s[:idx], s[idx+1:]
		r := new(big.Rat)
		if _, ok := r.SetString(base); !ok {
			return decimal{}, fmt.Errorf("invalid decimal %q", s)
		}
		e := new(big.Int)
		if _, ok := e.SetString(exp, 10); !ok {
			return decimal{}, fmt.Errorf("invalid exponent in %q", s)
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
		return decimal{r}, nil
	}
	r := new(big.Rat)
	if _, ok := r.SetString(s); !ok {
		return decimal{}, fmt.Errorf("invalid decimal %q", s)
	}
	return decimal{r}, nil
}

func (d decimal) add(o decimal) decimal { return decimal{new(big.Rat).Add(d.val, o.val)} }
func (d decimal) sub(o decimal) decimal { return decimal{new(big.Rat).Sub(d.val, o.val)} }
func (d decimal) mul(o decimal) decimal { return decimal{new(big.Rat).Mul(d.val, o.val)} }
func (d decimal) div(o decimal) decimal { return decimal{new(big.Rat).Quo(d.val, o.val)} }

func (d decimal) pow(n int) decimal {
	if n == 0 {
		return decimalFromInt(1)
	}
	base := d
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	result := decimalFromInt(1)
	for i := 0; i < n; i++ {
		result = result.mul(base)
	}
	if neg {
		result = decimalFromInt(1).div(result)
	}
	return result
}

func (d decimal) float64() float64 {
	f, _ := d.val.Float64()
	return f
}

func (d decimal) equal(o decimal) bool { return d.val.Cmp(o.val) == 0 }
func (d decimal) isZero() bool         { return d.val.Sign() == 0 }

func (d decimal) String() string {
	if d.val.IsInt() {
		return d.val.Num().String()
	}
	return d.val.FloatString(10)
}
