package fhir

import (
	"fmt"
	"math/big"
	"strings"
)

// Decimal is a FHIR decimal: an exact value together with the precision its
// source declared.
//
// FHIR R5 requires that "implementations SHALL handle decimal values in ways
// that preserve and respect the precision of the value as represented for
// presentation purposes", and states that "0.010 is regarded as different to
// 0.01". A float64 cannot carry that distinction and neither can a big.Rat: both
// hold 0.010 and 0.01 as the same number. Decimal keeps the value exact and the
// precision alongside it, which is the representation the specification itself
// suggests.
//
// The zero Decimal is zero with unlimited precision. Build one with ParseDecimal
// so the precision comes from the text, which is where FHIR expresses it.
type Decimal struct {
	value   *big.Rat
	sigFigs int // 0 means unlimited
}

// ParseDecimal reads a FHIR decimal from its textual form, taking the precision
// from the way it is written.
//
// Significant figures follow the usual convention: leading zeros are not
// significant, trailing zeros after a decimal point are.
//
//	"0.01"    1 significant figure
//	"0.010"   2 — the trailing zero is why FHIR calls these different values
//	"1.50"    3
//	"1.5e3"   2, the exponent does not add precision
//
// A value written without a decimal point — "100", "42" — is taken to have
// unlimited precision rather than a guessed one. Writing "1.00e2" states three
// figures unambiguously, which is the reason that notation exists.
func ParseDecimal(s string) (Decimal, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Decimal{}, fmt.Errorf("empty decimal")
	}
	v, ok := new(big.Rat).SetString(s)
	if !ok {
		return Decimal{}, fmt.Errorf("invalid decimal %q", s)
	}
	return Decimal{value: v, sigFigs: significantFigures(s)}, nil
}

// NewDecimal builds a Decimal from an exact value and an explicit precision, for
// a caller that has the two separately. A sigFigs of zero means unlimited.
func NewDecimal(value *big.Rat, sigFigs int) Decimal {
	if value == nil {
		return Decimal{}
	}
	if sigFigs < 0 {
		sigFigs = 0
	}
	return Decimal{value: new(big.Rat).Set(value), sigFigs: sigFigs}
}

// significantFigures counts the significant digits in a decimal literal.
func significantFigures(s string) int {
	// The exponent conveys magnitude, not precision.
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimLeft(s, "+-")

	if !strings.Contains(s, ".") {
		// No decimal point: the precision is not stated, so nothing is claimed.
		return 0
	}

	digits := strings.Replace(s, ".", "", 1)

	// Leading zeros place the value, they do not measure it. Zeros after the
	// first non-zero digit do, including trailing ones.
	trimmed := strings.TrimLeft(digits, "0")
	if trimmed == "" {
		// All zeros: "0.00" states two figures of resolution.
		return len(digits) - 1
	}
	return len(trimmed)
}

// Rat returns the exact value, as a copy.
func (d Decimal) Rat() *big.Rat {
	if d.value == nil {
		return new(big.Rat)
	}
	return new(big.Rat).Set(d.value)
}

// Float64 returns the value as the nearest float64, losing both exactness and
// the precision the source declared. Prefer String for presentation and Rat for
// arithmetic.
func (d Decimal) Float64() float64 {
	f, _ := d.Rat().Float64()
	return f
}

// SignificantFigures returns the precision the source declared, or 0 if it
// declared none.
func (d Decimal) SignificantFigures() int { return d.sigFigs }

// String renders the value at its declared precision, which is what FHIR asks
// implementations to preserve for presentation.
//
// One limit is inherent to plain decimal notation: when the precision is coarser
// than the integer part, the two cannot be told apart. 150 to two significant
// figures and to three both render as "150", because expressing the difference
// needs scientific notation ("1.5e2"). The declared precision is still available
// from SignificantFigures.
//
// A value with unlimited precision is rendered exactly when it can be — as an
// integer or a terminating decimal — and to a bounded number of digits when it
// cannot, since a repeating fraction has no finite form.
func (d Decimal) String() string {
	if d.value == nil {
		return "0"
	}
	if d.sigFigs <= 0 {
		if d.value.IsInt() {
			return d.value.Num().String()
		}
		return strings.TrimRight(d.value.FloatString(20), "0")
	}
	return d.round(d.sigFigs)
}

// round renders the value to n significant figures.
func (d Decimal) round(n int) string {
	if d.value.Sign() == 0 {
		// For zero there is no leading significant digit to place, so the declared
		// figures are decimal places: "0.0" states one, "0.00" states two.
		return "0." + strings.Repeat("0", n)
	}

	// The decimal exponent of the leading digit, so that the number of places
	// after the point follows from the requested significant figures.
	exp := decimalExponent(d.value)
	places := n - exp
	if places < 0 {
		places = 0
	}
	out := d.value.FloatString(places)

	// FloatString rounds to the requested places, which can carry into a new
	// leading digit (9.99 to three figures is 10.0, not 10.00).
	if rounded, ok := new(big.Rat).SetString(out); ok && decimalExponent(rounded) != exp && rounded.Sign() != 0 {
		places = n - decimalExponent(rounded)
		if places < 0 {
			places = 0
		}
		out = d.value.FloatString(places)
	}
	return out
}

// decimalExponent returns the position of the leading significant digit: 1 for
// values in [1,10), 2 for [10,100), 0 for [0.1,1), and so on.
func decimalExponent(r *big.Rat) int {
	if r.Sign() == 0 {
		return 1
	}
	abs := new(big.Rat).Abs(r)
	exp := 0
	ten := big.NewRat(10, 1)
	one := big.NewRat(1, 1)
	for abs.Cmp(one) >= 0 {
		abs.Quo(abs, ten)
		exp++
	}
	for abs.Cmp(big.NewRat(1, 10)) < 0 {
		abs.Mul(abs, ten)
		exp--
	}
	return exp
}
