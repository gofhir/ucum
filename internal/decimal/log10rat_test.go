package decimal

import (
	"fmt"
	"math"
	"math/big"
	"math/bits"
	"math/rand/v2"
	"testing"
	"time"
)

// oldStripTens y oldLog10Rat son las funciones de v4.5.0 tal como estaban. Solo
// comprueban que el cambio de 021 no altera la salida; no son el oráculo.
func oldStripTens(n *big.Int) (rest *big.Int, tens int) {
	ten := big.NewInt(10)
	q, m := new(big.Int), new(big.Int)
	for n.Sign() != 0 {
		q.QuoRem(n, ten, m)
		if m.Sign() != 0 {
			break
		}
		n, q = q, n
		tens++
	}
	return n, tens
}

func oldLog10Rat(r *big.Rat) float64 {
	if r.Sign() == 0 {
		return math.Inf(-1)
	}
	num, nk := oldStripTens(new(big.Int).Abs(r.Num()))
	den, dk := oldStripTens(new(big.Int).Set(r.Denom()))
	tens := float64(nk - dk)
	if num.Cmp(den) == 0 {
		return tens
	}
	f := new(big.Float).SetPrec(64).SetRat(new(big.Rat).SetFrac(num, den))
	mant := new(big.Float)
	exp := f.MantExp(mant)
	m, _ := mant.Float64()
	return tens + (math.Log10(m) + float64(exp)*math.Log10(2))
}

func bigPow(base int64, k int) *big.Int {
	return new(big.Int).Exp(big.NewInt(base), big.NewInt(int64(k)), nil)
}

// tensCounts da los t del barrido: de 0 a 300 y 2^j-1, 2^j, 2^j+1 hasta 4097.
func tensCounts() []int {
	seen := map[int]bool{}
	var out []int
	add := func(t int) {
		if t >= 0 && t <= 4097 && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	for t := 0; t <= 300; t++ {
		add(t)
	}
	for j := 0; j <= 12; j++ {
		add(1<<j - 1)
		add(1 << j)
		add(1<<j + 1)
	}
	return out
}

// tensMantissas son los m con m no divisible por 10: impar, par sin cinco,
// múltiplo de cinco sin dos y 1.
var tensMantissas = []int64{1, 3, 7, 2, 4, 6, 5, 25, 15, 12345678901, 2 << 40}

// TestStripTensAgainstDefinition: n = m·10^t con m sin diez finales; el esperado
// es (m, t), conocido por construcción.
func TestStripTensAgainstDefinition(t *testing.T) {
	for _, tt := range tensCounts() {
		p := bigPow(10, tt)
		for _, m := range tensMantissas {
			n := new(big.Int).Mul(big.NewInt(m), p)
			rest, tens := stripTens(n)
			if tens != tt || rest.Cmp(big.NewInt(m)) != 0 {
				t.Fatalf("stripTens(%d·10^%d) = (%v, %d), want (%d, %d)", m, tt, rest, tens, m, tt)
			}
		}
	}
	rest, tens := stripTens(new(big.Int))
	if rest.Sign() != 0 || tens != 0 {
		t.Errorf("stripTens(0) = (%v, %d), want (0, 0)", rest, tens)
	}
}

// TestStripTensDivisionCount: a lo sumo 2·⌈log2(t+1)⌉+1 QuoRem para t diez
// finales, y ninguna para un n impar o cero. Las asignaciones no sirven de cota
// (el stripTens de v4.5.0 asigna menos), por eso se cuentan los QuoRem. No
// es paralela: el gancho es una variable de paquete.
func TestStripTensDivisionCount(t *testing.T) {
	var count int
	stripTensDivision = func() { count++ }
	defer func() { stripTensDivision = nil }()
	worst := 0
	for _, tt := range tensCounts() {
		p := bigPow(10, tt)
		bound := 2*bits.Len(uint(tt)) + 1
		for _, m := range tensMantissas {
			n := new(big.Int).Mul(big.NewInt(m), p)
			odd := n.Bit(0) == 1
			count = 0
			stripTens(n)
			limit := bound
			if odd {
				limit = 0
			}
			if count > limit {
				t.Fatalf("stripTens(%d·10^%d) did %d divisions, want at most %d", m, tt, count, limit)
			}
			worst = max(worst, count)
		}
	}
	if worst == 0 {
		t.Error("the hook was never called")
	}
	count = 0
	stripTens(new(big.Int))
	if count != 0 {
		t.Errorf("stripTens(0) did %d divisions, want 0", count)
	}
}

// log10Int es log10 de un entero positivo grande por sus 53 bits altos más el
// resto del exponente binario; no pasa por Log10Rat ni por la función vieja.
func log10Int(n *big.Int) float64 {
	shift := max(0, n.BitLen()-53)
	top, _ := new(big.Float).SetInt(new(big.Int).Rsh(n, uint(shift))).Float64()
	return math.Log10(top) + float64(shift)*math.Log10(2)
}

// l10Case es r = ±m·10^a / (d·10^b); el esperado sale de cómo se construye.
type l10Case struct {
	name string
	neg  bool
	m, d *big.Int
	a, b int
}

func (c l10Case) rat() *big.Rat {
	num := new(big.Int).Mul(c.m, bigPow(10, c.a))
	if c.neg {
		num.Neg(num)
	}
	den := new(big.Int).Mul(c.d, bigPow(10, c.b))
	return new(big.Rat).SetFrac(num, den)
}

// want devuelve el esperado, si es exacto y la escala de la tolerancia.
func (c l10Case) want() (want float64, exact bool, scale float64) {
	if c.m.IsInt64() && c.m.Int64() == 1 && c.d.IsInt64() && c.d.Int64() == 1 {
		return float64(c.a - c.b), true, 0
	}
	// Los diez son exactos: la tolerancia escala con el resultado y con los
	// logaritmos de m y d, que es donde están los errores de redondeo.
	lm, ld := log10Int(c.m), log10Int(c.d)
	return float64(c.a-c.b) + lm - ld, false, math.Abs(float64(c.a-c.b)) + lm + ld
}

func checkL10(t *testing.T, c l10Case) {
	t.Helper()
	r := c.rat()
	got := Log10Rat(r)
	want, exact, scale := c.want()
	if exact {
		if got != want {
			t.Fatalf("%s: Log10Rat = %v, want exactly %v", c.name, got, want)
		}
	} else if math.Abs(got-want) > 1e-15*max(1, scale) {
		t.Fatalf("%s: Log10Rat = %v, want %v", c.name, got, want)
	}
	if old := oldLog10Rat(r); got != old {
		t.Fatalf("%s: Log10Rat = %v, v4.5.0 gave %v", c.name, got, old)
	}
}

func bi(n int64) *big.Int { return big.NewInt(n) }

func TestLog10RatAgainstOracle(t *testing.T) {
	cases := make([]l10Case, 0, 128)
	for _, neg := range []bool{false, true} {
		cases = append(cases,
			l10Case{"1", neg, bi(1), bi(1), 0, 0},
			l10Case{"3/10", neg, bi(3), bi(1), 0, 1},
			l10Case{"50", neg, bi(5), bi(1), 1, 0},
			l10Case{"10^7", neg, bi(1), bi(1), 7, 0},
			l10Case{"10^-7", neg, bi(1), bi(1), 0, 7},
			l10Case{"2^900000", neg, new(big.Int).Lsh(bi(1), 900000), bi(1), 0, 0},
			l10Case{"2^-900000", neg, bi(1), new(big.Int).Lsh(bi(1), 900000), 0, 0},
		)
		for _, ab := range [][2]int{
			{1, 0}, {0, 1}, {3, 5}, {5, 3}, {10, 1},
			{1, 10}, {40, 41}, {41, 40}, {200, 7}, {7, 200},
		} {
			m := new(big.Int).Mul(bigPow(2, ab[0]), bigPow(5, ab[1]))
			cases = append(cases, l10Case{"2^a·5^b", neg, m, bi(1), 0, 0})
		}
		for _, k := range []int{1, 2, 3, 10, 77, 300} {
			cases = append(cases,
				l10Case{"5^k/2^k", neg, bigPow(5, k), bigPow(2, k), 0, 0},
				l10Case{"2^k/5^k", neg, bigPow(2, k), bigPow(5, k), 0, 0},
				l10Case{"10^k", neg, bi(1), bi(1), k, 0},
				l10Case{"integer", neg, bigPow(7, k), bi(1), 0, 0},
				l10Case{"inverse", neg, bi(1), bigPow(7, k), 0, 0},
			)
		}
	}
	for i, c := range cases {
		c.name = fmt.Sprintf("%s#%d", c.name, i)
		checkL10(t, c)
	}

	// Barrido: 10^k por varias mantisas y denominadores, con signo.
	mants := []int64{1, 3, 7, 12345}
	dens := []int64{1, 3, 7, 16}
	for k := -400; k <= 400; k++ {
		for _, m := range mants {
			for _, d := range dens {
				for _, neg := range []bool{false, true} {
					c := l10Case{"sweep", neg, bi(m), bi(d), max(k, 0), max(-k, 0)}
					checkL10(t, c)
				}
			}
		}
	}

	// Racionales al azar de hasta 600 bits con hasta 79 diez a cada lado.
	rng := rand.New(rand.NewPCG(21, 2026))
	randInt := func() *big.Int {
		buf := make([]byte, 75)
		for i := range buf {
			buf[i] = byte(rng.Uint32())
		}
		n := new(big.Int).SetBytes(buf)
		n.Rsh(n, uint(rng.IntN(600)))
		return n.Add(n, bi(1))
	}
	for range 4000 {
		c := l10Case{"random", rng.IntN(2) == 0, randInt(), randInt(), rng.IntN(80), rng.IntN(80)}
		checkL10(t, c)
	}
}

// TestLog10RatDoubleRounding fija la salida actual donde el redondeo a 64 bits
// y después a 53 se nota (plan 021 §7, defecto previo). Los esperados salen de
// la regla, no del código: (2^65+2^12+1)/2^65 = 1+2^-53+2^-65 y
// (2^64+2^11+1)/2^64 = 1+2^-53+2^-64 dan 1+2^-53 a 64 bits (el segundo es un
// empate y va al par); su mantisa 0.5+2^-54 es un empate a 53 bits y va al par,
// 0.5, así que el log10 es log10(0.5) + log10(2) = 0. Con /2^66 el exponente
// baja en uno: 0 − log10(2). Con 65 bits el segundo no es empate y sube.
//
// Dos casos más fijan los 64 bits desde abajo y el modo de redondeo:
// (2^63+2^10+1)/2^63 = 1+2^-53+2^-63 es exacto a 64 bits, y su mantisa
// 0.5+2^-54+2^-64 sube a 0.5+2^-53 a 53 bits (con 63 bits sería un empate que
// va al par, 0.5). (2^65+2^13+2^12−1)/2^65 = 1+2^-52+2^-53−2^-65 sube a
// 1+2^-52+2^-53 a 64 bits, y su mantisa 0.5+2^-53+2^-54 es un empate a 53 bits
// que va al par, 0.5+2^-52 (truncando a 64 bits daría 0.5+2^-53). El esperado
// es Log10Rat del valor ya redondeado, 1+2^-52 o 1+2^-51, que cabe en 53 bits:
// los dos lados pasan por la misma suma final (el compilador puede fusionarla
// con la multiplicación de Log10, así que un math.Log10 aparte no sirve).
func TestLog10RatDoubleRounding(t *testing.T) {
	for _, c := range []struct {
		num     *big.Int
		rounded *big.Rat
	}{
		{new(big.Int).Add(new(big.Int).Lsh(bi(1), 63), bi(1<<10+1)), big.NewRat(1<<52+1, 1<<52)},
		{new(big.Int).Add(new(big.Int).Lsh(bi(1), 65), bi(1<<13+1<<12-1)), big.NewRat(1<<51+1, 1<<51)},
	} {
		shift := uint(c.num.BitLen() - 1)
		r := new(big.Rat).SetFrac(c.num, new(big.Int).Lsh(bi(1), shift))
		want := Log10Rat(c.rounded)
		if want == 0 {
			t.Fatalf("Log10Rat(%v) = 0: the rounded value must differ from 1", c.rounded)
		}
		if got := Log10Rat(r); got != want {
			t.Errorf("Log10Rat(%v/2^%d) = %v, want %v as for %v", c.num, shift, got, want, c.rounded)
		}
		if old := oldLog10Rat(r); old != want {
			t.Errorf("v4.5.0 copy gave %v for %v/2^%d, want %v", old, c.num, shift, want)
		}
	}
	for _, c := range []struct {
		hi, lo, shift uint
		want          float64
	}{
		{65, 12, 65, 0},
		{65, 12, 66, -math.Log10(2)},
		{64, 11, 64, 0},
	} {
		num := new(big.Int).Lsh(bi(1), c.hi)
		num.Add(num, new(big.Int).Lsh(bi(1), c.lo)).Add(num, bi(1))
		r := new(big.Rat).SetFrac(num, new(big.Int).Lsh(bi(1), c.shift))
		if got := Log10Rat(r); got != c.want {
			t.Errorf("Log10Rat((2^%d+2^%d+1)/2^%d) = %v, want %v", c.hi, c.lo, c.shift, got, c.want)
		}
		if old := oldLog10Rat(r); old != c.want {
			t.Errorf("v4.5.0 copy gave %v for (2^%d+2^%d+1)/2^%d, want %v", old, c.hi, c.lo, c.shift, c.want)
		}
	}
}

// checkQuotient64 compara quotient64 con la definición, en racionales exactos y
// sin math.Log10 (que según la arquitectura puede tapar un ulp, code review de
// 021, ronda 3): q tiene a lo sumo 64 bits, |q − x| ≤ ulp/2 y, en un empate, la
// mantisa entera de q es par. Si q es una potencia de dos y x está por debajo,
// el ulp es el de la binada de abajo.
func checkQuotient64(t *testing.T, num, den *big.Int) {
	t.Helper()
	q := quotient64(num, den)
	if q.MinPrec() > 64 {
		t.Fatalf("quotient64(%v, %v) = %v has %d bits", num, den, q.Text('p', 0), q.MinPrec())
	}
	qr, _ := q.Rat(nil)
	x := new(big.Rat).SetFrac(num, den)
	e := q.MantExp(nil) // q está en [2^(e-1), 2^e).
	shift := e - 64
	if qr.Cmp(pow2(e-1)) == 0 && x.Cmp(qr) < 0 {
		shift--
	}
	ulp := pow2(shift)
	diff := new(big.Rat).Sub(qr, x)
	diff.Abs(diff)
	switch c := diff.Cmp(new(big.Rat).Mul(ulp, big.NewRat(1, 2))); {
	case c > 0:
		t.Fatalf("quotient64(%v, %v) = %v is not the nearest 64-bit value", num, den, q.Text('p', 0))
	case c == 0 && new(big.Rat).Quo(qr, ulp).Num().Bit(0) == 1:
		t.Fatalf("quotient64(%v, %v) = %v breaks a tie away from even", num, den, q.Text('p', 0))
	}
}

// pow2 es 2^k exacto, también para k negativo.
func pow2(k int) *big.Rat {
	if k >= 0 {
		return new(big.Rat).SetInt(new(big.Int).Lsh(bi(1), uint(k)))
	}
	return new(big.Rat).SetFrac(bi(1), new(big.Int).Lsh(bi(1), uint(-k)))
}

// TestLog10RatQuotientAgainstDefinition: el cociente de 64 bits de Log10Rat,
// contra la definición. T, impar de 65 bits, es justo el punto medio entre dos
// valores de 64 bits: T/2^j es un empate, y T·(2^k+1) ± 1 sobre 2^k+1 queda
// apenas a un lado, lo que distingue redondear num o den antes de dividir.
func TestLog10RatQuotientAgainstDefinition(t *testing.T) {
	one := bi(1)
	two64 := new(big.Int).Lsh(one, 64)
	ts := []*big.Int{
		new(big.Int).Add(two64, bi(1)),
		new(big.Int).Add(two64, bi(3)),
		new(big.Int).Sub(new(big.Int).Lsh(one, 65), one),
		new(big.Int).Add(two64, bi(0x5555555555555555)),
	}
	for _, tt := range ts {
		for _, j := range []uint{0, 1, 64, 300} {
			checkQuotient64(t, tt, new(big.Int).Lsh(one, j))
		}
		for _, k := range []uint{1, 63, 64, 65, 100, 129, 200, 1000} {
			d := new(big.Int).Add(new(big.Int).Lsh(one, k), one)
			base := new(big.Int).Mul(tt, d)
			checkQuotient64(t, new(big.Int).Add(base, one), d)
			checkQuotient64(t, new(big.Int).Sub(base, one), d)
			checkQuotient64(t, d, new(big.Int).Add(base, one))
			checkQuotient64(t, d, new(big.Int).Sub(base, one))
			sh := new(big.Int).Lsh(tt, k)
			checkQuotient64(t, new(big.Int).Add(sh, one), new(big.Int).Lsh(one, k))
			checkQuotient64(t, new(big.Int).Sub(sh, one), new(big.Int).Lsh(one, k))
		}
	}
	// Casos de las búsquedas de las rondas 2 y 3 del code review.
	for _, s := range []string{
		"280926306166160480009/83",
		"29/96901176502440991122967",
		"49364182050185145651316274820321725/17",
		"39/217374111488735727110094548251699",
		"1606938044258991167658882319497848510602280165702655384486148/" +
			"1606938044258990275541962092341162602526925360265662480515075",
	} {
		r, ok := new(big.Rat).SetString(s)
		if !ok {
			t.Fatalf("bad rational %q", s)
		}
		checkQuotient64(t, r.Num(), r.Denom())
	}
	wide := new(big.Int).Lsh(one, 200)
	wide.Add(wide, new(big.Int).Lsh(one, 147)).Add(wide, new(big.Int).Lsh(one, 136)).Add(wide, one)
	checkQuotient64(t, wide, new(big.Int).Lsh(one, 200))
	// Al azar, de 1 a 400 bits a cada lado.
	rng := rand.New(rand.NewPCG(64, 2026))
	randInt := func() *big.Int {
		n := new(big.Int)
		for range 7 {
			n.Lsh(n, 64).Or(n, new(big.Int).SetUint64(rng.Uint64()))
		}
		n.Rsh(n, uint(rng.IntN(448)))
		return n.Add(n, one)
	}
	for range 3000 {
		checkQuotient64(t, randInt(), randInt())
	}
}

// TestLog10RatNoGCDCost: 2^2700000/3^1800000 es coprimo y no tiene diez. Se
// arma sin SetFrac (que repetiría el máximo común divisor) y Log10Rat debe
// terminar en milisegundos; un SetFrac de vuelta tarda varios segundos.
func TestLog10RatNoGCDCost(t *testing.T) {
	num := new(big.Int).Lsh(bi(1), 2700000)
	den := bigPow(3, 1800000)
	// Num y Denom devuelven referencias al interior; con una fracción previa de
	// divisor distinto de 1 se asignan sin normalizar.
	r := big.NewRat(1, 2)
	r.Num().Set(num)
	r.Denom().Set(den)
	done := make(chan float64, 1)
	go func() { done <- Log10Rat(r) }()
	want := 2700000*math.Log10(2) - 1800000*math.Log10(3)
	select {
	case got := <-done:
		if math.Abs(got-want) > 1e-15*2700000 {
			t.Errorf("Log10Rat(2^2700000/3^1800000) = %v, want %v", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Log10Rat(2^2700000/3^1800000) did not finish within 2 s")
	}
}

// TestLog10RatDoesNotAlterInput: Log10Rat quita los diez de copias, no de r.
func TestLog10RatDoesNotAlterInput(t *testing.T) {
	for _, c := range []l10Case{
		{"both sides", false, bi(3), bi(7), 5, 4},
		{"numerator", true, bi(3), bi(7), 9, 2},
		{"denominator", false, bi(11), bi(13), 2, 30},
		{"powers", false, bi(1), bi(1), 40, 3},
	} {
		r := c.rat()
		num, den := new(big.Int).Set(r.Num()), new(big.Int).Set(r.Denom())
		Log10Rat(r)
		if r.Num().Cmp(num) != 0 || r.Denom().Cmp(den) != 0 {
			t.Errorf("%s: Log10Rat changed r to %v/%v from %v/%v", c.name, r.Num(), r.Denom(), num, den)
		}
	}
}
