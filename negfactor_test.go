package ucum_test

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/gofhir/ucum/v4"
)

// Plan 015: un factor canónico negativo en la vía log10. UCUM §18 ■5 admite
// «a real number scalar» sin signo, así que una unidad puede valer -1 m, y §22
// ■4 (x = f⁻¹(α x')) da entonces un valor canónico negativo. Las funciones ln,
// lg y raíz de m/u existen solo si m y u tienen el mismo signo.
//
// Ningún esperado sale de la librería: salen de big.Rat (los valores de las
// unidades, escritos a mano) y de
// big.Float de 300 bits (exp y ln por serie).

const nfPrec = 300

// nfAdditions es lo que se agrega al XML embebido. Las unidades lineales llevan
// el valor de referencia de cada escala; las especiales usan las funciones del
// XML oficial (ln, lg, ld, lgTimes2, sqrt, pH, tanTimes100). Q es un prefijo
// propio de valor -1000.
func nfAdditions() string {
	var b strings.Builder
	lin := func(code, value, unit string) {
		fmt.Fprintf(&b, ` <unit Code="%s" CODE="%s" isMetric="no" class="dimless">
  <name>%s</name><printSymbol>%s</printSymbol><property>test</property>
  <value Unit="%s" UNIT="%s" value="%s">%s</value>
 </unit>
`, code, strings.ToUpper(code), code, code, unit, strings.ToUpper(unit), value, value)
	}
	fn := func(code, name, unit string) {
		fmt.Fprintf(&b, ` <unit Code="%s" CODE="%s" isMetric="no" isSpecial="yes" class="levels">
  <name>%s</name><printSymbol>%s</printSymbol><property>test</property>
  <value Unit="%s(%s %s)" UNIT="%s(%s %s)"><function name="%s" value="%s" Unit="%s"/></value>
 </unit>
`, code, strings.ToUpper(code), code, code, name, "1", unit,
			strings.ToUpper(name), "1", strings.ToUpper(unit), name, "1", unit)
	}
	b.WriteString(` <prefix Code="Q" CODE="QQ">
  <name>negative thousand</name><printSymbol>Q</printSymbol><value value="-1000">-1000</value>
 </prefix>
`)
	// X es 273.15/5²³ exacto (11 cifras): f(0)/α de XCel es -5²³, un punto medio
	// entre dos float64.
	b.WriteString(` <prefix Code="X" CODE="XX">
  <name>midpoint</name><printSymbol>X</printSymbol><value value="2.2913482752e-14">2.2913482752e-14</value>
 </prefix>
`)
	b.WriteString(` <prefix Code="J" CODE="JJ">
  <name>midpoint zero</name><printSymbol>J</printSymbol><value value="2.359296e-14">2.359296e-14</value>
 </prefix>
`)
	lin("[zero]", "0", "m")
	lin("[zK]", "0", "K")
	fn("[zln]", "ln", "[zero]")
	fn("[mln]", "ln", "m")
	fn("[zlnK]", "ln", "[zK]")
	lin("[zV]", "0", "V")
	lin("[zC]", "0", "mol/l")
	lin("[z1]", "0", "1")
	lin("[zrad]", "0", "rad")
	lin("[zsh]", "0", "m2/s4/Hz")
	fn("[zlg]", "lg", "[zero]")
	fn("[z2lg]", "lgTimes2", "[zV]")
	fn("[zpH]", "pH", "[zC]")
	fn("[zhpX]", "hpX", "[z1]")
	fn("[zlnr]", "ln", "[zrad]")
	fn("[zlnsh]", "ln", "[zsh]")
	lin("[tiny]", "1e-310", "rad")
	lin("[ntiny]", "-1", "[tiny]")
	lin("[tinyK]", "1e-310", "K")
	lin("[ntinyK]", "-1", "[tinyK]")
	lin("[neg]", "-1", "m")
	lin("[neg2]", "-2", "m")
	lin("[negW]", "-1", "W")
	lin("[negV]", "-1", "V")
	lin("[negM2]", "-1", "m2")
	lin("[negC]", "-1", "mol/l")
	lin("[nrad]", "-1", "rad")
	lin("[2rad]", "2", "rad")
	fn("[nlnB]", "ln", "[ntiny]")
	fn("[plnB]", "ln", "[tiny]")
	fn("[nlnK]", "ln", "[ntinyK]")
	fn("[nlg]", "lg", "[neg]")
	fn("[nlg2]", "lg", "[neg2]")
	fn("[plg]", "lg", "m")
	fn("[nBW]", "lg", "[negW]")
	fn("[nBV]", "lgTimes2", "[negV]")
	fn("[nld]", "ld", "[neg]")
	fn("[nsq]", "sqrt", "[negM2]")
	fn("[npH]", "pH", "[negC]")
	fn("[ntan]", "tanTimes100", "[nrad]")
	fn("[2tan]", "tanTimes100", "[2rad]")
	return b.String()
}

type nfModel struct {
	svc   ucum.Service
	exact ucum.ExactService
	err   error
}

var (
	nfOnce sync.Once
	nfM    nfModel
)

// nfLoad arma el servicio una vez para todos los tests del archivo.
func nfLoad(t testing.TB) *nfModel {
	t.Helper()
	nfOnce.Do(func() {
		raw, err := os.ReadFile("internal/essence/ucum-essence.xml")
		if err != nil {
			nfM.err = err
			return
		}
		xml := strings.Replace(string(raw), "</root>", nfAdditions()+"</root>", 1)
		svc, err := ucum.NewFromReader(strings.NewReader(xml))
		if err != nil {
			nfM.err = err
			return
		}
		nfM.svc = svc
		nfM.exact = svc.(ucum.ExactService)
	})
	if nfM.err != nil {
		t.Fatal(nfM.err)
	}
	return &nfM
}

func nfF() *big.Float { return new(big.Float).SetPrec(nfPrec) }

var (
	nfLn10 = bigLn(nfF().SetInt64(10))
	nfLn2  = bigLn(nfF().SetInt64(2))
)

// bigExp es e^x en big.Float: e^x = (e^(x/2^12))^(2^12), con la serie de Taylor
// en el argumento reducido (|x/2^12| < 1), con bits de sobra para los 12
// cuadrados.
func bigExp(x *big.Float) *big.Float {
	const s = 12
	const prec = nfPrec + 64
	r := new(big.Float).SetPrec(prec).Quo(x, new(big.Float).SetPrec(prec).SetInt64(1<<s))
	sum := new(big.Float).SetPrec(prec).SetInt64(1)
	term := new(big.Float).SetPrec(prec).SetInt64(1)
	eps := new(big.Float).SetPrec(prec).SetMantExp(big.NewFloat(1), -prec-8)
	for n := int64(1); n < 2000; n++ {
		term.Mul(term, r)
		term.Quo(term, new(big.Float).SetPrec(prec).SetInt64(n))
		sum.Add(sum, term)
		if new(big.Float).Abs(term).Cmp(eps) < 0 {
			break
		}
	}
	for i := 0; i < s; i++ {
		sum.Mul(sum, sum)
	}
	return sum
}

// nfPow es base^e para una base entera y un e racional o float.
func nfPow(lnBase, e *big.Float) *big.Float { return bigExp(nfF().Mul(e, lnBase)) }

func nfRatF(r *big.Rat) *big.Float { return nfF().SetRat(r) }

func nfFlt(f float64) *big.Float { return nfF().SetFloat64(f) }

func nfLog10(x *big.Float) *big.Float { return nfF().Quo(bigLn(x), nfLn10) }

func nfToF64(x *big.Float) float64 { f, _ := x.Float64(); return f }

// nfBound es la cota del error de la vía log10, escrita por caso (plan §4.8): la
// vía suma log10 |x| y log10 |factor| y materializa 10^l una vez, y fromLog10
// documenta que pierde unas |l|·2^-52·ln 10 cifras relativas. La cota es 4x eso
// (los dos términos del log10 y la escala final, ln 10 o log2 10, que multiplica
// el error). Nunca se afloja para que un test pase: el signo se exige exacto
// antes de mirar la cota.
func nfBound(l float64) float64 { return 4 * (math.Abs(l) + 1) * 0x1p-52 * math.Ln10 }

func nfUlp(w float64) float64 { return math.Nextafter(math.Abs(w), math.Inf(1)) - math.Abs(w) }

// nfCheck compara got con want: el signo exacto primero, y después la cota
// absoluta (un destino logarítmico) o relativa (una materialización 10^l) con
// l el log10 del intermedio.
func nfCheck(t *testing.T, what string, got, want, l float64, rel bool) {
	t.Helper()
	if want != 0 && math.Signbit(got) != math.Signbit(want) {
		t.Errorf("%s = %v, want %v: wrong sign", what, got, want)
		return
	}
	tol := nfBound(l) + 2*nfUlp(want)
	if rel {
		tol = math.Abs(want)*nfBound(l) + 2*nfUlp(want)
	}
	if math.Abs(got-want) > tol {
		t.Errorf("%s = %v, want %v (tolerance %.3g from l = %.4g)", what, got, want, tol, l)
	}
}

func nfWantDomain(t *testing.T, what string, got float64, err error) {
	t.Helper()
	if !errors.Is(err, ucum.ErrOutOfDomain) {
		t.Errorf("%s = (%v, %v), want ErrOutOfDomain", what, got, err)
	} else if errors.Is(err, ucum.ErrOutOfRange) {
		t.Errorf("%s: %v also wraps ErrOutOfRange", what, err)
	}
}

// nfUnitValue es el valor canónico de 1 de cada unidad lineal de la rejilla,
// escrito a mano desde el XML y las adiciones de este archivo (la base es el
// gramo: 1 V = 1000 g.m2/(s3.A) y 1 mol/l = 1000 mol/m3).
var nfUnitValue = map[string]string{
	"rad": "1", "mrad": "1/1000", "m": "1", "km": "1000", "m2": "1", "mm2": "1/1000000",
	"mol/l": "1000", "mmol/l": "1", "V": "1000", "mV": "1",
	"[tiny]": "1e-310", "[ntiny]": "-1e-310", "[neg]": "-1", "[negM2]": "-1",
	"[negC]": "-1000", "[negV]": "-1000",
}

// nfCan es el valor canónico exacto de v en una unidad lineal, o de 1 de ella.
func nfCan(t testing.TB, v *big.Rat, unit string) *big.Rat {
	t.Helper()
	u, ok := nfUnitValue[unit]
	if !ok {
		t.Fatalf("no canonical value written for %s", unit)
	}
	r, ok := new(big.Rat).SetString(u)
	if !ok {
		t.Fatalf("bad literal %q", u)
	}
	return r.Mul(r, v)
}

// nfRatio es m/u: el valor v de la unidad lineal dividido por la referencia ref.
func nfRatio(t testing.TB, v float64, unit, ref string) *big.Rat {
	t.Helper()
	one := big.NewRat(1, 1)
	r := nfCan(t, new(big.Rat).SetFloat64(v), unit)
	return r.Quo(r, nfCan(t, one, ref))
}

// nfScale describe una escala especial de prueba: f(m/u)/α (hacia la escala) y
// m/u en función del valor (desde ella).
type nfScale struct {
	code  string
	ref   string   // la unidad lineal que guarda la referencia u
	units []string // unidades lineales de la dimensión de la referencia
	neg   bool     // la referencia es negativa
	rel   bool     // la tolerancia hacia la escala es relativa (raíz)
	to    func(ratio *big.Float) *big.Float
	from  func(v float64) *big.Float
	vals  []float64 // valores de la escala
}

var nfScales = []nfScale{
	{
		code: "[nlnB]", ref: "[ntiny]", units: []string{"rad", "mrad"}, neg: true,
		to:   bigLn,
		from: func(v float64) *big.Float { return bigExp(nfFlt(v)) },
		vals: []float64{-710, -3, 0, 0.5, 100, 700, 705, 709.7, 709.78, 709.79, 710, 712, 714, 714.25, 714.3},
	},
	{
		code: "[nlg]", ref: "[neg]", units: []string{"m", "km"}, neg: true,
		to:   nfLog10,
		from: func(v float64) *big.Float { return nfPow(nfLn10, nfFlt(v)) },
		vals: []float64{-323, -320, -310.5, -309, -308, -307.65, -300, 0, 2.5, 100, 300},
	},
	{
		code: "[nld]", ref: "[neg]", units: []string{"m", "km"}, neg: true,
		to:   func(x *big.Float) *big.Float { return nfF().Quo(bigLn(x), nfLn2) },
		from: func(v float64) *big.Float { return nfPow(nfLn2, nfFlt(v)) },
		vals: []float64{-1070, -1060, -1030, -1022, -1000, -500, 0, 10, 1000, 1020},
	},
	{
		code: "[nsq]", ref: "[negM2]", units: []string{"m2", "mm2"}, neg: true, rel: true,
		to:   func(x *big.Float) *big.Float { return nfF().Sqrt(x) },
		from: func(v float64) *big.Float { return nfF().Mul(nfFlt(v), nfFlt(v)) },
		vals: []float64{0, 1, 3, 1e-162, 1e-160, 1e-155, 1e-154, 1e100, 1e150},
	},
	{
		code: "[npH]", ref: "[negC]", units: []string{"mol/l", "mmol/l"}, neg: true,
		to:   func(x *big.Float) *big.Float { return nfF().Neg(nfLog10(x)) },
		from: func(v float64) *big.Float { return nfPow(nfLn10, nfF().Neg(nfFlt(v))) },
		vals: []float64{-3, 0, 7.4, 300, 310.5, 320, 330},
	},
	{
		code: "[nBV]", ref: "[negV]", units: []string{"V", "mV"}, neg: true,
		to:   func(x *big.Float) *big.Float { return nfF().Mul(nfLog10(x), nfFlt(2)) },
		from: func(v float64) *big.Float { return nfPow(nfLn10, nfF().Quo(nfFlt(v), nfFlt(2))) },
		vals: []float64{-620, -0.3, 1, 7, 600},
	},
	// Con la referencia positiva (el XML oficial y sus gemelas): no cambia.
	{
		code: "[plnB]", ref: "[tiny]", units: []string{"rad", "mrad"},
		to:   bigLn,
		from: func(v float64) *big.Float { return bigExp(nfFlt(v)) },
		vals: []float64{-710, 0, 100, 700, 709.7, 709.8, 712, 714.25, 714.3},
	},
	{
		code: "[plg]", ref: "m", units: []string{"m", "km"},
		to:   nfLog10,
		from: func(v float64) *big.Float { return nfPow(nfLn10, nfFlt(v)) },
		vals: []float64{-323, -320, -310.5, -308, 0, 2.5, 300},
	},
	{
		code: "[pH]", ref: "mol/l", units: []string{"mol/l", "mmol/l"},
		to:   func(x *big.Float) *big.Float { return nfF().Neg(nfLog10(x)) },
		from: func(v float64) *big.Float { return nfPow(nfLn10, nfF().Neg(nfFlt(v))) },
		vals: []float64{-3, 0, 7.4, 310.5, 320, 330},
	},
	{
		code: "B[V]", ref: "V", units: []string{"V", "mV"},
		to:   func(x *big.Float) *big.Float { return nfF().Mul(nfLog10(x), nfFlt(2)) },
		from: func(v float64) *big.Float { return nfPow(nfLn10, nfF().Quo(nfFlt(v), nfFlt(2))) },
		vals: []float64{-620, -0.3, 1, 7, 600},
	},
}

// nfMags son las magnitudes del valor canónico de la rejilla hacia la escala:
// normales, en el borde, y subnormales (la rama x.v de log10Times).
var nfMags = []float64{1, 5, 1e-3, 1e-300, 1e-305, 1e-308, 1e-309, 1e-310, 1e-315, 1e-320, 5e-324, 1e300}

// nfLog10f es el log10 de un racional positivo, como float64.
func nfLog10f(r *big.Rat) float64 { return nfToF64(nfLog10(nfRatF(r))) }

// Tests 4.1 y 4.7: desde la escala hacia una unidad lineal, para ln (falla en la
// vía log10) y para lg, ld, raíz, pH y 2lg (guardas: pasan por la vía racional
// exacta) y sus gemelas de referencia positiva. La imagen es m = u·f⁻¹(α x')
// (§22 ■4). [nlnB] cruza el borde entre la vía directa y la log10 (e^v desborda
// sobre 709.78). La tolerancia es relativa, con l = log10 |m/u|.
func TestNegFromScale(t *testing.T) {
	m := nfLoad(t)
	for _, sc := range nfScales {
		for _, unit := range sc.units {
			factor := new(big.Rat).Quo(nfCan(t, big.NewRat(1, 1), sc.ref), nfCan(t, big.NewRat(1, 1), unit))
			for _, v := range sc.vals {
				g := sc.from(v)
				l := nfToF64(nfLog10(g))
				if v == 0 && sc.code != "[nsq]" {
					l = 0
				}
				wf := nfF().Mul(nfRatF(factor), g)
				if sc.code == "[nsq]" && v == 0 {
					wf = nfF()
				}
				want := nfToF64(wf)
				what := fmt.Sprintf("Convert(%v %s to %s)", v, sc.code, unit)
				got, err := m.svc.Convert(v, sc.code, unit)
				if math.IsInf(want, 0) || want == 0 && wf.Sign() != 0 {
					if !errors.Is(err, ucum.ErrOutOfRange) {
						t.Errorf("%s = (%v, %v), want ErrOutOfRange", what, got, err)
					}
					continue
				}
				if err != nil {
					t.Errorf("%s: %v, want %v", what, err, want)
					continue
				}
				nfCheck(t, what, got, want, l, true)
			}
		}
	}
}

// Test 4.1: la rejilla fina de [nlnB] entre 700 y 714.3, con signo y valor
// contra el oráculo y sin cambio de signo entre vecinos. El valor crece en
// magnitud con v: el canónico (negativo) decrece. Una inversión de monotonía es
// el defecto de 015.
func TestNegLnGridHasNoSignFlip(t *testing.T) {
	m := nfLoad(t)
	prev := math.Inf(1)
	prevV := 0.0
	for v := 700.0; v <= 714.3; v += 0.25 {
		got, err := m.svc.Convert(v, "[nlnB]", "rad")
		if err != nil {
			t.Fatalf("Convert(%v [nlnB] to rad): %v", v, err)
		}
		// m = -1e-310 rad · e^v.
		want := nfToF64(nfF().Mul(nfF().Neg(bigExp(nfFlt(v))), nfTiny()))
		nfCheck(t, fmt.Sprintf("Convert(%v [nlnB] to rad)", v), got, want, v/math.Ln10, true)
		if got >= prev {
			t.Errorf("Convert(%v [nlnB] to rad) = %v is not below Convert(%v) = %v: monotonicity broken", v, got, prevV, prev)
		}
		prev, prevV = got, v
	}
}

// nfTiny es 1e-310 exacto: el valor de [tiny] del XML propio.
func nfTiny() *big.Float {
	r, _ := new(big.Rat).SetString("1e-310")
	return nfRatF(r)
}

// Test 4.2: hacia la escala. Un valor canónico del mismo signo que la referencia
// da el valor del oráculo f(m/u)/α; uno de signo opuesto da ErrOutOfDomain (ln,
// lg, √ solo existen si m y u tienen el mismo signo), en la vía directa y en la
// log10, con la rejilla de subnormales (rama x.v) y los prefijos de unidad.
// La tolerancia es absoluta (un logaritmo) o relativa (la raíz), con
// l = log10 (m/u) del intermedio.
func TestNegToScale(t *testing.T) {
	m := nfLoad(t)
	for _, sc := range nfScales {
		for _, unit := range sc.units {
			for _, mag := range nfMags {
				for _, same := range []bool{true, false} {
					v := mag
					if sc.neg == same {
						v = -mag
					}
					what := fmt.Sprintf("Convert(%v %s to %s)", v, unit, sc.code)
					got, err := m.svc.Convert(v, unit, sc.code)
					if !same {
						nfWantDomain(t, what, got, err)
						continue
					}
					ratio := nfRatio(t, v, unit, sc.ref)
					if ratio.Sign() <= 0 {
						t.Fatalf("%s: premise: m/u = %v, want positive", what, ratio.RatString())
					}
					want := nfToF64(sc.to(nfRatF(ratio)))
					if err != nil {
						t.Errorf("%s: %v, want %v", what, err, want)
						continue
					}
					nfCheck(t, what, got, want, nfLog10f(ratio), sc.rel)
				}
			}
		}
	}
}

// Test 4.2: el caso del plan, -1 rad en [nlnB] es ln(1e310) = 713.8 (la vía
// log10 daba ErrOutOfDomain), y -1e-310 rad es ln 1 = 0 salvo el error de la
// vía log10 (plan §4.8): el valor exacto es 0 y la cota es nfBound(310), cuatro
// veces |l|·2^-52·ln10 con l = 310 (unos 6e-13); la librería da -3.1e-15.
func TestNegLnToScalePlanCases(t *testing.T) {
	m := nfLoad(t)
	got, err := m.svc.Convert(-1, "rad", "[nlnB]")
	want := nfToF64(nfF().Add(nfF().Mul(nfFlt(310), nfLn10), nfF()))
	if err != nil {
		t.Fatalf("Convert(-1 rad to [nlnB]) = (%v, %v), want %v", got, err, want)
	}
	nfCheck(t, "Convert(-1 rad to [nlnB])", got, want, 310, false)
	if math.Abs(got-713.8) > 0.1 {
		t.Errorf("Convert(-1 rad to [nlnB]) = %v, want about 713.8", got)
	}

	got, err = m.svc.Convert(-1e-310, "rad", "[nlnB]")
	if err != nil {
		t.Fatalf("Convert(-1e-310 rad to [nlnB]) = (%v, %v), want 0", got, err)
	}
	if math.Abs(got) > nfBound(310) {
		t.Errorf("Convert(-1e-310 rad to [nlnB]) = %v, want 0 within %.3g", got, nfBound(310))
	}
}

// Test 4.3: LogToLog (de una escala logarítmica a otra, sin pasar por la
// referencia). Con referencias de signo opuesto el logaritmo del destino no
// existe: ErrOutOfDomain, en los dos sentidos. Los pares son comparables.
func TestNegLogToLogOppositeSignIsOutOfDomain(t *testing.T) {
	m := nfLoad(t)
	pairs := [][2]string{
		{"[nBW]", "B[W]"}, {"[nBW]", "dB[W]"}, {"B[W]", "[nBW]"}, {"dB[W]", "[nBW]"},
		{"[nBV]", "B[V]"}, {"B[V]", "[nBV]"},
		{"[npH]", "[pH]"}, {"[pH]", "[npH]"},
		{"[nlg]", "[plg]"}, {"[plg]", "[nlg]"}, {"[plg]", "[nlg2]"}, {"[nlg2]", "[plg]"},
		{"[nlnB]", "[plnB]"}, {"[plnB]", "[nlnB]"},
	}
	for _, p := range pairs {
		for _, v := range []float64{1, 2.5, -3, 0.1, 0} {
			got, err := m.svc.Convert(v, p[0], p[1])
			nfWantDomain(t, fmt.Sprintf("Convert(%v %s to %s)", v, p[0], p[1]), got, err)
		}
	}
}

// Test 4.3: con los signos cancelados hay valor. La afinidad es
// r = v·(k_s/k_d) + l/k_d con l = log10 de la razón de las referencias (positiva):
//   - [nBW] a [nBW]: v;
//   - [nlg] a [nlg2]: m = -10^v m, y m/u2 = 10^v/2, así que v - lg 2;
//   - [nlg] a [nld]: m/u = 10^v, ld = v·log2 10.
//
// La tolerancia es la de la afinidad en float64 de special.LogToLog (la pendiente
// log10 2 y el desplazamiento l entran como float64): nfBound(1) con l = 1.
func TestNegLogToLogSameSignHasValue(t *testing.T) {
	m := nfLoad(t)
	lg2 := nfLog10(nfF().SetInt64(2))
	log2of10 := nfF().Quo(nfLn10, nfLn2)
	for _, v := range []float64{1, 2.5, -3, 0.1, 0, 40.25} {
		for _, tc := range []struct {
			from, to string
			want     *big.Float
			exact    bool
		}{
			{"[nBW]", "[nBW]", nfFlt(v), true},
			{"[nlg]", "[nlg]", nfFlt(v), true},
			{"[nlg]", "[nlg2]", nfF().Sub(nfFlt(v), lg2), false},
			{"[nlg2]", "[nlg]", nfF().Add(nfFlt(v), lg2), false},
			{"[nlg]", "[nld]", nfF().Mul(nfFlt(v), log2of10), false},
		} {
			what := fmt.Sprintf("Convert(%v %s to %s)", v, tc.from, tc.to)
			got, err := m.svc.Convert(v, tc.from, tc.to)
			if err != nil {
				t.Errorf("%s: %v, want a value", what, err)
				continue
			}
			want := nfToF64(tc.want)
			if tc.exact && got != want {
				t.Errorf("%s = %v, want exactly %v", what, got, want)
			} else if !tc.exact {
				nfCheck(t, what, got, want, 1+math.Abs(v), false)
			}
		}
	}
}

// Test 4.7, guarda de LogToLog: con referencias positivas ningún resultado
// cambia. Aquí dB[W] es 0.1 B[W] (prefijo d): 10 por el valor; B[kW] resta 3.
func TestNegLogToLogPositiveUnchanged(t *testing.T) {
	m := nfLoad(t)
	for _, v := range []float64{1, 2.5, -3, 0.1, 0} {
		for _, tc := range []struct {
			from, to string
			want     *big.Rat
		}{
			{"B[W]", "dB[W]", new(big.Rat).Mul(new(big.Rat).SetFloat64(v), big.NewRat(10, 1))},
			{"B[W]", "B[kW]", new(big.Rat).Sub(new(big.Rat).SetFloat64(v), big.NewRat(3, 1))},
			{"[plg]", "[plg]", new(big.Rat).SetFloat64(v)},
			{"[plnB]", "[plnB]", new(big.Rat).SetFloat64(v)},
			{"[pH]", "[pH]", new(big.Rat).SetFloat64(v)},
		} {
			want, _ := tc.want.Float64()
			got, err := m.svc.Convert(v, tc.from, tc.to)
			if err != nil || got != want {
				t.Errorf("Convert(%v %s to %s) = (%v, %v), want %v", v, tc.from, tc.to, got, err, want)
			}
		}
	}
}

// Test 4.4: Canonical, Multiply y Divide llevan el signo del factor. El esperado
// del canónico de v [nlnB] es -1e-310·e^v rad (§22 ■4), a la cota relativa de la
// vía log10 con l = v/ln10; Multiply por k y Divide de k por el canónico, lo
// mismo con el multiplicando y el cociente exactos.
func TestNegCanonicalMultiplyDivide(t *testing.T) {
	m := nfLoad(t)
	for _, v := range []float64{0, 700, 709.7, 709.8, 712, 714.25, 714.3} {
		what := fmt.Sprintf("%v [nlnB]", v)
		canon := nfF().Mul(nfF().Neg(bigExp(nfFlt(v))), nfTiny())
		l := v / math.Ln10
		wantC := nfToF64(canon)

		p, err := m.svc.Canonical(v, "[nlnB]")
		if err != nil || p.Code != "rad" {
			t.Errorf("Canonical(%s) = (%v, %v), want rad", what, p, err)
		} else {
			nfCheck(t, "Canonical("+what+")", p.Value, wantC, l, true)
		}

		x := ucum.Pair{Value: v, Code: "[nlnB]"}
		for _, k := range []float64{1, 3, -2, 0.25} {
			one := ucum.Pair{Value: k, Code: "1"}
			got, err := m.svc.Multiply(x, one)
			if err != nil {
				t.Errorf("Multiply(%s, %v): %v", what, k, err)
			} else {
				nfCheck(t, fmt.Sprintf("Multiply(%s, %v)", what, k), got.Value, nfToF64(nfF().Mul(canon, nfFlt(k))), l, true)
			}
			got, err = m.svc.Divide(x, one)
			if err != nil {
				t.Errorf("Divide(%s, %v): %v", what, k, err)
			} else {
				nfCheck(t, fmt.Sprintf("Divide(%s, %v)", what, k), got.Value, nfToF64(nfF().Quo(canon, nfFlt(k))), l, true)
			}
			got, err = m.svc.Divide(one, x)
			if wf := nfF().Quo(nfFlt(k), canon); math.IsInf(nfToF64(wf), 0) {
				// k/(-1e-310·e^v) pasa de float64 con v pequeño: es de rango.
				if !errors.Is(err, ucum.ErrOutOfRange) {
					t.Errorf("Divide(%v, %s) = (%v, %v), want ErrOutOfRange", k, what, got, err)
				}
			} else if err != nil {
				t.Errorf("Divide(%v, %s): %v", k, what, err)
			} else {
				nfCheck(t, fmt.Sprintf("Divide(%v, %s)", k, what), got.Value, nfToF64(nfF().Quo(nfFlt(k), canon)), l, true)
			}
		}
		// Con una unidad: -1.566 rad · 3 m = -4.7 rad.m.
		got, err := m.svc.Multiply(x, ucum.Pair{Value: 3, Code: "m"})
		if err != nil {
			t.Errorf("Multiply(%s, 3 m): %v", what, err)
		} else {
			nfCheck(t, "Multiply("+what+", 3 m)", got.Value, nfToF64(nfF().Mul(canon, nfFlt(3))), l, true)
		}
	}
}

// Test 4.4, la raíz con un valor subnormal: 1e-160 [nsq] es -1e-320 m2; el
// doble es -2e-320 (exacto, redondeado una vez). Los esperados salen
// del cuadrado exacto en big.Rat.
func TestNegSqrtSubnormalCanonicalMultiply(t *testing.T) {
	m := nfLoad(t)
	for _, v := range []float64{1e-160, 1e-155, 3, 1e-161} {
		sq := new(big.Rat).SetFloat64(v)
		sq.Mul(sq, sq)
		sq.Neg(sq) // u = -1 m2
		wantC, _ := new(big.Rat).Set(sq).Float64()
		p, err := m.svc.Canonical(v, "[nsq]")
		if err != nil || p.Value != wantC {
			t.Errorf("Canonical(%v [nsq]) = (%v, %v), want %v m2", v, p, err, wantC)
		}
		prod := new(big.Rat).Mul(sq, big.NewRat(2, 1))
		wantM, _ := prod.Float64()
		got, err := m.svc.Multiply(ucum.Pair{Value: v, Code: "[nsq]"}, ucum.Pair{Value: 2, Code: "1"})
		if err != nil || got.Value != wantM {
			t.Errorf("Multiply(%v [nsq], 2) = (%v, %v), want %v", v, got, err, wantM)
		}
	}
}

// Test 4.6: hacia la temperatura desde una escala ln de referencia negativa, la
// vía exacta de 012 con signo. [nlnK] = ln(-1e-310 K): m = -1e-310·e^v K y
// Cel = m - 273.15 (UCUM §21-22), redondeado una vez. En v = -400 el
// intermedio es -1e-484 K: -273.15 Cel sin cambio de lado; en 714.25, -1.566 K
// da -274.716 Cel y no +1.566 - 273.15.
func TestNegLnToTemperature(t *testing.T) {
	m := nfLoad(t)
	off, _ := new(big.Rat).SetString("27315/100")
	for _, v := range []float64{-400, -100, 0, 700, 709.7, 710, 714.25, 714.3} {
		mK := nfF().Mul(nfF().Neg(bigExp(nfFlt(v))), nfTiny())
		l := v / math.Ln10

		gotK, err := m.svc.Convert(v, "[nlnK]", "K")
		if wantK := nfToF64(mK); wantK == 0 {
			// -1e-310·e^-400 K redondea a cero: es de rango, no un valor.
			if !errors.Is(err, ucum.ErrOutOfRange) {
				t.Errorf("Convert(%v [nlnK] to K) = (%v, %v), want ErrOutOfRange", v, gotK, err)
			}
		} else if err != nil {
			t.Errorf("Convert(%v [nlnK] to K): %v", v, err)
		} else {
			nfCheck(t, fmt.Sprintf("Convert(%v [nlnK] to K)", v), gotK, wantK, l, true)
		}

		wantC := nfToF64(nfF().Sub(mK, nfRatF(off)))
		gotC, err := m.svc.Convert(v, "[nlnK]", "Cel")
		if err != nil {
			t.Errorf("Convert(%v [nlnK] to Cel): %v", v, err)
			continue
		}
		// El resultado es -273.15 + m con |m| hasta 1.6: la cota relativa de la
		// materialización de m (nfBound(l)·|m|) más 2 ulp del resultado.
		tol := math.Abs(nfToF64(mK))*nfBound(l) + 2*nfUlp(wantC)
		if math.Signbit(gotC) != math.Signbit(wantC) || math.Abs(gotC-wantC) > tol {
			t.Errorf("Convert(%v [nlnK] to Cel) = %v, want %v (tolerance %.3g)", v, gotC, wantC, tol)
		}
	}
}

// Test 4.7: lo que no cambia. El prefijo propio Q = -1000 ya funcionaba: 1 QB es
// -1000 B (el valor por el prefijo), -1 QCel son 1000 Cel, 1273.15 K.
func TestNegPrefixUnchanged(t *testing.T) {
	m := nfLoad(t)
	k, _ := new(big.Rat).SetString("127315/100")
	wantK, _ := k.Float64()
	for _, tc := range []struct {
		v        float64
		from, to string
		want     float64
	}{
		{1, "QB", "B", -1000},
		{-2, "QB", "B", 2000},
		{1, "QCel", "Cel", -1000},
		{-1, "QCel", "K", wantK},
		{1, "QCel", "K", -726.85},
	} {
		got, err := m.svc.Convert(tc.v, tc.from, tc.to)
		if err != nil || got != tc.want {
			t.Errorf("Convert(%v %s to %s) = (%v, %v), want %v", tc.v, tc.from, tc.to, got, err, tc.want)
		}
	}
}

// bigAtan es atan(x) por su serie, para |x| < 1.
func bigAtan(x *big.Float) *big.Float {
	x2 := nfF().Mul(x, x)
	term := nfF().Set(x)
	sum := nfF().Set(x)
	eps := nfF().SetMantExp(big.NewFloat(1), -nfPrec-8)
	for n := int64(3); n < 4000; n += 2 {
		term.Mul(term, x2)
		term.Neg(term)
		sum.Add(sum, nfF().Quo(term, nfF().SetInt64(n)))
		if nfF().Abs(term).Cmp(eps) < 0 {
			break
		}
	}
	return sum
}

// Test 4.7: la tangente ignora el valor de su referencia (plan §6, nativeUnit,
// ADR 0001). [ntan] = 100tan(-1 rad): 1 [ntan] da +atan(0.01) rad y no
// -atan(0.01); [2tan] = 100tan(2 rad) da lo mismo que [p'diop]. Hacia la escala,
// 1 rad da 100·tan 1 en las tres. Se documenta, no se arregla.
func TestNegTangentIgnoresItsReference(t *testing.T) {
	m := nfLoad(t)
	for _, v := range []float64{1, 10, 50, -20} {
		want := nfToF64(bigAtan(nfF().Quo(nfFlt(v), nfFlt(100))))
		for _, code := range []string{"[ntan]", "[2tan]", "[p'diop]"} {
			got, err := m.svc.Convert(v, code, "rad")
			if err != nil || ulps(got, want) > 2 {
				t.Errorf("Convert(%v %s to rad) = (%v, %v), want atan(%v/100) = %v", v, code, got, err, v, want)
			}
		}
	}
	wantT := bigTan100(big.NewRat(1, 1))
	for _, code := range []string{"[ntan]", "[2tan]", "[p'diop]"} {
		got, err := m.svc.Convert(1, "rad", code)
		if err != nil || ulps(got, wantT) > 2 {
			t.Errorf("Convert(1 rad to %s) = (%v, %v), want 100 tan 1 = %v", code, got, err, wantT)
		}
	}
}

// Ronda 1, hallazgo 1: el ε de toLinearScale conserva el signo del intermedio.
// Con el prefijo propio X (273.15/5²³) el origen de XCel cae justo en un punto
// medio entre dos float64: -5²³ es impar y mayor que 2⁵³. El intermedio de
// -800 [nlnK] es negativo (-1e-310·e^-800 K, sin float64), así que el valor
// exacto (ε - 273.15)/α es un poco menor que -5²³ y redondea hacia
// -5²³ - 1. Con el signo de ε perdido redondearía al otro lado.
func TestNegEpsilonKeepsItsSignAtAMidpoint(t *testing.T) {
	m := nfLoad(t)
	alpha, _ := new(big.Rat).SetString("2.2913482752e-14")
	off, _ := new(big.Rat).SetString("27315/100")
	mid := new(big.Int).Exp(big.NewInt(5), big.NewInt(23), nil)
	// Una cota del intermedio: |-1e-310·e^-800| < 1e-600, y solo importa su signo.
	tiny, _ := new(big.Rat).SetString("-1e-600")
	exact := new(big.Rat).Add(tiny, new(big.Rat).Neg(off))
	exact.Quo(exact, alpha)
	// El origen está en el punto medio exacto y ε lo empuja hacia -∞.
	if new(big.Rat).Quo(new(big.Rat).Neg(off), alpha).Cmp(new(big.Rat).SetInt(new(big.Int).Neg(mid))) != 0 {
		t.Fatal("premise: f(0)/alpha is not -5^23")
	}
	if exact.Cmp(new(big.Rat).SetInt(new(big.Int).Neg(mid))) >= 0 {
		t.Fatal("premise: the exact value is not below -5^23")
	}
	want, _ := exact.Float64()
	if want != -11920928955078126 {
		t.Fatalf("premise: nearest float64 = %v, want -11920928955078126", want)
	}
	for _, v := range []float64{-800, -1000, -2000} {
		got, err := m.svc.Convert(v, "[nlnK]", "XCel")
		if err != nil || got != want {
			t.Errorf("Convert(%v [nlnK] to XCel) = (%v, %v), want %v", v, got, err, want)
		}
	}
}

// Ronda 1, hallazgo 2: un origen que no es logarítmico con factor negativo
// hacia un destino logarítmico sí tiene valor (no pasa por LogToLog). Los
// esperados salen de big.Float: ln de |m|/|u| con m el canónico exacto.
func TestNegNonLogSourceIntoLogDestination(t *testing.T) {
	m := nfLoad(t)
	pi4 := nfF().Sub(nfF().Mul(nfFlt(4), bigAtan(nfF().Quo(nfFlt(1), nfFlt(5)))),
		bigAtan(nfF().Quo(nfFlt(1), nfFlt(239)))) // Machin: π/4
	k := func(s string) *big.Float { r, _ := new(big.Rat).SetString(s); return nfRatF(r) }
	for _, tc := range []struct {
		v        float64
		from, to string
		canon    *big.Float // |m| en la unidad base
		ref      *big.Float // |u|
		approx   float64
	}{
		{-300, "Cel", "[nlnK]", k("2685/100"), k("1e-310"), 717.09},
		{-1000, "Cel", "[nlnK]", k("72685/100"), k("1e-310"), 720.39},
		{-100, "[p'diop]", "[nlnB]", pi4, k("1e-310"), 713.56},
	} {
		what := fmt.Sprintf("Convert(%v %s to %s)", tc.v, tc.from, tc.to)
		want := nfToF64(bigLn(nfF().Quo(tc.canon, tc.ref)))
		got, err := m.svc.Convert(tc.v, tc.from, tc.to)
		if err != nil {
			t.Errorf("%s: %v, want %v", what, err, want)
			continue
		}
		nfCheck(t, what, got, want, 310, false)
		if math.Abs(got-tc.approx) > 0.01 {
			t.Errorf("%s = %v, want about %v", what, got, tc.approx)
		}
	}
}

// Ronda 1, hallazgo 3: con un factor cero (la unidad "0") los resultados
// cambiaron respecto de main, y los nuevos son los coherentes. 0 por cualquier
// cosa es 0, así que Multiply(-800 Np, 1 "0") es 0 de unidad 1 (antes
// ErrOutOfRange); el lg y el ln de 0 quedan fuera del dominio, como ya daba
// Convert(-800, "0", "Np"), así que 1e-310 y 1e300 "0" → Np son ErrOutOfDomain
// (antes ErrOutOfRange). Multiply(1e-200 m, 1e-200 "0") ya daba 0.
func TestNegZeroFactorIsCoherent(t *testing.T) {
	m := nfLoad(t)
	p, err := m.svc.Multiply(ucum.Pair{Value: -800, Code: "Np"}, ucum.Pair{Value: 1, Code: "0"})
	if err != nil || p.Value != 0 || p.Code != "1" {
		t.Errorf("Multiply(-800 Np, 1 0) = (%v, %v), want 0 of unit 1", p, err)
	}
	p, err = m.svc.Multiply(ucum.Pair{Value: 1e-200, Code: "m"}, ucum.Pair{Value: 1e-200, Code: "0"})
	if err != nil || p.Value != 0 || p.Code != "m" {
		t.Errorf("Multiply(1e-200 m, 1e-200 0) = (%v, %v), want 0 m", p, err)
	}
	for _, v := range []float64{-800, 1e-310, 1e300} {
		got, err := m.svc.Convert(v, "0", "Np")
		nfWantDomain(t, fmt.Sprintf("Convert(%v 0 to Np)", v), got, err)
	}
}

// Ronda 2, hallazgo 1: una referencia de valor 0 (canónico 0) no tiene
// logaritmo; entre dos escalas logarítmicas es ErrOutOfDomain, no un pánico.
func TestNegZeroReferenceLogToLogIsOutOfDomain(t *testing.T) {
	m := nfLoad(t)
	for _, tc := range []struct {
		v        float64
		from, to string
	}{
		{1, "[zln]", "[mln]"},
		{-800, "[zlnK]", "[nlnK]"},
	} {
		got, err := m.svc.Convert(tc.v, tc.from, tc.to)
		nfWantDomain(t, fmt.Sprintf("Convert(%v %s to %s)", tc.v, tc.from, tc.to), got, err)
	}
}

// Ronda 2, hallazgo 2: con un factor cero el intermedio es exactamente 0. El
// valor exacto es (0 - 273.15)/J, con J = 2.359296e-14 un punto medio de Cel,
// y se redondea una vez, al par.
func TestNegZeroFactorIntoTemperatureIsExact(t *testing.T) {
	m := nfLoad(t)
	alpha, _ := new(big.Rat).SetString("2.359296e-14")
	off, _ := new(big.Rat).SetString("27315/100")
	exact := new(big.Rat).Quo(new(big.Rat).Neg(off), alpha)
	want, _ := exact.Float64() // al más cercano, empate al par
	for _, v := range []float64{-800, -1000, 1} {
		got, err := m.svc.Convert(v, "[zlnK]", "JCel")
		if err != nil || got != want {
			t.Errorf("Convert(%v [zlnK] to JCel) = (%v, %v), want %v", v, got, err, want)
		}
	}
}

// Ronda 3, hallazgo 1: con una referencia de valor 0 el factor canónico es 0
// y el intermedio es exactamente 0 por cualquier vía (plan §2): el resultado no
// depende de si el valor cae en la vía directa o en la log10 (el borde de ln es
// 709). Hacia un destino lineal, de tangente y de raíz el resultado es 0.
func TestNegZeroReferenceGivesZeroOnEveryPath(t *testing.T) {
	m := nfLoad(t)
	for _, tc := range []struct{ from, to string }{
		{"[zln]", "m"}, {"[zlg]", "m"}, {"[z2lg]", "V"}, {"[zpH]", "mol/l"}, {"[zhpX]", "1"},
		{"[zlnK]", "K"}, {"[zlnr]", "[p'diop]"}, {"[zlnsh]", "[m/s2/Hz^(1/2)]"},
	} {
		for _, v := range []float64{1, -800, 800, 1e300} {
			got, err := m.svc.Convert(v, tc.from, tc.to)
			if err != nil || got != 0 {
				t.Errorf("Convert(%v %s to %s) = (%v, %v), want 0", v, tc.from, tc.to, got, err)
			}
			p, err := m.svc.Canonical(v, tc.from)
			if err != nil || p.Value != 0 {
				t.Errorf("Canonical(%v %s) = (%v, %v), want value 0", v, tc.from, p, err)
			}
		}
	}
}
