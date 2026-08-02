package ucum

import (
	"fmt"
	"math"
	"math/big"
)

// specialHandler is the conversion a special unit performs between its own scale
// and the reference quantity its definition names.
//
// A handler carries only the shape of that conversion. Every figure the
// definitions state — the reference unit and its multiplier — is read from the
// XML and applied by the canonicalizer, so no handler repeats a number that
// ucum-essence.xml already gives.
type specialHandler interface {
	code() string
	toCanonical(value float64) float64
	fromCanonical(value float64) float64
}

// ratHandler is implemented by the handlers whose mapping is a rational
// function, so it can be carried through big.Rat with no rounding at all. The
// logarithmic, trigonometric and square-root handlers deliberately do not
// implement it: their results are irrational in general.
type ratHandler interface {
	toCanonicalRat(value *big.Rat) *big.Rat
	fromCanonicalRat(value *big.Rat) *big.Rat
}

// linearHandler is implemented by the handlers whose scale is linear, so that a
// difference measured on that scale converts by the reference multiplier alone.
//
// A special unit inside an algebraic term denotes a difference rather than a
// point on its scale — a gradient of 1 [degF]/min is a rate of change, not a
// temperature — so its offset cancels while its scale remains. The logarithmic
// and trigonometric handlers do not implement it: 10^-pH does not decompose into
// a factor times a value, so a difference on that scale has no meaning.
type linearHandler interface {
	isLinear()
}

// nativeUnitHandler is implemented by handlers whose output is in a fixed unit
// regardless of what the definition names as its reference.
//
// The math.Atan call yields radians whether the unit is declared against rad
// ([p'diop]) or deg (%[slope]), so the canonicalizer scales by radians rather
// than by the declared reference. This is a property of the function, not of the
// definitions, which is why it belongs here.
type nativeUnitHandler interface {
	nativeUnit() string
}

// specialUse is a special unit as it appears in a code: its handler together
// with the scale factor of any prefix in front of it.
//
// UCUM §22.3 permits that prefix — "due to the requirement of the SI that does
// allow prefixes on the degree Celsius, special units may be scaled trough a
// prefix or an arbitrary numeric factor" — and §22.4 says where it applies. A
// scaled special unit is the quadruple s = (u, f_s, f_s-1, α), and:
//
//	x' = f_s(x) / α    from the proper unit to the special unit
//	x  = f_s-1(α x')   the reverse
//
// So α scales the *argument* of the conversion function, never its result. The
// difference is not cosmetic: scaling the result would move the origin of the
// scale along with the prefix, making 0 mCel a different temperature from 0 Cel,
// and would make 1 dB a factor of 1 — no gain at all — instead of 10^0.1.
type specialUse struct {
	handler specialHandler
	alpha   decimal
}

// code returns the special unit's code, without the prefix.
func (s specialUse) code() string { return s.handler.code() }

// toCanonical maps a value on the scaled special scale onto the proper unit:
// f_s-1(α x').
func (s specialUse) toCanonical(v float64) float64 {
	return s.handler.toCanonical(v * s.alpha.float64())
}

// fromCanonical maps a value on the proper unit onto the scaled special scale:
// f_s(x) / α.
func (s specialUse) fromCanonical(v float64) float64 {
	return s.handler.fromCanonical(v) / s.alpha.float64()
}

// toCanonicalRat is toCanonical without rounding. It reports false when the
// handler's mapping has no exact rational form.
func (s specialUse) toCanonicalRat(v *big.Rat) (*big.Rat, bool) {
	rh, ok := s.handler.(ratHandler)
	if !ok {
		return nil, false
	}
	return rh.toCanonicalRat(new(big.Rat).Mul(v, s.alpha.rat())), true
}

// fromCanonicalRat is fromCanonical without rounding. It reports false when the
// handler's mapping has no exact rational form.
func (s specialUse) fromCanonicalRat(v *big.Rat) (*big.Rat, bool) {
	rh, ok := s.handler.(ratHandler)
	if !ok {
		return nil, false
	}
	return new(big.Rat).Quo(rh.fromCanonicalRat(v), s.alpha.rat()), true
}

// isLinear reports whether the underlying scale is linear, which is what lets a
// difference on it convert by the reference multiplier alone.
func (s specialUse) isLinear() bool {
	_, ok := s.handler.(linearHandler)
	return ok
}

// nativeUnit returns the fixed unit the handler produces its result in, if it
// has one.
func (s specialUse) nativeUnit() (string, bool) {
	nh, ok := s.handler.(nativeUnitHandler)
	if !ok {
		return "", false
	}
	return nh.nativeUnit(), true
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

// specialFunctions maps a UCUM function name to the conversion it performs.
//
// The definitions name the function and give its reference quantity, but not the
// function's own constants, because the specification states those in prose. So
// the offsets and logarithm bases live here — keyed by function name rather than
// by unit code — while every scale, reference unit and multiplier is read from
// the XML. A unit that reuses an existing function needs no code at all: it works
// the moment the definitions declare it.
//
// Sources, all from the UCUM specification:
//
//   - §21-22, temperature: the Celsius, Fahrenheit and Reaumur scales. Only each
//     scale's origin is here; its size comes from its reference quantity.
//   - §44 Table 18: hpX(x) = -lg x, hpC(x) = -ln(x)/ln(100), and analogous
//     functions with bases 1,000 and 50,000.
//   - §44 Table 18: f_PD(α) = tan(α) × 100, written both "tanTimes100" and
//     "100tan".
//   - §45 Table 19: f_pH(x) = -lg x.
//   - §46 Table 20: "ln", "lg" and "2lg" are the natural logarithm, the decadic
//     logarithm, and the decadic logarithm times two, with their inverses.
//   - §47 Table 22: "ld", the binary logarithm.
//   - §48 Table 21: "sqrt", the square root with the square as its inverse.
var specialFunctions = map[string]func(code string) specialHandler{
	// Temperature. Each scale's origin is stated in that scale's own degrees, and
	// is the one figure the definitions do not carry.
	"Cel":   func(c string) specialHandler { return newOffsetHandler(c, "273.15") },
	"degF":  func(c string) specialHandler { return newOffsetHandler(c, "459.67") },
	"degRe": func(c string) specialHandler { return newOffsetHandler(c, "218.52") },

	// Logarithmic. expDivisor is 2 for lgTimes2, whose inverse is base^(v/2).
	"lg":       func(c string) specialHandler { return logHandler{unitCode: c, base: 10, expDivisor: 1} },
	"lgTimes2": func(c string) specialHandler { return logHandler{unitCode: c, base: 10, expDivisor: 2} },
	"ln":       func(c string) specialHandler { return logHandler{unitCode: c, base: math.E, expDivisor: 1} },
	"ld":       func(c string) specialHandler { return logHandler{unitCode: c, base: 2, expDivisor: 1} },
	"pH":       func(c string) specialHandler { return logHandler{unitCode: c, base: 10, expDivisor: 1, negate: true} },
	"hpX":      func(c string) specialHandler { return logHandler{unitCode: c, base: 10, expDivisor: 1, negate: true} },
	"hpC":      func(c string) specialHandler { return logHandler{unitCode: c, base: 100, expDivisor: 1, negate: true} },
	"hpM":      func(c string) specialHandler { return logHandler{unitCode: c, base: 1000, expDivisor: 1, negate: true} },
	"hpQ": func(c string) specialHandler {
		return logHandler{unitCode: c, base: 50000, expDivisor: 1, negate: true}
	},

	// Trigonometric. The specification writes the same function both ways, and the
	// XML uses "tanTimes100" for [p'diop] and "100tan" for %[slope].
	"tanTimes100": func(c string) specialHandler { return tanHandler{unitCode: c, factor: 100} },
	"100tan":      func(c string) specialHandler { return tanHandler{unitCode: c, factor: 100} },

	// Power.
	"sqrt": func(c string) specialHandler { return sqrtHandler{unitCode: c} },
}

// buildSpecialHandlers builds the handler for every special unit in a model.
//
// It fails if the definitions name a function this package does not implement,
// which is how a new UCUM release announces itself rather than silently
// misconverting a unit.
func buildSpecialHandlers(m *ucumModel) (map[string]specialHandler, error) {
	handlers := make(map[string]specialHandler)
	for _, du := range m.DefinedUnits {
		if !du.IsSpecial {
			continue
		}
		if du.Value == nil || du.Value.Function == nil {
			return nil, fmt.Errorf("special unit %q has no function definition", du.Code)
		}
		name := du.Value.Function.Name
		build, ok := specialFunctions[name]
		if !ok {
			return nil, fmt.Errorf("special unit %q uses unsupported function %q", du.Code, name)
		}
		handlers[du.Code] = build(du.Code)
	}
	return handlers, nil
}

// offsetHandler shifts the origin of a scale, in that scale's own units:
// canonical = value + offset.
//
// The size of the unit is not here. The definitions give it as the function's
// reference quantity — Cel is cel(1 K), degF is degf(5 K/9) — so the
// canonicalizer scales by it. This handler only moves the zero, which is why one
// type serves Celsius, Fahrenheit and Reaumur alike: they differ in size, and
// size is data.
type offsetHandler struct {
	unitCode  string
	offset    float64
	offsetRat *big.Rat
}

func newOffsetHandler(code, offset string) offsetHandler {
	r := mustRat(offset)
	f, _ := r.Float64()
	return offsetHandler{unitCode: code, offset: f, offsetRat: r}
}

func (h offsetHandler) code() string                    { return h.unitCode }
func (h offsetHandler) isLinear()                       {}
func (h offsetHandler) toCanonical(v float64) float64   { return v + h.offset }
func (h offsetHandler) fromCanonical(v float64) float64 { return v - h.offset }

func (h offsetHandler) toCanonicalRat(v *big.Rat) *big.Rat {
	return new(big.Rat).Add(v, h.offsetRat)
}

func (h offsetHandler) fromCanonicalRat(v *big.Rat) *big.Rat {
	return new(big.Rat).Sub(v, h.offsetRat)
}

// logHandler converts via canonical = base^(value/expDivisor), negated when the
// unit counts downwards ([pH], the homeopathic potencies).
type logHandler struct {
	unitCode   string
	base       float64
	expDivisor float64
	negate     bool
}

func (h logHandler) code() string { return h.unitCode }

func (h logHandler) toCanonical(v float64) float64 {
	e := v / h.expDivisor
	if h.negate {
		e = -e
	}
	return math.Pow(h.base, e)
}

func (h logHandler) fromCanonical(v float64) float64 {
	if h.negate {
		return -math.Log(v) * h.expDivisor / math.Log(h.base)
	}
	return math.Log(v) * h.expDivisor / math.Log(h.base)
}

// tanHandler converts via canonical = arctan(value/factor), expressed in the
// angle unit the definitions name (prism diopter, percent slope).
type tanHandler struct {
	unitCode string
	factor   float64
}

func (h tanHandler) code() string                    { return h.unitCode }
func (h tanHandler) nativeUnit() string              { return "rad" }
func (h tanHandler) toCanonical(v float64) float64   { return math.Atan(v / h.factor) }
func (h tanHandler) fromCanonical(v float64) float64 { return math.Tan(v) * h.factor }

// sqrtHandler converts via canonical = value^2.
type sqrtHandler struct {
	unitCode string
}

func (h sqrtHandler) code() string                    { return h.unitCode }
func (h sqrtHandler) toCanonical(v float64) float64   { return v * v }
func (h sqrtHandler) fromCanonical(v float64) float64 { return math.Sqrt(v) }
