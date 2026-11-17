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

// maxLiteralExponent acota la magnitud del exponente decimal de un literal. No
// es el tope de valores del motor: solo impide que un literal de pocos bytes
// (1e1000000000) arme un número arbitrario al cargar un XML propio.
const maxLiteralExponent = 9999

// FromString parses a decimal literal: an optional minus sign, digits with an
// optional fraction (or a fraction alone), and an optional exponent, as in
// "-5", ".5" or "9.9e-9". Nothing else is accepted, not even surrounding
// spaces, and an exponent beyond ±9999 is refused. An empty string is 1, which
// is what an omitted value means in the definitions.
func FromString(s string) (Decimal, error) {
	if s == "" {
		return FromInt(1), nil
	}
	mantissa, exp, ok := splitLiteral(s)
	if !ok {
		return Decimal{}, fmt.Errorf("invalid decimal %q", s)
	}
	if len(exp) > 4 { // sin ceros a la izquierda, más de cuatro cifras pasa de 9999.
		return Decimal{}, fmt.Errorf("exponent of %q is beyond ±%d", s, maxLiteralExponent)
	}
	r, ok := new(big.Rat).SetString(mantissa)
	if !ok {
		return Decimal{}, fmt.Errorf("invalid decimal %q", s)
	}
	n := 0
	for i := 0; i < len(exp); i++ {
		n = n*10 + int(exp[i]-'0')
	}
	if n > 0 {
		factor := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil))
		if negativeExponent(s) {
			r.Quo(r, factor)
		} else {
			r.Mul(r, factor)
		}
	}
	return Decimal{r}, nil
}

// negativeExponent dice si el literal ya validado lleva un exponente negativo.
func negativeExponent(s string) bool {
	i := strings.IndexAny(s, "eE")
	return i >= 0 && s[i+1] == '-'
}

// Con splitLiteral se valida s contra la gramática del plan 023: signo opcional,
// dígitos ASCII con fracción opcional (o fracción sola) y exponente opcional.
// Devuelve la mantisa y los dígitos del exponente sin ceros a la izquierda ("" si
// vale 0 o falta).
func splitLiteral(s string) (mantissa, exp string, ok bool) {
	i := 0
	if i < len(s) && s[i] == '-' {
		i++
	}
	intDigits := digitRun(s[i:])
	i += intDigits
	if i < len(s) && s[i] == '.' {
		i++
		frac := digitRun(s[i:])
		if frac == 0 {
			return "", "", false
		}
		i += frac
	} else if intDigits == 0 {
		return "", "", false
	}
	mantissa = s[:i]
	if i == len(s) {
		return mantissa, "", true
	}
	if s[i] != 'e' && s[i] != 'E' {
		return "", "", false
	}
	i++
	if i < len(s) && s[i] == '-' {
		i++
	}
	n := digitRun(s[i:])
	if n == 0 || i+n != len(s) {
		return "", "", false
	}
	return mantissa, strings.TrimLeft(s[i:], "0"), true
}

// digitRun cuenta los dígitos ASCII con que empieza s.
func digitRun(s string) int {
	n := 0
	for n < len(s) && s[n] >= '0' && s[n] <= '9' {
		n++
	}
	return n
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
//
// A zero base with a negative exponent is not allowed: it divides by zero and
// panics in big.Rat, so callers must check it first.
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
	// Un entero de hasta 2^53 es un float64 exacto, y big.Rat.Float64 asigna
	// memoria aun así: el factor de la mayoría de las escalas especiales es 1,
	// y se lee en cada conversión.
	if d.val.IsInt() && d.val.Num().IsInt64() {
		if n := d.val.Num().Int64(); n >= -1<<53 && n <= 1<<53 {
			return float64(n)
		}
	}
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
//
// Its cost is subquadratic in the number of digits of r: a few big.Int
// divisions, with no greatest common divisor.
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
	mant := new(big.Float)
	exp := quotient64(num, den).MantExp(mant)
	m, _ := mant.Float64()
	return tens + (math.Log10(m) + float64(exp)*math.Log10(2))
}

// quotient64 es num/den redondeado una vez a 64 bits, al más cercano y en un
// empate al par: el mismo cociente que hace Float.SetRat. Como num y den ya son
// coprimos, pasar por SetFrac(num, den) repetiría el máximo común divisor.
func quotient64(num, den *big.Int) *big.Float {
	f := new(big.Float).SetPrec(64).Quo(new(big.Float).SetInt(num), new(big.Float).SetInt(den))
	return f
}

// stripTensDivision es un gancho de test: stripTens lo llama una vez por cada
// división que hace.
var stripTensDivision func()

// stripTens quita de n los ceros finales y dice cuántos eran. Los cuenta con
// las potencias 10^(2^j): sube dividiendo por 10, 10^2, 10^4 y siguientes
// mientras la división sea exacta y después baja de la mayor a la menor
// dividiendo por cada una que entra. Si quedan v diez tras fallar en 10^(2^j), v < 2^j y la
// bajada los quita en binario: son O(log t) cocientes, no uno por cada diez.
// Un n impar o cero sale sin dividir. Modifica n.
func stripTens(n *big.Int) (rest *big.Int, tens int) {
	if n.Sign() == 0 || n.Bit(0) == 1 {
		return n, 0
	}
	// divide hace n/p; si es exacta, deja el cociente en n y devuelve true.
	q, m := new(big.Int), new(big.Int)
	divide := func(p *big.Int) bool {
		q.QuoRem(n, p, m)
		if stripTensDivision != nil {
			stripTensDivision()
		}
		if m.Sign() != 0 {
			return false
		}
		n, q = q, n
		return true
	}
	pows := []*big.Int{big.NewInt(10)}
	for {
		p := pows[len(pows)-1]
		if !divide(p) {
			break
		}
		tens += 1 << (len(pows) - 1)
		// Una potencia más larga que n no lo divide: no se eleva al cuadrado.
		if p.BitLen() > n.BitLen() {
			break
		}
		pows = append(pows, new(big.Int).Mul(p, p))
	}
	// Quedan menos de 2^(len-1) diez: la bajada los quita en binario.
	for i := len(pows) - 2; i >= 0; i-- {
		if divide(pows[i]) {
			tens += 1 << i
		}
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

// Sign returns -1, 0 or +1 as d is negative, zero or positive.
func (d Decimal) Sign() int { return d.val.Sign() }

func (d Decimal) String() string {
	if d.val.IsInt() {
		return d.val.Num().String()
	}
	return d.val.FloatString(10)
}
