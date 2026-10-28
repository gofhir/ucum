package engine

import (
	"math"
	"math/big"
	"testing"

	"github.com/gofhir/ucum/v4/internal/expr"
)

// Oráculo de rango (plan docs/plans/003-float-range.md, §2, tests B).
//
// Dice, para un valor finito en una unidad, el valor en las unidades base
// según la especificación y el XML, y si ese valor cabe en float64. No llama a
// nada del motor salvo sus parsers (x.parse y x.defParse, con checkSpelling):
// los valores de cada átomo salen de readEssence por inducción sobre las
// definiciones, y las funciones especiales, de las fórmulas de §21-22 y §44-48.
// TestRangeOracleIndependent lo verifica sobre el texto de este archivo.

// rangeKind es lo que la especificación dice del resultado.
type rangeKind int

const (
	rangeFits     rangeKind = iota // el resultado cabe en float64
	rangeOut                       // no cabe: desborda o redondea a cero sin serlo
	rangeOutside                   // no existe: fuera del dominio de la función
	rangeAtBorder                  // a menos de rangeBorder en log10 de un límite
)

// rangeWant es el resultado esperado. Si cabe, |got - value| tiene que ser a
// lo más absTol más relTol·|value|.
type rangeWant struct {
	kind   rangeKind
	value  float64
	relTol float64
	absTol float64
}

var (
	// Log10 del mayor float64.
	rangeLog10Max = math.Log10(math.MaxFloat64)
	// Log10 de 2^-1075, la mitad del menor subnormal: un valor de magnitud
	// menor o igual redondea a cero (el empate va al par).
	rangeLog10Zero = -1075 * math.Log10(2)
	// A esta distancia en log10 de un límite decide el redondeo de los log10,
	// no la regla, y el punto no se evalúa (se cuenta).
	rangeBorder = 1e-9
	// La separación entre subnormales.
	subnormalULP = math.SmallestNonzeroFloat64
)

// specTempOffset es el origen de cada escala de temperatura en sus propios
// grados: §30■4 (Cel), §43■4 (degF) y §43■5 (degRe).
var specTempOffset = map[string]*big.Rat{
	"Cel":   big.NewRat(27315, 100),
	"degF":  big.NewRat(45967, 100),
	"degRe": big.NewRat(21852, 100),
}

// specForwardOfLog10 es f(10^l) de cada función con forma log10: el valor en
// la unidad especial cuando el log10 del argumento es l. §44■3, §45■3, §46■3,
// §47■4 y §48■3.
var specForwardOfLog10 = map[string]func(l float64) float64{
	"pH":       func(l float64) float64 { return -l },
	"lg":       func(l float64) float64 { return l },
	"ln":       func(l float64) float64 { return l * math.Ln10 },
	"lgTimes2": func(l float64) float64 { return 2 * l },
	"ld":       func(l float64) float64 { return l / math.Log10(2) },
	"hpX":      func(l float64) float64 { return -l },
	"hpC":      func(l float64) float64 { return -l / 2 },
	"hpM":      func(l float64) float64 { return -l / 3 },
	"hpQ":      func(l float64) float64 { return -l / math.Log10(50000) },
}

// rangeOracle evalúa códigos contra el XML.
type rangeOracle struct {
	x    *javaContext
	memo map[string]*big.Rat
}

func newRangeOracle(t *testing.T) *rangeOracle {
	t.Helper()
	return &rangeOracle{x: newJavaContext(t, newTestService(t), nil, nil), memo: map[string]*big.Rat{}}
}

// atom es el valor de 1 de un átomo lineal en las unidades base: 1 para una
// unidad base; para una definida, el número de su definición por su
// expresión, evaluada con los átomos que nombra. Una especial o una que no
// se puede evaluar da false.
func (o *rangeOracle) atom(code string) (*big.Rat, bool) {
	if v, seen := o.memo[code]; seen {
		return v, v != nil
	}
	o.memo[code] = nil // corta un ciclo
	var out *big.Rat
	switch {
	case o.x.base[code]:
		out = big.NewRat(1, 1)
	case o.x.special[code]:
	default:
		def, ok := o.x.definedAs[code]
		if !ok {
			break
		}
		t, err := o.x.defParse(def)
		if err != nil {
			break
		}
		if f, ok := o.x.factorWith(t, o.atom); ok {
			out = f.Mul(f, o.x.definedValue[code])
		}
	}
	o.memo[code] = out
	return out, out != nil
}

// factor es el valor de 1 de una expresión lineal en las unidades base.
func (o *rangeOracle) factor(code string, p func(string) (*expr.Term, error)) (*big.Rat, bool) {
	t, err := p(code)
	if err != nil {
		return nil, false
	}
	return o.x.factorWith(t, o.atom)
}

// special es una unidad especial sola: su código, α (§22■4) y su función.
type rangeSpecial struct {
	code  string
	alpha *big.Rat
	fn    essenceFunction
}

func (o *rangeOracle) special(u string) (rangeSpecial, bool) {
	code, alpha, ok := o.x.loneSpecial(u)
	if !ok {
		return rangeSpecial{}, false
	}
	return rangeSpecial{code: code, alpha: alpha, fn: o.x.function[code]}, true
}

// ref es el valor de la referencia de la función en las unidades base: valor
// × unidad del elemento function del XML.
func (o *rangeOracle) ref(sp rangeSpecial) (*big.Rat, bool) {
	v, ok := new(big.Rat).SetString(sp.fn.Value)
	if !ok {
		return nil, false
	}
	f, ok := o.factor(sp.fn.Unit, o.x.defParse)
	if !ok {
		return nil, false
	}
	return f.Mul(f, v), true
}

// hasLog10Form: la inversa es una potencia (§44-48), así que el log10 del
// valor canónico se calcula sin materializarlo.
func hasLog10Form(fn string) bool {
	_, ok := specInverseLog10[fn]
	return ok || fn == "sqrt"
}

// inverseLog10 es log10 |f⁻¹(a)| para las funciones con forma log10. Para
// sqrt, a = 0 no tiene log10; quien llama lo trata antes.
func inverseLog10(fn string, a float64) float64 {
	if fn == "sqrt" {
		return 2 * math.Log10(math.Abs(a))
	}
	return specInverseLog10[fn](a)
}

// ratLog10 es log10 de un racional positivo, sin pasar por float64: por la
// mantisa y el exponente binario de big.Float.
func ratLog10(r *big.Rat) float64 {
	f := new(big.Float).SetPrec(200).SetRat(r)
	mant := new(big.Float)
	exp := f.MantExp(mant)
	m, _ := mant.Float64()
	return math.Log10(m) + float64(exp)*math.Log10(2)
}

// tenTo es 10^l redondeado una vez a float64, subnormales incluidos.
func tenTo(l float64) float64 {
	n := math.Floor(l)
	f := new(big.Float).SetPrec(200).SetFloat64(math.Pow(10, l-n))
	p := new(big.Float).SetPrec(200).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(math.Abs(n))), nil))
	if n < 0 {
		f.Quo(f, p)
	} else {
		f.Mul(f, p)
	}
	v, _ := f.Float64()
	return v
}

// wantFromLog10 clasifica un valor de magnitud 10^l y signo sign; tol es el
// error relativo que se acepta.
func wantFromLog10(l, sign, tol float64) rangeWant {
	switch {
	case math.Abs(l-rangeLog10Max) < rangeBorder || math.Abs(l-rangeLog10Zero) < rangeBorder:
		return rangeWant{kind: rangeAtBorder}
	case l > rangeLog10Max || l < rangeLog10Zero:
		return rangeWant{kind: rangeOut}
	}
	return rangeWant{kind: rangeFits, value: math.Copysign(tenTo(l), sign), relTol: tol, absTol: subnormalULP}
}

// fromRat clasifica un valor exacto: el motor lo calcula en racionales y lo
// redondea una vez, así que el esperado es exacto.
func fromRat(r *big.Rat) rangeWant {
	f, _ := r.Float64()
	if math.IsInf(f, 0) || f == 0 && r.Sign() != 0 {
		return rangeWant{kind: rangeOut}
	}
	return rangeWant{kind: rangeFits, value: f}
}

// log10Tol es el error relativo que se acepta para 10^l: el de l, que crece
// con |l|, y no menos de 1e-13.
func log10Tol(l float64) float64 {
	return math.Max(1e-13, math.Abs(l)*0x1p-49)
}

// canonical es el esperado de Canonical(v, code). Da false si el código no
// se puede evaluar: quien llama lo reporta como falla, no lo salta.
func (o *rangeOracle) canonical(v *big.Rat, code string) (rangeWant, bool) {
	sp, isSpecial := o.special(code)
	if !isSpecial {
		if o.x.anyAtom(code, func(a string) bool { return o.x.special[a] || o.x.arbitrary[a] }) {
			return rangeWant{}, false
		}
		f, ok := o.factor(code, o.x.parse)
		if !ok {
			return rangeWant{}, false
		}
		return fromRat(f.Mul(f, v)), true
	}
	av := new(big.Rat).Mul(v, sp.alpha)
	name := sp.fn.Name
	if off, ok := specTempOffset[name]; ok {
		// §22■4 y §30■4: m = (αv + origen) × referencia, exacto.
		ref, ok := o.ref(sp)
		if !ok {
			return rangeWant{}, false
		}
		m := new(big.Rat).Add(av, off)
		return fromRat(m.Mul(m, ref)), true
	}
	a, _ := av.Float64()
	switch {
	case name == "100tan" || name == "tanTimes100":
		// §44■4: f⁻¹(x) = arctan(x/100), en radianes. Es inyectiva y vale 0
		// solo en 0.
		m := math.Atan(a / 100)
		if m == 0 && v.Sign() != 0 {
			return rangeWant{kind: rangeOut}, true
		}
		return rangeWant{kind: rangeFits, value: m, relTol: 1e-15}, true
	case hasLog10Form(name):
		ref, ok := o.ref(sp)
		if !ok || ref.Sign() <= 0 {
			return rangeWant{}, false
		}
		if name == "sqrt" && a == 0 {
			return rangeWant{kind: rangeFits}, true
		}
		l := inverseLog10(name, a) + ratLog10(ref)
		return wantFromLog10(l, 1, log10Tol(l)), true
	}
	return rangeWant{}, false
}

// toSpecial es el esperado de Convert(v, from, to) con to una unidad especial
// con forma log10 y from una especial con forma log10 o una lineal:
// r = f(m / ref) / α, con m en log10 (§22■4).
func (o *rangeOracle) toSpecial(v *big.Rat, from, to string) (rangeWant, bool) {
	dst, ok := o.special(to)
	if !ok || !hasLog10Form(dst.fn.Name) {
		return rangeWant{}, false
	}
	dref, ok := o.ref(dst)
	if !ok || dref.Sign() <= 0 {
		return rangeWant{}, false
	}
	// lm y sign: log10 y signo del valor canónico de v en from; scale suma
	// las magnitudes de los términos de lm, para la tolerancia. Van en
	// big.Float porque αv puede no caber en float64 (1e300 YB) aunque el
	// resultado sí quepa.
	lm, scale := bigF(0), bigF(0)
	var sign float64
	if src, ok := o.special(from); ok {
		if !hasLog10Form(src.fn.Name) {
			return rangeWant{}, false
		}
		sref, ok := o.ref(src)
		if !ok || sref.Sign() <= 0 {
			return rangeWant{}, false
		}
		av := new(big.Rat).Mul(v, src.alpha)
		rl := bigF(ratLog10(sref))
		switch {
		case src.fn.Name == "sqrt" && av.Sign() == 0:
			sign = 0
		case src.fn.Name == "sqrt":
			// §48: log10 (αv)² = 2·log10 |αv|.
			inv := bigF(2 * ratLog10(new(big.Rat).Abs(av)))
			lm.Add(inv, rl)
			scale.Add(new(big.Float).Abs(inv), new(big.Float).Abs(rl))
			sign = 1
		default:
			// specInverseLog10 es lineal: log10 f⁻¹(a) = a·k.
			inv := new(big.Float).SetPrec(200).SetRat(av)
			inv.Mul(inv, bigF(specInverseLog10[src.fn.Name](1)))
			lm.Add(inv, rl)
			scale.Add(new(big.Float).Abs(inv), new(big.Float).Abs(rl))
			sign = 1
		}
	} else {
		if o.x.anyAtom(from, func(a string) bool { return o.x.special[a] || o.x.arbitrary[a] }) {
			return rangeWant{}, false
		}
		f, ok := o.factor(from, o.x.parse)
		if !ok {
			return rangeWant{}, false
		}
		m := f.Mul(f, v)
		sign = float64(m.Sign())
		if sign != 0 {
			lm = bigF(ratLog10(new(big.Rat).Abs(m)))
			scale = new(big.Float).Abs(lm)
		}
	}
	if dst.fn.Name == "sqrt" {
		// §48: f(x) = √x; un negativo no tiene imagen y √0 = 0.
		switch {
		case sign < 0:
			return rangeWant{kind: rangeOutside}, true
		case sign == 0:
			return rangeWant{kind: rangeFits}, true
		}
		lmf, _ := lm.Float64()
		alpha, _ := dst.alpha.Float64()
		l := (lmf-ratLog10(dref))/2 - math.Log10(alpha)
		return wantFromLog10(l, 1, log10Tol(l)), true
	}
	// Las logarítmicas: f solo existe para valores positivos.
	if sign <= 0 {
		return rangeWant{kind: rangeOutside}, true
	}
	// f(10^l) también es lineal en l: f(10^l) = l·g.
	rl := bigF(ratLog10(dref))
	g := bigF(specForwardOfLog10[dst.fn.Name](1))
	alpha := new(big.Float).SetPrec(200).SetRat(dst.alpha)
	q := new(big.Float).SetPrec(200).Sub(lm, rl)
	q.Mul(q, g).Quo(q, alpha)
	tolB := new(big.Float).SetPrec(200).Add(scale, new(big.Float).Abs(rl))
	tolB.Mul(tolB, new(big.Float).Abs(g)).Quo(tolB, alpha)
	tol, _ := tolB.Mul(tolB, bigF(0x1p-48)).Float64()
	// |q| - MaxFloat64, en big.Float: en float64, |q| ya sería +Inf.
	over := new(big.Float).Abs(q)
	over.Sub(over, new(big.Float).SetFloat64(math.MaxFloat64))
	d, _ := over.Float64()
	switch {
	case math.Abs(d) <= tol:
		return rangeWant{kind: rangeAtBorder}, true
	case d > 0:
		return rangeWant{kind: rangeOut}, true
	}
	r, _ := q.Float64()
	return rangeWant{kind: rangeFits, value: r, absTol: tol}, true
}

// bigF es v en un big.Float de 200 bits.
func bigF(v float64) *big.Float { return new(big.Float).SetPrec(200).SetFloat64(v) }

// accepts dice si un valor está dentro de la tolerancia del esperado.
func (w rangeWant) accepts(got float64) bool {
	if math.IsInf(got, 0) || math.IsNaN(got) {
		return false
	}
	return math.Abs(got-w.value) <= w.absTol+w.relTol*math.Abs(w.value)
}
