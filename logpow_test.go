package ucum_test

import (
	"math"
	"math/big"
	"math/rand/v2"
	"testing"

	"github.com/gofhir/ucum/v4"
)

// Plan 009 (#75): en una escala logarítmica de base entera, con exponente
// entero, Canonical da el float64 más cercano a la potencia exacta. Los
// esperados salen de la definición de cada función (UCUM §22,
// ucum-essence.xml: lg es 10^x, hpC es 100^-x…) evaluada con big.Rat, no del
// código de la librería.

// nearestPow es el float64 más cercano a base^n, redondeado una vez.
func nearestPow(base int64, n int) float64 {
	m := n
	if m < 0 {
		m = -m
	}
	p := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(base), big.NewInt(int64(m)), nil))
	if n < 0 {
		p.Inv(p)
	}
	f, _ := p.Float64()
	return f
}

// exactStep dice si v es exactamente n·perStep: si no, el exponente de v no
// es n (n·1e21 no siempre es un float64) y el caso no prueba nada.
func exactStep(v float64, n int, perStep float64) bool {
	want := new(big.Rat).Mul(new(big.Rat).SetInt64(int64(n)), new(big.Rat).SetFloat64(perStep))
	return new(big.Rat).SetFloat64(v).Cmp(want) == 0
}

// isNormal: el plan excluye los resultados subnormales (§3).
func isNormal(f float64) bool {
	return !math.IsInf(f, 0) && math.Abs(f) >= 0x1p-1022
}

func newService(t *testing.T) ucum.Service {
	t.Helper()
	s, err := ucum.New()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// Test 1: la rejilla de exponentes enteros, en las unidades sin factor y en
// los prefijos de B. Se prueba en v = ±n/α, el valor de la escala cuyo
// exponente es n.
func TestLogScaleIntegerExponentIsNearest(t *testing.T) {
	s := newService(t)
	for _, tc := range []struct {
		code string
		base int64
		// perStep es el v que sube el exponente en uno: 1/α, con su signo.
		perStep float64
	}{
		{"B", 10, 1},
		{"dB", 10, 10},
		{"cB", 10, 100},
		{"mB", 10, 1000},
		// α = 10^-9, 10^-15, 10^-18, 10^-21 no son float64: v·α no siempre da
		// el entero, y decide el exponente exacto.
		{"nB", 10, 1e9},
		{"fB", 10, 1e15},
		{"aB", 10, 1e18},
		{"zB", 10, 1e21},
		{"[hp'_X]", 10, -1},
		{"[hp'_C]", 100, -1},
		{"[hp'_M]", 1000, -1},
		{"[hp'_Q]", 50000, -1},
		{"bit_s", 2, 1},
	} {
		checked, off := 0, 0
		for n := -1100; n <= 1100; n++ {
			want := nearestPow(tc.base, n)
			if !isNormal(want) || want == 0 {
				continue
			}
			v := float64(n) * tc.perStep
			if !exactStep(v, n, tc.perStep) {
				continue
			}
			p, err := s.Canonical(v, tc.code)
			checked++
			if err != nil || p.Value != want {
				off++
				if off <= 3 {
					t.Errorf("Canonical(%v, %q) = (%v, %v), want %v, the nearest float64 of %d^%d", v, tc.code, p.Value, err, want, tc.base, n)
				}
			}
		}
		if off > 3 {
			t.Errorf("%s: %d of %d exponents off the nearest float64", tc.code, off, checked)
		}
		t.Logf("%s: %d exponents checked", tc.code, checked)
		if checked < 30 {
			t.Errorf("%s: only %d exponents checked", tc.code, checked)
		}
	}
}

// Test 3, guarda de regresión (no derivada de la especificación): con
// exponente no entero el resultado no cambia respecto de main, que es
// math.Pow(base, e) en las unidades sin factor.
func TestLogScaleNonIntegerExponentUnchanged(t *testing.T) {
	s := newService(t)
	r := rand.New(rand.NewPCG(9, 75))
	vs := make([]float64, 0, 310)
	// Los tres últimos pasan el filtro barato (a menos de 1e-9 de un entero)
	// y los descarta el chequeo exacto.
	vs = append(vs, 7.4, 0.5, -0.5, 1e-17, 2.25, -307.5, 300.1, 300.00000001, -2.0000000000001, 1+0x1p-40)
	for range 300 {
		vs = append(vs, (r.Float64()*600-300)+0.37)
	}
	for _, v := range vs {
		if v == math.Trunc(v) {
			continue
		}
		for _, tc := range []struct {
			code string
			want float64
		}{
			{"B", math.Pow(10, v)},
			{"[hp'_C]", math.Pow(100, -v)},
			{"bit_s", math.Pow(2, v)},
		} {
			if !isNormal(tc.want) {
				continue
			}
			p, err := s.Canonical(v, tc.code)
			if err != nil || p.Value != tc.want {
				t.Errorf("Canonical(%v, %q) = (%v, %v), want %v as on main", v, tc.code, p.Value, err, tc.want)
			}
		}
	}
	// Con prefijo, un exponente casi entero pasa el filtro y lo descarta el
	// exponente exacto: 2900.0000001 dB no es 10^290.
	tenth := 0.1
	for _, v := range []float64{2900.0000001, -30.000000001, 2040 + 0x1p-30} {
		want := math.Pow(10, v*tenth)
		p, err := s.Canonical(v, "dB")
		if err != nil || p.Value != want {
			t.Errorf("Canonical(%v, \"dB\") = (%v, %v), want %v as on main", v, p.Value, err, want)
		}
	}
}

// Test 4: las fronteras del plan, que siguen como en main.
func TestLogScaleDocumentedBoundaries(t *testing.T) {
	s := newService(t)
	// Una variable, para que el valor sea el de math.Pow en tiempo de
	// ejecución, el que da main.
	three := 3.0
	for _, tc := range []struct {
		v    float64
		code string
		want float64
		why  string
	}{
		// Opción A: 10^-1 ya es el float64 más cercano; el factor de la
		// unidad redondea otra vez (#75, opción B).
		{1, "[pH]", 6.022140760000001e+25, "the unit's factor rounds a second time"},
		// 0.3·1000 da 300 en float64, el exponente que usa math.Pow: el mismo
		// resultado que 300 B, como en main.
		{0.3, "kB", 1e300, "0.3 kB is 300 B"},
		// Subnormal: mapScalar lo rehace por log10.
		{-308, "B", 9.999999999999994e-309, "a subnormal result goes through log10"},
		// ln no tiene base entera.
		{3, "Np", math.Pow(math.E, three), "the base of ln is not an integer"},
	} {
		p, err := s.Canonical(tc.v, tc.code)
		if err != nil || p.Value != tc.want {
			t.Errorf("Canonical(%v, %q) = (%v, %v), want %v: %s", tc.v, tc.code, p.Value, err, tc.want, tc.why)
		}
	}
	if p, err := s.Canonical(300, "B"); err != nil || p.Value != 1e300 {
		t.Errorf("Canonical(300, \"B\") = (%v, %v), want 1e300", p.Value, err)
	}
}

// Test 5: Canonical es monótona en una escala de base entera (UCUM §22: la
// función es base^x, monótona en x). Se prueba en cada exponente entero y en
// sus vecinos en float64, donde la potencia exacta se encuentra con
// math.Pow.
func TestLogScaleMonotonicAtIntegerExponents(t *testing.T) {
	s := newService(t)
	for _, tc := range []struct {
		code    string
		base    int64
		perStep float64
	}{
		{"B", 10, 1}, {"dB", 10, 10}, {"cB", 10, 100}, {"mB", 10, 1000},
		{"nB", 10, 1e9}, {"fB", 10, 1e15},
		{"[hp'_C]", 100, -1}, {"[hp'_Q]", 50000, -1},
	} {
		off := 0
		for n := -1100; n <= 1100; n++ {
			if !isNormal(nearestPow(tc.base, n)) {
				continue
			}
			v := float64(n) * tc.perStep
			if !exactStep(v, n, tc.perStep) {
				continue
			}
			lo, hi := math.Nextafter(v, math.Inf(-1)), math.Nextafter(v, math.Inf(1))
			pl, err1 := s.Canonical(lo, tc.code)
			pv, err2 := s.Canonical(v, tc.code)
			ph, err3 := s.Canonical(hi, tc.code)
			if err1 != nil || err2 != nil || err3 != nil {
				continue
			}
			ok := pl.Value <= pv.Value && pv.Value <= ph.Value
			if tc.perStep < 0 {
				ok = pl.Value >= pv.Value && pv.Value >= ph.Value
			}
			if !ok {
				off++
				if off <= 3 {
					t.Errorf("%s around %v: Canonical gives %v, %v, %v; not monotonic", tc.code, v, pl.Value, pv.Value, ph.Value)
				}
			}
		}
		if off > 3 {
			t.Errorf("%s: %d integer exponents where Canonical is not monotonic", tc.code, off)
		}
	}
}
