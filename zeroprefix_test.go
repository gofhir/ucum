package ucum_test

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofhir/ucum/v4"
	"github.com/gofhir/ucum/v4/internal/essence"
	"github.com/gofhir/ucum/v4/internal/model"
)

// Plan 016: un prefijo o una unidad de valor 0. UCUM §27 ■2 define el valor de
// un prefijo como «the scalar value by which the unit atom is multiplied» y
// §18 ■5 el de una unidad como «a real number scalar»: ninguno excluye el 0.
// Una unidad escalada especial es x = f⁻¹(α x') y su inversa x' = f(x)/α
// (§22 ■4): con α = 0 la primera vale f⁻¹(0) para todo x' y la segunda no
// existe, así que el destino con α = 0 es ErrDivisionByZero. Una potencia
// negativa de un valor 0 tampoco existe.
//
// El XML oficial no tiene ningún valor 0. Estos tests le agregan el prefijo O
// (valor 0), [zero] (0 m), [zcel] (cel(0 K)) y una copia métrica
// (isMetric="yes") de cada función especial que el XML nombra, para que cada
// handler tenga un destino con prefijo O.
//
// Ningún esperado sale de la librería: salen de §22 ■4 y de la aritmética
// exacta (OCel → K es f⁻¹(0) = 273.15 K). Un pánico es un fallo del test, no
// una caída del binario.
//
// El prefijo se llama O (Code) y ZZO (CODE, el vocabulario insensible a
// mayúsculas): "O" no es ningún prefijo del XML en ninguno de los dos, y
// ninguna unidad oficial se parte como O + átomo ("Ohm", "Oe" y "osm" son
// átomos propios); ZZO no es prefijo de ningún código (ZA y ZO son los
// prefijos zetta y zepto, y ninguno empieza por ZZ).
const zpPrefixCI = "ZZO"

const zpPrefixXML = ` <prefix Code="O" CODE="ZZO">
  <name>zero prefix</name><printSymbol>O</printSymbol><value value="0">0</value>
 </prefix>
 <unit Code="[zero]" CODE="[ZERO]" isMetric="yes" class="dimless">
  <name>zero</name><printSymbol>zero</printSymbol><property>test</property>
  <value Unit="m" UNIT="M" value="0">0</value>
 </unit>
 <unit Code="[zcel]" CODE="[ZCEL]" isMetric="no" isSpecial="yes" class="levels">
  <name>zcel</name><printSymbol>zcel</printSymbol><property>test</property>
  <value Unit="cel(0 K)" UNIT="CEL(0 K)"><function name="Cel" value="0" Unit="K"/></value>
 </unit>
`

// zpSpecial es una unidad especial: la oficial o una copia métrica.
type zpSpecial struct {
	code, ci string
	metric   bool
	ref      string // la unidad de la referencia, como la declara la función
	dim      string // la forma canónica de esa unidad, para agrupar por dimensión
}

type zpWorld struct {
	svc     ucum.Service
	exact   ucum.ExactService
	ci      ucum.Service
	m       *model.Model
	specs   []zpSpecial // todas las especiales: oficiales y copias
	err     error
	prefixd []zpSpecial // las métricas (destinos con prefijo O)
}

var (
	zpOnce sync.Once
	zpW    zpWorld
)

// zpCopies escribe una copia métrica por cada función distinta (nombre, valor
// y unidad) que nombra el XML oficial, leída del modelo, y devuelve su XML.
func zpCopies(off *model.Model) string {
	type key struct{ name, value, unit string }
	seen := map[key]bool{}
	var b strings.Builder
	n := 0
	for _, u := range off.DefinedUnits {
		if u.Value == nil || u.Value.Function == nil {
			continue
		}
		f := u.Value.Function
		k := key{f.Name, f.Value.String(), f.Unit}
		if seen[k] {
			continue
		}
		seen[k] = true
		code := fmt.Sprintf("[zf%d_%s]", n, f.Name)
		fmt.Fprintf(&b, ` <unit Code="%s" CODE="%s" isMetric="yes" isSpecial="yes" class="levels">
  <name>copy %d</name><printSymbol>zf</printSymbol><property>test</property>
  <value Unit="%s(%s %s)" UNIT="%s(%s %s)"><function name="%s" value="%s" Unit="%s"/></value>
 </unit>
`, code, strings.ToUpper(code), n, f.Name, k.value, f.Unit,
			strings.ToUpper(f.Name), k.value, strings.ToUpper(f.Unit), f.Name, k.value, f.Unit)
		n++
	}
	return b.String()
}

// zpLoad arma los tres servicios una vez para todos los tests del archivo.
func zpLoad(t testing.TB) *zpWorld {
	t.Helper()
	zpOnce.Do(func() {
		zpW.err = zpBuild(&zpW)
	})
	if zpW.err != nil {
		t.Fatal(zpW.err)
	}
	return &zpW
}

func zpBuild(w *zpWorld) error {
	raw, err := os.ReadFile("internal/essence/ucum-essence.xml")
	if err != nil {
		return err
	}
	off, err := essence.Load(nil)
	if err != nil {
		return err
	}
	copies := zpCopies(off)
	xml := strings.Replace(string(raw), "</root>", zpPrefixXML+copies+"</root>", 1)
	if w.svc, err = ucum.NewFromReader(strings.NewReader(xml)); err != nil {
		return err
	}
	ex, ok := w.svc.(ucum.ExactService)
	if !ok {
		return errors.New("service is not an ExactService")
	}
	w.exact = ex
	if w.ci, err = ucum.NewCaseInsensitiveFromReader(strings.NewReader(xml)); err != nil {
		return err
	}
	if w.m, err = essence.Load(strings.NewReader(xml)); err != nil {
		return err
	}
	var specs []zpSpecial
	for _, u := range w.m.DefinedUnits {
		if u.Value == nil || u.Value.Function == nil {
			continue
		}
		s := zpSpecial{code: u.Code, ci: u.CodeCI, metric: u.IsMetric, ref: u.Value.Function.Unit}
		if p, e := w.svc.Canonical(1, s.ref); e == nil {
			s.dim = p.Code
		} else {
			s.dim = s.ref
		}
		specs = append(specs, s)
	}
	w.specs = specs
	sort.Slice(w.specs, func(i, j int) bool { return w.specs[i].code < w.specs[j].code })
	for _, s := range w.specs {
		if s.metric {
			w.prefixd = append(w.prefixd, s)
		}
	}
	return nil
}

// zpRes es el resultado de una llamada protegida: un pánico queda en pan.
type zpRes struct {
	v   float64
	err error
	pan any
}

func zpGuard(f func() (float64, error)) (r zpRes) {
	defer func() {
		if p := recover(); p != nil {
			r.pan = p
		}
	}()
	r.v, r.err = f()
	return r
}

func zpConvert(s ucum.Service, v float64, from, to string) zpRes {
	return zpGuard(func() (float64, error) { return s.Convert(v, from, to) })
}

func zpConvertRat(s ucum.ExactService, v float64, from, to string) zpRes {
	return zpGuard(func() (float64, error) {
		r, err := s.ConvertRat(new(big.Rat).SetFloat64(v), from, to)
		if r == nil {
			return 0, err
		}
		f, _ := r.Float64()
		return f, err
	})
}

func zpCanonical(s ucum.Service, v float64, code string) zpRes {
	return zpGuard(func() (float64, error) {
		p, err := s.Canonical(v, code)
		return p.Value, err
	})
}

func zpCanonicalRat(s ucum.ExactService, code string) zpRes {
	return zpGuard(func() (float64, error) {
		p, err := s.CanonicalRat(big.NewRat(1, 1), code)
		if p.Value == nil {
			return 0, err
		}
		f, _ := p.Value.Float64()
		return f, err
	})
}

// zpTally junta los fallos de un barrido: un pánico por cada una de las miles
// de entradas inundaría la salida, así que se cuentan y se muestran las
// primeras.
type zpTally struct {
	n     int
	first []string
}

func (z *zpTally) add(format string, args ...any) {
	z.n++
	if len(z.first) < 4 {
		z.first = append(z.first, fmt.Sprintf(format, args...))
	}
}

func (z *zpTally) report(t *testing.T, what string) {
	t.Helper()
	if z.n > 0 {
		t.Errorf("%s: %d failures, first: %s", what, z.n, strings.Join(z.first, "; "))
	}
}

// zpWantZeroDiv exige ErrDivisionByZero y ningún pánico.
func zpWantZeroDiv(z *zpTally, what string, r zpRes) {
	switch {
	case r.pan != nil:
		z.add("%s panicked: %v", what, r.pan)
	case !errors.Is(r.err, ucum.ErrDivisionByZero):
		z.add("%s = (%v, %v), want ErrDivisionByZero", what, r.v, r.err)
	case errors.Is(r.err, ucum.ErrOutOfDomain):
		z.add("%s: %v also wraps ErrOutOfDomain", what, r.err)
	}
}

// zpWantFiniteOrErr exige no pánico y que un resultado sin error sea finito.
func zpWantFiniteOrErr(z *zpTally, what string, r zpRes) {
	switch {
	case r.pan != nil:
		z.add("%s panicked: %v", what, r.pan)
	case r.err == nil && (math.IsNaN(r.v) || math.IsInf(r.v, 0)):
		z.add("%s = %v with nil error", what, r.v)
	}
}

// Plan 016 §4.1: lo que hoy es un pánico da ErrDivisionByZero.
func TestZeroPrefixPanicsAreDivisionByZero(t *testing.T) {
	w := zpLoad(t)
	var z zpTally
	for _, c := range []struct{ from, to string }{
		{"K", "OCel"}, {"Np", "ONp"}, {"B", "OB"}, {"[hp'_X]", "ONp"},
		{"1", "ONp"}, {"%", "OB"}, {"%-1", "ONp"},
	} {
		zpWantZeroDiv(&z, fmt.Sprintf("Convert(1, %q, %q)", c.from, c.to), zpConvert(w.svc, 1, c.from, c.to))
		zpWantZeroDiv(&z, fmt.Sprintf("ConvertRat(1, %q, %q)", c.from, c.to), zpConvertRat(w.exact, 1, c.from, c.to))
	}
	// [zcel] = cel(0 K): el 0 multiplica por fuera de la función, así que vale
	// 0·f⁻¹(x') = 0 K para todo x'. Hacia OCel el destino sigue teniendo α = 0, y
	// como destino [zcel] tiene factor canónico 0: las dos son división por cero.
	zpWantZeroDiv(&z, `ConvertRat(1, "[zcel]", "OCel")`, zpConvertRat(w.exact, 1, "[zcel]", "OCel"))
	zpWantZeroDiv(&z, `Convert(1, "[zcel]", "OCel")`, zpConvert(w.svc, 1, "[zcel]", "OCel"))
	zpWantZeroDiv(&z, `Convert(1, "K", "[zcel]")`, zpConvert(w.svc, 1, "K", "[zcel]"))
	zpWantZeroDiv(&z, `ConvertRat(1, "K", "[zcel]")`, zpConvertRat(w.exact, 1, "K", "[zcel]"))
	// Una potencia negativa de un valor 0, en cada rama de canonicalizeSymbol:
	// base (OK-1, Om-1), arbitraria, adimensional y con valor.
	for _, code := range []string{
		"Om-1", "OK-1", "O[IU]-1", "Obit-1", "[zero]-1", "O[zero]-2", "/[zero]2", "m/Om2",
		"/Om", "Om-2", "O[zero]-1",
	} {
		zpWantZeroDiv(&z, fmt.Sprintf("Canonical(1, %q)", code), zpCanonical(w.svc, 1, code))
		zpWantZeroDiv(&z, fmt.Sprintf("CanonicalRat(1, %q)", code), zpCanonicalRat(w.exact, code))
		zpWantZeroDiv(&z, fmt.Sprintf("Convert(1, %q, %q)", code, "1"), zpConvert(w.svc, 1, code, "1"))
	}
	z.report(t, "panics became ErrDivisionByZero")
}

// zpOrigins devuelve los orígenes de un destino especial: su referencia, toda
// especial de la misma dimensión (la misma familia y otra logarítmica) y, si la
// referencia es adimensional, 1, % y 10*.
func zpOrigins(w *zpWorld, d zpSpecial, ci bool) []string {
	var out []string
	add := func(c string) { out = append(out, c) }
	if ci {
		if u := w.m.Lookup(d.ref); u != nil {
			add(u.CodeCI)
		}
	} else {
		add(d.ref)
	}
	for _, s := range w.specs {
		if s.dim != d.dim {
			continue
		}
		if ci {
			add(s.ci)
		} else {
			add(s.code)
		}
	}
	if d.dim == "1" {
		add("1")
		add("%")
		add("10*")
	}
	return out
}

var zpValues = []float64{0, 1, -1, 2.5, 1e300, 5e-324, math.Inf(1), math.Inf(-1), math.NaN()}

// Plan 016 §4.2: todo destino especial con prefijo O (las métricas del XML y
// las copias de cada función) es ErrDivisionByZero, antes que el dominio y
// antes que una entrada no finita (#55).
func TestZeroPrefixSpecialDestinations(t *testing.T) {
	w := zpLoad(t)
	if len(w.prefixd) < 20 {
		t.Fatalf("only %d metric special units: the copies were not generated", len(w.prefixd))
	}
	for _, ci := range []bool{false, true} {
		svc, exact, name := w.svc, w.exact, "case-sensitive"
		if ci {
			svc, name = w.ci, "case-insensitive"
			// El servicio insensible también es exacto: ConvertRat pasa por la
			// misma guarda de α.
			exact = w.ci.(ucum.ExactService)
		}
		var z zpTally
		for _, d := range w.prefixd {
			to := "O" + d.code
			if ci {
				to = zpPrefixCI + d.ci
			}
			for _, from := range zpOrigins(w, d, ci) {
				for _, v := range zpValues {
					zpWantZeroDiv(&z, fmt.Sprintf("Convert(%v, %q, %q)", v, from, to), zpConvert(svc, v, from, to))
					if !math.IsNaN(v) && !math.IsInf(v, 0) {
						zpWantZeroDiv(&z, fmt.Sprintf("ConvertRat(%v, %q, %q)", v, from, to), zpConvertRat(exact, v, from, to))
					}
				}
			}
		}
		z.report(t, name)
	}
}

// Plan 016 §4.3: lo que no cambia. Un origen con α = 0 existe en todo handler
// (f⁻¹(0): Cel 273.15, ln 1) y una unidad lineal de valor 0 sigue valiendo 0.
func TestZeroPrefixUnchanged(t *testing.T) {
	w := zpLoad(t)
	near := func(what string, r zpRes, want float64) {
		t.Helper()
		if r.pan != nil || r.err != nil || math.Abs(r.v-want) > 1e-12*math.Max(1, math.Abs(want)) {
			t.Errorf("%s = (%v, %v, panic %v), want %v", what, r.v, r.err, r.pan, want)
		}
	}
	for _, v := range []float64{1, -3, 2.5} {
		// x = f⁻¹(α x') con α = 0 no depende de x': f⁻¹(0) = 273.15 K.
		near(fmt.Sprintf("Convert(%v, OCel, K)", v), zpConvert(w.svc, v, "OCel", "K"), 273.15)
		near(fmt.Sprintf("ConvertRat(%v, OCel, K)", v), zpConvertRat(w.exact, v, "OCel", "K"), 273.15)
		// ln: x = e^0 = 1 de la referencia, que en Np es ln(1) = 0.
		near(fmt.Sprintf("Convert(%v, ONp, Np)", v), zpConvert(w.svc, v, "ONp", "Np"), 0)
		near(fmt.Sprintf("Convert(%v, Om, m)", v), zpConvert(w.svc, v, "Om", "m"), 0)
		near(fmt.Sprintf("ConvertRat(%v, Om, m)", v), zpConvertRat(w.exact, v, "Om", "m"), 0)
		near(fmt.Sprintf("Canonical(%v, Om)", v), zpCanonical(w.svc, v, "Om"), 0)
	}
	if r, err := w.exact.ConvertRat(big.NewRat(1, 1), "OCel", "K"); err != nil || r.Cmp(big.NewRat(27315, 100)) != 0 {
		t.Errorf("ConvertRat(1, OCel, K) = (%v, %v), want exactly 27315/100", r, err)
	}
	var z zpTally
	for _, c := range [][2]string{{"m", "Om"}, {"m", "O[zero]"}, {"[zero]", "Om"}} {
		zpWantZeroDiv(&z, fmt.Sprintf("Convert(1, %q, %q)", c[0], c[1]), zpConvert(w.svc, 1, c[0], c[1]))
		zpWantZeroDiv(&z, fmt.Sprintf("ConvertRat(1, %q, %q)", c[0], c[1]), zpConvertRat(w.exact, 1, c[0], c[1]))
		zpWantZeroDiv(&z, fmt.Sprintf("ConversionFactor(%q, %q)", c[0], c[1]), zpGuard(func() (float64, error) {
			_, err := w.exact.ConversionFactor(c[0], c[1])
			return 0, err
		}))
	}
	zpWantZeroDiv(&z, `Canonical(1, "/Om")`, zpCanonical(w.svc, 1, "/Om"))
	zpWantZeroDiv(&z, "Divide(1 Om, 2 Om)", zpDivide(w.svc, 1, "Om", 2, "Om"))
	zpWantZeroDiv(&z, "Divide(1 m, 2 Om)", zpDivide(w.svc, 1, "m", 2, "Om"))
	z.report(t, "unchanged errors")
}

func zpDivide(s ucum.Service, a float64, ac string, b float64, bc string) zpRes {
	return zpGuard(func() (float64, error) {
		p, err := s.Divide(ucum.Pair{Value: a, Code: ac}, ucum.Pair{Value: b, Code: bc})
		return p.Value, err
	})
}

// zpProd es el resultado de Multiply o Divide: el par, el error y un pánico.
type zpProd struct {
	p   ucum.Pair
	err error
	pan any
}

func zpProduct(s ucum.Service, divide bool, a float64, ac string, b float64, bc string) (r zpProd) {
	defer func() { r.pan = recover() }()
	x, y := ucum.Pair{Value: a, Code: ac}, ucum.Pair{Value: b, Code: bc}
	if divide {
		r.p, r.err = s.Divide(x, y)
	} else {
		r.p, r.err = s.Multiply(x, y)
	}
	return r
}

// Plan 016 §4.4: Multiply y Divide con un factor 0 dan un valor. El resultado
// es 0 en la unidad que corresponde a Om·m-2 = 0 m-1 y Om/m2 = 0 /m; la unidad
// se verifica por su equivalencia con /m (1 de ella vale 1 /m), no por su
// escritura.
func TestZeroPrefixMultiplyDivide(t *testing.T) {
	w := zpLoad(t)
	for _, c := range []struct {
		name   string
		divide bool
		ac     string
		b      float64
		bc     string
		want   float64
		unit   string
	}{
		{"Multiply(1 Om, 2 m-2)", false, "Om", 2, "m-2", 0, "/m"},
		{"Divide(1 Om, 2 m2)", true, "Om", 2, "m2", 0, "/m"},
		{"Multiply(1 Om, 2 m)", false, "Om", 2, "m", 0, "m2"},
		{"Divide(1 Om, 2 m)", true, "Om", 2, "m", 0, "1"},
	} {
		got := zpProduct(w.svc, c.divide, 1, c.ac, c.b, c.bc)
		if got.pan != nil || got.err != nil || got.p.Value != c.want {
			t.Errorf("%s = (%v, %v, panic %v), want value %v", c.name, got.p, got.err, got.pan, c.want)
			continue
		}
		if r := zpConvert(w.svc, 1, got.p.Code, c.unit); r.pan != nil || r.err != nil || r.v != 1 {
			t.Errorf("%s: unit %q is not equivalent to %q: (%v, %v)", c.name, got.p.Code, c.unit, r.v, r.err)
		}
	}
	// Om0 es (Om)^0: el exponente 0 no invierte nada y el cociente existe.
	got := zpProduct(w.svc, true, 1, "Om0", 2, "m")
	if got.pan != nil || got.err != nil || math.IsNaN(got.p.Value) || math.IsInf(got.p.Value, 0) {
		t.Errorf("Divide(1 Om0, 2 m) = (%v, %v, panic %v), want a finite value", got.p, got.err, got.pan)
	}
}

// zpSweepCodes elige un subconjunto por clase: las unidades base,
// las cuatro primeras de cada clase del XML y las que se agregan.
func zpSweepCodes(w *zpWorld) (cs, ci []string) {
	perClass := map[string]int{}
	pick := func(code, codeCI string) {
		cs = append(cs, code)
		ci = append(ci, codeCI)
	}
	for _, b := range w.m.BaseUnits {
		pick(b.Code, b.CodeCI)
	}
	for _, u := range w.m.DefinedUnits {
		if perClass[u.Class] >= 4 || strings.HasPrefix(u.Code, "[zf") {
			continue
		}
		perClass[u.Class]++
		pick(u.Code, u.CodeCI)
	}
	for _, s := range w.specs {
		if s.metric {
			pick(s.code, s.ci)
		}
	}
	return cs, ci
}

// zpSweepForms arma, para cada código, sus formas con y sin el prefijo, con
// exponentes -2…2, el inverso y el cociente con el cuadrado.
func zpSweepForms(codes []string, prefix string) []string {
	var forms []string
	for _, c := range codes {
		for _, p := range []string{"", prefix} {
			for e := -2; e <= 2; e++ {
				if e == 1 {
					forms = append(forms, p+c)
				} else {
					forms = append(forms, fmt.Sprintf("%s%s%d", p, c, e))
				}
			}
			forms = append(forms, "/"+p+c, "m/"+p+c+"2")
		}
	}
	return forms
}

// Plan 016 §4.5: un barrido acotado de las unidades del XML con y sin O,
// exponentes -2…2, como origen y destino de Convert y de Canonical, en el
// servicio sensible e insensible a mayúsculas: ni pánico ni NaN o ±Inf sin
// error.
func TestZeroPrefixSweepFiniteOrError(t *testing.T) {
	start := time.Now()
	w := zpLoad(t)
	csCodes, ciCodes := zpSweepCodes(w)
	anchorsCS := []string{"1", "m", "K", "Cel", "Np", "B", "B[W]", "[pH]", "[degF]", "Om", "OCel", "ONp", "OB",
		"[zero]", "[zcel]", "[hp'_X]", "Om-1", "[zero]-1", "O[zero]", "%", "10*"}
	var anchorsCI []string
	for _, a := range anchorsCS {
		code, pre := a, ""
		if strings.HasPrefix(a, "O") && w.m.Lookup(a) == nil {
			code, pre = a[1:], zpPrefixCI
		}
		base, exp := code, ""
		if i := strings.IndexAny(code, "-"); i > 0 {
			base, exp = code[:i], code[i:]
		}
		if u := w.m.Lookup(base); u != nil {
			anchorsCI = append(anchorsCI, pre+u.CodeCI+exp)
		}
	}
	for _, c := range []struct {
		name    string
		svc     ucum.Service
		forms   []string
		anchors []string
	}{
		{"case-sensitive", w.svc, zpSweepForms(csCodes, "O"), anchorsCS},
		{"case-insensitive", w.ci, zpSweepForms(ciCodes, zpPrefixCI), anchorsCI},
	} {
		var z zpTally
		calls := 0
		for _, f := range c.forms {
			calls++
			zpWantFiniteOrErr(&z, fmt.Sprintf("Canonical(1, %q)", f), zpCanonical(c.svc, 1, f))
			for _, a := range c.anchors {
				calls += 2
				zpWantFiniteOrErr(&z, fmt.Sprintf("Convert(1, %q, %q)", f, a), zpConvert(c.svc, 1, f, a))
				zpWantFiniteOrErr(&z, fmt.Sprintf("Convert(1, %q, %q)", a, f), zpConvert(c.svc, 1, a, f))
			}
		}
		z.report(t, c.name)
		t.Logf("%s: %d forms, %d calls", c.name, len(c.forms), calls)
	}
	t.Logf("sweep took %v", time.Since(start))
}
