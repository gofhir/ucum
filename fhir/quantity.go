package fhir

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/gofhir/ucum/v4"
)

// Quantity is the part of a FHIR Quantity this package can reason about.
//
// System is optional: an empty System is taken to mean UCUMSystem, since that is
// the only system these operations apply to. Setting it to anything else is an
// error rather than a silent assumption — a Quantity coded in SNOMED or in a
// local system has no UCUM code to canonicalize.
type Quantity struct {
	Value  float64
	Code   string
	System string

	// Exact overrides Value when non-nil, and is used exactly as given.
	//
	// It exists because a FHIR decimal is serialized as text and a float64
	// cannot hold most of them: the JSON value 0.01 becomes the float64 nearest
	// 1/100, which is not 1/100. An exact comparison notices, so
	// Compare(1 mg/dL, 0.01 g/L) reports "not equal" for values a reader would
	// call equal. Carrying the decimal itself avoids the question:
	//
	//	v, _ := new(big.Rat).SetString("0.01")
	//	q := fhir.Quantity{Exact: v, Code: "g/L"}
	//
	// Prefer this wherever the value came from FHIR JSON rather than from a
	// computation.
	Exact *big.Rat
}

// rat returns the quantity's value as an exact rational, preferring Exact. It
// returns nil when Value is not finite, since that has no rational form.
func (q Quantity) rat() *big.Rat {
	if q.Exact != nil {
		return new(big.Rat).Set(q.Exact)
	}
	return new(big.Rat).SetFloat64(q.Value)
}

// float returns the quantity's value as a float64, for the non-rational scales.
func (q Quantity) float() float64 {
	if q.Exact != nil {
		f, _ := q.Exact.Float64()
		return f
	}
	return q.Value
}

// usesUCUM reports whether the quantity's system is UCUM, treating the empty
// string as UCUM.
func (q Quantity) usesUCUM() bool {
	return q.System == "" || q.System == UCUMSystem
}

// ErrNotUCUMSystem is returned when a Quantity carries a system other than
// UCUMSystem, in which case its code is not a UCUM code.
var ErrNotUCUMSystem = errors.New("quantity system is not " + UCUMSystem)

// Comparator compares FHIR Quantity values by canonicalizing them, which is what
// a server has to do to answer a quantity search with gt, lt, ge or le, and what
// any comparison across units requires.
//
// It holds a UCUM service, so build one and share it: the service is safe for
// concurrent use and caches parsed expressions.
type Comparator struct {
	svc ucum.ExactService
}

// NewComparator builds a Comparator over the embedded UCUM definitions.
func NewComparator() (*Comparator, error) {
	svc, err := ucum.NewExact()
	if err != nil {
		return nil, err
	}
	return &Comparator{svc: svc}, nil
}

// NewComparatorWith builds a Comparator over a service the caller already holds,
// including one built from custom definitions via ucum.NewFromReader.
func NewComparatorWith(svc ucum.ExactService) *Comparator {
	return &Comparator{svc: svc}
}

// Compare returns -1, 0 or 1 as a is less than, equal to, or greater than b,
// after converting both onto a common scale. It fails if the units are not
// comparable.
//
// The comparison is exact wherever the units allow it: both operands go through
// the exact rational API, so it does not turn on the last bits of a float64.
// That matters for the boundary cases a search comparator lands on — 1 L versus
// 1000 mL compares equal, where dividing rounded factors could make it not.
//
// For units on a scale with no exact rational form — pH, bel, prism diopter —
// the comparison falls back to float64, since no exact answer exists.
//
// Temperature is handled correctly, which is the case most likely to be wrong
// elsewhere: 100 [degF] compares below 50 Cel.
func (c *Comparator) Compare(a, b Quantity) (int, error) {
	if err := checkSystems(a, b); err != nil {
		return 0, err
	}
	ra, rb, err := c.canonicalPair(a, b)
	if err != nil {
		return 0, err
	}
	if ra != nil {
		return ra.Cmp(rb), nil
	}

	// Non-rational scale: compare in float64.
	fa, err := c.svc.Canonical(a.float(), a.Code)
	if err != nil {
		return 0, err
	}
	fb, err := c.svc.Canonical(b.float(), b.Code)
	if err != nil {
		return 0, err
	}
	if fa.Code != fb.Code {
		return 0, fmt.Errorf("quantities are not comparable: %s vs %s", fa.Code, fb.Code)
	}
	switch {
	case fa.Value < fb.Value:
		return -1, nil
	case fa.Value > fb.Value:
		return 1, nil
	default:
		return 0, nil
	}
}

// canonicalPair maps both quantities onto their canonical scales exactly. It
// returns nil rationals, and no error, when a scale has no exact rational form,
// which tells Compare to use the float64 path.
func (c *Comparator) canonicalPair(a, b Quantity) (ra, rb *big.Rat, err error) {
	va, vb := a.rat(), b.rat()
	if va == nil || vb == nil {
		return nil, nil, fmt.Errorf("quantity value is not finite")
	}

	ca, err := c.svc.CanonicalRat(va, a.Code)
	if err != nil {
		if errors.Is(err, ucum.ErrNotRational) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	cb, err := c.svc.CanonicalRat(vb, b.Code)
	if err != nil {
		if errors.Is(err, ucum.ErrNotRational) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	if ca.Code != cb.Code {
		return nil, nil, fmt.Errorf("quantities are not comparable: %s vs %s", ca.Code, cb.Code)
	}
	return ca.Value, cb.Value, nil
}

// ConvertDecimal converts a value between units, preserving the precision its
// source declared.
//
// The conversion itself is exact — it runs through the rational API — and the
// result carries the same significant figures as the input. That follows from
// how precision propagates: a unit conversion multiplies by an exactly known
// factor, and multiplying by an exact quantity neither adds nor removes
// significant figures.
//
//	d, _ := fhir.ParseDecimal("1.50")            // 3 significant figures
//	out, _ := c.ConvertDecimal(d, "g/L", "mg/dL")
//	out.String()                                 // "150", still 3 figures
//
// It fails for the scales with no exact rational form — pH, bel, prism diopter —
// where the result would be an approximation and claiming the input's precision
// for it would be a lie. Use Convert on the service for those.
func (c *Comparator) ConvertDecimal(d Decimal, from, to string) (Decimal, error) {
	out, err := c.svc.ConvertRat(d.Rat(), from, to)
	if err != nil {
		return Decimal{}, err
	}
	return NewDecimal(out, d.SignificantFigures()), nil
}

// Add returns a + b, in the units of a.
//
// FHIRPath defines arithmetic over quantities and says that "implementations
// that do support units shall do so as specified by [UCUM]", so the right
// operand is converted to the unit of the left one before the addition. The
// conversion and the sum are both exact, which is the point: ten quantities of
// 0.1 L add up to exactly 1 L, where ten float64 tenths do not.
//
// It fails with ucum.ErrNotLinear if either quantity sits on a non-ratio scale —
// see the note on Sub — and with an error if the units are not comparable.
func (c *Comparator) Add(a, b Quantity) (Quantity, error) {
	return c.additive(a, b, true)
}

// Sub returns a - b, in the units of a, under the same rules as Add.
//
// Both refuse the non-ratio scales rather than returning a number. Adding two
// points on an affine scale gives an answer that depends on the unit it is
// carried out in: 20 Cel + 20 Cel is 40 Cel, while the same sum done in kelvin
// is 586.3 K, which is 313.15 Cel. Neither is more correct than the other, so
// there is nothing to return. The same holds for the logarithmic scales, where
// adding two bels is not adding their values at all.
//
// A temperature *difference* is a different quantity, and UCUM has no separate
// unit for it. Where a rate or a gradient is meant, name the compound unit and
// convert it — Convert(1, "Cel/min", "K/min") — which is the reading in which
// the offset cancels.
func (c *Comparator) Sub(a, b Quantity) (Quantity, error) {
	return c.additive(a, b, false)
}

// additive is the shared body of Add and Sub.
func (c *Comparator) additive(a, b Quantity, add bool) (Quantity, error) {
	if err := checkSystems(a, b); err != nil {
		return Quantity{}, err
	}

	// ConversionFactor does three jobs here: it rejects a unit on a non-ratio
	// scale with ErrNotLinear, which is exactly the case that has no answer; it
	// rejects incommensurable units; and it returns the exact factor for
	// everything else.
	factor, err := c.svc.ConversionFactor(b.Code, a.Code)
	if err != nil {
		return Quantity{}, err
	}

	av, bv := a.rat(), b.rat()
	if av == nil || bv == nil {
		return Quantity{}, fmt.Errorf("quantity value is not finite")
	}

	bv.Mul(bv, factor)
	if add {
		av.Add(av, bv)
	} else {
		av.Sub(av, bv)
	}
	return Quantity{Exact: av, Code: a.Code, System: a.System}, nil
}

// Comparable reports whether two quantities can be compared at all, that is,
// whether their units share a canonical form.
func (c *Comparator) Comparable(a, b Quantity) (bool, error) {
	if err := checkSystems(a, b); err != nil {
		return false, err
	}
	return c.svc.IsComparable(a.Code, b.Code)
}

// checkSystems rejects quantities that are not coded in UCUM.
func checkSystems(qs ...Quantity) error {
	for _, q := range qs {
		if !q.usesUCUM() {
			return fmt.Errorf("%q: %w", q.System, ErrNotUCUMSystem)
		}
	}
	return nil
}

// CanonicalKey returns the canonical unit code of a quantity, which is the key a
// server can index quantity values under so that a search comparator only has to
// compare numbers.
//
// Two quantities with the same key are comparable; the value to store alongside
// it is the second return, the quantity restated in that unit.
func (c *Comparator) CanonicalKey(q Quantity) (code string, value float64, err error) {
	if err := checkSystems(q); err != nil {
		return "", 0, err
	}
	p, err := c.svc.Canonical(q.float(), q.Code)
	if err != nil {
		return "", 0, err
	}
	return p.Code, p.Value, nil
}
