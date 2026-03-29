package ucum

import "math"

// specialHandler converts between a special unit and its canonical base.
type specialHandler interface {
	code() string
	units() string
	toCanonical(value float64) float64
	fromCanonical(value float64) float64
}

// specialHandlers maps special unit codes to their handlers.
var specialHandlers = map[string]specialHandler{
	// Temperature (offset units).
	"Cel":     offsetHandler{unitCode: "Cel", unitExpr: "K", offset: 273.15},
	"[degF]":  affineHandler{unitCode: "[degF]", unitExpr: "K", scale: 5.0 / 9.0, offset: 459.67},
	"[degRe]": affineHandler{unitCode: "[degRe]", unitExpr: "K", scale: 5.0 / 4.0, offset: 273.15},

	// Logarithmic.
	"[pH]":     logHandler{unitCode: "[pH]", unitExpr: "mol/l", base: 10, negate: true},
	"Np":       logHandler{unitCode: "Np", unitExpr: "1", base: math.E},
	"B":        logHandler{unitCode: "B", unitExpr: "1", base: 10},
	"B[SPL]":   logHandler{unitCode: "B[SPL]", unitExpr: "10*-5.Pa", base: 10, factor: 2},
	"B[V]":     logHandler{unitCode: "B[V]", unitExpr: "V", base: 10, factor: 2},
	"B[mV]":    logHandler{unitCode: "B[mV]", unitExpr: "mV", base: 10, factor: 2},
	"B[uV]":    logHandler{unitCode: "B[uV]", unitExpr: "uV", base: 10, factor: 2},
	"B[10.nV]": logHandler{unitCode: "B[10.nV]", unitExpr: "10*-9.V", base: 10, factor: 2},
	"B[W]":     logHandler{unitCode: "B[W]", unitExpr: "W", base: 10},
	"B[kW]":    logHandler{unitCode: "B[kW]", unitExpr: "kW", base: 10},
	"bit_s":    logHandler{unitCode: "bit_s", unitExpr: "1", base: 2},

	// Trigonometric.
	"[p'diop]": tanHandler{unitCode: "[p'diop]", unitExpr: "rad", factor: 100},
	"%[slope]": tanHandler{unitCode: "%[slope]", unitExpr: "deg", factor: 100},

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
}

func (h offsetHandler) code() string                    { return h.unitCode }
func (h offsetHandler) units() string                   { return h.unitExpr }
func (h offsetHandler) toCanonical(v float64) float64   { return v + h.offset }
func (h offsetHandler) fromCanonical(v float64) float64 { return v - h.offset }

// affineHandler converts via canonical = (value + offset) * scale (Fahrenheit, Reaumur).
type affineHandler struct {
	unitCode, unitExpr string
	scale, offset      float64
}

func (h affineHandler) code() string                    { return h.unitCode }
func (h affineHandler) units() string                   { return h.unitExpr }
func (h affineHandler) toCanonical(v float64) float64   { return (v + h.offset) * h.scale }
func (h affineHandler) fromCanonical(v float64) float64 { return v/h.scale - h.offset }

// logHandler converts via canonical = base^(value*factor) or base^(-value*factor) if negate.
type logHandler struct {
	unitCode, unitExpr string
	base               float64
	factor             float64 // multiplier for exponent (default 1)
	negate             bool
}

func (h logHandler) code() string  { return h.unitCode }
func (h logHandler) units() string { return h.unitExpr }
func (h logHandler) toCanonical(v float64) float64 {
	f := h.effectiveFactor()
	if h.negate {
		return math.Pow(h.base, -v*f)
	}
	return math.Pow(h.base, v*f)
}
func (h logHandler) fromCanonical(v float64) float64 {
	f := h.effectiveFactor()
	if h.negate {
		return -math.Log(v) / (math.Log(h.base) * f)
	}
	return math.Log(v) / (math.Log(h.base) * f)
}
func (h logHandler) effectiveFactor() float64 {
	if h.factor == 0 {
		return 1
	}
	return h.factor
}

// tanHandler converts via canonical = arctan(value/factor) (prism diopter, percent slope).
type tanHandler struct {
	unitCode, unitExpr string
	factor             float64
}

func (h tanHandler) code() string                    { return h.unitCode }
func (h tanHandler) units() string                   { return h.unitExpr }
func (h tanHandler) toCanonical(v float64) float64   { return math.Atan(v / h.factor) }
func (h tanHandler) fromCanonical(v float64) float64 { return math.Tan(v) * h.factor }

// sqrtHandler converts via canonical = value^2.
type sqrtHandler struct {
	unitCode, unitExpr string
}

func (h sqrtHandler) code() string                    { return h.unitCode }
func (h sqrtHandler) units() string                   { return h.unitExpr }
func (h sqrtHandler) toCanonical(v float64) float64   { return v * v }
func (h sqrtHandler) fromCanonical(v float64) float64 { return math.Sqrt(v) }
