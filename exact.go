package ucum

import (
	"errors"
	"fmt"
	"math/big"
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

// RatPair is a value with its UCUM unit code, held as an exact rational.
type RatPair struct {
	Value *big.Rat
	Code  string
}

// ExactService extends Service with exact rational arithmetic, for callers that
// need results free of float64 rounding — currency-style decimal storage,
// comparisons that must not depend on the last bits, or conversion factors that
// are cached and composed.
//
// It is additive: the float64 methods of Service behave the same and are still
// the right choice for the logarithmic and trigonometric scales, which have no
// exact rational form.
//
// The service returned by New and NewFromReader also satisfies this interface,
// so a type assertion covers a Service you already hold, including one built
// from custom definitions:
//
//	svc, err := ucum.NewFromReader(defs)
//	exact := svc.(ucum.ExactService)
type ExactService interface {
	Service

	// ConversionFactor returns the exact factor to multiply a value in `from`
	// by to obtain the value in `to`. It fails with ErrNotLinear if either unit
	// is special, and with an error if the units are not commensurable. The
	// result can be cached and composed by the caller.
	ConversionFactor(from, to string) (*big.Rat, error)

	// ConvertRat converts an exact value between units, including the affine
	// temperature scales. It fails with ErrNotRational when the mapping has no
	// exact rational form.
	ConvertRat(value *big.Rat, from, to string) (*big.Rat, error)

	// CanonicalRat returns the exact canonical (base-unit) form of a value.
	// It fails with ErrNotRational under the same conditions as ConvertRat.
	CanonicalRat(value *big.Rat, code string) (RatPair, error)
}

// NewExact creates a Service with the exact rational API, using the embedded
// ucum-essence.xml definitions.
func NewExact() (ExactService, error) {
	return newService(nil)
}

// ConversionFactor returns the exact multiplicative factor from -> to.
func (s *service) ConversionFactor(from, to string) (*big.Rat, error) {
	srcTerm, srcCan, err := s.canonicalParts(from)
	if err != nil {
		return nil, conversionError(from, to, err)
	}
	dstTerm, dstCan, err := s.canonicalParts(to)
	if err != nil {
		return nil, conversionError(from, to, err)
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
	if dstCan.value.isZero() {
		return nil, conversionError(from, to, ErrDivisionByZero)
	}
	return new(big.Rat).Quo(srcCan.value.rat(), dstCan.value.rat()), nil
}

// ConvertRat converts an exact value from one unit to another.
func (s *service) ConvertRat(value *big.Rat, from, to string) (*big.Rat, error) {
	if value == nil {
		return nil, fmt.Errorf("ucum: ConvertRat: %w", ErrNilValue)
	}

	srcTerm, srcCan, err := s.canonicalParts(from)
	if err != nil {
		return nil, conversionError(from, to, err)
	}
	dstTerm, dstCan, err := s.canonicalParts(to)
	if err != nil {
		return nil, conversionError(from, to, err)
	}
	if err := requireComparable(from, to, srcCan, dstCan); err != nil {
		return nil, err
	}
	if dstCan.value.isZero() {
		return nil, conversionError(from, to, ErrDivisionByZero)
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
	srcTerm, dstTerm *term
	srcCan, dstCan   *canonical
	from, to         string
}

// convertRatCore performs the exact conversion. Callers must have checked
// comparability and a non-zero destination factor.
func (s *service) convertRatCore(value *big.Rat, p convertParts) (*big.Rat, error) {
	// Step 1: map the source value onto its canonical scale.
	result, err := s.toCanonicalRat(p.srcTerm, p.from, value)
	if err != nil {
		return nil, err
	}

	// Step 2: multiplicative conversion, exact throughout.
	result.Mul(result, p.srcCan.value.rat())
	result.Quo(result, p.dstCan.value.rat())

	// Step 3: map onto the destination scale.
	return s.fromCanonicalRat(p.dstTerm, p.to, result)
}

// CanonicalRat returns the exact canonical form of a value.
func (s *service) CanonicalRat(value *big.Rat, code string) (RatPair, error) {
	if value == nil {
		return RatPair{}, fmt.Errorf("ucum: CanonicalRat: %w", ErrNilValue)
	}

	t, can, err := s.canonicalParts(code)
	if err != nil {
		return RatPair{}, validationError(code, err)
	}
	mapped, err := s.toCanonicalRat(t, code, value)
	if err != nil {
		return RatPair{}, err
	}
	return RatPair{
		Value: mapped.Mul(mapped, can.value.rat()),
		Code:  composeCanonicalUnits(can),
	}, nil
}

// toCanonicalRat maps value onto the canonical scale of the unit denoted by t,
// exactly. It fails with ErrNotRational for the non-rational special scales.
func (s *service) toCanonicalRat(t *term, code string, value *big.Rat) (*big.Rat, error) {
	use := s.specialUseForTerm(t)
	if use == nil {
		return new(big.Rat).Set(value), nil
	}
	mapped, ok := use.toCanonicalRat(value)
	if !ok {
		return nil, fmt.Errorf("ucum: %q: %w", code, ErrNotRational)
	}
	return mapped, nil
}

// fromCanonicalRat maps a canonical value onto the scale of the unit denoted by
// t, exactly.
func (s *service) fromCanonicalRat(t *term, code string, value *big.Rat) (*big.Rat, error) {
	use := s.specialUseForTerm(t)
	if use == nil {
		return value, nil
	}
	mapped, ok := use.fromCanonicalRat(value)
	if !ok {
		return nil, fmt.Errorf("ucum: %q: %w", code, ErrNotRational)
	}
	return mapped, nil
}

// requireComparable reports whether two canonical forms share the same base
// units, which is the precondition for any conversion between them.
func requireComparable(from, to string, srcCan, dstCan *canonical) error {
	srcUnits := composeCanonicalUnits(srcCan)
	dstUnits := composeCanonicalUnits(dstCan)
	if srcUnits != dstUnits {
		return &ConversionError{
			From:    from,
			To:      to,
			Message: fmt.Sprintf("units are not comparable: %s vs %s", srcUnits, dstUnits),
		}
	}
	return nil
}
