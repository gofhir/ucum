package engine

import (
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/gofhir/ucum/v4/internal/decimal"
	"github.com/gofhir/ucum/v4/internal/expr"
	"github.com/gofhir/ucum/v4/internal/special"
	"github.com/gofhir/ucum/v4/internal/ucumerr"
)

// Resultados que float64 no puede llevar (#59, plan
// docs/plans/003-float-range.md, §2).
//
// Con una entrada finita, un resultado que no cabe en float64 es
// ucumerr.ErrOutOfRange, igual que uno distinto de cero que redondea a cero; un
// subnormal distinto de cero se acepta, porque es el valor redondeado una vez.
// Un valor sin imagen en la escala especial de destino es
// ucumerr.ErrOutOfDomain. Una entrada no finita no pasa por aquí (#55).
//
// La vía float64 materializaba f⁻¹(αx) antes de multiplicar por el factor
// canónico y de aplicar la f del destino, así que un intermedio fuera del rango
// normal desbordaba, se iba a cero o perdía cifras aunque el resultado cupiera:
// 618 B[SPL] pasa por 10^309 y vale 2·10^307. Por eso scalar lleva, cuando
// hace falta, el log10 del valor en vez del valor, y lo materializa una vez.

// scalar es un valor en la escala de referencia: un float64 o, cuando float64
// no lo lleva sin perderlo, su signo y el log10 de su magnitud.
type scalar struct {
	v    float64 // el valor, si no es log
	sign float64 // -1, 0 o 1, si es log
	l10  float64 // log10 |valor|, si es log
	log  bool
}

// normal: finito y de magnitud al menos 2^-1022, donde float64 tiene todas
// sus cifras.
func normal(v float64) bool {
	return !math.IsInf(v, 0) && math.Abs(v) >= smallestNormal
}

// mapScalar lleva un valor finito a la escala de referencia de use; sin use,
// lo deja como está. Una unidad con forma log10 da esa forma cuando el valor no
// es normal. Entre dos unidades logarítmicas no se llega aquí: special.LogToLog
// convierte sin la escala de referencia, porque 10^(1e-17) redondea a 1 y el
// valor se perdería aunque cupiera.
func mapScalar(use *special.Use, value float64) (scalar, error) {
	if use == nil {
		return scalar{v: value}, nil
	}
	m := use.ToCanonical(value)
	if l, ok := use.ToCanonicalLog10(value); ok {
		// 0² = 0 es un valor, no un subdesborde.
		if m == 0 && value == 0 {
			return scalar{}, nil
		}
		// α·v puede desbordar aunque v sea finito (1e306 kB): entonces ni el
		// log10 cabe en float64.
		if math.IsInf(l, 0) {
			return scalar{}, ucumerr.ErrOutOfRange
		}
		if !normal(m) {
			return scalar{sign: 1, l10: l, log: true}, nil
		}
		return scalar{v: m}, nil
	}
	// Sin forma log10 (tan, Cel, [degF] y [degRe]): f⁻¹ es inyectiva y
	// finita en todo ℝ, así que un ±Inf, o un 0 que no sale de 0, es
	// redondeo de float64.
	if math.IsInf(m, 0) || m == 0 && value != 0 {
		return scalar{}, ucumerr.ErrOutOfRange
	}
	return scalar{v: m}, nil
}

// log10Times da el signo y log10 |x·factor| sin multiplicar.
func (x scalar) log10Times(factor *big.Rat) (sign, l float64) {
	// Todo factor canónico es positivo.
	switch {
	case x.log:
		return x.sign, x.l10 + decimal.Log10Rat(factor)
	case x.v == 0:
		return 0, math.Inf(-1)
	}
	return math.Copysign(1, x.v), decimal.Log10Abs(x.v) + decimal.Log10Rat(factor)
}

// linearTimes da x·factor como float64, redondeado una vez.
func (x scalar) linearTimes(factor *big.Rat) (float64, error) {
	if x.log {
		return fromLog10(x.log10Times(factor))
	}
	out := decimal.MulExact(x.v, factor)
	if math.IsInf(out, 0) || out == 0 && x.v != 0 {
		return 0, ucumerr.ErrOutOfRange
	}
	return out, nil
}

// fromLog10 materializa sign·10^l. Pierde unas |l|·2^-52·ln 10 cifras
// relativas, por eso solo se usa cuando la vía directa no cabe.
func fromLog10(sign, l float64) (float64, error) {
	if sign == 0 {
		return 0, nil
	}
	v := math.Pow(10, l)
	if math.IsInf(v, 0) || v == 0 {
		return 0, ucumerr.ErrOutOfRange
	}
	return math.Copysign(v, sign), nil
}

// toDestination lleva x·factor a la escala de dst, o lo deja en la escala
// lineal sin dst (§22■4: r = f(m/u)/α).
//
// Mientras x·factor sea normal, va directo, como antes de #59: por log10 de
// x más log10 del factor perdería cifras sin necesidad (1000 W en B[kW] daría
// -1.8e-16 y no 0, porque log10(1/1000) no es -3 exacto). Solo un valor
// fuera del rango normal va por log10.
func toDestination(x scalar, factor *big.Rat, dst *special.Use) (float64, error) {
	if dst == nil {
		return x.linearTimes(factor)
	}
	if !dst.HasLog10() {
		// tan y las escalas de temperatura: f es finita en todo valor
		// finito.
		m, err := x.linearTimes(factor)
		if err != nil {
			return 0, err
		}
		return dst.FromCanonical(m), nil
	}
	if !x.log {
		if m := decimal.MulExact(x.v, factor); normal(m) || x.v == 0 {
			if !dst.InDomain(m) {
				return 0, ucumerr.ErrOutOfDomain
			}
			if dst.IsLogarithmic() {
				r, _ := dst.FromCanonicalLog10(math.Log10(m))
				return r, nil
			}
			return dst.FromCanonical(m), nil
		}
	}
	sign, l := x.log10Times(factor)
	if !dst.InDomain(sign) {
		return 0, ucumerr.ErrOutOfDomain
	}
	r, _ := dst.FromCanonicalLog10(l)
	// Un logaritmo vale 0 legítimamente (lg 1); la raíz de un positivo, no.
	if math.IsInf(r, 0) || r == 0 && !dst.IsLogarithmic() {
		return 0, ucumerr.ErrOutOfRange
	}
	return r, nil
}

// combineScalars es x1·x2 o x1/x2, por el factor de las unidades. Un divisor
// cero es ucumerr.ErrDivisionByZero; uno muy chico, no: 10^x nunca vale cero,
// así que el rango se decide antes que el cero.
func combineScalars(x1, x2 scalar, div bool, factor *big.Rat) (float64, error) {
	// Un divisor cero da Inf o NaN aquí y llega a la vía log10 con signo 0.
	if !x1.log && !x2.log {
		c := x1.v * x2.v
		if div {
			c = x1.v / x2.v
		}
		if normal(c) || c == 0 && x1.v == 0 {
			return scalar{v: c}.linearTimes(factor)
		}
	}
	one := big.NewRat(1, 1)
	s1, l1 := x1.log10Times(one)
	s2, l2 := x2.log10Times(one)
	if div {
		if s2 == 0 {
			return 0, ucumerr.ErrDivisionByZero
		}
		l2 = -l2
	}
	return scalar{sign: s1 * s2, l10: l1 + l2, log: true}.linearTimes(factor)
}

// convertFloat es Convert por float64, para una entrada finita en una escala
// no racional. Entre dos unidades logarítmicas la conversión es afín y no pasa
// por la escala de referencia; un destino logarítmico recibe el log10 del
// valor (toDestination).
func (s *Service) convertFloat(value float64, p convertParts) (float64, error) {
	srcUse, dstUse := s.specialUseForTerm(p.srcTerm), s.specialUseForTerm(p.dstTerm)
	factor := new(big.Rat).Quo(p.srcCan.Value.Rat(), p.dstCan.Value.Rat())
	fail := func(err error) (float64, error) {
		return 0, ucumerr.Conversion(p.from, p.to, valueError(err, value, p.from, false))
	}
	if srcUse != nil && dstUse != nil {
		if exact, ok := special.LogToLog(*srcUse, *dstUse, value, decimal.Log10Rat(factor)); ok {
			// 1e-300 yB es 10^-324 B, que redondea a cero.
			r, err := roundRat(exact)
			if err != nil {
				return fail(err)
			}
			return r, nil
		}
	}
	x, err := mapScalar(srcUse, value)
	if err != nil {
		return fail(err)
	}
	out, err := toDestination(x, factor, dstUse)
	if err != nil {
		return fail(err)
	}
	return out, nil
}

// combineFloat es combine por float64, para operandos finitos con una escala
// no racional.
func (s *Service) combineFloat(t1, t2 *expr.Term, v1, v2 Pair, div bool, factor *big.Rat) (float64, error) {
	x1, err := mapScalar(s.specialUseForTerm(t1), v1.Value)
	if err != nil {
		return 0, ucumerr.Validation(v1.Code, valueError(err, v1.Value, v1.Code, false))
	}
	x2, err := mapScalar(s.specialUseForTerm(t2), v2.Value)
	if err != nil {
		return 0, ucumerr.Validation(v2.Code, valueError(err, v2.Value, v2.Code, false))
	}
	out, err := combineScalars(x1, x2, div, factor)
	switch {
	case errors.Is(err, ucumerr.ErrDivisionByZero):
		return 0, zeroDivisorValue(v2)
	case err != nil:
		return 0, ucumerr.Validation(v1.Code, combineError(err, v1, v2, div, false))
	}
	return out, nil
}

// roundRat redondea un resultado exacto una vez.
func roundRat(exact *big.Rat) (float64, error) {
	out, _ := exact.Float64()
	if math.IsInf(out, 0) || out == 0 && exact.Sign() != 0 {
		return 0, ucumerr.ErrOutOfRange
	}
	return out, nil
}

// valueError agrega al error de rango el valor y la unidad que lo produjeron,
// y, si el resultado es exacto, dónde obtenerlo: la API exacta no redondea.
func valueError(err error, value float64, code string, exact bool) error {
	if exact {
		return fmt.Errorf("%v %s: %w; the exact API (NewExact) gives it as a *big.Rat", value, code, err)
	}
	return fmt.Errorf("%v %s: %w", value, code, err)
}
