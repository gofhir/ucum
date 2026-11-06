package fhir

import (
	"fmt"
	"math/big"
	"math/rand"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Plan 020: decimalExponent, round, scientific y mantissa sin costo cuadrático
// y con la misma salida. FHIR `decimal`: «The precision of the decimal value has
// significance», así que String no puede cambiar un solo carácter.
//
// Los esperados salen de contar dígitos de la representación decimal (refExp) y
// de un oráculo que redondea la cadena de dígitos (refString), no del código
// bajo prueba. La implementación vigente, copiada abajo como old*, queda solo
// como comprobación extra.

// refPow10 devuelve 10^k exacto, con k negativo o positivo.
func refPow10(k int) *big.Rat {
	n := k
	if n < 0 {
		n = -n
	}
	p := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
	if k < 0 {
		return new(big.Rat).SetFrac(big.NewInt(1), p)
	}
	return new(big.Rat).SetInt(p)
}

// refExp da la posición del primer dígito significativo contando dígitos: la
// cantidad de dígitos de la parte entera si |r| >= 1, y 1-k si no, con k la
// menor cantidad de ceros que hay que agregar al numerador para alcanzar el
// divisor (comparación de cadenas de igual largo).
func refExp(r *big.Rat) int {
	if r.Sign() == 0 {
		return 1
	}
	num := new(big.Int).Abs(r.Num())
	den := r.Denom()
	if num.Cmp(den) >= 0 {
		q := new(big.Int).Quo(num, den)
		return len(q.String())
	}
	ns, ds := num.String(), den.String()
	// num < den implica len(ns) <= len(ds).
	scaled := ns + strings.Repeat("0", len(ds)-len(ns))
	k := len(ds) - len(ns)
	if scaled < ds {
		k++
	}
	return 1 - k
}

// refDigits devuelve la expansión decimal de |r| como dígitos sin ceros a la
// izquierda, junto con la posición e de su primer dígito (igual que refExp), con
// al menos want dígitos y, para valores sin expansión finita, truncada.
func refDigits(r *big.Rat, want int) (digits string, first int) {
	abs := new(big.Rat).Abs(r)
	places := want + 60
	if e := refExp(abs); e < 0 {
		places += -e
	}
	s := abs.FloatString(places)
	ip, fp, _ := strings.Cut(s, ".")
	if ip != "0" {
		return strings.TrimRight(ip+fp, "0"), len(ip)
	}
	z := len(fp) - len(strings.TrimLeft(fp, "0"))
	return strings.TrimRight(fp[z:], "0"), -z
}

// refString es el oráculo de Decimal.String con n cifras significativas: redondea
// la cadena de dígitos con mitades lejos del cero y escribe notación plana si
// quedan lugares después del punto, científica si no. El acarreo mueve el
// exponente; en notación plana los lugares nunca bajan de cero.
func refString(v *big.Rat, n int) string {
	if v.Sign() == 0 {
		return "0." + strings.Repeat("0", n)
	}
	digits, e := refDigits(v, n+1)
	digits += strings.Repeat("0", n+1)
	keep, next := []byte(digits[:n]), digits[n]
	if next >= '5' {
		i := n - 1
		for ; i >= 0; i-- {
			if keep[i] == '9' {
				keep[i] = '0'
				continue
			}
			keep[i]++
			break
		}
		if i < 0 {
			keep = append([]byte{'1'}, keep[:n-1]...)
			e++
		}
	}
	sign := ""
	if v.Sign() < 0 {
		sign = "-"
	}
	d := string(keep)
	// La notación se decide con el exponente anterior al redondeo.
	if n-refExp(v) < 0 {
		m := d[:1]
		if n > 1 {
			m += "." + d[1:]
		}
		return sign + m + "e" + strconv.Itoa(e-1)
	}
	switch {
	case e >= n:
		return sign + d + strings.Repeat("0", e-n)
	case e > 0:
		return sign + d[:e] + "." + d[e:]
	default:
		return sign + "0." + strings.Repeat("0", -e) + d
	}
}

// Copia de la implementación vigente (v4.4.10), solo como comprobación extra.

func oldExp(r *big.Rat) int {
	if r.Sign() == 0 {
		return 1
	}
	abs := new(big.Rat).Abs(r)
	exp := 0
	ten := big.NewRat(10, 1)
	one := big.NewRat(1, 1)
	for abs.Cmp(one) >= 0 {
		abs.Quo(abs, ten)
		exp++
	}
	for abs.Cmp(big.NewRat(1, 10)) < 0 {
		abs.Mul(abs, ten)
		exp--
	}
	return exp
}

func oldMantissa(v *big.Rat, n, exp int) (string, bool) {
	shift := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exp-1)), nil))
	out := new(big.Rat).Quo(v, shift).FloatString(n - 1)
	if r, ok := new(big.Rat).SetString(out); ok {
		if new(big.Rat).Abs(r).Cmp(big.NewRat(10, 1)) >= 0 {
			return out, true
		}
	}
	return out, false
}

func oldScientific(v *big.Rat, n, exp int) string {
	out, carried := oldMantissa(v, n, exp)
	if carried {
		exp++
		out, _ = oldMantissa(v, n, exp)
	}
	return out + "e" + strconv.Itoa(exp-1)
}

func oldRound(v *big.Rat, n int) string {
	if v.Sign() == 0 {
		return "0." + strings.Repeat("0", n)
	}
	exp := oldExp(v)
	places := n - exp
	if places < 0 {
		return oldScientific(v, n, exp)
	}
	out := v.FloatString(places)
	if rounded, ok := new(big.Rat).SetString(out); ok && oldExp(rounded) != exp && rounded.Sign() != 0 {
		places = n - oldExp(rounded)
		if places < 0 {
			places = 0
		}
		out = v.FloatString(places)
	}
	return out
}

// expValues arma el conjunto de valores positivos de los barridos.
func expValues() []*big.Rat {
	var out []*big.Rat
	add := func(r *big.Rat) { out = append(out, r) }
	eps := new(big.Rat).SetFrac(big.NewInt(1), new(big.Int).Exp(big.NewInt(10), big.NewInt(30), nil))
	up := new(big.Rat).Add(big.NewRat(1, 1), eps)
	down := new(big.Rat).Sub(big.NewRat(1, 1), eps)
	for k := -400; k <= 400; k++ {
		p := refPow10(k)
		add(p)
		add(new(big.Rat).Mul(p, up))
		add(new(big.Rat).Mul(p, down))
		add(new(big.Rat).Mul(p, big.NewRat(1, 3)))
		add(new(big.Rat).Mul(p, big.NewRat(9995, 1000)))
		add(new(big.Rat).Mul(p, big.NewRat(999, 1000)))
		// Denominadores impares.
		if k%5 == 0 {
			add(new(big.Rat).Mul(p, big.NewRat(2, 7)))
			add(new(big.Rat).Mul(p, big.NewRat(31, 11)))
			add(new(big.Rat).Mul(p, big.NewRat(995, 99)))
		}
	}
	// Ramas de corrección: numeradores justo bajo 10^k sobre denominadores justo
	// bajo una potencia de dos, y el inverso (justo bajo 2^m sobre 10^j - 1).
	// TestDecimalExpCorrectionBranches afirma que estas entradas recorren las dos
	// correcciones de decimalExponent.
	for k := 1; k <= 300; k += 7 {
		for _, m := range []int{1, 2, 7, 64, 127, 300} {
			p2 := new(big.Int).Lsh(big.NewInt(1), uint(m))
			p10 := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(k)), nil)
			num := new(big.Int).Sub(p10, big.NewInt(1))
			add(new(big.Rat).SetFrac(num, new(big.Int).Sub(p2, big.NewInt(1))))
			add(new(big.Rat).SetFrac(num, p2))
			add(new(big.Rat).SetFrac(new(big.Int).Add(p10, big.NewInt(1)), p2))
			add(new(big.Rat).SetFrac(new(big.Int).Sub(p2, big.NewInt(1)), new(big.Int).Sub(p10, big.NewInt(1))))
			add(new(big.Rat).SetFrac(p2, p10))
			add(new(big.Rat).SetFrac(new(big.Int).Sub(p2, big.NewInt(1)), p10))
		}
	}
	rng := rand.New(rand.NewSource(20))
	for i := 0; i < 600; i++ {
		nb, db := 1+rng.Intn(300), 1+rng.Intn(300)
		n := new(big.Int).Rand(rng, new(big.Int).Lsh(big.NewInt(1), uint(nb)))
		d := new(big.Int).Rand(rng, new(big.Int).Lsh(big.NewInt(1), uint(db)))
		n.Add(n, big.NewInt(1))
		d.Add(d, big.NewInt(1))
		add(new(big.Rat).SetFrac(n, d))
	}
	return out
}

func TestDecimalExpAgainstDigitCount(t *testing.T) {
	for i, v := range expValues() {
		for _, r := range []*big.Rat{v, new(big.Rat).Neg(v)} {
			want := refExp(r)
			if got := decimalExponent(r); got != want {
				t.Fatalf("value %d (%s): decimalExponent = %d, digit count says %d", i, r.String(), got, want)
			}
			if got := oldExp(r); got != want {
				t.Fatalf("test helper oldExp disagrees with refExp on value %d: %d vs %d", i, got, want)
			}
		}
	}
	if got := decimalExponent(new(big.Rat)); got != 1 {
		t.Errorf("decimalExponent(0) = %d, want 1", got)
	}
}

// TestDecimalExpCorrectionBranches cuenta con el gancho exponentCorrection las
// veces que la estimación por bits necesita corregirse y afirma que las dos
// ramas (hacia arriba y hacia abajo) se recorren con las entradas de expValues.
// No es paralela: el gancho es una variable de paquete y las pruebas paralelas
// del paquete empiezan cuando terminan las secuenciales.
func TestDecimalExpCorrectionBranches(t *testing.T) {
	var up, down int
	exponentCorrection = func(u bool) {
		if u {
			up++
		} else {
			down++
		}
	}
	defer func() { exponentCorrection = nil }()
	for _, v := range expValues() {
		decimalExponent(v)
	}
	if up == 0 || down == 0 {
		t.Errorf("correction branches exercised: up %d, down %d; both must be positive", up, down)
	}
}

// TestDecimalExpPowersOfTenBoundary fija el borde: 10^k abre la posición k+1 y su
// antecesor inmediato la deja en k.
func TestDecimalExpPowersOfTenBoundary(t *testing.T) {
	for k := -400; k <= 400; k++ {
		p := refPow10(k)
		if got := decimalExponent(p); got != k+1 {
			t.Fatalf("decimalExponent(10^%d) = %d, want %d", k, got, k+1)
		}
		below := new(big.Rat).Mul(p, big.NewRat(999999999, 1000000000))
		if got := decimalExponent(below); got != k {
			t.Fatalf("decimalExponent(0.999999999*10^%d) = %d, want %d", k, got, k)
		}
	}
}

func TestDecimalStringAgainstOracle(t *testing.T) {
	vals := expValues()
	figs := []int{1, 2, 3, 5, 17}
	for i, v := range vals {
		// Todos los k de las potencias entran en el barrido de exponentes; aquí un
		// tercio de los valores basta y mantiene la corrida bajo unos segundos.
		if i%3 != 0 {
			continue
		}
		for j, n := range figs {
			x := v
			if (i+j)%2 == 1 {
				x = new(big.Rat).Neg(v)
			}
			got := NewDecimal(x, n).String()
			want := refString(x, n)
			if got != want {
				t.Fatalf("value %d (%s) at %d figures: String = %q, oracle %q", i, x.String(), n, got, want)
			}
			if old := oldRound(x, n); old != got {
				t.Fatalf("value %d (%s) at %d figures: String = %q, v4.4.10 gave %q", i, x.String(), n, got, old)
			}
		}
	}
}

func TestDecimalStringZeroAndUnlimited(t *testing.T) {
	for _, n := range []int{1, 2, 3, 5, 17} {
		if got, want := NewDecimal(new(big.Rat), n).String(), "0."+strings.Repeat("0", n); got != want {
			t.Errorf("zero at %d figures: %q, want %q", n, got, want)
		}
	}
	cases := []struct {
		v    *big.Rat
		want string
	}{
		{big.NewRat(42, 1), "42"},
		{big.NewRat(-42, 1), "-42"},
		{big.NewRat(5, 2), "2.5"},
		{big.NewRat(1, 8), "0.125"},
		{big.NewRat(-1, 3), "-0.33333333333333333333"},
		{new(big.Rat), "0"},
	}
	for _, c := range cases {
		for _, n := range []int{0, -3} {
			if got := NewDecimal(c.v, n).String(); got != c.want {
				t.Errorf("unlimited %s (n=%d): %q, want %q", c.v, n, got, c.want)
			}
		}
	}
}

// TestDecimalStringExactHalves cubre las mitades exactas en la última cifra
// conservada, que se redondean lejos del cero en notación plana y científica.
func TestDecimalStringExactHalves(t *testing.T) {
	cases := []struct {
		num, den int64
		n        int
		want     string
	}{
		{25, 1, 1, "3e1"},
		{-25, 1, 1, "-3e1"},
		{35, 1, 1, "4e1"},
		{-35, 1, 1, "-4e1"},
		{95, 1, 1, "1e2"},
		{-95, 1, 1, "-1e2"},
		{995, 1, 2, "1.0e3"},
		{-995, 1, 2, "-1.0e3"},
		{1500, 1, 1, "2e3"},
		{-1500, 1, 1, "-2e3"},
		{2500, 1, 1, "3e3"},
		{-2500, 1, 1, "-3e3"},
		{5, 2, 1, "3"},
		{-5, 2, 1, "-3"},
		{19, 2, 1, "10"},
		{-19, 2, 1, "-10"},
		{5, 4, 2, "1.3"},
		{-5, 4, 2, "-1.3"},
		{1, 8, 2, "0.13"},
		{-1, 8, 2, "-0.13"},
		{1, 4, 1, "0.3"},
		{-1, 4, 1, "-0.3"},
		{1, 2, 1, "0.5"},
		{-1, 2, 1, "-0.5"},
		{5, 100000, 1, "0.00005"},
		{25, 1000000, 1, "0.00003"},
	}
	for _, c := range cases {
		v := big.NewRat(c.num, c.den)
		got := NewDecimal(v, c.n).String()
		if got != c.want {
			t.Errorf("%s at %d figures: %q, want %q", v, c.n, got, c.want)
		}
		if o := refString(v, c.n); o != c.want {
			t.Errorf("oracle %s at %d figures: %q, table %q", v, c.n, o, c.want)
		}
	}
	// Mitades en cada potencia: -0.5·10^k y 2.5·10^k con una cifra.
	for k := -60; k <= 60; k++ {
		for _, num := range []int64{-5, 5, 15, -15, 25, 35} {
			v := new(big.Rat).Mul(big.NewRat(num, 10), refPow10(k))
			got := NewDecimal(v, 1).String()
			if want := refString(v, 1); got != want {
				t.Fatalf("%s at one figure: %q, oracle %q", v, got, want)
			}
		}
	}
}

// TestDecimalExpAdditivePrecision comprueba que Add y Sub, que leen el
// exponente por decimalPlaces y figuresForPlaces, asignan las cifras que
// da el conteo de dígitos.
func TestDecimalExpAdditivePrecision(t *testing.T) {
	vals := expValues()
	rng := rand.New(rand.NewSource(21))
	for i := 0; i < 1000; i++ {
		a, b := vals[rng.Intn(len(vals))], vals[rng.Intn(len(vals))]
		na, nb := rng.Intn(6), rng.Intn(6)
		da, db := NewDecimal(a, na), NewDecimal(b, nb)
		for _, sub := range []bool{false, true} {
			var sum *big.Rat
			var got Decimal
			if sub {
				sum, got = new(big.Rat).Sub(a, b), da.Sub(db)
			} else {
				sum, got = new(big.Rat).Add(a, b), da.Add(db)
			}
			places, limited := 0, false
			for _, p := range []struct {
				v *big.Rat
				n int
			}{{a, na}, {b, nb}} {
				if p.n <= 0 {
					continue
				}
				pl := p.n - refExp(p.v)
				if !limited || pl < places {
					places, limited = pl, true
				}
			}
			want := 0
			if limited {
				want = places + refExp(sum)
				if want < 1 {
					want = 1
				}
			}
			if got.SignificantFigures() != want {
				t.Fatalf("pair %d (%s, %s, figs %d/%d, sub=%v): %d figures, want %d", i, a, b, na, nb, sub, got.SignificantFigures(), want)
			}
		}
	}
}

// probe corre f en una gorra con límite de 2 s. Es la sonda del criterio de
// salida del plan 020 §4: en el código vigente estas entradas tardan segundos o
// minutos y la prueba falla; con el arreglo terminan en milisegundos. Las
// gorutinas que no terminan siguen corriendo en segundo plano hasta que acaba el
// proceso de prueba (memoria y CPU de la corrida que falla).
//
// Las cotas de asignaciones con las mismas entradas están en
// TestDecimalStringWorstCaseAllocs.
func probe(t *testing.T, name string, f func() string, check func(string) string) {
	t.Run(name, func(t *testing.T) {
		t.Parallel()
		done := make(chan string, 1)
		go func() { done <- f() }()
		select {
		case out := <-done:
			if msg := check(out); msg != "" {
				t.Error(msg)
			}
		case <-time.After(2 * time.Second):
			t.Errorf("%s did not finish within 2 s", name)
		}
	})
}

func TestDecimalStringWorstCaseCost(t *testing.T) {
	t.Parallel()
	mustParse := func(s string) Decimal {
		d, err := ParseDecimal(s)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	probe(t, "1.0e999999", func() string { return mustParse("1.0e999999").String() }, func(s string) string {
		if s != "1.0e999999" {
			return "unexpected output " + s
		}
		return ""
	})
	probe(t, "1.0e-999999", func() string { return mustParse("1.0e-999999").String() }, func(s string) string {
		if len(s) != 1000002 || !strings.HasSuffix(s, "10") {
			return fmt.Sprintf("unexpected output of length %d", len(s))
		}
		return ""
	})
	probe(t, "add 1.0e30000 and 1.0e-30000", func() string {
		return mustParse("1.0e30000").Add(mustParse("1.0e-30000")).String()
	}, func(s string) string {
		if !strings.HasPrefix(s, "1.0e30000") {
			return "unexpected output prefix " + s[:min(len(s), 20)]
		}
		return ""
	})
	probe(t, "1/(3*10^300000) at 3 figures", func() string {
		den := new(big.Int).Mul(big.NewInt(3), new(big.Int).Exp(big.NewInt(10), big.NewInt(300000), nil))
		return NewDecimal(new(big.Rat).SetFrac(big.NewInt(1), den), 3).String()
	}, func(s string) string {
		if len(s) != 300005 || !strings.HasSuffix(s, "333") {
			return fmt.Sprintf("unexpected output of length %d", len(s))
		}
		return ""
	})
	probe(t, "ConvertDecimal 1.5 '300 to rad300", func() string {
		c, err := NewComparator()
		if err != nil {
			return "ERR " + err.Error()
		}
		out, err := c.ConvertDecimal(mustParse("1.5"), "'300", "rad300")
		if err != nil {
			return "ERR " + err.Error()
		}
		return out.String()
	}, func(s string) string {
		if strings.HasPrefix(s, "ERR ") {
			return s
		}
		return ""
	})
}

// worstCaseAllocs mide con runtime.MemStats las asignaciones (mallocs y bytes)
// de una corrida de f. Corre una vez antes para calentar cachés y descartar el
// costo de inicialización, y mide la segunda.
func worstCaseAllocs(f func() string) (mallocs, bytes uint64) {
	f()
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.Mallocs - before.Mallocs, after.TotalAlloc - before.TotalAlloc
}

// TestDecimalStringWorstCaseAllocs acota las asignaciones de las entradas de
// TestDecimalStringWorstCaseCost (plan 020 §5.4). No es paralela: MemStats es
// global y las pruebas paralelas del paquete empiezan al terminar las
// secuenciales. Las cotas son alrededor del doble de lo medido el 2026-11-06 (Go
// 1.27.2, arm64: 169, 189, 260, 158 y 233 mallocs; 18, 23, 1.4, 6.9 MB y 317 KB)
// y muy por debajo de lo que costaría un cociente por dígito.
func TestDecimalStringWorstCaseAllocs(t *testing.T) {
	mustParse := func(s string) Decimal {
		d, err := ParseDecimal(s)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	cmp, err := NewComparator()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name       string
		f          func() string
		maxMallocs uint64
		maxKiB     uint64
	}{
		{"1.0e999999", func() string { return mustParse("1.0e999999").String() }, 400, 40 << 10},
		{"1.0e-999999", func() string { return mustParse("1.0e-999999").String() }, 400, 48 << 10},
		{"add 1.0e30000 and 1.0e-30000", func() string {
			return mustParse("1.0e30000").Add(mustParse("1.0e-30000")).String()
		}, 600, 4 << 10},
		{"1/(3*10^300000) at 3 figures", func() string {
			den := new(big.Int).Mul(big.NewInt(3), new(big.Int).Exp(big.NewInt(10), big.NewInt(300000), nil))
			return NewDecimal(new(big.Rat).SetFrac(big.NewInt(1), den), 3).String()
		}, 400, 16 << 10},
		{"ConvertDecimal 1.5 '300 to rad300", func() string {
			out, err := cmp.ConvertDecimal(mustParse("1.5"), "'300", "rad300")
			if err != nil {
				return "ERR " + err.Error()
			}
			return out.String()
		}, 600, 1 << 10},
	}
	for _, c := range cases {
		done := make(chan struct{})
		var mallocs, bytes uint64
		go func() {
			mallocs, bytes = worstCaseAllocs(c.f)
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatalf("%s did not finish within 10 s", c.name)
		}
		t.Logf("%s: %d mallocs, %d bytes", c.name, mallocs, bytes)
		if mallocs > c.maxMallocs || bytes > c.maxKiB<<10 {
			t.Errorf("%s: %d mallocs (max %d), %d bytes (max %d KiB)", c.name, mallocs, c.maxMallocs, bytes, c.maxKiB)
		}
	}
}
