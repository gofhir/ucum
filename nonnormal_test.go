package ucum_test

import (
	"errors"
	"math"
	"math/big"
	"testing"

	"github.com/gofhir/ucum/v4"
	"github.com/gofhir/ucum/v4/internal/essence"
)

// Plan 010 (#75): donde la vía float64 materializa por log10 y todo operando
// tiene valor exacto, el resultado es el float64 más cercano al valor exacto.
// Los esperados salen de la función de cada escala (UCUM §22) por el factor de
// su definición en ucum-essence.xml, leído con la API exacta.

// logUnit es una unidad logarítmica de base entera con su definición del XML:
// <value Unit="2lg(2 10*-5.Pa)"> es función lgTimes2, valor 2, unidad 10*-5.Pa.
type logUnit struct {
	code    string
	base    int64
	perStep float64 // el v que sube el exponente en uno, con su signo
	refVal  *big.Rat
	refUnit string
}

// logFunctions es la semántica de cada función de base entera (UCUM §46
// Tabla 20 y §47 Tabla 22): base, divisor del exponente y si cuenta hacia
// abajo. Valor, unidad y prefijo salen del XML.
var logFunctions = map[string]struct {
	base    int64
	divisor float64
	negate  bool
}{
	"lg": {10, 1, false}, "lgTimes2": {10, 2, false}, "ld": {2, 1, false},
	"pH": {10, 1, true}, "hpX": {10, 1, true}, "hpC": {100, 1, true},
	"hpM": {1000, 1, true}, "hpQ": {50000, 1, true},
}

// logUnitCodes son las unidades del test, como prefijo y átomo del XML.
var logUnitCodes = []struct{ prefix, atom string }{
	{"", "B"}, {"d", "B"}, {"n", "B"}, {"f", "B"},
	{"", "B[SPL]"}, {"", "B[V]"}, {"", "B[mV]"}, {"", "B[uV]"}, {"", "B[10.nV]"},
	{"", "B[W]"}, {"", "B[kW]"}, {"", "[pH]"},
	{"", "[hp'_X]"}, {"", "[hp'_C]"}, {"", "[hp'_M]"}, {"", "[hp'_Q]"},
	{"", "bit_s"},
}

// loadLogUnits arma la tabla desde ucum-essence.xml.
func loadLogUnits(t *testing.T) []logUnit {
	t.Helper()
	m, err := essence.Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]logUnit, 0, len(logUnitCodes))
	for _, c := range logUnitCodes {
		atom := m.Lookup(c.atom)
		if atom == nil || atom.Value == nil || atom.Value.Function == nil {
			t.Fatalf("%s: not a special unit in the XML", c.atom)
		}
		fn := atom.Value.Function
		sem, ok := logFunctions[fn.Name]
		if !ok {
			t.Fatalf("%s: function %s has no integer base", c.atom, fn.Name)
		}
		// perStep = ±divisor/α, con α el factor del prefijo.
		step := new(big.Rat).SetFloat64(sem.divisor)
		if c.prefix != "" {
			p := m.LookupPrefix(c.prefix)
			if p == nil {
				t.Fatalf("prefix %s not in the XML", c.prefix)
			}
			step.Quo(step, p.Value.Rat())
		}
		if sem.negate {
			step.Neg(step)
		}
		perStep, _ := step.Float64()
		out = append(out, logUnit{c.prefix + c.atom, sem.base, perStep, fn.Value.Rat(), fn.Unit})
	}
	return out
}

func unitByCode(t *testing.T, units []logUnit, code string) logUnit {
	t.Helper()
	for _, u := range units {
		if u.code == code {
			return u
		}
	}
	t.Fatalf("%s not in the table", code)
	return logUnit{}
}

func powRat(base int64, n int) *big.Rat {
	m := n
	if m < 0 {
		m = -m
	}
	p := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(base), big.NewInt(int64(m)), nil))
	if n < 0 {
		p.Inv(p)
	}
	return p
}

// refFactor es el valor canónico de la referencia de la definición.
func refFactor(t *testing.T, s ucum.Service, u logUnit) ucum.RatPair {
	t.Helper()
	p, err := s.(ucum.ExactService).CanonicalRat(u.refVal, u.refUnit)
	if err != nil {
		t.Fatalf("CanonicalRat(%s %s): %v", u.refVal.RatString(), u.refUnit, err)
	}
	return p
}

// fits: el valor exacto redondea a un float64 finito y distinto de cero.
func fits(r *big.Rat) (float64, bool) {
	f, _ := r.Float64()
	return f, !math.IsInf(f, 0) && f != 0
}

func nonNormal(r *big.Rat) bool {
	f, _ := r.Float64()
	return math.IsInf(f, 0) || math.Abs(f) < 0x1p-1022
}

// Test 1: la rejilla del tramo que hoy va por log10 (intermedio base^n no
// normal) con resultado que cabe.
func TestNonNormalIntermediateIsNearest(t *testing.T) {
	s := newService(t)
	for _, u := range loadLogUnits(t) {
		ref := refFactor(t, s, u)
		checked, off := 0, 0
		for n := -1200; n <= 1200; n++ {
			v := float64(n) * u.perStep
			if !exactStep(v, n, u.perStep) {
				continue
			}
			// perStep ya lleva el signo de las escalas que cuentan hacia abajo.
			pow := powRat(u.base, n)
			if !nonNormal(pow) {
				continue
			}
			want, ok := fits(new(big.Rat).Mul(pow, ref.Value))
			if !ok {
				continue
			}
			checked++
			p, err := s.Canonical(v, u.code)
			if err != nil || p.Value != want || p.Code != ref.Code {
				off++
				if off <= 3 {
					t.Errorf("Canonical(%v, %q) = (%v %q, %v), want %v %q", v, u.code, p.Value, p.Code, err, want, ref.Code)
				}
			}
		}
		if off > 3 {
			t.Errorf("%s: %d of %d off", u.code, off, checked)
		}
	}
}

// Test 2: Convert a una unidad lineal de escala muy distinta.
func TestNonNormalConvertIsNearest(t *testing.T) {
	s := newService(t)
	for _, tc := range []struct {
		v        float64
		from, to string
		want     *big.Rat
	}{
		{-600, "B", "10*-300", powRat(10, -300)},
		// 10^-310 mol/l en mmol/l es 10^-307.
		{310, "[pH]", "mmol/L", powRat(10, -307)},
		// 618 B[SPL] = 10^309 · 2·10^-5 Pa = 2·10^304 Pa.
		{618, "B[SPL]", "Pa", new(big.Rat).Mul(powRat(10, 304), big.NewRat(2, 1))},
		// 10^-900 en 10^-600 es 10^-300: el exponente pasa de 600 y sigue
		// exacto (tope de costo, no de float64).
		{-900, "B", "10*-600", powRat(10, -300)},
	} {
		want, _ := tc.want.Float64()
		got, err := s.Convert(tc.v, tc.from, tc.to)
		if err != nil || got != want {
			t.Errorf("Convert(%v, %q, %q) = (%v, %v), want %v", tc.v, tc.from, tc.to, got, err, want)
		}
	}
}

// Test 3: Multiply y Divide.
func TestNonNormalProductIsNearest(t *testing.T) {
	s := newService(t)
	ex := s.(ucum.ExactService)
	molL, err := ex.CanonicalRat(big.NewRat(1, 1), "mol/l")
	if err != nil {
		t.Fatal(err)
	}
	bin := func(f float64) *big.Rat { return new(big.Rat).SetFloat64(f) }
	for _, tc := range []struct {
		name string
		a, b ucum.Pair
		div  bool
		want *big.Rat
	}{
		{"-308 B · 1", ucum.Pair{Value: -308, Code: "B"}, ucum.Pair{Value: 1, Code: "1"}, false, powRat(10, -308)},
		{"-308 B · 2 m", ucum.Pair{Value: -308, Code: "B"}, ucum.Pair{Value: 2, Code: "m"}, false, new(big.Rat).Mul(powRat(10, -308), big.NewRat(2, 1))},
		{"-600 B · 1e300", ucum.Pair{Value: -600, Code: "B"}, ucum.Pair{Value: 1e300, Code: "1"}, false, new(big.Rat).Mul(powRat(10, -600), bin(1e300))},
		{"-308 B / 2 m", ucum.Pair{Value: -308, Code: "B"}, ucum.Pair{Value: 2, Code: "m"}, true, new(big.Rat).Quo(powRat(10, -308), big.NewRat(2, 1))},
		// Operandos normales, resultado no normal.
		{"170 [pH] · 150 [pH]", ucum.Pair{Value: 170, Code: "[pH]"}, ucum.Pair{Value: 150, Code: "[pH]"}, false,
			new(big.Rat).Mul(powRat(10, -320), new(big.Rat).Mul(molL.Value, molL.Value))},
		{"1 / 308 B", ucum.Pair{Value: 1, Code: "1"}, ucum.Pair{Value: 308, Code: "B"}, true, powRat(10, -308)},
		// Un operando de escala lineal aporta el binario del float64 que da su
		// escala, como en el resto de la librería: Cel es v + 273.15 K (UCUM
		// §21-22), sumado en float64.
		{"-308 B · 1 Cel", ucum.Pair{Value: -308, Code: "B"}, ucum.Pair{Value: 1, Code: "Cel"}, false, new(big.Rat).Mul(powRat(10, -308), bin(1+273.15))},
		// Un operando lineal subnormal es su binario exacto: no es 1e-310.
		// El factor de la unidad devuelve el resultado al rango: el tope mira
		// el resultado con el factor, no los operandos.
		{"-600 B · 1 10*300", ucum.Pair{Value: -600, Code: "B"}, ucum.Pair{Value: 1, Code: "10*300"}, false, powRat(10, -300)},
		{"1 10*300 / 600 B", ucum.Pair{Value: 1, Code: "10*300"}, ucum.Pair{Value: 600, Code: "B"}, true, powRat(10, -300)},
		// Exponente no entero con intermedio normal: aporta el binario del
		// float64 que da su escala, 10^-170.5 por math.Pow (UCUM §22, pH es
		// -lg), antes del factor; el factor (mol/l)² va exacto.
		{"170.5 [pH] · 150 [pH]", ucum.Pair{Value: 170.5, Code: "[pH]"}, ucum.Pair{Value: 150, Code: "[pH]"}, false,
			new(big.Rat).Mul(bin(math.Pow(10, -170.5)), new(big.Rat).Mul(powRat(10, -150), new(big.Rat).Mul(molL.Value, molL.Value)))},
		// Un operando normal de exponente entero aporta la potencia exacta,
		// no el float64 de math.Pow.
		{"2 [pH] · 307 [pH]", ucum.Pair{Value: 2, Code: "[pH]"}, ucum.Pair{Value: 307, Code: "[pH]"}, false,
			new(big.Rat).Mul(powRat(10, -309), new(big.Rat).Mul(molL.Value, molL.Value))},
		// Prefijos n, f, a: el exponente float64 v·α no es entero
		// (-457.00000000000006), el exacto sí (regla de 009), y decide la
		// potencia exacta, no la partida.
		{"-4.57e11 nB · 1 10*457", ucum.Pair{Value: -4.57e11, Code: "nB"}, ucum.Pair{Value: 1, Code: "10*457"}, false, big.NewRat(1, 1)},
		{"-3.66e17 fB · 1 10*366", ucum.Pair{Value: -3.66e17, Code: "fB"}, ucum.Pair{Value: 1, Code: "10*366"}, false, big.NewRat(1, 1)},
		{"-3.98e20 aB · 1 10*398", ucum.Pair{Value: -3.98e20, Code: "aB"}, ucum.Pair{Value: 1, Code: "10*398"}, false, big.NewRat(1, 1)},
		// Resultado subnormal: el tope de la multiplicación no corta en 10^-309.
		{"-310 B · 3", ucum.Pair{Value: -310, Code: "B"}, ucum.Pair{Value: 3, Code: "1"}, false, new(big.Rat).Mul(powRat(10, -310), big.NewRat(3, 1))},
		// El signo del otro operando se conserva.
		{"-600 B · -1e300", ucum.Pair{Value: -600, Code: "B"}, ucum.Pair{Value: -1e300, Code: "1"}, false, new(big.Rat).Mul(powRat(10, -600), bin(-1e300))},
		{"320 B · 1e-310", ucum.Pair{Value: 320, Code: "B"}, ucum.Pair{Value: 1e-310, Code: "1"}, false, new(big.Rat).Mul(powRat(10, 320), bin(1e-310))},
	} {
		want, _ := tc.want.Float64()
		op := s.Multiply
		if tc.div {
			op = s.Divide
		}
		p, err := op(tc.a, tc.b)
		if err != nil || p.Value != want {
			t.Errorf("%s = (%v, %v), want %v", tc.name, p.Value, err, want)
		}
	}
}

// Test 4, guarda de regresión (no derivada de la especificación): donde hoy
// no se pasa por log10, el resultado es el de main; con exponente no entero en
// el tramo no normal, la potencia partida.
func TestNonNormalLeavesOtherPathsAlone(t *testing.T) {
	s := newService(t)
	// Vía directa con factor: main redondea la potencia y luego la multiplicación
	// (opción B de #75, fuera).
	units := loadLogUnits(t)
	for _, code := range []string{"[pH]", "B[kW]", "B[SPL]"} {
		u := unitByCode(t, units, code)
		ref := refFactor(t, s, u)
		for n := -300; n <= 300; n++ {
			v := float64(n) * u.perStep
			// perStep ya lleva el signo de las escalas que cuentan hacia abajo.
			pow := powRat(u.base, n)
			if nonNormal(pow) {
				continue
			}
			pf, _ := pow.Float64()
			want, ok := fits(new(big.Rat).Mul(new(big.Rat).SetFloat64(pf), ref.Value))
			if !ok || math.Abs(want) < 0x1p-1022 {
				continue
			}
			p, err := s.Canonical(v, u.code)
			if err != nil || p.Value != want {
				t.Errorf("Canonical(%v, %q) = (%v, %v), want %v as on main (double rounding)", v, u.code, p.Value, err, want)
			}
		}
	}
	// Guarda de regresión (repite la implementación, no es oráculo; el
	// oráculo es TestNonNormalSplitAccuracy): un exponente no entero en el
	// tramo no normal vale 10^-308 · bin(math.Pow(10, -0.5)), redondeado una
	// vez con el factor (plan 010, §9).
	split := new(big.Rat).Mul(powRat(10, -308), new(big.Rat).SetFloat64(math.Pow(10, -0.5)))
	want, _ := split.Float64()
	if p, err := s.Canonical(-308.5, "B"); err != nil || p.Value != want {
		t.Errorf("Canonical(-308.5, \"B\") = (%v, %v), want %v", p.Value, err, want)
	}
	// Np (base e) no tiene valor racional: la multiplicación sigue por log10, sin
	// pánico, cerca de e^-710·10^300.
	if p, err := s.Multiply(ucum.Pair{Value: -710, Code: "Np"}, ucum.Pair{Value: 1e300, Code: "1"}); err != nil || math.Abs(p.Value/(math.Exp(-710)*1e300)-1) > 1e-12 {
		t.Errorf("Multiply(-710 Np, 1e300) = (%v, %v), want about %v", p.Value, err, math.Exp(-710)*1e300)
	}
	want, _ = new(big.Rat).Mul(split, big.NewRat(2, 1)).Float64()
	if p, err := s.Multiply(ucum.Pair{Value: -308.5, Code: "B"}, ucum.Pair{Value: 2, Code: "m"}); err != nil || p.Value != want {
		t.Errorf("Multiply(-308.5 B, 2 m) = (%v, %v), want %v", p.Value, err, want)
	}
}

// Test 5: monotonía en el tramo, con los vecinos en float64 (el de -3080 dB
// tiene el mismo exponente float64, -308).
func TestNonNormalMonotonic(t *testing.T) {
	s := newService(t)
	units := loadLogUnits(t)
	for _, code := range []string{"B", "dB", "B[SPL]", "[pH]"} {
		u := unitByCode(t, units, code)
		off := 0
		for n := -1200; n <= 1200; n++ {
			v := float64(n) * u.perStep
			lo, hi := math.Nextafter(v, math.Inf(-1)), math.Nextafter(v, math.Inf(1))
			pl, e1 := s.Canonical(lo, u.code)
			pv, e2 := s.Canonical(v, u.code)
			ph, e3 := s.Canonical(hi, u.code)
			if e1 != nil || e2 != nil || e3 != nil {
				continue
			}
			ok := pl.Value <= pv.Value && pv.Value <= ph.Value
			if u.perStep < 0 {
				ok = pl.Value >= pv.Value && pv.Value >= ph.Value
			}
			if !ok {
				off++
				if off <= 3 {
					t.Errorf("%s around %v: %v, %v, %v; not monotonic", u.code, v, pl.Value, pv.Value, ph.Value)
				}
			}
		}
		if off > 3 {
			t.Errorf("%s: %d points not monotonic", u.code, off)
		}
	}
}

// Test 6: errores y entradas no finitas.
func TestNonNormalErrorsAndNonFinite(t *testing.T) {
	s := newService(t)
	for _, tc := range []struct {
		v    float64
		code string
	}{{-400, "B"}, {700, "B[SPL]"}} {
		if _, err := s.Canonical(tc.v, tc.code); !errors.Is(err, ucum.ErrOutOfRange) {
			t.Errorf("Canonical(%v, %q) = %v, want ErrOutOfRange", tc.v, tc.code, err)
		}
	}
	b := ucum.Pair{Value: -308, Code: "B"}
	if _, err := s.Multiply(b, ucum.Pair{Value: 4.9e-324, Code: "1"}); !errors.Is(err, ucum.ErrOutOfRange) {
		t.Errorf("Multiply(-308 B, 4.9e-324) = %v, want ErrOutOfRange", err)
	}
	for _, z := range []float64{0, math.Copysign(0, -1)} {
		if _, err := s.Divide(ucum.Pair{Value: -320, Code: "B"}, ucum.Pair{Value: z, Code: "m"}); !errors.Is(err, ucum.ErrDivisionByZero) {
			t.Errorf("Divide(-320 B, %v m) = %v, want ErrDivisionByZero", z, err)
		}
	}
	// #55: una entrada no finita pasa sin verificar, como hoy.
	for _, x := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		for _, div := range []bool{false, true} {
			op := s.Multiply
			if div {
				op = s.Divide
			}
			if _, err := op(b, ucum.Pair{Value: x, Code: "1"}); err != nil {
				t.Errorf("Multiply/Divide(-308 B, %v) div=%v = %v, want it passed through with no error", x, div, err)
			}
		}
	}
}
