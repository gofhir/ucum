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

	// La escala especial y el valor de origen, para obtener el valor racional
	// solo si hace falta (rat). Por valor: un puntero haría escapar el Use
	// al heap en cada conversión.
	use special.Use
	src float64

	// exact es el valor racional de una escala con forma racional
	// (temperatura), que mapSpecial ya calculó para redondearlo: una caché de
	// ToCanonicalExact, que daría el mismo valor. Se copia al usarlo: quien
	// llama lo muta.
	exact *big.Rat
}

// maxExactResultLog10 acota |log10| del resultado que se calcula exacto: más
// allá no cabe en float64 (el menor subnormal es 4.9e-324) y la vía log10 ya
// da ErrOutOfRange.
const maxExactResultLog10 = 330

// rat da el valor de x como racional, o nil si no lo tiene (#75, plan 010):
//   - guardado por log10 (intermedio no normal): special.ToCanonicalNear, la
//     potencia exacta, la partida si el exponente no es entero, o el cuadrado
//     exacto de sqrt;
//   - con intermedio normal y exponente entero, la potencia exacta;
//   - una escala de temperatura: su valor exacto, nunca el float64 de la suma;
//   - si no, el binario exacto del float64 finito que la vía float64 ya usa,
//     como con Cel (un exponente no entero aporta el de math.Pow).
//
// Np, un valor sobre el tope de costo o uno no finito no lo tienen, y quien
// llama sigue por log10.
func (x scalar) rat() *big.Rat {
	if x.exact != nil {
		return new(big.Rat).Set(x.exact)
	}
	if x.use.Handler != nil {
		if x.log {
			if r, ok := x.use.ToCanonicalNear(x.src); ok {
				return r
			}
			return nil
		}
		if r, ok := x.use.ToCanonicalExact(x.src); ok {
			return r
		}
	}
	if x.log {
		return nil
	}
	// RatFromFloat da nil con NaN y ±Inf.
	return decimal.RatFromFloat(x.v)
}

// exactPower da la potencia exacta de x cuando no está guardado por log10 y la
// escala es logarítmica de base entera con exponente entero (la regla de 009):
// el valor que la vía float64 redondea una vez en x.v. Una escala de
// temperatura da su valor exacto (plan 012). Con otra escala, o con un
// exponente no entero, no hay (el resultado no es racional).
func (x scalar) exactPower() (*big.Rat, bool) {
	if x.log || x.use.Handler == nil {
		return nil, false
	}
	if x.exact != nil {
		return new(big.Rat).Set(x.exact), true
	}
	return x.use.ToCanonicalExact(x.src)
}

// isOne: el factor es exactamente 1. Todo factor canónico es positivo.
func isOne(factor *big.Rat) bool {
	return factor.IsInt() && factor.Num().IsInt64() && factor.Num().Int64() == 1
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
	x, err := mapSpecial(use, value)
	x.use, x.src = *use, value
	return x, err
}

// mapSpecial es mapScalar con una escala especial.
func mapSpecial(use *special.Use, value float64) (scalar, error) {
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
	// Una escala con forma racional (temperatura): el valor exacto, redondeado
	// una vez (plan 012). La suma en float64 cancela (-273.15 Cel da 0, no los
	// 2.3e-14 K del binario) y redondea dos veces, con la multiplicación.
	if r, ok := use.ToCanonicalExact(value); ok {
		return mapRat(r), nil
	}
	// Sin forma log10 ni racional (tan): f⁻¹ es inyectiva y finita en todo ℝ,
	// así que un ±Inf, o un 0 que no sale de 0, es redondeo de float64.
	if math.IsInf(m, 0) || m == 0 && value != 0 {
		return scalar{}, ucumerr.ErrOutOfRange
	}
	return scalar{v: m}, nil
}

// mapRat es mapSpecial para el valor exacto r de una escala con forma racional.
// Un cero exacto es un valor (-273150 mCel es 0 K), no un subdesborde. Si el
// redondeo no es normal (solo con un prefijo grande: 1.7e308 MCel es 1.7e314 K)
// el valor va por log10, como el intermedio no normal de 010, y rat() da r.
func mapRat(r *big.Rat) scalar {
	if r.Sign() == 0 {
		return scalar{}
	}
	if f, _ := r.Float64(); normal(f) {
		return scalar{v: f, exact: r}
	}
	return scalar{sign: float64(r.Sign()), l10: decimal.Log10Rat(r), log: true, exact: r}
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

// directTimes da x·factor, para un x que no está guardado por log10, redondeado
// una vez. Con potencia exacta y un factor distinto de 1 es base^n·factor
// exacto (#75, plan 011): redondear primero la potencia y después la multiplicación
// dejaba el resultado a 1 ulp del más cercano. Con factor 1 la potencia ya está
// redondeada una vez en x.v, y no se paga aritmética racional. El resultado
// puede ser ±Inf o cero, como el de decimal.MulExact: quien llama decide.
func (x scalar) directTimes(factor *big.Rat) float64 {
	if !isOne(factor) {
		if r, ok := x.exactPower(); ok {
			out, _ := r.Mul(r, factor).Float64()
			return out
		}
	}
	return decimal.MulExact(x.v, factor)
}

// linearTimes da x·factor como float64, redondeado una vez.
func (x scalar) linearTimes(factor *big.Rat) (float64, error) {
	if x.log {
		sign, l := x.log10Times(factor)
		// Donde la vía log10 perdería cifras, el valor exacto, si lo hay.
		if math.Abs(l) <= maxExactResultLog10 {
			if r := x.rat(); r != nil {
				return roundRat(r.Mul(r, factor))
			}
		}
		return fromLog10(sign, l)
	}
	out := x.directTimes(factor)
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
		return toLinearScale(x, factor, dst)
	}
	if !x.log {
		if m := x.directTimes(factor); normal(m) || x.v == 0 {
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

// smallestNormalLog10 es log10 del menor float64 normal, 2^-1022.
var smallestNormalLog10 = math.Log10(smallestNormal)

// toLinearScale es toDestination hacia una escala sin forma log10: tan y las
// de temperatura, donde f es finita en todo valor finito.
func toLinearScale(x scalar, factor *big.Rat, dst *special.Use) (float64, error) {
	// Una escala de temperatura: el exacto de x·factor pasa por f una vez, y
	// se redondea una vez (plan 012). Sin tope de 330: el origen devuelve al
	// rango un intermedio que no cabe (-400 [lgK] es -273.15 Cel).
	if r := x.rat(); r != nil {
		if out, ok := dst.FromCanonicalRat(r.Mul(r, factor)); ok {
			return roundRat(out)
		}
	}
	// Sin exacto (la base de ln no es racional, o bajo el tope de costo de
	// 10^-5000) y bajo el rango normal, el intermedio ε no alcanza a mover
	// f(ε)/α al otro lado de un punto medio entre dos float64 con los prefijos
	// del XML (un prefijo propio de cientos de cifras sí podría): se redondea
	// f(ε')/α, con ε' el menor subnormal con el signo de ε, para que un origen
	// justo en un punto medio quede del lado de ε.
	if x.log {
		if sign, l := x.log10Times(factor); l < smallestNormalLog10 {
			eps := new(big.Rat).SetFloat64(math.Copysign(0x1p-1074, sign))
			if out, ok := dst.FromCanonicalRat(eps); ok {
				return roundRat(out)
			}
		}
	}
	// Un intermedio sin exacto entra como el binario de su float64, como un
	// operando lineal (la regla de 010), y se redondea una vez: (m - origen)/α en
	// float64 redondearía dos veces.
	m, err := x.linearTimes(factor)
	if err != nil {
		return 0, err
	}
	if r := decimal.RatFromFloat(m); r != nil {
		if out, ok := dst.FromCanonicalRat(r); ok {
			return roundRat(out)
		}
	}
	return dst.FromCanonical(m), nil
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
			if out, ok, err := exactDirect(x1, x2, div, factor); ok {
				return out, err
			}
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
	// Donde la vía log10 perdería cifras, el valor exacto de los dos
	// operandos, si lo tienen (#75).
	if math.Abs(l1+l2+decimal.Log10Rat(factor)) <= maxExactResultLog10 {
		if r1, r2 := x1.rat(), x2.rat(); r1 != nil && r2 != nil {
			if div {
				r1.Quo(r1, r2)
			} else {
				r1.Mul(r1, r2)
			}
			return roundRat(r1.Mul(r1, factor))
		}
	}
	return scalar{sign: s1 * s2, l10: l1 + l2, log: true}.linearTimes(factor)
}

// exactDirect es la vía directa de combineScalars (#75, plan 011): r1 ⊙ r2 ·
// factor exacto, redondeado una vez. Cada operando entra con la potencia exacta
// si la tiene y, si no, con el binario de su float64, como en el tramo no
// normal (rat). Redondear primero x1·x2 dejaba el resultado a 1 ulp, y un valor
// con potencia exacta quedaba fuera de orden con sus vecinos: 0 [pH] es 10^0
// exacto, y ±5e-324 [pH] dan math.Pow = 1. No hay atajo con factor 1: la
// multiplicación por el otro operando redondea igual (Multiply(-1 B, 3 1) es
// nearest(3/10)). Un divisor cero no llega: x1.v/x2.v es infinito o NaN y no
// entra en la vía directa. Con un operando no finito devuelve false y sigue el
// camino de float64, que da los mismos errores que antes.
func exactDirect(x1, x2 scalar, div bool, factor *big.Rat) (out float64, exact bool, err error) {
	r1, ok1 := x1.exactPower()
	r2, ok2 := x2.exactPower()
	if !ok1 {
		r1 = decimal.RatFromFloat(x1.v)
	}
	if !ok2 {
		r2 = decimal.RatFromFloat(x2.v)
	}
	// RatFromFloat da nil con NaN y ±Inf.
	if r1 == nil || r2 == nil {
		return 0, false, nil
	}
	if div {
		r1.Quo(r1, r2)
	} else {
		r1.Mul(r1, r2)
	}
	out, err = roundRat(r1.Mul(r1, factor))
	return out, true, err
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
