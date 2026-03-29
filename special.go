package ucum

import "math"

// SpecialHandler converts between a special unit and its canonical base.
type SpecialHandler interface {
	Code() string
	Units() string
	ToCanonical(value float64) float64
	FromCanonical(value float64) float64
}

// specialHandlers maps special unit codes to their handlers.
var specialHandlers = map[string]SpecialHandler{
	// Temperature (offset units)
	"Cel":     offsetHandler{code: "Cel", units: "K", offset: 273.15},
	"[degF]":  affineHandler{code: "[degF]", units: "K", scale: 5.0 / 9.0, offset: 459.67},
	"[degRe]": affineHandler{code: "[degRe]", units: "K", scale: 5.0 / 4.0, offset: 273.15},

	// Logarithmic
	"[pH]":     logHandler{code: "[pH]", units: "mol/l", base: 10, negate: true},
	"Np":       logHandler{code: "Np", units: "1", base: math.E},
	"B":        logHandler{code: "B", units: "1", base: 10},
	"B[SPL]":   logHandler{code: "B[SPL]", units: "10*-5.Pa", base: 10, factor: 2},
	"B[V]":     logHandler{code: "B[V]", units: "V", base: 10, factor: 2},
	"B[mV]":    logHandler{code: "B[mV]", units: "mV", base: 10, factor: 2},
	"B[uV]":    logHandler{code: "B[uV]", units: "uV", base: 10, factor: 2},
	"B[10.nV]": logHandler{code: "B[10.nV]", units: "10*-9.V", base: 10, factor: 2},
	"B[W]":     logHandler{code: "B[W]", units: "W", base: 10},
	"B[kW]":    logHandler{code: "B[kW]", units: "kW", base: 10},
	"bit_s":    logHandler{code: "bit_s", units: "1", base: 2},

	// Trigonometric
	"[p'diop]": tanHandler{code: "[p'diop]", units: "rad", factor: 100},
	"%[slope]": tanHandler{code: "%[slope]", units: "deg", factor: 100},

	// Power
	"[m/s2/Hz^(1/2)]": sqrtHandler{code: "[m/s2/Hz^(1/2)]", units: "m2/s4/Hz"},

	// Homeopathic
	"[hp'_X]": logHandler{code: "[hp'_X]", units: "1", base: 10, negate: true},
	"[hp'_C]": logHandler{code: "[hp'_C]", units: "1", base: 100, negate: true},
	"[hp'_M]": logHandler{code: "[hp'_M]", units: "1", base: 1000, negate: true},
	"[hp'_Q]": logHandler{code: "[hp'_Q]", units: "1", base: 50000, negate: true},
}

// offsetHandler: canonical = value + offset (Celsius)
type offsetHandler struct {
	code, units string
	offset      float64
}

func (h offsetHandler) Code() string                    { return h.code }
func (h offsetHandler) Units() string                   { return h.units }
func (h offsetHandler) ToCanonical(v float64) float64   { return v + h.offset }
func (h offsetHandler) FromCanonical(v float64) float64 { return v - h.offset }

// affineHandler: canonical = (value + offset) * scale (Fahrenheit, Reaumur)
type affineHandler struct {
	code, units   string
	scale, offset float64
}

func (h affineHandler) Code() string                    { return h.code }
func (h affineHandler) Units() string                   { return h.units }
func (h affineHandler) ToCanonical(v float64) float64   { return (v + h.offset) * h.scale }
func (h affineHandler) FromCanonical(v float64) float64 { return v/h.scale - h.offset }

// logHandler: canonical = base^(value*factor) or base^(-value*factor) if negate
type logHandler struct {
	code, units string
	base        float64
	factor      float64 // multiplier for exponent (default 1)
	negate      bool
}

func (h logHandler) Code() string  { return h.code }
func (h logHandler) Units() string { return h.units }
func (h logHandler) ToCanonical(v float64) float64 {
	f := h.effectiveFactor()
	if h.negate {
		return math.Pow(h.base, -v*f)
	}
	return math.Pow(h.base, v*f)
}
func (h logHandler) FromCanonical(v float64) float64 {
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

// tanHandler: canonical = arctan(value/factor) (prism diopter, percent slope)
type tanHandler struct {
	code, units string
	factor      float64
}

func (h tanHandler) Code() string                    { return h.code }
func (h tanHandler) Units() string                   { return h.units }
func (h tanHandler) ToCanonical(v float64) float64   { return math.Atan(v / h.factor) }
func (h tanHandler) FromCanonical(v float64) float64 { return math.Tan(v) * h.factor }

// sqrtHandler: canonical = value^2
type sqrtHandler struct {
	code, units string
}

func (h sqrtHandler) Code() string                    { return h.code }
func (h sqrtHandler) Units() string                   { return h.units }
func (h sqrtHandler) ToCanonical(v float64) float64   { return v * v }
func (h sqrtHandler) FromCanonical(v float64) float64 { return math.Sqrt(v) }
