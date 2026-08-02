package engine

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/gofhir/ucum/v4/internal/decimal"
	"github.com/gofhir/ucum/v4/internal/expr"
	"github.com/gofhir/ucum/v4/internal/model"
	"github.com/gofhir/ucum/v4/internal/ucumerr"
)

// The conversion API: Canonical, Convert, IsComparable, Multiply and Divide.
//
// Each of them maps values onto a canonical scale and combines them there. The
// exact rational path is preferred and rounds once at the end; the float64
// handlers are the fallback for the scales that have no rational form.

// Canonical returns the canonical (base-unit) form of a value+code pair.
func (s *Service) Canonical(value float64, code string) (Pair, error) {
	v, can, err := s.canonicalScalar(value, code)
	if err != nil {
		return Pair{}, ucumerr.Validation(code, err)
	}
	return Pair{Value: v, Code: expr.ComposeCanonicalUnits(can)}, nil
}

// canonicalScalar maps value onto the canonical scale of code and scales it by
// the canonical factor, rounding once.
func (s *Service) canonicalScalar(value float64, code string) (float64, *model.Canonical, error) {
	t, can, err := s.canonicalParts(code)
	if err != nil {
		return 0, nil, err
	}

	// Preferred path: map and scale entirely in exact arithmetic.
	if rv := decimal.RatFromFloat(value); rv != nil {
		mapped, err := s.toCanonicalRat(t, code, rv)
		switch {
		case err == nil:
			out, _ := mapped.Mul(mapped, can.Value.Rat()).Float64()
			return out, can, nil
		case !errors.Is(err, ErrNotRational):
			return 0, nil, err
		}
	}

	// Non-rational scale or non-finite value: use the float64 handler.
	return decimal.MulExact(s.mapFloat(t, value), can.Value.Rat()), can, nil
}

// mapFloat maps value onto the canonical scale of an already-resolved expr.Term,
// using the float64 handler. It serves the scales that have no exact rational
// form; everything else goes through toCanonicalRat.
//
// Special units sit on non-ratio scales, so the multiplicative factor alone
// does not describe them: the handler has to map the value first (Cel adds an
// offset, [pH] exponentiates, B[V] takes a power). Skipping it would silently
// return the raw input value, making canonical forms non-comparable.
func (s *Service) mapFloat(t *expr.Term, value float64) float64 {
	if use := s.specialUseForTerm(t); use != nil {
		return use.ToCanonical(value)
	}
	return value
}

// specialContextOf reports how special units in this expr.Term are to be read. A expr.Term
// that is a lone special symbol is standalone; anything else puts its units into
// an algebraic relationship.
func (s *Service) specialContextOf(t *expr.Term) specialContext {
	if s.specialUseForTerm(t) != nil {
		return specialStandalone
	}
	return specialAsDelta
}

// canonicalParts parses a code and canonicalizes it, returning both the AST
// (needed to detect special units) and the canonical form.
func (s *Service) canonicalParts(code string) (*expr.Term, *model.Canonical, error) {
	t, err := s.parseCached(code)
	if err != nil {
		return nil, nil, err
	}
	can, err := s.canonicalizeTerm(t, s.specialContextOf(t))
	if err != nil {
		return nil, nil, err
	}
	return t, can, nil
}

// Convert converts a value from one unit to another.
func (s *Service) Convert(value float64, from, to string) (float64, error) {
	srcTerm, err := s.parseCached(from)
	if err != nil {
		return 0, ucumerr.Conversion(from, to, err)
	}
	dstTerm, err := s.parseCached(to)
	if err != nil {
		return 0, ucumerr.Conversion(from, to, err)
	}

	srcCan, err := s.getCanonical(from)
	if err != nil {
		return 0, ucumerr.Conversion(from, to, err)
	}
	dstCan, err := s.getCanonical(to)
	if err != nil {
		return 0, ucumerr.Conversion(from, to, err)
	}

	// Check comparability: canonical unit strings must match.
	if err := requireComparable(from, to, srcCan, dstCan); err != nil {
		return 0, err
	}

	// A zero destination factor ("0", "m/0" cancels to it) would divide by zero.
	if dstCan.Value.IsZero() {
		return 0, ucumerr.Conversion(from, to, ucumerr.ErrDivisionByZero)
	}

	parts := convertParts{
		srcTerm: srcTerm, dstTerm: dstTerm,
		srcCan: srcCan, dstCan: dstCan,
		from: from, to: to,
	}

	// Preferred path: run the whole conversion in exact rational arithmetic and
	// round once at the end. Combining canonical factors that were each already
	// rounded to float64 loses exactness even when the true result is
	// representable — L -> mL came out as 1000.0000000000001 because 1/1000 and
	// 1/1000000 are not binary fractions while their exact quotient is 1000 —
	// and the affine temperature handlers compounded it further.
	if rv := decimal.RatFromFloat(value); rv != nil {
		exact, err := s.convertRatCore(rv, parts)
		if err == nil {
			out, _ := exact.Float64()
			return out, nil
		}
		if !errors.Is(err, ErrNotRational) {
			return 0, ucumerr.Conversion(from, to, err)
		}
		// Non-rational scale: fall through to the float64 handlers below.
	}

	result := value

	// Step 1: If source is special, convert value to canonical first.
	if srcUse := s.specialUseForTerm(srcTerm); srcUse != nil {
		result = srcUse.ToCanonical(result)
	}

	// Step 2: Multiplicative conversion, exact factor with a single rounding.
	result = decimal.MulExact(result, new(big.Rat).Quo(srcCan.Value.Rat(), dstCan.Value.Rat()))

	// Step 3: If dest is special, convert from canonical.
	if dstUse := s.specialUseForTerm(dstTerm); dstUse != nil {
		result = dstUse.FromCanonical(result)
	}

	return result, nil
}

// IsComparable returns true if the two unit codes have the same canonical units.
func (s *Service) IsComparable(code1, code2 string) (bool, error) {
	can1, err := s.getCanonical(code1)
	if err != nil {
		return false, ucumerr.Validation(code1, err)
	}
	can2, err := s.getCanonical(code2)
	if err != nil {
		return false, ucumerr.Validation(code2, err)
	}
	return expr.ComposeCanonicalUnits(can1) == expr.ComposeCanonicalUnits(can2), nil
}

// Multiply multiplies two value/unit pairs.
func (s *Service) Multiply(v1, v2 Pair) (Pair, error) {
	return s.combine(v1, v2, expr.OpMultiplication)
}

// Divide divides two value/unit pairs.
func (s *Service) Divide(v1, v2 Pair) (Pair, error) {
	return s.combine(v1, v2, expr.OpDivision)
}

// combine is the shared body of Multiply and Divide. Both map each operand onto
// its canonical scale, combine the operands, combine the canonical factors and
// round once; they differ in the operator and in division having to reject a
// zero divisor.
func (s *Service) combine(v1, v2 Pair, op expr.Operator) (Pair, error) {
	t1, can1, err := s.canonicalParts(v1.Code)
	if err != nil {
		return Pair{}, ucumerr.Validation(v1.Code, err)
	}
	t2, can2, err := s.canonicalParts(v2.Code)
	if err != nil {
		return Pair{}, ucumerr.Validation(v2.Code, err)
	}

	div := op == expr.OpDivision
	if div && can2.Value.IsZero() {
		return Pair{}, ucumerr.Validation(v2.Code, ucumerr.ErrDivisionByZero)
	}

	sign := 1
	factor := can1.Value.Rat()
	if div {
		sign = -1
		factor.Quo(factor, can2.Value.Rat())
	} else {
		factor.Mul(factor, can2.Value.Rat())
	}
	code := expr.ComposeCanonicalUnits(&model.Canonical{Units: mergeUnitLists(can1.Units, can2.Units, sign)})

	// Preferred path: both operands mapped and combined in exact arithmetic,
	// with a single rounding at the end.
	m1, m2, err := s.mappedRats(t1, t2, v1, v2)
	switch {
	case err == nil && m1 != nil:
		if div {
			// Defensive: a special operand can only map to exactly zero if its
			// input is exactly the bottom of its scale, which float64 cannot
			// represent for Cel or [degF]. Guarded so big.Rat.Quo cannot panic.
			if m2.Sign() == 0 {
				return Pair{}, zeroDivisorValue(v2)
			}
			m1.Quo(m1, m2)
		} else {
			m1.Mul(m1, m2)
		}
		out, _ := m1.Mul(m1, factor).Float64()
		return Pair{Value: out, Code: code}, nil
	case err != nil && !errors.Is(err, ErrNotRational):
		return Pair{}, ucumerr.Validation(v1.Code, err)
	}

	// Non-rational scale or non-finite value: use the float64 handlers.
	val1, val2 := s.mapFloat(t1, v1.Value), s.mapFloat(t2, v2.Value)
	combined := val1 * val2
	if div {
		if val2 == 0 {
			return Pair{}, zeroDivisorValue(v2)
		}
		combined = val1 / val2
	}
	return Pair{Value: decimal.MulExact(combined, factor), Code: code}, nil
}

// zeroDivisorValue reports a divisor whose *value* is zero, as opposed to a unit
// expression that cancels to zero. Both are ucumerr.ErrDivisionByZero, since a caller
// asking "was this a division by zero?" means the same question either way, but
// the message says which one happened.
func zeroDivisorValue(divisor Pair) error {
	return &ucumerr.ValidationError{
		Code:    divisor.Code,
		Message: fmt.Sprintf("the divisor has a value of zero (%v %s)", divisor.Value, divisor.Code),
		Offset:  -1,
		Err:     ucumerr.ErrDivisionByZero,
	}
}

// mappedRats maps both operands onto their canonical scales exactly, without the
// canonical factors. It returns nil results when an operand is not finite, and
// ErrNotRational when a scale has no rational form.
func (s *Service) mappedRats(t1, t2 *expr.Term, v1, v2 Pair) (m1, m2 *big.Rat, err error) {
	r1, r2 := decimal.RatFromFloat(v1.Value), decimal.RatFromFloat(v2.Value)
	if r1 == nil || r2 == nil {
		return nil, nil, nil
	}
	m1, err = s.toCanonicalRat(t1, v1.Code, r1)
	if err != nil {
		return nil, nil, err
	}
	m2, err = s.toCanonicalRat(t2, v2.Code, r2)
	if err != nil {
		return nil, nil, err
	}
	return m1, m2, nil
}
