package ucum_test

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"
	"sync"
	"testing"

	"github.com/gofhir/ucum/v4"
	"github.com/gofhir/ucum/v4/internal/essence"
)

// Plan 011 (#75, opción B): una escala logarítmica de base entera con
// exponente entero da el resultado exacto base^n · (resto) · factor, redondeado
// una vez. Los esperados salen de las definiciones de ucum-essence.xml (UCUM §22:
// la función y después la unidad) evaluadas con big.Rat; el factor de la
// referencia sale de CanonicalRat, la API exacta. Ningún esperado sale de correr
// la ruta que se prueba.

// allLogUnits son todos los átomos del XML cuya función tiene base entera
// (logFunctions), sin prefijo y con cada prefijo del XML, menos las
// combinaciones que la gramática rechaza.
func allLogUnits(t *testing.T, s ucum.Service) []logUnit {
	t.Helper()
	m, err := essence.Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	var codes []struct{ prefix, atom string }
	for _, a := range m.DefinedUnits {
		if a.Value == nil || a.Value.Function == nil {
			continue
		}
		if _, ok := logFunctions[a.Value.Function.Name]; !ok {
			continue
		}
		prefixes := make([]string, 1, 1+len(m.Prefixes))
		for _, p := range m.Prefixes {
			prefixes = append(prefixes, p.Code)
		}
		for _, p := range prefixes {
			if s.Validate(p+a.Code) == nil {
				codes = append(codes, struct{ prefix, atom string }{p, a.Code})
			}
		}
	}
	return buildLogUnits(t, codes)
}

// logPoint es un valor n de una escala logarítmica cuyo exponente exacto es n.
type logPoint struct {
	u   logUnit
	n   int
	v   float64      // el valor de la escala, exacto: v·α/divisor = ±n
	pow *big.Rat     // base^n
	ref ucum.RatPair // el valor canónico de la referencia de la definición
}

// value es base^n · referencia: el valor canónico exacto del punto.
func (p logPoint) value() *big.Rat { return new(big.Rat).Mul(p.pow, p.ref.Value) }

// exponentValue es el v cuyo exponente exacto es n (v·α/divisor = ±n), o false
// si ningún float64 lo es.
func exponentValue(u logUnit, n int) (float64, bool) {
	e := new(big.Rat).SetInt64(int64(n))
	e.Mul(e, new(big.Rat).SetFloat64(u.divisor))
	e.Quo(e, u.alpha)
	if u.negate {
		e.Neg(e)
	}
	v, _ := e.Float64()
	return v, !math.IsInf(v, 0) && new(big.Rat).SetFloat64(v).Cmp(e) == 0
}

// normalExp: base^n es un float64 normal.
func normalExp(base int64, n int) bool { return !nonNormal(powRat(base, n)) }

// exponentRange da el menor y el mayor n con base^n normal.
func exponentRange(base int64) (lo, hi int) {
	k := int(310/math.Log10(float64(base))) + 2
	hi = k
	for !normalExp(base, hi) {
		hi--
	}
	lo = -k
	for !normalExp(base, lo) {
		lo++
	}
	return lo, hi
}

// pointsOf da los puntos de u con intermedio normal que elige pick.
func pointsOf(t *testing.T, s ucum.Service, u logUnit, pick func(n, lo, hi int) bool) []logPoint {
	t.Helper()
	ref := refFactor(t, s, u)
	lo, hi := exponentRange(u.base)
	var out []logPoint
	for n := lo; n <= hi; n++ {
		if !pick(n, lo, hi) {
			continue
		}
		v, ok := exponentValue(u, n)
		if !ok {
			continue
		}
		out = append(out, logPoint{u: u, n: n, v: v, pow: powRat(u.base, n), ref: ref})
	}
	return out
}

// strideN es el paso de la rejilla: con él quedan unos 20.000 puntos de
// Canonical, más todos los |n| ≤ 30 y los bordes del rango normal.
const strideN = 5

var (
	gridOnce   sync.Once
	gridPoints []logPoint
	gridUnits  []logUnit
)

// factorGrid son los puntos de todas las unidades, armados una vez.
func factorGrid(t *testing.T, s ucum.Service) ([]logUnit, []logPoint) {
	t.Helper()
	gridOnce.Do(func() {
		gridUnits = allLogUnits(t, s)
		for _, u := range gridUnits {
			gridPoints = append(gridPoints, pointsOf(t, s, u, func(n, lo, hi int) bool {
				return n >= -30 && n <= 30 || n%strideN == 0 || n <= lo+2 || n >= hi-2
			})...)
		}
	})
	if len(gridPoints) == 0 {
		t.Fatal("no log points")
	}
	return gridUnits, gridPoints
}

// thin deja uno de cada k puntos.
func thin(pts []logPoint, k int) []logPoint {
	var out []logPoint
	for i := 0; i < len(pts); i += k {
		out = append(out, pts[i])
	}
	return out
}

// nearestOf redondea una vez al float64 más cercano, y dice si cabe: finito y
// distinto de cero.
func nearestOf(r *big.Rat) (float64, bool) { return fits(r) }

type report struct {
	t            *testing.T
	name         string
	checked, off int
}

func (r *report) fail(format string, args ...any) {
	r.off++
	if r.off <= 5 {
		r.t.Errorf(format, args...)
	}
}

func (r *report) done(atLeast int) {
	r.t.Helper()
	if r.off > 5 {
		r.t.Errorf("%s: %d of %d points off the nearest float64", r.name, r.off, r.checked)
	}
	if r.checked < atLeast {
		r.t.Errorf("%s: only %d points checked, want at least %d", r.name, r.checked, atLeast)
	}
	r.t.Logf("%s: %d points checked, %d off", r.name, r.checked, r.off)
}

// Test 1: la rejilla de Canonical, en todas las unidades logarítmicas de base
// entera del XML: el float64 más cercano a base^n · factor.
func TestLogFactorCanonicalIsNearest(t *testing.T) {
	s := newService(t)
	units, pts := factorGrid(t, s)
	t.Logf("%d units, %d grid points", len(units), len(pts))
	r := &report{t: t, name: "Canonical"}
	for _, p := range pts {
		want, ok := nearestOf(p.value())
		if !ok || !isNormal(want) {
			continue
		}
		r.checked++
		got, err := s.Canonical(p.v, p.u.code)
		if err != nil || got.Value != want || got.Code != p.ref.Code {
			r.fail("Canonical(%v, %q) = (%v %q, %v), want %v %q (n = %d)", p.v, p.u.code, got.Value, got.Code, err, want, p.ref.Code, p.n)
		}
	}
	r.done(15000)
}

// Test 2: la rejilla de Convert a 3·10^K·referencia, y a otra unidad de la
// misma dimensión.
func TestLogFactorConvertIsNearest(t *testing.T) {
	s := newService(t)
	ex := s.(ucum.ExactService)
	_, pts := factorGrid(t, s)
	toCache := map[string]ucum.RatPair{}
	target := func(dest string) ucum.RatPair {
		if p, ok := toCache[dest]; ok {
			return p
		}
		p, err := ex.CanonicalRat(big.NewRat(1, 1), dest)
		if err != nil {
			t.Fatalf("CanonicalRat(1, %q): %v", dest, err)
		}
		toCache[dest] = p
		return p
	}
	check := func(r *report, p logPoint, dest string) {
		to := target(dest)
		if to.Code != p.ref.Code {
			t.Fatalf("%s and %s are not of the same dimension: %q vs %q", p.u.code, dest, p.ref.Code, to.Code)
		}
		want, ok := nearestOf(new(big.Rat).Quo(p.value(), to.Value))
		if !ok || !isNormal(want) {
			return
		}
		r.checked++
		got, err := s.Convert(p.v, p.u.code, dest)
		if err != nil || got != want {
			r.fail("Convert(%v, %q, %q) = (%v, %v), want %v (n = %d)", p.v, p.u.code, dest, got, err, want, p.n)
		}
	}
	for _, k := range []int{-2, 0, 5} {
		r := &report{t: t, name: fmt.Sprintf("Convert to 3.10*%d.ref", k)}
		for _, p := range thin(pts, 6) {
			dest := fmt.Sprintf("3.10*%d", k)
			if p.u.refUnit != "1" {
				dest += "." + p.u.refUnit
			}
			check(r, p, dest)
		}
		r.done(3000)
	}
	for _, tc := range []struct{ from, to string }{{"[pH]", "mmol/L"}, {"B[SPL]", "kPa"}, {"B[W]", "mW"}} {
		r := &report{t: t, name: "Convert " + tc.from + " to " + tc.to}
		for _, p := range pts {
			if p.u.code == tc.from {
				check(r, p, tc.to)
			}
		}
		r.done(100)
	}
}

// ws son los otros operandos de Multiply y Divide: el 1 de 1/(n [pH]), un
// negativo y un subnormal entre ellos.
var ws = []float64{3, 0.7, 1.1, 1e10, 7e-5, -3, 5e-324, 1}

// Test 3: Multiply y Divide por un operando lineal, en los dos órdenes. El
// operando lineal es su binario exacto por el factor de su unidad.
func TestLogFactorProductIsNearest(t *testing.T) {
	s := newService(t)
	ex := s.(ucum.ExactService)
	_, pts := factorGrid(t, s)
	inch, err := ex.CanonicalRat(big.NewRat(1, 1), "[in_i]")
	if err != nil {
		t.Fatal(err)
	}
	// "1" no tiene factor; [in_i] = 2.54 cm (ucum-essence.xml).
	lins := []struct {
		code   string
		factor *big.Rat
	}{{"1", big.NewRat(1, 1)}, {"[in_i]", inch.Value}}
	r := &report{t: t, name: "Multiply and Divide"}
	for _, p := range thin(pts, 40) {
		l := p.value()
		for _, w := range ws {
			for _, lin := range lins {
				lv := new(big.Rat).Mul(new(big.Rat).SetFloat64(w), lin.factor)
				a, b := ucum.Pair{Value: p.v, Code: p.u.code}, ucum.Pair{Value: w, Code: lin.code}
				for _, tc := range []struct {
					name string
					got  func() (ucum.Pair, error)
					want *big.Rat
				}{
					{"Multiply(log, lin)", func() (ucum.Pair, error) { return s.Multiply(a, b) }, new(big.Rat).Mul(l, lv)},
					{"Multiply(lin, log)", func() (ucum.Pair, error) { return s.Multiply(b, a) }, new(big.Rat).Mul(l, lv)},
					{"Divide(log, lin)", func() (ucum.Pair, error) { return s.Divide(a, b) }, new(big.Rat).Quo(l, lv)},
					{"Divide(lin, log)", func() (ucum.Pair, error) { return s.Divide(b, a) }, new(big.Rat).Quo(lv, l)},
				} {
					want, ok := nearestOf(tc.want)
					if !ok {
						continue
					}
					r.checked++
					got, err := tc.got()
					if err != nil || got.Value != want {
						r.fail("%s with %v %s and %v %s = (%v, %v), want %v", tc.name, p.v, p.u.code, w, lin.code, got.Value, err, want)
					}
				}
			}
		}
	}
	r.done(5000)
}

// Test 3b: el caso del plan, Multiply(-1 B, 3 1) = nearest(3/10), con factor 1
// en la unidad: la multiplicación por el otro operando redondea igual.
func TestLogFactorProductWithUnitFactorOne(t *testing.T) {
	s := newService(t)
	want, _ := big.NewRat(3, 10).Float64()
	p, err := s.Multiply(ucum.Pair{Value: -1, Code: "B"}, ucum.Pair{Value: 3, Code: "1"})
	if err != nil || p.Value != want {
		t.Errorf("Multiply(-1 B, 3) = (%v, %v), want %v, the nearest float64 of 3/10", p.Value, err, want)
	}
}

// Test 4: dos operandos logarítmicos, con los dos exponentes enteros, con uno
// solo y con ninguno. Un exponente no entero aporta el binario exacto del
// float64 de math.Pow, y el resultado se redondea una vez (plan 011, §10).
func TestLogFactorTwoLogOperands(t *testing.T) {
	s := newService(t)
	units := loadLogUnits(t)
	pairs := []struct{ a, b string }{{"[pH]", "B[W]"}, {"B[W]", "[pH]"}, {"B", "B[SPL]"}, {"[hp'_C]", "B"}, {"B[SPL]", "B[SPL]"}}
	sample := func(code string) []logPoint {
		u := unitByCode(t, units, code)
		return pointsOf(t, s, u, func(n, lo, hi int) bool { return n%31 == 0 || n <= lo+1 || n >= hi-1 })
	}
	// pow es math.Pow de main: base^e con e = ±v/divisor.
	pow := func(u logUnit, v float64) float64 {
		e := v / u.divisor
		if u.negate {
			e = -e
		}
		return math.Pow(float64(u.base), e)
	}
	// Un exponente no entero: v desplazado 0.37 (math.Pow, sin cambios).
	frac := func(p logPoint) (float64, bool) {
		v := p.v + 0.37
		return v, isNormal(pow(p.u, v))
	}
	for _, mode := range []string{"both integer", "first integer", "second integer", "neither integer"} {
		r := &report{t: t, name: "two log operands, " + mode}
		for _, pr := range pairs {
			for _, p1 := range sample(pr.a) {
				for _, p2 := range sample(pr.b) {
					v1, v2 := p1.v, p2.v
					l1, l2 := p1.value(), p2.value()
					m1, m2 := 0.0, 0.0
					// Un operando de exponente no entero aporta el binario del
					// float64 de math.Pow, por el factor de su unidad.
					if mode == "second integer" || mode == "neither integer" {
						var ok bool
						if v1, ok = frac(p1); !ok {
							continue
						}
						m1 = pow(p1.u, v1)
						l1 = new(big.Rat).Mul(new(big.Rat).SetFloat64(m1), p1.ref.Value)
					}
					if mode == "first integer" || mode == "neither integer" {
						var ok bool
						if v2, ok = frac(p2); !ok {
							continue
						}
						m2 = pow(p2.u, v2)
						l2 = new(big.Rat).Mul(new(big.Rat).SetFloat64(m2), p2.ref.Value)
					}
					wantMul, wantDiv := new(big.Rat).Mul(l1, l2), new(big.Rat).Quo(l1, l2)
					// Sin exponente entero, el caso de la vía directa: m1·m2 y m1/m2
					// normales en float64 (si no, es el tramo de 010).
					if mode == "neither integer" && (!isNormal(m1*m2) || !isNormal(m1/m2)) {
						continue
					}
					a, b := ucum.Pair{Value: v1, Code: p1.u.code}, ucum.Pair{Value: v2, Code: p2.u.code}
					if want, ok := nearestOf(wantMul); ok {
						r.checked++
						if got, err := s.Multiply(a, b); err != nil || got.Value != want {
							r.fail("Multiply(%v %s, %v %s) = (%v, %v), want %v", v1, p1.u.code, v2, p2.u.code, got.Value, err, want)
						}
					}
					if want, ok := nearestOf(wantDiv); ok {
						r.checked++
						if got, err := s.Divide(a, b); err != nil || got.Value != want {
							r.fail("Divide(%v %s, %v %s) = (%v, %v), want %v", v1, p1.u.code, v2, p2.u.code, got.Value, err, want)
						}
					}
				}
			}
		}
		r.done(500)
	}
}

// Test 5: monotonía en la vía directa. Se prueba en ±4 vecinos float64 de
// cada punto, con factor 1 (B) y distinto de 1, en el sentido que corresponde
// (la escala que cuenta hacia abajo, un operando negativo o la división lo
// invierten), y alrededor de la frontera normal/subnormal del resultado.
func TestLogFactorMonotonic(t *testing.T) {
	s := newService(t)
	units := loadLogUnits(t)
	type op struct {
		name string
		dir  float64 // +1 si el resultado crece con v (sin el signo de la escala)
		do   func(v float64, code string) (float64, error)
	}
	pr := func(v float64, code string) ucum.Pair { return ucum.Pair{Value: v, Code: code} }
	ops := []op{
		{"Canonical", 1, func(v float64, code string) (float64, error) {
			p, err := s.Canonical(v, code)
			return p.Value, err
		}},
		{"Multiply by 3", 1, func(v float64, code string) (float64, error) {
			p, err := s.Multiply(pr(v, code), pr(3, "1"))
			return p.Value, err
		}},
		{"Multiply by -3", -1, func(v float64, code string) (float64, error) {
			p, err := s.Multiply(pr(-3, "1"), pr(v, code))
			return p.Value, err
		}},
		{"Multiply by 1.1 [in_i]", 1, func(v float64, code string) (float64, error) {
			p, err := s.Multiply(pr(v, code), pr(1.1, "[in_i]"))
			return p.Value, err
		}},
		{"Divide by 0.7", 1, func(v float64, code string) (float64, error) {
			p, err := s.Divide(pr(v, code), pr(0.7, "1"))
			return p.Value, err
		}},
		{"1.1 divided", -1, func(v float64, code string) (float64, error) {
			p, err := s.Divide(pr(1.1, "1"), pr(v, code))
			return p.Value, err
		}},
		{"-1.1 divided", 1, func(v float64, code string) (float64, error) {
			p, err := s.Divide(pr(-1.1, "1"), pr(v, code))
			return p.Value, err
		}},
	}
	for _, code := range []string{"B", "dB", "[pH]", "B[SPL]", "B[W]", "B[kW]", "nB", "[hp'_C]"} {
		u := unitByCode(t, units, code)
		refLog := math.Log10(refFloat(u))
		pts := pointsOf(t, s, u, func(n, lo, hi int) bool {
			// n = 0 entra: v = 0 es la potencia exacta 1, y sus vecinos ±5e-324,
			// sin exponente entero, dan math.Pow = 1 (plan 011, §10).
			border := math.Abs(float64(n)*math.Log10(float64(u.base))+refLog) >= 305
			return n%17 == 0 || n >= -6 && n <= 6 || border || n <= lo+2 || n >= hi-2
		})
		for _, o := range ops {
			sign := o.dir
			if u.negate {
				sign = -sign
			}
			off, checked, inner := 0, 0, 0
			for _, p := range pts {
				var vals [9]float64
				var errs [9]error
				vals[4], errs[4] = o.do(p.v, code)
				lo, hi := p.v, p.v
				for i := 1; i <= 4; i++ {
					lo, hi = math.Nextafter(lo, math.Inf(-1)), math.Nextafter(hi, math.Inf(1))
					vals[4-i], errs[4-i] = o.do(lo, code)
					vals[4+i], errs[4+i] = o.do(hi, code)
				}
				// Solo junto al borde la operación puede desbordar: |log10| del
				// canónico del punto de al menos 300 (el mayor float64 es 1.8e308
				// y las operaciones lo mueven a lo más una década: por 3, 0.7, 1.1
				// o [in_i]). Lejos del borde, ErrOutOfRange es un defecto.
				c, _ := p.value().Float64()
				edge := math.Abs(math.Log10(math.Abs(c))) >= 300
				if !edge {
					inner++
				}
				prev, have := 0.0, false
				for i := range vals {
					if edge && errors.Is(errs[i], ucum.ErrOutOfRange) {
						continue
					}
					if errs[i] != nil || math.IsNaN(vals[i]) || math.IsInf(vals[i], 0) {
						t.Errorf("%s %s around %v: neighbor %d gives (%v, %v)", o.name, code, p.v, i-4, vals[i], errs[i])
						continue
					}
					if have {
						checked++
						if sign*(vals[i]-prev) < 0 {
							off++
							if off <= 3 {
								t.Errorf("%s %s around %v: %v then %v; not monotonic", o.name, code, p.v, prev, vals[i])
							}
						}
					}
					prev, have = vals[i], true
				}
			}
			if off > 3 {
				t.Errorf("%s %s: %d of %d steps not monotonic", o.name, code, off, checked)
			}
			// Que el desborde del borde no vacíe la prueba: todo punto lejos
			// del borde compara sus 8 pasos.
			if want := 8 * inner; checked < want {
				t.Errorf("%s %s: %d steps checked, want at least %d", o.name, code, checked, want)
			}
		}
	}
}

// refFloat es el factor de la referencia de la definición como float64.
func refFloat(u logUnit) float64 {
	f, _ := u.refVal.Float64()
	return f
}

// Test 6, guarda de regresión (no derivada de la especificación): lo que no
// cambia es la fórmula de main, round(bin(math.Pow) · factor): exponentes no
// enteros en todas las unidades, Np, cNp, Cel y [p'diop].
func TestLogFactorUnchangedOutsideIntegerExponents(t *testing.T) {
	s := newService(t)
	units, _ := factorGrid(t, s)
	vs := make([]float64, 0, 70)
	vs = append(vs, 7.4, 0.5, -0.5, 2.25, 1e-17, 0.37, -250.6, 123.37, 299.9, -2.0000001)
	for i := range 60 {
		vs = append(vs, float64(i)*5.13-150.3)
	}
	r := &report{t: t, name: "non-integer exponents"}
	for _, u := range units {
		if u.alpha.Cmp(big.NewRat(1, 1)) != 0 {
			continue
		}
		ref := refFactor(t, s, u)
		for _, v := range vs {
			e := v / u.divisor
			if e == math.Trunc(e) {
				continue
			}
			if u.negate {
				e = -e
			}
			m := math.Pow(float64(u.base), e)
			if !isNormal(m) {
				continue
			}
			want, ok := nearestOf(new(big.Rat).Mul(new(big.Rat).SetFloat64(m), ref.Value))
			if !ok || !isNormal(want) {
				continue
			}
			r.checked++
			got, err := s.Canonical(v, u.code)
			if err != nil || got.Value != want {
				r.fail("Canonical(%v, %q) = (%v, %v), want %v as on main", v, u.code, got.Value, err, want)
			}
		}
	}
	r.done(500)

	// Np (base e, sin valor racional), cNp (α = 1/100), Cel (v + 273.15 exacto,
	// UCUM §21) y [p'diop] (atan(v/100), §44).
	for _, v := range []float64{-30, -1, 0, 1, 2, 3, 7.4, 100, -150.5} {
		if p, err := s.Canonical(v, "Np"); err != nil || p.Value != math.Pow(math.E, v) {
			t.Errorf("Canonical(%v, \"Np\") = (%v, %v), want %v", v, p.Value, err, math.Pow(math.E, v))
		}
		alpha := 0.01
		if p, err := s.Canonical(v, "cNp"); err != nil || p.Value != math.Pow(math.E, v*alpha) {
			t.Errorf("Canonical(%v, \"cNp\") = (%v, %v), want %v", v, p.Value, err, math.Pow(math.E, v*alpha))
		}
		celsius, _ := new(big.Rat).Add(new(big.Rat).SetFloat64(v), big.NewRat(27315, 100)).Float64()
		if p, err := s.Canonical(v, "Cel"); err != nil || p.Value != celsius {
			t.Errorf("Canonical(%v, \"Cel\") = (%v, %v), want %v", v, p.Value, err, celsius)
		}
		if p, err := s.Canonical(v, "[p'diop]"); err != nil || p.Value != math.Atan(v/100) {
			t.Errorf("Canonical(%v, \"[p'diop]\") = (%v, %v), want %v", v, p.Value, err, math.Atan(v/100))
		}
	}
}

// outcome es lo que da redondear un valor exacto una vez, con la regla de
// rango de #59: ±Inf, o cero desde un valor distinto de cero, es
// ErrOutOfRange; un subnormal es un valor.
func outcome(r *big.Rat) (v float64, rangeErr bool) {
	f, _ := r.Float64()
	if math.IsInf(f, 0) || f == 0 && r.Sign() != 0 {
		return 0, true
	}
	return f, false
}

// Test 7: los bordes de rango, con esperados exactos, a cada lado de los
// umbrales efectivos de redondeo: el punto medio entre MaxFloat64 y 2^1024, y
// la mitad del menor subnormal 2^-1075 (que empata hacia cero, el par).
func TestLogFactorRangeThresholds(t *testing.T) {
	s := newService(t)
	ex := s.(ucum.ExactService)
	inch, err := ex.CanonicalRat(big.NewRat(1, 1), "[in_i]")
	if err != nil {
		t.Fatal(err)
	}
	maxF := new(big.Rat).SetFloat64(math.MaxFloat64)
	// MaxFloat64 + 2^970 = 2^1024 - 2^970, el punto medio hacia 2^1024.
	overflow := new(big.Rat).Add(maxF, new(big.Rat).SetInt(new(big.Int).Lsh(big.NewInt(1), 970)))
	underflow := new(big.Rat).SetFrac(big.NewInt(1), new(big.Int).Lsh(big.NewInt(1), 1075))
	check := func(a, b ucum.Pair, div bool, exact *big.Rat) {
		t.Helper()
		want, rangeErr := outcome(exact)
		op, name := s.Multiply, "Multiply"
		if div {
			op, name = s.Divide, "Divide"
		}
		p, err := op(a, b)
		switch {
		case rangeErr && !errors.Is(err, ucum.ErrOutOfRange):
			t.Errorf("%s(%v %s, %v %s) = (%v, %v), want ErrOutOfRange (exact %s)", name, a.Value, a.Code, b.Value, b.Code, p.Value, err, exact.FloatString(5))
		case !rangeErr && (err != nil || p.Value != want):
			t.Errorf("%s(%v %s, %v %s) = (%v, %v), want %v", name, a.Value, a.Code, b.Value, b.Code, p.Value, err, want)
		}
	}
	scan := func(threshold *big.Rat, code string, n int, other string, otherFactor *big.Rat, sign float64) {
		t.Helper()
		u := unitByCode(t, loadLogUnits(t), code)
		ref := refFactor(t, s, u)
		l := new(big.Rat).Mul(powRat(u.base, n), ref.Value)
		l.Mul(l, otherFactor)
		w, _ := new(big.Rat).Quo(threshold, l).Float64()
		v, ok := exponentValue(u, n)
		if !ok {
			t.Fatalf("no exact value for %s n = %d", code, n)
		}
		xs := make([]float64, 1, 7)
		xs[0] = w
		for _, dir := range []float64{math.Inf(-1), math.Inf(1)} {
			x := w
			for range 3 {
				x = math.Nextafter(x, dir)
				xs = append(xs, x)
			}
		}
		for _, x := range xs {
			ob := ucum.Pair{Value: sign * x, Code: other}
			lv := new(big.Rat).Mul(new(big.Rat).SetFloat64(sign*x), otherFactor)
			exact := new(big.Rat).Mul(new(big.Rat).Mul(powRat(u.base, n), ref.Value), lv)
			check(ucum.Pair{Value: v, Code: code}, ob, false, exact)
			check(ob, ucum.Pair{Value: v, Code: code}, false, exact)
			q := new(big.Rat).Quo(new(big.Rat).Mul(powRat(u.base, n), ref.Value), lv)
			if q.Sign() != 0 {
				check(ucum.Pair{Value: v, Code: code}, ob, true, q)
			}
		}
	}
	one := big.NewRat(1, 1)
	for _, sign := range []float64{1, -1} {
		scan(overflow, "B", 300, "1", one, sign)
		scan(overflow, "[pH]", -20, "1", one, sign)
		scan(overflow, "B[W]", 280, "[in_i]", inch.Value, sign)
		scan(underflow, "B", -300, "1", one, sign)
		scan(underflow, "B[SPL]", -600, "1", one, sign)
	}

	// Un empate exacto hacia el par: 2^-1074 · 1/2 = 2^-1075 va a cero (error de
	// rango), y apenas más es el menor subnormal, que es un valor.
	bit := ucum.Pair{Value: -1074, Code: "bit_s"}
	check(bit, ucum.Pair{Value: 0.5, Code: "1"}, false, underflow)
	check(bit, ucum.Pair{Value: math.Nextafter(0.5, 1), Code: "1"}, false,
		new(big.Rat).Mul(powRat(2, -1074), new(big.Rat).SetFloat64(math.Nextafter(0.5, 1))))
	// Un subnormal como resultado de un exponente entero es un valor.
	if p, err := s.Canonical(-1074, "bit_s"); err != nil || p.Value != 0x1p-1074 {
		t.Errorf("Canonical(-1074, bit_s) = (%v, %v), want 2^-1074", p.Value, err)
	}
	if _, err := s.Canonical(-1075, "bit_s"); !errors.Is(err, ucum.ErrOutOfRange) {
		t.Errorf("Canonical(-1075, bit_s) error = %v, want ErrOutOfRange (the tie goes to zero)", err)
	}
}

// Test 7b: división por cero con dividendo de potencia exacta, y las
// entradas no finitas.
func TestLogFactorZeroDivisorAndNonFinite(t *testing.T) {
	s := newService(t)
	for _, z := range []float64{0, math.Copysign(0, -1)} {
		for _, a := range []ucum.Pair{{Value: 3, Code: "B"}, {Value: 2, Code: "[pH]"}, {Value: -320, Code: "B"}, {Value: 1, Code: "B[SPL]"}} {
			for _, unit := range []string{"1", "m"} {
				if p, err := s.Divide(a, ucum.Pair{Value: z, Code: unit}); !errors.Is(err, ucum.ErrDivisionByZero) {
					t.Errorf("Divide(%v %s, %v %s) = (%v, %v), want ErrDivisionByZero", a.Value, a.Code, z, unit, p.Value, err)
				}
			}
		}
	}
	// #55: una entrada no finita pasa sin verificar, sin pánico, con la
	// aritmética de IEEE 754 sobre un valor canónico positivo y finito: x·c y
	// x/c tienen el signo de x (NaN sigue NaN), y c/x es cero. El signo del
	// cero no se compara: la biblioteca no lo conserva, tampoco en main ni con
	// unidades lineales (Divide(2 m, -Inf) es +0).
	same := func(got, want float64) bool {
		return math.IsNaN(want) && math.IsNaN(got) || got == want
	}
	for _, x := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		for _, code := range []string{"[pH]", "B[SPL]"} {
			a, b := ucum.Pair{Value: 2, Code: code}, ucum.Pair{Value: x, Code: "1"}
			times, over := x, math.Copysign(0, x)
			if math.IsNaN(x) {
				over = x
			}
			for _, tc := range []struct {
				name string
				do   func(a, b ucum.Pair) (ucum.Pair, error)
				a, b ucum.Pair
				want float64
			}{
				{"Multiply", s.Multiply, a, b, times},
				{"Multiply", s.Multiply, b, a, times},
				{"Divide", s.Divide, b, a, times},
				{"Divide", s.Divide, a, b, over},
			} {
				if p, err := tc.do(tc.a, tc.b); err != nil || !same(p.Value, tc.want) {
					t.Errorf("%s(%v %s, %v %s) = (%v, %v), want %v passed through", tc.name, tc.a.Value, tc.a.Code, tc.b.Value, tc.b.Code, p.Value, err, tc.want)
				}
			}
		}
	}
}

// Test 7c: los topes de costo de la potencia exacta: 10^310 es el tope de la
// potencia que cabe en float64 (310/311) y 10^5000 el de la potencia exacta
// fuera del rango normal (5000/5001). El cociente de dos escalas B es 10^10
// exacto hasta el tope de costo; más allá la vía log10 pierde unas cifras.
// (El exponente de una unidad escrita, 10*N, está limitado a ±1000: no se
// puede usar de destino.)
func TestLogFactorCostCaps(t *testing.T) {
	s := newService(t)
	want := 1e10
	for _, n := range []int{310, 311, 5000} {
		a, b := ucum.Pair{Value: float64(n), Code: "B"}, ucum.Pair{Value: float64(n - 10), Code: "B"}
		if p, err := s.Divide(a, b); err != nil || p.Value != want {
			t.Errorf("Divide(%d B, %d B) = (%v, %v), want %v", n, n-10, p.Value, err, want)
		}
	}
	p, err := s.Divide(ucum.Pair{Value: 5001, Code: "B"}, ucum.Pair{Value: 4991, Code: "B"})
	if err != nil || math.Abs(p.Value/want-1) > 1e-9 {
		t.Errorf("Divide(5001 B, 4991 B) = (%v, %v), want about %v", p.Value, err, want)
	}
}

// Test 9: una definición propia (NewFromReader) con sqrt sobre 3: el
// intermedio de la escala logarítmica es base^n/3 redondeado una vez, y
// después la f del destino (UCUM §22: r = f(m/u)/α, con sqrt la raíz). Con el
// XML incluido no se llega a este camino.
func TestLogFactorToSqrtDestination(t *testing.T) {
	const custom = `<?xml version="1.0" encoding="ascii"?>
<root xmlns="http://unitsofmeasure.org/ucum-essence" version="9.9" revision="N/A"
      revision-date="2099-01-01">
 <base-unit Code="m" CODE="M" dim="L"><name>meter</name><printSymbol>m</printSymbol><property>length</property></base-unit>
 <unit Code="B" CODE="B" isMetric="yes" isSpecial="yes" class="levels">
  <name>bel</name><printSymbol>B</printSymbol><property>level</property>
  <value Unit="lg(1 1)" UNIT="LG(1 1)"><function name="lg" value="1" Unit="1"/></value>
 </unit>
 <unit Code="[sq3]" CODE="[SQ3]" isMetric="no" isSpecial="yes" class="misc">
  <name>root of a third</name><printSymbol/><property>test</property>
  <value Unit="sqrt(1 3)" UNIT="SQRT(1 3)"><function name="sqrt" value="1" Unit="3"/></value>
 </unit>
</root>`
	svc, err := ucum.NewFromReader(strings.NewReader(custom))
	if err != nil {
		t.Fatal(err)
	}
	off, checked := 0, 0
	// 10^n/3 normal: n desde -307 (10^-307/3 = 3.3e-308 es normal).
	for n := -307; n <= 300; n++ {
		m, _ := new(big.Rat).Mul(powRat(10, n), big.NewRat(1, 3)).Float64()
		if !isNormal(m) {
			continue
		}
		want := math.Sqrt(m)
		checked++
		got, err := svc.Convert(float64(n), "B", "[sq3]")
		if err != nil || got != want {
			off++
			if off <= 5 {
				t.Errorf("Convert(%d, B, [sq3]) = (%v, %v), want %v", n, got, err, want)
			}
		}
	}
	if off > 5 {
		t.Errorf("%d of %d off", off, checked)
	}
	// El caso del plan: main da 1.8257418583505536e-152.
	if got, err := svc.Convert(-303, "B", "[sq3]"); err != nil || got != 1.8257418583505538e-152 {
		t.Errorf("Convert(-303, B, [sq3]) = (%v, %v), want 1.8257418583505538e-152", got, err)
	}
}

// Test 10: casos a mano, con el factor escrito desde el texto del XML:
// mol = 6.02214076e23, l = dm3 = 10^-3 m3, B[SPL] = 2·10^-5 Pa,
// Pa = N/m2, N = kg.m/s2, W = J/s, J = N.m (el canónico usa g: 1 kg = 10^3 g).
func TestLogFactorByHand(t *testing.T) {
	s := newService(t)
	for _, tc := range []struct {
		v    float64
		code string
		want float64
		by   string
	}{
		// pH: 10^-v mol/l = 10^-v · 6.02214076e23 / 10^-3 m3 = 6.02214076e26 · 10^-v.
		{1, "[pH]", 6.02214076e+25, "10^-1 · 6.02214076e26"},
		{2, "[pH]", 6.02214076e+24, "10^-2 · 6.02214076e26"},
		{-3, "[pH]", 6.02214076e+29, "10^3 · 6.02214076e26"},
		// B[SPL] = 2lg(2·10^-5 Pa): 10^(v/2) · 2·10^-5 Pa, y 1 Pa = 10^3 g/(m.s2).
		{-2, "B[SPL]", 2e-3, "10^-1 · 2·10^-5 · 10^3"},
		{2, "B[SPL]", 2e-1, "10^1 · 2·10^-5 · 10^3"},
		// B[W]: 10^v W, y 1 W = 10^3 g.m2/s3.
		{-7, "B[W]", 1e-4, "10^-7 · 10^3"},
		{2, "B[W]", 1e5, "10^2 · 10^3"},
	} {
		p, err := s.Canonical(tc.v, tc.code)
		if err != nil || p.Value != tc.want {
			t.Errorf("Canonical(%v, %q) = (%v, %v), want %v (%s)", tc.v, tc.code, p.Value, err, tc.want, tc.by)
		}
	}
}
