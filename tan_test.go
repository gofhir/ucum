package ucum_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"os"
	"os/exec"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/gofhir/ucum/v4"
	"github.com/gofhir/ucum/v4/internal/essence"
)

// Plan 014: un ángulo α con |α| ≥ π/2 hacia una escala de tangente no tiene
// valor en ella. UCUM §44 ■4 (tabla 18): f_PD(α) = tan(α)×100 y
// f_PD⁻¹(x) = arctan(x/100). La función arctan devuelve solo (-π/2, π/2), así que
// el par es inverso solo ahí: fuera, el valor de tan no vuelve al ángulo, y la
// respuesta es ErrOutOfDomain (antes que ErrOutOfRange).
//
// Los esperados no salen de la librería: las escalas de tangente se buscan en
// el XML por el nombre de su función (tanTimes100, 100tan), el ángulo de cada
// unidad de origen sale de ExactService.CanonicalRat, π es el valor de [pi] del
// XML (un big.Rat, nunca math.Pi) y tan se calcula en big.Float.

// tanFixture es lo que todos los tests de tangente comparten, cargado una vez.
type tanFixture struct {
	scales []string // los códigos de las escalas de tangente del XML
	exact  ucum.ExactService
	svc    ucum.Service
	pi     *big.Rat // [pi] del XML
	halfPi *big.Rat
	err    error
}

var (
	tanOnce sync.Once
	tanFix  tanFixture
)

func loadTan(t testing.TB) *tanFixture {
	t.Helper()
	tanOnce.Do(func() {
		f := &tanFix
		m, err := essence.Load(nil)
		if err != nil {
			f.err = err
			return
		}
		for _, du := range m.DefinedUnits {
			if du.Code == "[pi]" {
				f.pi = du.Value.Value.Rat()
			}
			if du.IsSpecial && du.Value.Function != nil &&
				(du.Value.Function.Name == "tanTimes100" || du.Value.Function.Name == "100tan") {
				f.scales = append(f.scales, du.Code)
			}
		}
		if f.pi == nil || len(f.scales) != 2 {
			f.err = fmt.Errorf("XML has [pi]=%v and tangent scales %v, want a value and two scales", f.pi, f.scales)
			return
		}
		sort.Strings(f.scales)
		f.halfPi = new(big.Rat).Quo(f.pi, big.NewRat(2, 1))
		if f.exact, f.err = ucum.NewExact(); f.err != nil {
			return
		}
		f.svc, f.err = ucum.New()
	})
	if tanFix.err != nil {
		t.Fatal(tanFix.err)
	}
	return &tanFix
}

// angle es v unidades de from, en radianes, exacto. La unidad rad es la base
// de los ángulos, así que su valor canónico es el ángulo.
func (f *tanFixture) angle(t testing.TB, v float64, from string) *big.Rat {
	t.Helper()
	p, err := f.exact.CanonicalRat(new(big.Rat).SetFloat64(v), from)
	if err != nil {
		t.Fatalf("CanonicalRat(%v %s): %v", v, from, err)
	}
	if p.Code != "rad" {
		t.Fatalf("canonical unit of %s is %q, want rad", from, p.Code)
	}
	return p.Value
}

// inside: el intervalo (-π/2, π/2) es abierto, porque en ±π/2 tan no existe.
func (f *tanFixture) inside(angle *big.Rat) bool {
	return new(big.Rat).Abs(angle).Cmp(f.halfPi) < 0
}

// bigTan es tan(x), con x exacto, en big.Float de 300 bits: sin y cos por su
// serie de Taylor, que converge de prisa para |x| < π/2.
func bigTan(x *big.Rat) *big.Float {
	const prec = 300
	xf := new(big.Float).SetPrec(prec).SetRat(x)
	x2 := new(big.Float).SetPrec(prec).Mul(xf, xf)
	eps := new(big.Float).SetPrec(prec).SetMantExp(big.NewFloat(1), -prec-8)
	series := func(first *big.Float, start int) *big.Float {
		sum := new(big.Float).SetPrec(prec).Set(first)
		term := new(big.Float).SetPrec(prec).Set(first)
		for n := start; n < start+400; n += 2 {
			term.Mul(term, x2)
			term.Quo(term, new(big.Float).SetPrec(prec).SetInt64(int64(n*(n+1))))
			term.Neg(term)
			sum.Add(sum, term)
			if new(big.Float).Abs(term).Cmp(eps) < 0 {
				break
			}
		}
		return sum
	}
	sin := series(xf, 2)
	cos := series(new(big.Float).SetPrec(prec).SetInt64(1), 1)
	return new(big.Float).SetPrec(prec).Quo(sin, cos)
}

// bigTan100 es 100·tan(x) redondeado una vez a float64.
func bigTan100(x *big.Rat) float64 {
	r := bigTan(x)
	r.Mul(r, new(big.Float).SetPrec(300).SetInt64(100))
	out, _ := r.Float64()
	return out
}

// ulps es la distancia en unidades de última cifra entre dos float64 finitos.
func ulps(a, b float64) int64 {
	ord := func(f float64) int64 {
		i := int64(math.Float64bits(f))
		if i < 0 {
			i = math.MinInt64 - i
		}
		return i
	}
	d := ord(a) - ord(b)
	if d < 0 {
		d = -d
	}
	return d
}

func wantTanDomain(t *testing.T, what string, v float64, from, to string, err error) {
	t.Helper()
	if !errors.Is(err, ucum.ErrOutOfDomain) {
		t.Errorf("%s = %v, want ErrOutOfDomain", what, err)
		return
	}
	if errors.Is(err, ucum.ErrOutOfRange) {
		t.Errorf("%s = %v, also wraps ErrOutOfRange", what, err)
	}
	msg := err.Error()
	// Juntos, «valor unidad»: el prefijo del error ya nombra las dos unidades.
	if !strings.Contains(msg, fmt.Sprint(v)+" "+from) || !strings.Contains(msg, to) {
		t.Errorf("%s: message %q does not name %v %s and %s", what, msg, v, from, to)
	}
	if !strings.Contains(msg, "outside the domain") || strings.Contains(msg, "NewExact") {
		t.Errorf("%s: message %q, want the domain text and no NewExact hint", what, msg)
	}
}

type angleCase struct {
	v    float64
	unit string
}

// Test 1: fuera del dominio, con las dos escalas. Cada caso se confirma contra
// el π/2 del XML antes de exigir el error.
func TestTanOutsideDomainIsOutOfDomain(t *testing.T) {
	f := loadTan(t)
	cases := []angleCase{
		{2, "rad"}, {math.Pi, "rad"}, {-2, "rad"}, {10, "rad"},
		{200, "deg"}, {-90, "deg"}, {90, "deg"},
		{100, "gon"}, {5400, "'"}, {324000, "''"}, {0.002, "krad"},
		{1e300, "rad"}, {1e300, "Yrad"}, {1, "10*400.rad"},
		{-1e300, "Yrad"}, {-100, "gon"}, {-5400, "'"},
	}
	for _, tc := range cases {
		if a := f.angle(t, tc.v, tc.unit); f.inside(a) {
			t.Fatalf("test table: %v %s = %s rad is inside the domain", tc.v, tc.unit, a.FloatString(5))
		}
		for _, to := range f.scales {
			what := fmt.Sprintf("Convert(%v %s to %s)", tc.v, tc.unit, to)
			_, err := f.svc.Convert(tc.v, tc.unit, to)
			wantTanDomain(t, what, tc.v, tc.unit, to, err)
		}
	}
}

// Test 2: dentro del dominio. Un oráculo independiente (tan en big.Float) con
// el margen escrito, y aparte la guarda de regresión contra main.
func TestTanInsideDomain(t *testing.T) {
	f := loadTan(t)
	var cases []angleCase
	for k := -15; k <= 15; k++ {
		cases = append(cases, angleCase{float64(k) / 10, "rad"})
	}
	for _, v := range []float64{1.6e-3, 1e-10, 1.5707, 1.57, -1.5707, 0.7853981633974483} {
		cases = append(cases, angleCase{v, "rad"})
	}
	for _, v := range []float64{-89.9, -89, -75, -60, -45, -30, -1, 0.001, 30, 45, 60, 75, 89, 89.9, 89.99999} {
		cases = append(cases, angleCase{v, "deg"})
	}
	cases = append(cases, angleCase{1.6, "mrad"}, angleCase{0.0015, "krad"}, angleCase{99, "gon"},
		angleCase{-5000, "'"}, angleCase{300000, "''"})

	for _, tc := range cases {
		angle := f.angle(t, tc.v, tc.unit)
		if !f.inside(angle) {
			t.Fatalf("test table: %v %s is outside the domain", tc.v, tc.unit)
		}
		af, _ := angle.Float64()
		// Margen: el ángulo se redondea una vez a float64 (error relativo de
		// media unidad), y tan lo amplifica por el número de condición
		// κ = 2x/sin 2x; a eso se suman 3 ulps de math.Tan y de la multiplicación
		// por 100.
		kappa := 1.0
		if af != 0 {
			kappa = math.Abs(2 * af / math.Sin(2*af))
		}
		bound := 3 + int64(math.Ceil(kappa))
		oracle := bigTan100(angle)
		guard := 100 * math.Tan(af)

		for _, to := range f.scales {
			got, err := f.svc.Convert(tc.v, tc.unit, to)
			if err != nil {
				t.Errorf("Convert(%v %s to %s) = %v, want a value", tc.v, tc.unit, to, err)
				continue
			}
			if d := ulps(got, oracle); d > bound {
				t.Errorf("Convert(%v %s to %s) = %v, oracle %v: %d ulps, bound %d", tc.v, tc.unit, to, got, oracle, d, bound)
			}
			// Guarda de regresión, no oráculo: es la regla de 010 (el ángulo
			// canónico redondeado una vez, y math.Tan), que main ya cumple.
			if got != guard {
				t.Errorf("regression guard: Convert(%v %s to %s) = %v, main gives 100*math.Tan(%v) = %v", tc.v, tc.unit, to, got, af, guard)
			}
		}
	}

	// 45 deg es π/4, y 100·tan(π/4) = 100: a 2 ulps.
	for _, to := range f.scales {
		got, err := f.svc.Convert(45, "deg", to)
		if err != nil || ulps(got, 100) > 2 {
			t.Errorf("Convert(45 deg to %s) = (%v, %v), want 100 within 2 ulps", to, got, err)
		}
	}
}

// Test 3: el borde con big.Rat. El valor de π/2 es la mitad del [pi] del XML, y el
// intervalo es abierto.
func TestTanDomainEdge(t *testing.T) {
	f := loadTan(t)

	// Las premisas del plan: math.Pi/2 está bajo el π/2 del XML, y su vecino
	// superior no.
	pi2 := math.Pi / 2
	up := math.Nextafter(pi2, math.Inf(1))
	if new(big.Rat).SetFloat64(pi2).Cmp(f.halfPi) >= 0 || new(big.Rat).SetFloat64(up).Cmp(f.halfPi) < 0 {
		t.Fatalf("premise: math.Pi/2 and its upper neighbor should straddle the XML π/2")
	}

	check := func(label string, v float64, unit string, wantInside bool) {
		t.Helper()
		angle := f.angle(t, v, unit)
		if f.inside(angle) != wantInside {
			t.Fatalf("%s: premise on the exact angle %s rad is wrong", label, angle.FloatString(20))
		}
		for _, to := range f.scales {
			got, err := f.svc.Convert(v, unit, to)
			what := fmt.Sprintf("Convert(%v %s to %s)", v, unit, to)
			if !wantInside {
				wantTanDomain(t, what, v, unit, to, err)
				continue
			}
			if err != nil || math.IsInf(got, 0) || math.IsNaN(got) || (got > 0) != (v > 0) {
				t.Errorf("%s = (%v, %v), want a finite value with the sign of the angle", what, got, err)
			}
		}
	}

	// rad: los float64 vecinos de π/2 del XML, a los dos lados del cero.
	x := pi2
	for i := 0; i < 2; i++ {
		x = math.Nextafter(x, 0)
	}
	for i := 0; i < 5; i++ {
		for _, s := range []float64{1, -1} {
			angle := f.angle(t, s*x, "rad")
			check(fmt.Sprintf("%v rad", s*x), s*x, "rad", f.inside(angle))
		}
		x = math.Nextafter(x, math.Inf(1))
	}
	check("math.Pi/2 rad", pi2, "rad", true)
	check("math.Pi/2 + 1 ulp rad", up, "rad", false)
	check("-math.Pi/2 rad", -pi2, "rad", true)
	check("-(math.Pi/2 + 1 ulp) rad", -up, "rad", false)

	// deg: 90 es exactamente π/2, y queda fuera; 1 ulp por debajo, dentro.
	d := 90.0
	for i := 0; i < 2; i++ {
		d = math.Nextafter(d, 0)
	}
	for i := 0; i < 5; i++ {
		for _, s := range []float64{1, -1} {
			angle := f.angle(t, s*d, "deg")
			check(fmt.Sprintf("%v deg", s*d), s*d, "deg", f.inside(angle))
		}
		d = math.Nextafter(d, math.Inf(1))
	}
	check("90 deg", 90, "deg", false)
	check("-90 deg", -90, "deg", false)
	check("90 deg - 1 ulp", math.Nextafter(90, 0), "deg", true)
	check("90 deg + 1 ulp", math.Nextafter(90, 200), "deg", false)
}

// Test 4: ningún cambio de signo ni retroceso entre dos ángulos aceptados
// vecinos, y lo aceptado es exactamente (-π/2, π/2).
func TestTanMonotonic(t *testing.T) {
	f := loadTan(t)
	sweep := func(unit string, step, limit, edge float64) {
		var vs []float64
		for v := -limit; v <= limit; v += step {
			vs = append(vs, v)
		}
		for i, e := 0, edge; i < 6; i++ {
			vs = append(vs, e, -e)
			e = math.Nextafter(e, 0)
		}
		for i, e := 0, edge; i < 6; i++ {
			vs = append(vs, e, -e)
			e = math.Nextafter(e, math.Inf(1))
		}
		sort.Float64s(vs)
		for _, to := range f.scales {
			prev, havePrev, missed := 0.0, false, 0
			for _, v := range vs {
				got, err := f.svc.Convert(v, unit, to)
				in := f.inside(f.angle(t, v, unit))
				switch {
				case !in:
					// Se informan los tres primeros: son decenas de ángulos.
					if !errors.Is(err, ucum.ErrOutOfDomain) {
						missed++
						if missed <= 3 {
							t.Errorf("Convert(%v %s to %s) = (%v, %v), want ErrOutOfDomain", v, unit, to, got, err)
						}
					}
					continue
				case err != nil:
					t.Errorf("Convert(%v %s to %s) = %v, want a value", v, unit, to, err)
					continue
				}
				if (v > 0 && got < 0) || (v < 0 && got > 0) {
					t.Errorf("Convert(%v %s to %s) = %v: sign differs from the angle", v, unit, to, got)
				}
				if havePrev && got < prev {
					t.Errorf("Convert(%v %s to %s) = %v after %v: not monotonic", v, unit, to, got, prev)
				}
				prev, havePrev = got, true
			}
			if missed > 3 {
				t.Errorf("%s to %s: %d angles outside the domain without ErrOutOfDomain in all", unit, to, missed)
			}
		}
	}
	sweep("rad", 0.05, 3, math.Pi/2)
	sweep("deg", 2.5, 180, 90)
}

// tanXML es un modelo mínimo: rad, y sobre él una escala de tangente con el
// código y la función del XML de UCUM. El parámetro withPi agrega [pi] con el valor del XML
// (digits); withLn agrega una escala ln de un ángulo, [lnA] = ln(1 rad).
func tanXML(withPi bool, digits string, withLn bool) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="ascii"?>
<root xmlns="http://unitsofmeasure.org/ucum-essence" version="9.9" revision="N/A"
      revision-date="2099-01-01">
 <base-unit Code="rad" CODE="RAD" dim="A"><name>radian</name><printSymbol>rad</printSymbol><property>plane angle</property></base-unit>
`)
	if withPi {
		fmt.Fprintf(&b, ` <unit Code="[pi]" CODE="[PI]" isMetric="no" class="dimless">
  <name>the number pi</name><printSymbol>pi</printSymbol><property>number</property>
  <value Unit="1" UNIT="1" value="%s">pi</value>
 </unit>
`, digits)
	}
	b.WriteString(` <unit Code="[p'diop]" CODE="[P'DIOP]" isMetric="no" isSpecial="yes" class="clinical">
  <name>prism diopter</name><printSymbol>PD</printSymbol><property>refraction of a prism</property>
  <value Unit="100tan(1 rad)" UNIT="100TAN(1 RAD)"><function name="tanTimes100" value="1" Unit="rad"/></value>
 </unit>
`)
	if withLn {
		b.WriteString(` <unit Code="[lnA]" CODE="[LNA]" isMetric="no" isSpecial="yes" class="heat">
  <name>lnA</name><printSymbol>lnA</printSymbol><property>plane angle</property>
  <value Unit="ln(1 rad)" UNIT="LN(1 RAD)"><function name="ln" value="1" Unit="rad"/></value>
 </unit>
`)
	}
	b.WriteString("</root>")
	return b.String()
}

// Test 5: un origen sin valor racional (ln de un ángulo, propio). El ángulo es
// e^v rad, que supera π/2 en v > ln(π/2) = 0.4516. 700 materializa (e^700 =
// 1e304) y 710 no (e^710 desborda float64): los dos son ErrOutOfDomain, no uno
// de dominio y otro de rango.
func TestTanScaleFromALogOriginWithoutExactValue(t *testing.T) {
	f := loadTan(t)
	svc, err := ucum.NewFromReader(strings.NewReader(tanXML(true, f.pi.FloatString(64), true)))
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []float64{700, 710, 0.5, 1, 100, 1e6} {
		_, err := svc.Convert(v, "[lnA]", "[p'diop]")
		wantTanDomain(t, fmt.Sprintf("Convert(%v [lnA] to [p'diop])", v), v, "[lnA]", "[p'diop]", err)
	}
	// Dentro: e^v rad con v bajo ln(π/2); 100·tan del ángulo, a un margen
	// relativo de 1e-12 (math.Exp, y un κ de hasta 20 cerca del borde).
	for _, v := range []float64{-10, -1, 0, 0.4} {
		got, err := svc.Convert(v, "[lnA]", "[p'diop]")
		want := bigTan100(new(big.Rat).SetFloat64(math.Exp(v)))
		if err != nil || math.Abs(got-want) > 1e-12*math.Abs(want) {
			t.Errorf("Convert(%v [lnA] to [p'diop]) = (%v, %v), want %v", v, got, err, want)
		}
	}
}

// Test 6: lo que no cambia.
func TestTanUnchanged(t *testing.T) {
	f := loadTan(t)
	halfPiF, _ := f.halfPi.Float64()

	// De la escala a un ángulo: f⁻¹ = arctan(x/100) está en (-π/2, π/2) para
	// todo x finito, también para los valores enormes, el cero y los negativos.
	for _, from := range f.scales {
		for _, v := range []float64{0, 1, -1, 100, -100, 1e-300, -1e-300, 1e10, -1e10, 1e300, -1e300, 1e308, -math.MaxFloat64, math.MaxFloat64} {
			got, err := f.svc.Convert(v, from, "rad")
			if err != nil || math.Abs(got) > halfPiF {
				t.Errorf("Convert(%v %s to rad) = (%v, %v), want a value in [-π/2, π/2]", v, from, got, err)
				continue
			}
			// math.Atan solo como guarda de regresión: arctan(x/100).
			if want := math.Atan(v / 100); math.Abs(v) < 1e300 && got != want {
				t.Errorf("regression guard: Convert(%v %s to rad) = %v, main gives %v", v, from, got, want)
			}
			if math.Abs(v) >= 1e300 && got != math.Copysign(halfPiF, v) {
				t.Errorf("Convert(%v %s to rad) = %v, want ±π/2 of the XML = %v", v, from, got, halfPiF)
			}
		}
		// 100 es 45 deg, a 1 ulp.
		if got, err := f.svc.Convert(100, from, "deg"); err != nil || ulps(got, 45) > 1 {
			t.Errorf("Convert(100 %s to deg) = (%v, %v), want 45", from, got, err)
		}
	}

	// Un valor no finito pasa sin verificar (#55): tan(±Inf) es NaN, sin error.
	for _, in := range []float64{math.Inf(1), math.Inf(-1), math.NaN()} {
		for _, to := range f.scales {
			if got, err := f.svc.Convert(in, "rad", to); err != nil || !math.IsNaN(got) {
				t.Errorf("Convert(%v rad to %s) = (%v, %v), want NaN and no error", in, to, got, err)
			}
		}
	}
}

// Un XML propio sin [pi] no tiene el chequeo, como hoy: el borde sale de [pi]
// por su código. Las unidades deg y gon, y los minutos y segundos de arco, se
// definen con [pi] y tampoco están en este modelo, así que el ángulo va en rad.
func TestTanWithoutPiHasNoDomainCheck(t *testing.T) {
	svc, err := ucum.NewFromReader(strings.NewReader(tanXML(false, "", false)))
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []float64{2, 10, -2, 0.5} {
		got, err := svc.Convert(v, "rad", "[p'diop]")
		// Guarda de regresión: el valor literal de tan, como en main.
		if want := 100 * math.Tan(v); err != nil || got != want {
			t.Errorf("Convert(%v rad to [p'diop]) = (%v, %v), want %v: no [pi], no domain", v, got, err, want)
		}
	}
}

// El mismo modelo con [pi]: 2 rad ya es de dominio (contraste del test anterior).
func TestTanSameModelWithPiHasTheDomainCheck(t *testing.T) {
	f := loadTan(t)
	with, err := ucum.NewFromReader(strings.NewReader(tanXML(true, f.pi.FloatString(64), false)))
	if err != nil {
		t.Fatal(err)
	}
	_, err = with.Convert(2, "rad", "[p'diop]")
	wantTanDomain(t, "Convert(2 rad to [p'diop]) with [pi]", 2, "rad", "[p'diop]", err)
}

// tanCustomXML es tanXML con un [pi] a gusto: piUnit y piValue son los de su
// <value>; ten agrega la unidad 10* para un [pi] que la nombra; extra son
// unidades que se agregan al final.
func tanCustomXML(piUnit, piValue string, ten bool, extra string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="ascii"?>
<root xmlns="http://unitsofmeasure.org/ucum-essence" version="9.9" revision="N/A"
      revision-date="2099-01-01">
 <base-unit Code="rad" CODE="RAD" dim="A"><name>radian</name><printSymbol>rad</printSymbol><property>plane angle</property></base-unit>
`)
	if ten {
		b.WriteString(` <unit Code="10*" CODE="10*" isMetric="no" class="dimless">
  <name>the number ten for arbitrary powers</name><printSymbol>10</printSymbol><property>number</property>
  <value Unit="1" UNIT="1" value="10">10</value>
 </unit>
`)
	}
	fmt.Fprintf(&b, ` <unit Code="[pi]" CODE="[PI]" isMetric="no" class="dimless">
  <name>the number pi</name><printSymbol>pi</printSymbol><property>number</property>
  <value Unit="%s" UNIT="%s" value="%s">pi</value>
 </unit>
 <unit Code="[p'diop]" CODE="[P'DIOP]" isMetric="no" isSpecial="yes" class="clinical">
  <name>prism diopter</name><printSymbol>PD</printSymbol><property>refraction of a prism</property>
  <value Unit="100tan(1 rad)" UNIT="100TAN(1 RAD)"><function name="tanTimes100" value="1" Unit="rad"/></value>
 </unit>
`, piUnit, strings.ToUpper(piUnit), piValue)
	b.WriteString(extra)
	b.WriteString("</root>")
	return b.String()
}

// [pi] sale del valor canónico de 1 [pi] que calcula el servicio, no de su texto:
// 0.314159…×10* vale π, y 2 rad sigue fuera de dominio. Un [pi] con dimensión
// (en rad), cero o negativo no es el número π: no hay chequeo, como sin [pi].
func TestTanPiComesFromItsCanonicalValue(t *testing.T) {
	f := loadTan(t)
	tenth := new(big.Rat).Quo(f.pi, big.NewRat(10, 1)).FloatString(80)
	scaled, err := ucum.NewFromReader(strings.NewReader(tanCustomXML("10*", tenth, true, "")))
	if err != nil {
		t.Fatal(err)
	}
	_, err = scaled.Convert(2, "rad", "[p'diop]")
	wantTanDomain(t, "Convert(2 rad to [p'diop]) with [pi] = 0.314…×10*", 2, "rad", "[p'diop]", err)
	if got, err := scaled.Convert(1.5, "rad", "[p'diop]"); err != nil || got != bigTan100(new(big.Rat).SetFloat64(1.5)) {
		t.Errorf("Convert(1.5 rad to [p'diop]) = (%v, %v), want %v", got, err, bigTan100(new(big.Rat).SetFloat64(1.5)))
	}
	for name, xml := range map[string]string{
		"[pi] in rad": tanCustomXML("rad", f.pi.FloatString(64), false, ""),
		"[pi] < 0":    tanCustomXML("1", "-3.14", false, ""),
		"[pi] = 0":    tanCustomXML("1", "0", false, ""),
	} {
		svc, err := ucum.NewFromReader(strings.NewReader(xml))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, v := range []float64{0, 2} {
			want := bigTan100(new(big.Rat).SetFloat64(v))
			if got, err := svc.Convert(v, "rad", "[p'diop]"); err != nil || got != want {
				t.Errorf("%s: Convert(%v rad to [p'diop]) = (%v, %v), want %v and no domain check", name, v, got, err, want)
			}
		}
	}
}

// bigLn es ln(x) en big.Float de 300 bits, para x > 0: x = m·2^e con m en
// [0.5, 1), ln m = 2·atanh((m-1)/(m+1)) por su serie, y e·ln 2.
func bigLn(x *big.Float) *big.Float {
	const prec = 300
	series := func(m *big.Float) *big.Float {
		one := new(big.Float).SetPrec(prec).SetInt64(1)
		z := new(big.Float).SetPrec(prec).Quo(new(big.Float).Sub(m, one), new(big.Float).Add(m, one))
		z2 := new(big.Float).SetPrec(prec).Mul(z, z)
		term := new(big.Float).SetPrec(prec).Set(z)
		sum := new(big.Float).SetPrec(prec).Set(z)
		eps := new(big.Float).SetPrec(prec).SetMantExp(big.NewFloat(1), -prec-8)
		for n := int64(3); n < 2000; n += 2 {
			term.Mul(term, z2)
			sum.Add(sum, new(big.Float).SetPrec(prec).Quo(term, new(big.Float).SetPrec(prec).SetInt64(n)))
			if new(big.Float).Abs(term).Cmp(eps) < 0 {
				break
			}
		}
		return sum.Mul(sum, new(big.Float).SetPrec(prec).SetInt64(2))
	}
	ln2 := series(new(big.Float).SetPrec(prec).SetFloat64(0.5))
	ln2.Neg(ln2)
	m := new(big.Float).SetPrec(prec)
	e := x.MantExp(m)
	out := series(m)
	return out.Add(out, new(big.Float).SetPrec(prec).Mul(new(big.Float).SetPrec(prec).SetInt64(int64(e)), ln2))
}

// Test 5b: el ángulo sin valor exacto cerca de π/2. [tiny] = 1e-310 rad y
// [lnB] = ln(1 [tiny]): el ángulo es e^v·1e-310 rad, que llega a π/2 en
// v* = ln(π/2) + 310·ln 10 ≈ 714.25 (con el π del XML, en big.Float). Bajo v*
// hay valor; sobre v*, ErrOutOfDomain, también en el margen de 1e-9 del log10,
// donde decide la comparación exacta del ángulo materializado.
//
// Esperados: solo del oráculo big.Float (el v del caso es un float64, su valor
// exacto se compara con v*); Convert a rad es solo una lectura de control.
func TestTanScaleFromALogOriginNearHalfPi(t *testing.T) {
	f := loadTan(t)
	const extra = ` <unit Code="[tiny]" CODE="[TINY]" isMetric="no" class="dimless">
  <name>tiny</name><printSymbol>tiny</printSymbol><property>plane angle</property>
  <value Unit="rad" UNIT="RAD" value="1e-310">1e-310</value>
 </unit>
 <unit Code="[lnB]" CODE="[LNB]" isMetric="no" isSpecial="yes" class="heat">
  <name>lnB</name><printSymbol>lnB</printSymbol><property>plane angle</property>
  <value Unit="ln(1 [tiny])" UNIT="LN(1 [TINY])"><function name="ln" value="1" Unit="[tiny]"/></value>
 </unit>
`
	svc, err := ucum.NewFromReader(strings.NewReader(tanCustomXML("1", f.pi.FloatString(64), false, extra)))
	if err != nil {
		t.Fatal(err)
	}
	const prec = 300
	ten := new(big.Float).SetPrec(prec).SetInt64(10)
	vstar := new(big.Float).SetPrec(prec).Quo(new(big.Float).SetPrec(prec).SetRat(f.pi), new(big.Float).SetPrec(prec).SetInt64(2))
	vstar = bigLn(vstar)
	vstar.Add(vstar, new(big.Float).SetPrec(prec).Mul(new(big.Float).SetPrec(prec).SetInt64(310), bigLn(ten)))
	vf, _ := vstar.Float64()
	if math.Abs(vf-714.25296) > 1e-4 {
		t.Fatalf("v* = %v, want about 714.25296", vf)
	}
	// Dentro del margen de 1e-9: -1e-12 y -2e-13 están bajo v* (hay valor) y los
	// float64 vecinos de v* caen a cada lado. El lado esperado sale siempre del
	// oráculo, contra el valor exacto del float64. Un margen mayor que +1e-9 o
	// cero decide igual con el oráculo (equivalentes); los casos que atan el
	// margen por abajo son los de -1e-12 y -2e-13, que un margen de -1e-12
	// rechazaría sin motivo.
	cases := []float64{vf - 1e-6, vf - 1e-10, vf - 1e-12, vf - 2e-13,
		math.Nextafter(vf, math.Inf(-1)), vf, math.Nextafter(vf, math.Inf(1)),
		vf + 1e-10, vf + 1e-6}
	for _, v := range cases {
		d := v - vf
		rad, rerr := svc.Convert(v, "[lnB]", "rad")
		t.Logf("sanity: v* %+g: Convert(%v [lnB] to rad) = (%v, %v)", d, v, rad, rerr)
		got, err := svc.Convert(v, "[lnB]", "[p'diop]")
		if new(big.Float).SetPrec(prec).SetFloat64(v).Cmp(vstar) >= 0 {
			wantTanDomain(t, fmt.Sprintf("Convert(%v [lnB] to [p'diop])", v), v, "[lnB]", "[p'diop]", err)
			continue
		}
		if err != nil || math.IsNaN(got) || math.IsInf(got, 0) || got <= 0 {
			t.Errorf("Convert(%v [lnB] to [p'diop]) = (%v, %v), want a positive value below π/2", v, got, err)
		}
	}
}

// Test 5c: ln(1 K) con [pi] en el modelo: una escala de temperatura no es de
// tangente, así que su valor y su error no son de dominio. Con IsTangent
// siempre verdadero, 2 [lgK] (100 K, sobre π/2) daría ErrOutOfDomain.
func TestTanLogOriginToTemperatureIsNotATangentScale(t *testing.T) {
	raw, err := os.ReadFile("internal/essence/ucum-essence.xml")
	if err != nil {
		t.Fatal(err)
	}
	const lgK = ` <unit Code="[lgK]" CODE="[LGK]" isMetric="no" isSpecial="yes" class="heat">
  <name>lgK</name><printSymbol>lgK</printSymbol><property>temperature</property>
  <value Unit="lg(1 K)" UNIT="LG(1 K)"><function name="lg" value="1" Unit="K"/></value>
 </unit>
</root>`
	xml := strings.Replace(string(raw), "</root>", lgK, 1)
	if xml == string(raw) {
		t.Fatal("no </root> in the definitions")
	}
	svc, err := ucum.NewFromReader(strings.NewReader(xml))
	if err != nil {
		t.Fatal(err)
	}
	// 10^v K en Cel es 10^v - 273.15 (UCUM §21-22): el esperado sale de la
	// resta exacta, redondeada una vez.
	for _, tc := range []struct {
		v    float64
		want float64
	}{{-400, -273.15}, {2, -173.15}, {3, 726.85}} {
		got, err := svc.Convert(tc.v, "[lgK]", "Cel")
		if err != nil || got != tc.want {
			t.Errorf("Convert(%v, [lgK], Cel) = (%v, %v), want %v", tc.v, got, err, tc.want)
		}
	}
	// 10^400 K no cabe: es de rango, no de dominio.
	if _, err := svc.Convert(400, "[lgK]", "Cel"); !errors.Is(err, ucum.ErrOutOfRange) || errors.Is(err, ucum.ErrOutOfDomain) {
		t.Errorf("Convert(400, [lgK], Cel) = %v, want ErrOutOfRange only", err)
	}
}

// Una definición que vuelve a sí misma no se puede expandir: el servicio carga
// (la guarda está en la canonicalización, no en la carga) y 1 [pi] da un error
// que nombra la unidad, en vez de desbordar la pila, que recover no atrapa. Sin
// [pi] utilizable no hay chequeo de dominio y el resto sigue como antes.
//
// El desbordamiento de pila mata el binario sin imprimir «--- FAIL», así que la
// carga corre en un subproceso (este mismo test con una variable de entorno) y
// su salida distinta de cero es el fallo del test.
func TestTanCyclicDefinitionIsAnErrorNotACrash(t *testing.T) {
	if os.Getenv("UCUM_TAN_CYCLE_CHILD") != "1" {
		cmd := exec.CommandContext(context.Background(), os.Args[0], "-test.run=^TestTanCyclicDefinitionIsAnErrorNotACrash$")
		cmd.Env = append(os.Environ(), "UCUM_TAN_CYCLE_CHILD=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			if len(out) > 600 {
				out = out[:600]
			}
			t.Fatalf("loading a cyclic definition failed in a subprocess: %v\n%s", err, out)
		}
		return
	}
	// Con la pila acotada, un mutante sin guarda muere en un instante en vez de
	// crecer hasta el tope por defecto (1 GB).
	debug.SetMaxStack(64 << 20)
	unit := func(code, of string) string {
		return fmt.Sprintf(` <unit Code=%q CODE=%q isMetric="no" class="dimless">
  <name>cycle</name><printSymbol>c</printSymbol><property>number</property>
  <value Unit=%q UNIT=%q value="1">c</value>
 </unit>
`, code, strings.ToUpper(code), of, strings.ToUpper(of))
	}
	cycles := map[string]struct{ xml, in, name string }{
		"self": {tanCustomXML("[pi]", "1", false, ""), "[pi]", "[pi]"},
		"two units": {tanCustomXML("[pi2]", "1", false,
			` <unit Code="[pi2]" CODE="[PI2]" isMetric="no" class="dimless">
  <name>cycle</name><printSymbol>pi2</printSymbol><property>number</property>
  <value Unit="[pi]" UNIT="[PI]" value="1">pi</value>
 </unit>
`), "[pi]", "[pi"},
		// El ciclo no incluye la raíz: [pi] entra en un ciclo de [pi2] y [pi3].
		"cycle without the root": {tanCustomXML("[pi2]", "1", false, unit("[pi2]", "[pi3]")+unit("[pi3]", "[pi2]")),
			"[pi]", ""},
	}
	for name, c := range cycles {
		svc, err := ucum.NewFromReader(strings.NewReader(c.xml))
		if err != nil {
			t.Fatalf("%s: NewFromReader: %v", name, err)
		}
		_, err = svc.Canonical(1, c.in)
		if err == nil {
			t.Errorf("%s: Canonical(1, %q) succeeded, want a cycle error", name, c.in)
			continue
		}
		msg := err.Error()
		named := strings.Contains(msg, c.name)
		if name == "cycle without the root" {
			named = strings.Contains(msg, "[pi2]") || strings.Contains(msg, "[pi3]")
		}
		if !strings.Contains(msg, "definition cycle") || !named || strings.Contains(msg, "expand unit") {
			t.Errorf("%s: Canonical(1, %q) error = %v, want a short one that names the cycle", name, c.in, err)
		}
		want := bigTan100(new(big.Rat).SetFloat64(2))
		if got, err := svc.Convert(2, "rad", "[p'diop]"); err != nil || got != want {
			t.Errorf("%s: Convert(2 rad to [p'diop]) = (%v, %v), want %v and no domain check", name, got, err, want)
		}
	}

	// Una cadena sin ciclo más profunda que cualquier búfer fijo (30 niveles)
	// se expande bien: [c0] = 2 [c1], …, [c29] = 2 [c30], [c30] = 3 rad.
	const levels = 30
	var chain strings.Builder
	for i := 0; i < levels; i++ {
		fmt.Fprintf(&chain, ` <unit Code="[c%d]" CODE="[C%d]" isMetric="no" class="dimless">
  <name>chain</name><printSymbol>c</printSymbol><property>number</property>
  <value Unit="[c%d]" UNIT="[C%d]" value="2">c</value>
 </unit>
`, i, i, i+1, i+1)
	}
	fmt.Fprintf(&chain, ` <unit Code="[c%d]" CODE="[C%d]" isMetric="no" class="dimless">
  <name>chain</name><printSymbol>c</printSymbol><property>number</property>
  <value Unit="rad" UNIT="RAD" value="3">c</value>
 </unit>
`, levels, levels)
	svc, err := ucum.NewFromReader(strings.NewReader(tanCustomXML("[pi]", "3.141592653589793", false, chain.String())))
	if err != nil {
		t.Fatalf("chain: NewFromReader: %v", err)
	}
	// 3×2^30 es exacto en float64.
	want := 3 * float64(uint64(1)<<levels)
	if got, err := svc.Convert(1, "[c0]", "rad"); err != nil || got != want {
		t.Errorf("Convert(1, [c0], rad) = (%v, %v), want %v", got, err, want)
	}
}
