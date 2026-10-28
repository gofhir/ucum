// Package special holds the conversions for UCUM's special units, the ones on
// non-ratio scales: the temperature scales, the logarithmic ones, the
// trigonometric ones and the square root.
//
// A handler carries only the shape of its conversion. Every figure the
// definitions state — the reference unit, its multiplier, the scale factor of a
// prefix — is read from the XML and applied by the caller, so no handler repeats
// a number ucum-essence.xml already gives.
package special

import (
	"fmt"
	"math"
	"math/big"

	"github.com/gofhir/ucum/v4/internal/decimal"
	"github.com/gofhir/ucum/v4/internal/model"
)

// Handler is the conversion a special unit performs between its own scale
// and the reference quantity its definition names.
//
// A handler carries only the shape of that conversion. Every figure the
// definitions state — the reference unit and its multiplier — is read from the
// XML and applied by the canonicalizer, so no handler repeats a number that
// ucum-essence.xml already gives.
type Handler interface {
	code() string
	toCanonical(value float64) float64
	fromCanonical(value float64) float64

	// inDomain dice si f está definida en un valor de la escala de
	// referencia. Solo importa el signo, así que se le puede pasar el signo.
	inDomain(value float64) bool
}

// RatHandler is implemented by the handlers whose mapping is a rational
// function, so it can be carried through big.Rat with no rounding at all. The
// logarithmic, trigonometric and square-root handlers deliberately do not
// implement it: their results are irrational in general.
type RatHandler interface {
	toCanonicalRat(value *big.Rat) *big.Rat
	fromCanonicalRat(value *big.Rat) *big.Rat
}

// LinearHandler is implemented by the handlers whose scale is linear, so that a
// difference measured on that scale converts by the reference multiplier alone.
//
// A special unit inside an algebraic term denotes a difference rather than a
// point on its scale — a gradient of 1 [degF]/min is a rate of change, not a
// temperature — so its offset cancels while its scale remains. The logarithmic
// and trigonometric handlers do not implement it: 10^-pH does not decompose into
// a factor times a value, so a difference on that scale has no meaning.
type LinearHandler interface {
	isLinear()
}

// NativeUnitHandler is implemented by handlers whose output is in a fixed unit
// regardless of what the definition names as its reference.
//
// The math.Atan call yields radians whether the unit is declared against rad
// ([p'diop]) or deg (%[slope]), so the canonicalizer scales by radians rather
// than by the declared reference. This is a property of the function, not of the
// definitions, which is why it belongs here.
type NativeUnitHandler interface {
	nativeUnit() string
}

// Log10Handler is implemented by the handlers whose inverse is a power: the
// logarithmic ones, whose inverse is b^x, and the square root, whose inverse is
// the square (§44-48). Through it a value on the reference scale is carried as
// its log10 instead of being materialized.
//
// That is what keeps a conversion through a logarithmic scale right where
// float64 falls short: 1000 B is 10^1000 on the reference scale, which no
// float64 holds, while the same quantity in dB is 10000.
type Log10Handler interface {
	// toCanonicalLog10 da log10 |f⁻¹(v)|.
	toCanonicalLog10(v float64) float64

	// fromCanonicalLog10 da f(x) para x = 10^l.
	fromCanonicalLog10(l float64) float64

	// logarithmic dice si f es un logaritmo: entonces f(10^l) es lineal en l
	// y nunca necesita formar 10^l.
	logarithmic() bool
}

// Use is a special unit as it appears in a code: its handler together
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
type Use struct {
	// Handler is the conversion the unit performs.
	Handler Handler

	// Alpha is the scale factor of any prefix in front of it, or 1.
	Alpha decimal.Decimal
}

// Code returns the special unit's code, without the prefix.
func (s Use) Code() string { return s.Handler.code() }

// ToCanonical maps a value on the scaled special scale onto the proper unit:
// f_s-1(α x').
func (s Use) ToCanonical(v float64) float64 {
	return s.Handler.toCanonical(v * s.Alpha.Float64())
}

// FromCanonical maps a value on the proper unit onto the scaled special scale:
// f_s(x) / α.
func (s Use) FromCanonical(v float64) float64 {
	return s.Handler.fromCanonical(v) / s.Alpha.Float64()
}

// ToCanonicalRat is toCanonical without rounding. It reports false when the
// handler's mapping has no exact rational form.
func (s Use) ToCanonicalRat(v *big.Rat) (*big.Rat, bool) {
	rh, ok := s.Handler.(RatHandler)
	if !ok {
		return nil, false
	}
	return rh.toCanonicalRat(new(big.Rat).Mul(v, s.Alpha.Rat())), true
}

// FromCanonicalRat is fromCanonical without rounding. It reports false when the
// handler's mapping has no exact rational form.
func (s Use) FromCanonicalRat(v *big.Rat) (*big.Rat, bool) {
	rh, ok := s.Handler.(RatHandler)
	if !ok {
		return nil, false
	}
	return new(big.Rat).Quo(rh.fromCanonicalRat(v), s.Alpha.Rat()), true
}

// InDomain reports whether f_s is defined at a value on the reference scale: a
// logarithm only at a positive value, the square root at a non-negative one.
func (s Use) InDomain(v float64) bool { return s.Handler.inDomain(v) }

// ToCanonicalLog10 is ToCanonical in log10: log10 |f_s⁻¹(α x')|. It returns
// false if the handler has no such form.
func (s Use) ToCanonicalLog10(v float64) (float64, bool) {
	lh, ok := s.Handler.(Log10Handler)
	if !ok {
		return 0, false
	}
	return lh.toCanonicalLog10(v * s.Alpha.Float64()), true
}

// FromCanonicalLog10 is FromCanonical for a value given by its log10 l:
// f_s(10^l) / α. It returns false if the handler has no such form.
func (s Use) FromCanonicalLog10(l float64) (float64, bool) {
	lh, ok := s.Handler.(Log10Handler)
	if !ok {
		return 0, false
	}
	return lh.fromCanonicalLog10(l) / s.Alpha.Float64(), true
}

// HasLog10 reports whether the handler of s can carry a value as its log10.
func (s Use) HasLog10() bool {
	_, ok := s.Handler.(Log10Handler)
	return ok
}

// IsLogarithmic reports whether f_s is a logarithm, whose value
// FromCanonicalLog10 gives without ever forming 10^l.
func (s Use) IsLogarithmic() bool {
	lh, ok := s.Handler.(Log10Handler)
	return ok && lh.logarithmic()
}

// LogToLog converts v from one logarithmic unit to another without going
// through the reference scale. Both are linear in log10 of the reference value,
// so the conversion is affine in v:
//
//	r = v · (α_s/α_d) · (k_s/k_d) + l / (k_d · α_d)
//
// where k is the slope of each function (log10 of f⁻¹ per unit) and l is log10
// of the ratio of the references. It returns r as an exact rational, for the
// caller to round once, so 1e300 YB to YB does not form 10^324 and a result
// that cancels keeps its digits. It returns false if either unit is not
// logarithmic. Both v and l must be finite.
func LogToLog(src, dst Use, v, l float64) (*big.Rat, bool) {
	hs, ok := src.Handler.(logHandler)
	if !ok {
		return nil, false
	}
	hd, ok := dst.Handler.(logHandler)
	if !ok {
		return nil, false
	}
	// Todo va en racionales y se redondea una vez: los dos términos casi se
	// cancelan en 5.78 B[uV] a dB[mV] (57.8 - 60); k_s/k_d es -1/3 entre B y
	// [hp'_M]; y l/(k_d·α_d) con α_d = 10^-24 no es exacto en float64. Las
	// pendientes de base 10, 100 y 1000 (±1, ±1/2, ±2, ±3) sí lo son, y l
	// también, porque toda referencia de UCUM es una potencia de 10.
	ks, kd := new(big.Rat).SetFloat64(hs.slope()), new(big.Rat).SetFloat64(hd.slope())
	r := new(big.Rat).SetFloat64(v)
	r.Mul(r, new(big.Rat).Quo(src.Alpha.Rat(), dst.Alpha.Rat()))
	r.Mul(r, ks)
	r.Quo(r, kd)
	offset := new(big.Rat).SetFloat64(l)
	offset.Quo(offset, kd)
	offset.Quo(offset, dst.Alpha.Rat())
	return r.Add(r, offset), true
}

// IsLinear reports whether the underlying scale is linear, which is what lets a
// difference on it convert by the reference multiplier alone.
func (s Use) IsLinear() bool {
	_, ok := s.Handler.(LinearHandler)
	return ok
}

// NativeUnit returns the fixed unit the handler produces its result in, if it
// has one.
func (s Use) NativeUnit() (string, bool) {
	nh, ok := s.Handler.(NativeUnitHandler)
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

// functions maps a UCUM function name to the conversion it performs.
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
var functions = map[string]func(code string) Handler{
	// Temperature. Each scale's origin is stated in that scale's own degrees, and
	// is the one figure the definitions do not carry.
	"Cel":   func(c string) Handler { return newOffsetHandler(c, "273.15") },
	"degF":  func(c string) Handler { return newOffsetHandler(c, "459.67") },
	"degRe": func(c string) Handler { return newOffsetHandler(c, "218.52") },

	// Logarithmic. expDivisor is 2 for lgTimes2, whose inverse is base^(v/2).
	"lg":       func(c string) Handler { return logHandler{unitCode: c, base: 10, expDivisor: 1} },
	"lgTimes2": func(c string) Handler { return logHandler{unitCode: c, base: 10, expDivisor: 2} },
	"ln":       func(c string) Handler { return logHandler{unitCode: c, base: math.E, expDivisor: 1} },
	"ld":       func(c string) Handler { return logHandler{unitCode: c, base: 2, expDivisor: 1} },
	"pH":       func(c string) Handler { return logHandler{unitCode: c, base: 10, expDivisor: 1, negate: true} },
	"hpX":      func(c string) Handler { return logHandler{unitCode: c, base: 10, expDivisor: 1, negate: true} },
	"hpC":      func(c string) Handler { return logHandler{unitCode: c, base: 100, expDivisor: 1, negate: true} },
	"hpM":      func(c string) Handler { return logHandler{unitCode: c, base: 1000, expDivisor: 1, negate: true} },
	"hpQ": func(c string) Handler {
		return logHandler{unitCode: c, base: 50000, expDivisor: 1, negate: true}
	},

	// Trigonometric. The specification writes the same function both ways, and the
	// XML uses "tanTimes100" for [p'diop] and "100tan" for %[slope].
	"tanTimes100": func(c string) Handler { return tanHandler{unitCode: c, factor: 100} },
	"100tan":      func(c string) Handler { return tanHandler{unitCode: c, factor: 100} },

	// Power.
	"sqrt": func(c string) Handler { return sqrtHandler{unitCode: c} },
}

// BuildHandlers builds the handler for every special unit in a model.
//
// It fails if the definitions name a function this package does not implement,
// which is how a new UCUM release announces itself rather than silently
// misconverting a unit.
func BuildHandlers(m *model.Model) (map[string]Handler, error) {
	handlers := make(map[string]Handler)
	for _, du := range m.DefinedUnits {
		if !du.IsSpecial {
			continue
		}
		if du.Value == nil || du.Value.Function == nil {
			return nil, fmt.Errorf("special unit %q has no function definition", du.Code)
		}
		name := du.Value.Function.Name
		build, ok := functions[name]
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
func (h offsetHandler) inDomain(float64) bool           { return true }

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

// Un logaritmo solo existe para un argumento positivo (§45-47).
func (h logHandler) inDomain(v float64) bool { return v > 0 }
func (h logHandler) logarithmic() bool       { return true }

// slope es log10 de f⁻¹ por unidad de la escala: ±log10(base)/expDivisor.
// Con base 10 es ±1 o ±1/2, exactos.
func (h logHandler) slope() float64 {
	k := math.Log10(h.base) / h.expDivisor
	if h.negate {
		return -k
	}
	return k
}

// log10 de base^e es e·log10(base).
func (h logHandler) toCanonicalLog10(v float64) float64   { return v * h.slope() }
func (h logHandler) fromCanonicalLog10(l float64) float64 { return l / h.slope() }

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
func (h tanHandler) inDomain(float64) bool           { return true }

// sqrtHandler converts via canonical = value^2.
type sqrtHandler struct {
	unitCode string
}

func (h sqrtHandler) code() string                    { return h.unitCode }
func (h sqrtHandler) toCanonical(v float64) float64   { return v * v }
func (h sqrtHandler) fromCanonical(v float64) float64 { return math.Sqrt(v) }

// La raíz cuadrada real no existe para un negativo (§48).
func (h sqrtHandler) inDomain(v float64) bool { return v >= 0 }
func (h sqrtHandler) logarithmic() bool       { return false }

// toCanonicalLog10 de 0 es -Inf: quien llama trata el 0 antes, porque 0² = 0
// es un valor y no un desborde.
func (h sqrtHandler) toCanonicalLog10(v float64) float64   { return 2 * decimal.Log10Abs(v) }
func (h sqrtHandler) fromCanonicalLog10(l float64) float64 { return math.Pow(10, l/2) }
