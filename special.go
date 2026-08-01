package ucum

import (
	"math"
	"math/big"
)

// specialHandler converts between a special unit and its canonical base.
type specialHandler interface {
	code() string
	units() string
	toCanonical(value float64) float64
	fromCanonical(value float64) float64
}

// ratHandler is implemented by the special handlers whose mapping is a rational
// function, so it can be carried through big.Rat with no rounding at all. The
// logarithmic, trigonometric and square-root handlers deliberately do not
// implement it: their results are irrational in general.
type ratHandler interface {
	toCanonicalRat(value *big.Rat) *big.Rat
	fromCanonicalRat(value *big.Rat) *big.Rat
}

// mustRat parses an exact decimal or fraction literal from this file. The input
// is never external, so a parse failure is a programming error.
func mustRat(s string) *big.Rat {
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		panic("ucum: invalid rational literal " + s)
	}
	return r
}

// specialHandlers maps special unit codes to their handlers.
var specialHandlers = map[string]specialHandler{
	// Temperature (offset units). Constants are given as exact decimal literals
	// so that the rational and float64 paths cannot drift apart.
	"Cel":    newOffsetHandler("Cel", "K", "273.15"),
	"[degF]": newAffineHandler("[degF]", "K", "5/9", "459.67"),
	// The Reaumur offset is expressed on the Reaumur scale, not the Celsius one:
	// 0 Re = 273.15 K requires (0 + offset) * 5/4 = 273.15, so offset = 273.15 * 4/5.
	"[degRe]": newAffineHandler("[degRe]", "K", "5/4", "218.52"),

	// Logarithmic. expDivisor mirrors the UCUM function: 1 for lg, ln and ld,
	// 2 for lgTimes2, whose inverse is base^(v/2) and not base^(2v).
	"[pH]": logHandler{unitCode: "[pH]", unitExpr: "mol/l", base: 10, negate: true},
	"Np":   logHandler{unitCode: "Np", unitExpr: "1", base: math.E},
	"B":    logHandler{unitCode: "B", unitExpr: "1", base: 10},
	// The reference level is 2x10^-5 Pa (20 uPa, the hearing threshold), which
	// the XML spells as value="2" over Unit="10*-5.Pa". The factor belongs in
	// the reference expression, not in the exponent.
	"B[SPL]":   logHandler{unitCode: "B[SPL]", unitExpr: "2.10*-5.Pa", base: 10, expDivisor: 2},
	"B[V]":     logHandler{unitCode: "B[V]", unitExpr: "V", base: 10, expDivisor: 2},
	"B[mV]":    logHandler{unitCode: "B[mV]", unitExpr: "mV", base: 10, expDivisor: 2},
	"B[uV]":    logHandler{unitCode: "B[uV]", unitExpr: "uV", base: 10, expDivisor: 2},
	"B[10.nV]": logHandler{unitCode: "B[10.nV]", unitExpr: "10*-9.V", base: 10, expDivisor: 2},
	"B[W]":     logHandler{unitCode: "B[W]", unitExpr: "W", base: 10},
	"B[kW]":    logHandler{unitCode: "B[kW]", unitExpr: "kW", base: 10},
	"bit_s":    logHandler{unitCode: "bit_s", unitExpr: "1", base: 2},

	// Trigonometric. atan returns radians, so a handler declared against another
	// angle unit has to convert into it: perRadian is how many of unitExpr make
	// up one radian.
	"[p'diop]": tanHandler{unitCode: "[p'diop]", unitExpr: "rad", factor: 100, perRadian: 1},
	"%[slope]": tanHandler{unitCode: "%[slope]", unitExpr: "deg", factor: 100, perRadian: 180 / math.Pi},

	// Power.
	"[m/s2/Hz^(1/2)]": sqrtHandler{unitCode: "[m/s2/Hz^(1/2)]", unitExpr: "m2/s4/Hz"},

	// Homeopathic.
	"[hp'_X]": logHandler{unitCode: "[hp'_X]", unitExpr: "1", base: 10, negate: true},
	"[hp'_C]": logHandler{unitCode: "[hp'_C]", unitExpr: "1", base: 100, negate: true},
	"[hp'_M]": logHandler{unitCode: "[hp'_M]", unitExpr: "1", base: 1000, negate: true},
	"[hp'_Q]": logHandler{unitCode: "[hp'_Q]", unitExpr: "1", base: 50000, negate: true},
}

// offsetHandler converts via canonical = value + offset (Celsius).
type offsetHandler struct {
	unitCode, unitExpr string
	offset             float64
	offsetRat          *big.Rat
}

func newOffsetHandler(code, expr, offset string) offsetHandler {
	r := mustRat(offset)
	f, _ := r.Float64()
	return offsetHandler{unitCode: code, unitExpr: expr, offset: f, offsetRat: r}
}

func (h offsetHandler) code() string                    { return h.unitCode }
func (h offsetHandler) units() string                   { return h.unitExpr }
func (h offsetHandler) toCanonical(v float64) float64   { return v + h.offset }
func (h offsetHandler) fromCanonical(v float64) float64 { return v - h.offset }

func (h offsetHandler) toCanonicalRat(v *big.Rat) *big.Rat {
	return new(big.Rat).Add(v, h.offsetRat)
}

func (h offsetHandler) fromCanonicalRat(v *big.Rat) *big.Rat {
	return new(big.Rat).Sub(v, h.offsetRat)
}

// affineHandler converts via canonical = (value + offset) * scale (Fahrenheit, Reaumur).
type affineHandler struct {
	unitCode, unitExpr string
	scale, offset      float64
	scaleRat           *big.Rat
	offsetRat          *big.Rat
}

func newAffineHandler(code, expr, scale, offset string) affineHandler {
	sr, or := mustRat(scale), mustRat(offset)
	sf, _ := sr.Float64()
	of, _ := or.Float64()
	return affineHandler{
		unitCode: code, unitExpr: expr,
		scale: sf, offset: of,
		scaleRat: sr, offsetRat: or,
	}
}

func (h affineHandler) code() string                    { return h.unitCode }
func (h affineHandler) units() string                   { return h.unitExpr }
func (h affineHandler) toCanonical(v float64) float64   { return (v + h.offset) * h.scale }
func (h affineHandler) fromCanonical(v float64) float64 { return v/h.scale - h.offset }

func (h affineHandler) toCanonicalRat(v *big.Rat) *big.Rat {
	return new(big.Rat).Mul(new(big.Rat).Add(v, h.offsetRat), h.scaleRat)
}

func (h affineHandler) fromCanonicalRat(v *big.Rat) *big.Rat {
	return new(big.Rat).Sub(new(big.Rat).Quo(v, h.scaleRat), h.offsetRat)
}

// logHandler converts via canonical = base^(value/expDivisor), negated when the
// unit counts downwards ([pH], the homeopathic potencies).
//
// The divisor comes straight from the UCUM function name: lg, ln and ld are
// value = log_base(canonical), so the divisor is 1; lgTimes2 is
// value = 2*lg(canonical), so the divisor is 2 and the inverse is 10^(v/2).
type logHandler struct {
	unitCode, unitExpr string
	base               float64
	expDivisor         float64 // exponent divisor (default 1)
	negate             bool
}

func (h logHandler) code() string  { return h.unitCode }
func (h logHandler) units() string { return h.unitExpr }
func (h logHandler) toCanonical(v float64) float64 {
	e := v / h.effectiveDivisor()
	if h.negate {
		e = -e
	}
	return math.Pow(h.base, e)
}
func (h logHandler) fromCanonical(v float64) float64 {
	d := h.effectiveDivisor()
	if h.negate {
		return -math.Log(v) * d / math.Log(h.base)
	}
	return math.Log(v) * d / math.Log(h.base)
}
func (h logHandler) effectiveDivisor() float64 {
	if h.expDivisor == 0 {
		return 1
	}
	return h.expDivisor
}

// tanHandler converts via canonical = arctan(value/factor), expressed in the
// unit the handler declares (prism diopter, percent slope).
//
// The math.Atan call yields radians while the declared unit may be something
// else, so
// the result is scaled by perRadian — the number of unitExpr in one radian.
// Without that, the raw radian figure would be relabelled as the declared unit and
// then scaled again by that unit's canonical factor.
type tanHandler struct {
	unitCode, unitExpr string
	factor             float64
	perRadian          float64 // units of unitExpr per radian (1 when unitExpr is rad)
}

func (h tanHandler) code() string  { return h.unitCode }
func (h tanHandler) units() string { return h.unitExpr }

func (h tanHandler) toCanonical(v float64) float64 {
	return math.Atan(v/h.factor) * h.effectivePerRadian()
}

func (h tanHandler) fromCanonical(v float64) float64 {
	return math.Tan(v/h.effectivePerRadian()) * h.factor
}

func (h tanHandler) effectivePerRadian() float64 {
	if h.perRadian == 0 {
		return 1
	}
	return h.perRadian
}

// sqrtHandler converts via canonical = value^2.
type sqrtHandler struct {
	unitCode, unitExpr string
}

func (h sqrtHandler) code() string                    { return h.unitCode }
func (h sqrtHandler) units() string                   { return h.unitExpr }
func (h sqrtHandler) toCanonical(v float64) float64   { return v * v }
func (h sqrtHandler) fromCanonical(v float64) float64 { return math.Sqrt(v) }
