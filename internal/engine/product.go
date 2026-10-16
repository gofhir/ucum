package engine

import (
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/gofhir/ucum/v4/internal/expr"
	"github.com/gofhir/ucum/v4/internal/model"
)

// The result unit of Multiply and Divide.
//
// The product of two quantities is written in the operands' own units rather
// than in base units: 12 cm times 3 cm is 36 cm2, not 0.0036 m2. FHIRPath gives
// it that way, and a caller who multiplied centimeters expects centimeters back.
//
// Each operand is flattened into its atoms, with their prefixes and exponents,
// and the atoms are merged: the same atom adds its exponents, and where it
// appears with two prefixes both are read in the smaller one, so cm times m is
// cm2. Different atoms stay apart even when they share a dimension, because
// nothing in UCUM says which of L and dm3 a caller would rather see.

// productFactor is one atom of a product unit, with the prefix it is written in
// and its net exponent.
type productFactor struct {
	unit     *model.Unit
	prefix   *model.Prefix
	exponent int
}

// productUnit accumulates the factors of a product unit: its atoms in the order
// they first appear, and the integer factors folded into one rational.
type productUnit struct {
	factors []productFactor
	numeric *big.Rat
}

// productCode composes the result unit of t1 op t2 from the operands' own
// atoms. It reports false for an operand it cannot write that way — a special
// unit, whose meaning depends on whether it stands alone, or an arbitrary unit —
// and the caller then keeps the canonical unit.
func productCode(t1, t2 *expr.Term, div bool) (string, bool) {
	p := &productUnit{numeric: big.NewRat(1, 1)}
	sign := 1
	if div {
		sign = -1
	}
	if !p.addTerm(t1, 1) || !p.addTerm(t2, sign) {
		return "", false
	}
	return p.compose(), true
}

// addTerm flattens a term into the product, with its exponents multiplied by
// sign. A term means Comp op Term, so the right side of a division takes the
// opposite sign whatever it holds: "a/(b.c)" divides by both b and c.
func (p *productUnit) addTerm(t *expr.Term, sign int) bool {
	if t == nil {
		return true
	}
	if !p.addComponent(t.Comp, sign) {
		return false
	}
	if t.Term == nil {
		return true
	}
	if t.Op == expr.OpDivision {
		sign = -sign
	}
	return p.addTerm(t.Term, sign)
}

func (p *productUnit) addComponent(c expr.Component, sign int) bool {
	switch v := c.(type) {
	case *expr.Factor:
		// An annotation parses as the factor 1, so it is dropped here along with
		// the unity: it carries no meaning to combine. A zero factor would leave
		// the composed unit with no canonical factor to divide by.
		if v.Value == 0 {
			return false
		}
		f := new(big.Rat).SetInt64(int64(v.Value))
		if sign < 0 {
			p.numeric.Quo(p.numeric, f)
		} else {
			p.numeric.Mul(p.numeric, f)
		}
		return true
	case *expr.Symbol:
		if v.Unit.IsSpecial || v.Unit.IsArbitrary {
			return false
		}
		p.addSymbol(v, v.Exponent*sign)
		return true
	case *expr.Term:
		return p.addTerm(v, sign)
	default:
		return false
	}
}

// addSymbol merges an atom into the product. An atom already present adds its
// exponent and keeps whichever prefix is the smaller, the more granular unit.
func (p *productUnit) addSymbol(sym *expr.Symbol, exponent int) {
	for i := range p.factors {
		f := &p.factors[i]
		if f.unit.Code != sym.Unit.Code {
			continue
		}
		f.exponent += exponent
		if prefixRat(sym.Prefix).Cmp(prefixRat(f.prefix)) < 0 {
			f.prefix = sym.Prefix
		}
		return
	}
	p.factors = append(p.factors, productFactor{unit: sym.Unit, prefix: sym.Prefix, exponent: exponent})
}

// prefixRat returns the value of a prefix, or 1 for none.
func prefixRat(pfx *model.Prefix) *big.Rat {
	if pfx == nil {
		return big.NewRat(1, 1)
	}
	return pfx.Value.Rat()
}

// compose writes the product as a UCUM code: the integer factor first, the atoms
// joined with "." and negative exponents written as exponents, and a fractional
// factor's denominator as a trailing division, since a number takes no exponent.
// An atom whose exponent canceled is left out, and an empty product is "1".
func (p *productUnit) compose() string {
	var parts []string
	if num := p.numeric.Num(); !num.IsInt64() || num.Int64() != 1 {
		parts = append(parts, num.String())
	}
	for _, f := range p.factors {
		if f.exponent == 0 {
			continue
		}
		var sb strings.Builder
		if f.prefix != nil {
			sb.WriteString(f.prefix.Code)
		}
		sb.WriteString(f.unit.Code)
		if f.exponent != 1 {
			sb.WriteString(strconv.Itoa(f.exponent))
		}
		parts = append(parts, sb.String())
	}
	code := "1"
	if len(parts) > 0 {
		code = strings.Join(parts, ".")
	}
	if !p.numeric.IsInt() {
		code += "/" + p.numeric.Denom().String()
	}
	return code
}

// smallestNormal is the smallest positive normal float64, 2^-1022. Below it a
// float64 carries fewer than 53 significant bits.
const smallestNormal = 0x1p-1022

// productResult rewrites the canonical result of a product in the operands' own
// units. The exact result, before rounding, is in the canonical unit want.
//
// The composed code is parsed back and canonicalized, and used only if it has
// the canonical unit want: that is what makes it the same quantity, and it also
// catches a code the parser refuses, such as an exponent past MaxExponent. The
// value is then exact divided by the code's canonical factor, which is v1 op v2
// times the ratio of the prefixes that merged.
//
// The code is written in case-sensitive atoms, as canonical forms are, and so
// it is parsed back in that vocabulary whatever this service resolves.
func (s *Service) productResult(t1, t2 *expr.Term, div bool, exact *big.Rat, want string) (Pair, bool) {
	code, ok := productCode(t1, t2, div)
	if !ok {
		return Pair{}, false
	}
	t, err := s.defParser.Parse(code)
	if err != nil {
		return Pair{}, false
	}
	can, err := s.canonicalizeTerm(t, s.specialContextOf(t))
	if err != nil || can.Value.IsZero() || expr.ComposeCanonicalUnits(can) != want {
		return Pair{}, false
	}
	// Moving to a smaller prefix can take a value past what float64 holds — an
	// atto-liter to the hundredth power — where the canonical one may still fit.
	// A product that overflowed, or fell below the normal range where float64
	// loses precision, is left to the canonical path.
	out, _ := new(big.Rat).Quo(exact, can.Value.Rat()).Float64()
	if math.IsInf(out, 0) || (exact.Sign() != 0 && math.Abs(out) < smallestNormal) {
		return Pair{}, false
	}
	return Pair{Value: out, Code: code}, true
}
