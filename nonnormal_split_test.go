package ucum_test

import (
	"fmt"
	"math"
	"math/big"
	"math/rand/v2"
	"testing"

	"github.com/gofhir/ucum/v4"
	"github.com/gofhir/ucum/v4/internal/essence"
)

// Plan 010, §9: en el tramo no normal, un exponente no entero vale
// base^n · math.Pow(base, f), con e = n + f y |f| ≤ ½, y sqrt vale (α·v)²
// exacto. Los esperados salen de la función de cada escala (UCUM §22,
// §45-48) en big.Float o big.Rat, con la referencia leída del XML.

// destFor es la unidad "3.10*K" por la referencia de u: lleva base^n·ref de
// vuelta al rango normal cuando K ≈ log10 de base^n.
func destFor(u logUnit, k int) string {
	if u.refUnit == "1" {
		return fmt.Sprintf("3.10*%d", k)
	}
	return fmt.Sprintf("3.10*%d.%s", k, u.refUnit)
}

// Test 7: monotonía con factor distinto de 1 (ronda 2 del code review):
// Multiply por 3 10*K y Convert a 3.10*K, alrededor de exponentes enteros y
// medios del tramo no normal, en todas las unidades de base entera.
func TestNonNormalMonotonicWithFactor(t *testing.T) {
	s := newService(t)
	for _, u := range loadLogUnits(t) {
		lb := math.Log10(float64(u.base))
		off := 0
		// Exponentes t = k/2 con |t·log10 base| entre 300 y 600, cada 23
		// medios: cruza la frontera normal y f = ±½.
		for k := int(math.Ceil(600 / lb)); k <= int(1200/lb); k += 23 {
			for _, sign := range []float64{-1, 1} {
				exp := sign * float64(k) / 2
				l := exp * lb // log10 de base^exp
				kk := int(math.Round(l))
				v := exp * u.perStep
				ops := []struct {
					name string
					f    func(float64) (float64, error)
				}{
					{"Multiply", func(x float64) (float64, error) {
						p, err := s.Multiply(ucum.Pair{Value: x, Code: u.code}, ucum.Pair{Value: 3, Code: fmt.Sprintf("10*%d", -kk)})
						return p.Value, err
					}},
					{"Convert", func(x float64) (float64, error) {
						return s.Convert(x, u.code, destFor(u, kk))
					}},
				}
				for _, op := range ops {
					x := v
					for range 4 {
						x = math.Nextafter(x, math.Inf(-1))
					}
					prev, have := 0.0, false
					for range 9 {
						got, err := op.f(x)
						if err != nil {
							t.Fatalf("%s(%v %s): %v", op.name, x, u.code, err)
						}
						if have && (u.perStep > 0 && got < prev || u.perStep < 0 && got > prev) {
							off++
							if off <= 3 {
								t.Errorf("%s(%v %s) = %v after %v: not monotonic", op.name, x, u.code, got, prev)
							}
						}
						prev, have = got, true
						x = math.Nextafter(x, math.Inf(1))
					}
				}
			}
		}
		if off > 3 {
			t.Errorf("%s: %d steps not monotonic", u.code, off)
		}
	}
	// Los testigos de la ronda 2.
	for _, tc := range []struct {
		v          float64
		code, dest string
		mul        bool
		down       bool
	}{
		{-966, "B[SPL]", "10*300", true, false},
		{125, "[hp'_Q]", "3.10*-300", false, true},
	} {
		at := func(x float64) float64 {
			if tc.mul {
				p, err := s.Multiply(ucum.Pair{Value: x, Code: tc.code}, ucum.Pair{Value: 3, Code: tc.dest})
				if err != nil {
					t.Fatal(err)
				}
				return p.Value
			}
			got, err := s.Convert(x, tc.code, tc.dest)
			if err != nil {
				t.Fatal(err)
			}
			return got
		}
		lo, mid, hi := at(math.Nextafter(tc.v, math.Inf(-1))), at(tc.v), at(math.Nextafter(tc.v, math.Inf(1)))
		ok := lo <= mid && mid <= hi
		if tc.down {
			ok = lo >= mid && mid >= hi
		}
		if !ok {
			t.Errorf("%v %s around %s: %v, %v, %v; not monotonic", tc.v, tc.code, tc.dest, lo, mid, hi)
		}
	}
}

// powFloat da base^(n + j/2^m) con prec bits: m raíces sucesivas de la base
// y potencias enteras, sin math.Pow.
func powFloat(base int64, n int, j int64, m, prec uint) *big.Float {
	b := new(big.Float).SetPrec(prec).SetInt64(base)
	root := new(big.Float).SetPrec(prec).Set(b)
	for range m {
		root.Sqrt(root)
	}
	ipow := func(x *big.Float, e int64) *big.Float {
		r := new(big.Float).SetPrec(prec).SetInt64(1)
		neg := e < 0
		if neg {
			e = -e
		}
		p := new(big.Float).SetPrec(prec).Set(x)
		for ; e > 0; e >>= 1 {
			if e&1 == 1 {
				r.Mul(r, p)
			}
			p.Mul(p, p)
		}
		if neg {
			r.Quo(new(big.Float).SetPrec(prec).SetInt64(1), r)
		}
		return r
	}
	out := ipow(b, int64(n))
	return out.Mul(out, ipow(root, j))
}

// ulpsOff es |got − want| en ulps de want.
func ulpsOff(got float64, want *big.Float) float64 {
	w, _ := want.Float64()
	ulp := math.Nextafter(math.Abs(w), math.Inf(1)) - math.Abs(w)
	d := new(big.Float).SetPrec(want.Prec()).SetFloat64(got)
	d.Sub(d, want)
	df, _ := d.Float64()
	return math.Abs(df) / ulp
}

// Test 8: exactitud de la potencia partida contra base^e en big.Float de 400
// bits, con fracciones diádicas al azar (no solo ½, donde math.Pow usa Sqrt).
// La cota es la medida en la ronda adversarial: math.Pow(base, f) calcula
// Exp(f·Log(base)), y el error crece con ln(base).
func TestNonNormalSplitAccuracy(t *testing.T) {
	s := newService(t)
	ex := s.(ucum.ExactService)
	const m = 12
	rng := rand.New(rand.NewPCG(10, 75))
	for _, u := range loadLogUnits(t) {
		// v = e·perStep es exacto solo con |perStep| de 1 o 2.
		if math.Abs(u.perStep) != 1 && math.Abs(u.perStep) != 2 {
			continue
		}
		// math.Pow(base, f) más medio ulp del redondeo final.
		tol := 3.0
		if u.base > 10 {
			tol = 6
		}
		ref := refFactor(t, s, u)
		lb := math.Log10(float64(u.base))
		worst := 0.0
		for range 300 {
			// n con |n·log10 base| entre 320 y 900, de los dos lados.
			n := int(320/lb) + rng.IntN(int(580/lb)+1)
			if rng.IntN(2) == 0 {
				n = -n
			}
			j := rng.Int64N(1<<m-1) - (1<<(m-1) - 1) // |j/2^m| < ½
			e := float64(n) + float64(j)/(1<<m)
			v := e * u.perStep
			// perStep lleva el signo de la escala: el valor es base^(v/perStep),
			// es decir base^e.
			sn, sj := n, j
			kk := -int(math.Round(float64(sn) * lb))
			factor, err := ex.CanonicalRat(big.NewRat(1, 1), fmt.Sprintf("10*%d", kk))
			if err != nil {
				t.Fatal(err)
			}
			want := powFloat(u.base, sn, sj, m, 400)
			want.Mul(want, new(big.Float).SetPrec(400).SetRat(ref.Value))
			want.Mul(want, new(big.Float).SetPrec(400).SetRat(factor.Value))
			p, err := s.Multiply(ucum.Pair{Value: v, Code: u.code}, ucum.Pair{Value: 1, Code: fmt.Sprintf("10*%d", kk)})
			if err != nil {
				t.Fatalf("Multiply(%v %s, 1 10*%d): %v", v, u.code, kk, err)
			}
			if d := ulpsOff(p.Value, want); d > worst {
				worst = d
				if d > tol {
					t.Errorf("Multiply(%v %s, 1 10*%d) = %v, %.2f ulps from %s", v, u.code, kk, p.Value, d, want.Text('g', 20))
				}
			}
		}
		t.Logf("%s: worst %.2f ulps (tolerance %v)", u.code, worst, tol)
	}
}

// sqrtUnit es la unidad de función sqrt del XML, con su valor y referencia.
func sqrtUnit(t *testing.T) (code string, alpha *big.Rat, refUnit string) {
	t.Helper()
	m, err := essence.Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range m.DefinedUnits {
		if u.Value != nil && u.Value.Function != nil && u.Value.Function.Name == "sqrt" {
			return u.Code, u.Value.Function.Value.Rat(), u.Value.Function.Unit
		}
	}
	t.Fatal("no sqrt unit in the XML")
	return "", nil, ""
}

// Test 9: sqrt en el tramo no normal vale (α·v)² exacto (UCUM §48), por el
// factor, redondeado una vez.
func TestNonNormalSqrtIsNearest(t *testing.T) {
	s := newService(t)
	ex := s.(ucum.ExactService)
	code, alpha, refUnit := sqrtUnit(t)
	ref, err := ex.CanonicalRat(big.NewRat(1, 1), refUnit)
	if err != nil {
		t.Fatal(err)
	}
	square := func(v float64) *big.Rat {
		r := new(big.Rat).Mul(new(big.Rat).SetFloat64(v), alpha)
		return r.Mul(r, r)
	}
	// Convert a 10*-300 por la referencia: 5000 entradas seguidas (hoy, 13
	// salidas distintas).
	dest := "10*-300." + refUnit
	to, err := ex.CanonicalRat(big.NewRat(1, 1), dest)
	if err != nil {
		t.Fatal(err)
	}
	off := 0
	v := 7e-162
	check := func(v float64) {
		want, _ := new(big.Rat).Quo(new(big.Rat).Mul(square(v), ref.Value), to.Value).Float64()
		got, err := s.Convert(v, code, dest)
		if err != nil || got != want {
			off++
			if off <= 3 {
				t.Errorf("Convert(%v, %q, %q) = (%v, %v), want %v", v, code, dest, got, err, want)
			}
		}
	}
	check(1.5e-160)
	for range 5000 {
		check(v)
		v = math.Nextafter(v, 1)
	}
	if off > 3 {
		t.Errorf("Convert from %s: %d of 5001 not nearest", code, off)
	}
	for _, tc := range []struct{ v, other float64 }{{1e-160, 1e300}, {2e160, 1e-300}} {
		want, _ := new(big.Rat).Mul(new(big.Rat).Mul(square(tc.v), ref.Value), new(big.Rat).SetFloat64(tc.other)).Float64()
		p, err := s.Multiply(ucum.Pair{Value: tc.v, Code: code}, ucum.Pair{Value: tc.other, Code: "1"})
		if err != nil || p.Value != want {
			t.Errorf("Multiply(%v %s, %v) = (%v, %v), want %v", tc.v, code, tc.other, p.Value, err, want)
		}
	}
}
