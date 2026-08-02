package engine

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/gofhir/ucum/v4/internal/expr"
	"github.com/gofhir/ucum/v4/internal/model"

	"github.com/gofhir/ucum/v4/internal/ucumerr"
)

// Errors reported by the exact API when no exact result exists.
var (
	// ErrNotLinear is returned by ConversionFactor when one of the units sits on
	// a non-ratio scale. Between Cel and [degF] the relation is affine
	// (Cel = ([degF] - 32) * 5/9), so no single multiplicative factor describes
	// it; use ConvertRat instead.
	ErrNotLinear = errors.New("unit is not on a ratio scale: no single conversion factor exists")

	// ErrNotRational is returned by ConvertRat and CanonicalRat when a special
	// unit's mapping is not a rational function — logarithmic ([pH], B, Np,
	// bit_s, [hp'_X]), trigonometric ([p'diop], %[slope]) or square root. Their
	// results are irrational in general, so they cannot be represented exactly
	// as a *big.Rat. Use Convert or Canonical for those.
	ErrNotRational = errors.New("unit conversion is not rational: no exact rational result exists")

	// ErrNilValue is returned when a *big.Rat argument is nil.
	ErrNilValue = errors.New("value must not be nil")
)

// ConversionFactor returns the exact multiplicative factor from -> to.
func (s *Service) ConversionFactor(from, to string) (*big.Rat, error) {
	srcTerm, srcCan, err := s.canonicalParts(from)
	if err != nil {
		return nil, ucumerr.Conversion(from, to, err)
	}
	dstTerm, dstCan, err := s.canonicalParts(to)
	if err != nil {
		return nil, ucumerr.Conversion(from, to, err)
	}

	// A special unit has no single factor: its scale is affine or worse. The
	// source is reported first so the error is deterministic when both are.
	if s.specialUseForTerm(srcTerm) != nil {
		return nil, fmt.Errorf("ucum: %q: %w", from, ErrNotLinear)
	}
	if s.specialUseForTerm(dstTerm) != nil {
		return nil, fmt.Errorf("ucum: %q: %w", to, ErrNotLinear)
	}

	if err := requireComparable(from, to, srcCan, dstCan); err != nil {
		return nil, err
	}
	if dstCan.Value.IsZero() {
		return nil, ucumerr.Conversion(from, to, ucumerr.ErrDivisionByZero)
	}
	return new(big.Rat).Quo(srcCan.Value.Rat(), dstCan.Value.Rat()), nil
}

// ConvertRat converts an exact value from one unit to another.
func (s *Service) ConvertRat(value *big.Rat, from, to string) (*big.Rat, error) {
	if value == nil {
		return nil, fmt.Errorf("ucum: ConvertRat: %w", ErrNilValue)
	}

	srcTerm, srcCan, err := s.canonicalParts(from)
	if err != nil {
		return nil, ucumerr.Conversion(from, to, err)
	}
	dstTerm, dstCan, err := s.canonicalParts(to)
	if err != nil {
		return nil, ucumerr.Conversion(from, to, err)
	}
	if err := requireComparable(from, to, srcCan, dstCan); err != nil {
		return nil, err
	}
	if dstCan.Value.IsZero() {
		return nil, ucumerr.Conversion(from, to, ucumerr.ErrDivisionByZero)
	}

	return s.convertRatCore(value, convertParts{
		srcTerm: srcTerm, dstTerm: dstTerm,
		srcCan: srcCan, dstCan: dstCan,
		from: from, to: to,
	})
}

// convertParts is the validated input of an exact conversion: both ASTs, both
// canonical forms, and the original codes for error messages.
type convertParts struct {
	srcTerm, dstTerm *expr.Term
	srcCan, dstCan   *model.Canonical
	from, to         string
}

// convertRatCore performs the exact conversion. Callers must have checked
// comparability and a non-zero destination factor.
func (s *Service) convertRatCore(value *big.Rat, p convertParts) (*big.Rat, error) {
	// Step 1: map the source value onto its canonical scale.
	result, err := s.toCanonicalRat(p.srcTerm, p.from, value)
	if err != nil {
		return nil, err
	}

	// Step 2: multiplicative conversion, exact throughout.
	result.Mul(result, p.srcCan.Value.Rat())
	result.Quo(result, p.dstCan.Value.Rat())

	// Step 3: map onto the destination scale.
	return s.fromCanonicalRat(p.dstTerm, p.to, result)
}

// CanonicalRat returns the exact canonical form of a value.
func (s *Service) CanonicalRat(value *big.Rat, code string) (RatPair, error) {
	if value == nil {
		return RatPair{}, fmt.Errorf("ucum: CanonicalRat: %w", ErrNilValue)
	}

	t, can, err := s.canonicalParts(code)
	if err != nil {
		return RatPair{}, ucumerr.Validation(code, err)
	}
	mapped, err := s.toCanonicalRat(t, code, value)
	if err != nil {
		return RatPair{}, err
	}
	return RatPair{
		Value: mapped.Mul(mapped, can.Value.Rat()),
		Code:  expr.ComposeCanonicalUnits(can),
	}, nil
}

// toCanonicalRat maps value onto the canonical scale of the unit denoted by t,
// exactly. It fails with ErrNotRational for the non-rational special scales.
func (s *Service) toCanonicalRat(t *expr.Term, code string, value *big.Rat) (*big.Rat, error) {
	use := s.specialUseForTerm(t)
	if use == nil {
		return new(big.Rat).Set(value), nil
	}
	mapped, ok := use.ToCanonicalRat(value)
	if !ok {
		return nil, fmt.Errorf("ucum: %q: %w", code, ErrNotRational)
	}
	return mapped, nil
}

// fromCanonicalRat maps a canonical value onto the scale of the unit denoted by
// t, exactly.
func (s *Service) fromCanonicalRat(t *expr.Term, code string, value *big.Rat) (*big.Rat, error) {
	use := s.specialUseForTerm(t)
	if use == nil {
		return value, nil
	}
	mapped, ok := use.FromCanonicalRat(value)
	if !ok {
		return nil, fmt.Errorf("ucum: %q: %w", code, ErrNotRational)
	}
	return mapped, nil
}

// requireComparable reports whether two canonical forms share the same base
// units, which is the precondition for any conversion between them.
func requireComparable(from, to string, srcCan, dstCan *model.Canonical) error {
	srcUnits := expr.ComposeCanonicalUnits(srcCan)
	dstUnits := expr.ComposeCanonicalUnits(dstCan)
	if srcUnits != dstUnits {
		return &ucumerr.ConversionError{
			From:    from,
			To:      to,
			Message: fmt.Sprintf("units are not comparable: %s vs %s", srcUnits, dstUnits),
		}
	}
	return nil
}
