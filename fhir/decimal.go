package fhir

import (
	"fmt"
	"math/big"
	"strconv"
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
// When the precision is coarser than the integer part, plain decimal notation
// cannot express it — "150" says nothing about whether its trailing zero is
// significant — so the value is rendered in scientific notation, which can:
// 150 to two significant figures is "1.5e2". Re-parsing that recovers both the
// value and the precision.
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
		// Fewer figures than the integer part has digits: plain notation would
		// silently claim precision the value does not have.
		return d.scientific(n, exp)
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

// scientific renders the value as a mantissa in [1,10) with n significant
// figures, followed by the power of ten. It is used when plain decimal notation
// cannot carry the precision.
//
// It is reached only when n is smaller than the number of integer digits, so the
// exponent is at least 2 and the shift is always a division.
func (d Decimal) scientific(n, exp int) string {
	out, carried := d.mantissa(n, exp)
	if carried {
		// Rounding took the mantissa to 10, which belongs in the exponent:
		// 996 to two figures is 1.0e3, not 10e2.
		exp++
		out, _ = d.mantissa(n, exp)
	}
	return out + "e" + strconv.Itoa(exp-1)
}

// mantissa divides the value down to [1,10) and renders it to n significant
// figures, reporting whether rounding carried it up to 10.
func (d Decimal) mantissa(n, exp int) (string, bool) {
	shift := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exp-1)), nil))
	out := new(big.Rat).Quo(d.value, shift).FloatString(n - 1)
	if r, ok := new(big.Rat).SetString(out); ok {
		if new(big.Rat).Abs(r).Cmp(big.NewRat(10, 1)) >= 0 {
			return out, true
		}
	}
	return out, false
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

// Arithmetic on Decimal keeps the value exact and propagates the declared
// precision, which are two different things and follow two different rules.
//
// The value is always carried exactly, as a rational: 1.0 divided by 3.0 holds
// 1/3, not 0.333. Only the rendering is rounded, and only to the precision the
// result is entitled to.
//
// The precision rules are the ordinary ones from measurement, and they differ
// between the two families of operation:
//
//   - Multiplication and division carry significant figures: a product is no
//     better known than its worst-known factor, so the result takes the smaller
//     count.
//   - Addition and subtraction carry decimal places: a sum is only as resolved
//     as its coarsest addend. 1.23 + 4.5 is 5.7, not 5.73, because the second
//     addend says nothing about hundredths.
//
// Conflating the two is a common mistake — the Java reference implementation
// applies the significant-figure rule to addition, and carries a TODO saying it
// should be using absolute precision instead.
//
// An operand with unlimited precision, written without a decimal point, does not
// limit the result: an exact count or conversion factor neither adds nor removes
// precision.

// Mul multiplies two decimals, taking the smaller significant-figure count.
func (d Decimal) Mul(o Decimal) Decimal {
	return Decimal{
		value:   new(big.Rat).Mul(d.Rat(), o.Rat()),
		sigFigs: combineFigures(d.sigFigs, o.sigFigs),
	}
}

// Div divides two decimals, taking the smaller significant-figure count. It
// fails when the divisor is zero.
func (d Decimal) Div(o Decimal) (Decimal, error) {
	if o.Rat().Sign() == 0 {
		return Decimal{}, fmt.Errorf("division by zero")
	}
	return Decimal{
		value:   new(big.Rat).Quo(d.Rat(), o.Rat()),
		sigFigs: combineFigures(d.sigFigs, o.sigFigs),
	}, nil
}

// Add adds two decimals, keeping the decimal places of the coarser operand.
func (d Decimal) Add(o Decimal) Decimal {
	return d.additive(o, new(big.Rat).Add(d.Rat(), o.Rat()))
}

// Sub subtracts o from d, keeping the decimal places of the coarser operand.
func (d Decimal) Sub(o Decimal) Decimal {
	return d.additive(o, new(big.Rat).Sub(d.Rat(), o.Rat()))
}

// additive assigns the precision of a sum or difference. The resolution of the
// result is that of its coarsest operand, measured in decimal places, which then
// has to be expressed back as a significant-figure count for the result's own
// magnitude.
func (d Decimal) additive(o Decimal, sum *big.Rat) Decimal {
	switch {
	case d.sigFigs <= 0 && o.sigFigs <= 0:
		return Decimal{value: sum}
	case d.sigFigs <= 0:
		return Decimal{value: sum, sigFigs: figuresForPlaces(sum, o.decimalPlaces())}
	case o.sigFigs <= 0:
		return Decimal{value: sum, sigFigs: figuresForPlaces(sum, d.decimalPlaces())}
	}

	places := d.decimalPlaces()
	if p := o.decimalPlaces(); p < places {
		places = p
	}
	return Decimal{value: sum, sigFigs: figuresForPlaces(sum, places)}
}

// decimalPlaces returns how many digits after the point the declared precision
// amounts to, which is the significant-figure count less the position of the
// leading digit.
func (d Decimal) decimalPlaces() int {
	if d.value == nil {
		return 0
	}
	return d.sigFigs - decimalExponent(d.value)
}

// figuresForPlaces converts a resolution in decimal places back into a
// significant-figure count for the given value.
func figuresForPlaces(v *big.Rat, places int) int {
	figs := places + decimalExponent(v)
	if figs < 1 {
		// The result is coarser than its own leading digit — 0.4 - 0.35 to one
		// decimal place. One figure is the least that can be reported.
		return 1
	}
	return figs
}

// combineFigures takes the smaller of two significant-figure counts, treating
// unlimited precision as no constraint.
func combineFigures(a, b int) int {
	switch {
	case a <= 0:
		return b
	case b <= 0:
		return a
	case a < b:
		return a
	default:
		return b
	}
}
