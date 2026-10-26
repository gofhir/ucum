package fhir_test

import (
	"errors"
	"math"
	"math/big"
	"sort"
	"testing"

	"github.com/gofhir/ucum/v4"
	"github.com/gofhir/ucum/v4/fhir"
)

// Plan 001 (issue #54): CanonicalKey debe usar Exact tal cual y redondear a
// float64 una sola vez. Ningún valor esperado de este archivo sale de correr
// CanonicalKey.

func mustRat(t testing.TB, s string) *big.Rat {
	t.Helper()
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		t.Fatalf("bad rational %q", s)
	}
	return r
}

func newComparator(t testing.TB) (*fhir.Comparator, ucum.ExactService) {
	t.Helper()
	svc, err := ucum.NewExact()
	if err != nil {
		t.Fatal(err)
	}
	return fhir.NewComparatorWith(svc), svc
}

// Test 1: el esperado es un racional calculado a mano desde la fuente citada;
// el test solo lo redondea una vez con big.Rat.Float64, que redondea al más
// cercano.
func TestCanonicalKeyExactByHand(t *testing.T) {
	c, _ := newComparator(t)
	cases := []struct {
		value, code   string
		wantCode, rat string
		source        string
	}{
		// El caso del issue. ucum-essence.xml: dL = 10^-1 L, L = dm3 = 10^-3 m3,
		// mg = 10^-3 g; 0.07 × 10^-3 / 10^-4 = 7/10.
		{"0.07", "mg/dL", "g.m-3", "7/10", "issue #54"},
		// mL = 10^-3 dm3 = 10^-6 m3.
		{"1.5", "mL", "m3", "3/2000000", "ucum-essence.xml: l = dm3"},
		// [in_i] = 2.54 cm.
		{"0.1", "[in_i]", "m", "127/50000", "ucum-essence.xml: [in_i] = 2.54 cm"},
		// UCUM §43 ■4: f_F⁻¹(x) = 5/9 (x + 459.67); (100 + 459.67) × 5/9.
		{"100", "[degF]", "K", "279835/900", "UCUM §43 ■4"},
		// Y = 10^24 y y = 10^-24: el valor no cabe en float64 antes de
		// convertir, pero el canónico sí.
		{"1e-330", "Ym", "m", "1e-306", "ucum-essence.xml: Y = 1e24"},
		{"1e330", "ym", "m", "1e306", "ucum-essence.xml: y = 1e-24"},
		// 2^53 + 1 cae justo entre dos float64; IEEE 754 desempata al par,
		// 2^53. Con el desempate «lejos del cero» daría 2^53 + 2.
		{"9007199254740993", "m", "m", "9007199254740992", "IEEE 754 round half to even"},
		// 1 + 2^-53 es el punto medio entre 1 y 1 + 2^-52. Apenas por encima
		// sube, apenas por debajo baja: un redondeo intermedio a 64 bits pierde
		// el exceso y desempata mal.
		{"1.00000000000000011102230246251565404236316680908203125000001", "m", "m",
			"4503599627370497/4503599627370496", "IEEE 754 round to nearest"},
		{"1.00000000000000011102230246251565404236316680908203124999999", "m", "m", "1", "IEEE 754 round to nearest"},
		// Con ε ≈ 1e-200, para que tampoco alcance un intermedio de 200 bits.
		{"1.00000000000000011102230246251565404236316680908203125000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000001", "m", "m",
			"4503599627370497/4503599627370496", "IEEE 754 round to nearest"},
		// Lo mismo a través de una conversión: mg = 10^-3 g.
		{"1000.00000000000011102230246251565404236316680908203125000001", "mg", "g",
			"4503599627370497/4503599627370496", "IEEE 754 round to nearest"},
		// Un decimal de 16 cifras, largo realista en FHIR, a 2.2e-27 por
		// debajo de un punto medio: un intermedio del ancho del racional
		// redondea dos veces y sube.
		{"1.0000021281657826", "m", "m", "5000010640828913/5000000000000000", "IEEE 754 round to nearest"},
		// UCUM §30 ■4: f_C⁻¹(x) = x + 273.15; 37.5 + 273.15 = 310.65.
		{"37.5", "Cel", "K", "6213/20", "UCUM §30 ■4"},
		// Negativos: -5 + 273.15 = 268.15, y el caso del issue con signo.
		{"-5", "Cel", "K", "5363/20", "UCUM §30 ■4"},
		{"-0.07", "mg/dL", "g.m-3", "-7/10", "issue #54"},
	}
	for _, tc := range cases {
		// Value distinto de Exact: Exact tiene que ganar siempre, no solo
		// cuando Value es cero.
		q := fhir.Quantity{Value: 999, Exact: mustRat(t, tc.value), Code: tc.code, System: fhir.UCUMSystem}
		code, got, err := c.CanonicalKey(q)
		if err != nil {
			t.Errorf("CanonicalKey(%s %s): %v", tc.value, tc.code, err)
			continue
		}
		want, _ := mustRat(t, tc.rat).Float64()
		if code != tc.wantCode || got != want {
			t.Errorf("CanonicalKey(%s %s) = (%q, %v), want (%q, %v) [%s]",
				tc.value, tc.code, code, got, tc.wantCode, want, tc.source)
		}
	}
}

// Test 2: en una escala no racional no existe respuesta exacta, así que el
// valor sale de float64. El esperado se calcula a mano desde la función de
// ucum-essence.xml, con una tolerancia de pocos ulp para el orden de las
// operaciones (la desviación medida es de 1 ulp en [pH] y 0 en los demás).
// Value difiere de Exact a propósito: Exact tiene que ganar también en este
// camino.
func TestCanonicalKeyNonRationalUsesExact(t *testing.T) {
	c, _ := newComparator(t)
	// ucum-essence.xml: mol = 6.02214076e23, l = 10^-3 m3, V = kg.m2.s-2.C-1
	// y kg = 10^3 g.
	const avogadro = 6.02214076e23
	for _, tc := range []struct {
		value, code, wantCode string
		want                  float64
	}{
		// [pH] = pH(1 mol/l): x pH es 10^-x mol/l.
		{"7.4", "[pH]", "m-3", math.Pow(10, -7.4) * avogadro / 1e-3},
		// B[V] = lgTimes2(1 V): x B[V] es 10^(x/2) V.
		{"1", "B[V]", "C-1.g.m2.s-2", math.Pow(10, 0.5) * 1e3},
		// %[slope] = 100tan(1 rad) (atributo value; el elemento function dice
		// Unit="deg"; el XML no es coherente): x %[slope] es atan(x/100) rad.
		{"50", "%[slope]", "rad", math.Atan(0.5)},
	} {
		code, got, err := c.CanonicalKey(fhir.Quantity{Value: 999, Exact: mustRat(t, tc.value), Code: tc.code})
		if err != nil {
			t.Errorf("CanonicalKey(%s %s): %v", tc.value, tc.code, err)
			continue
		}
		if code != tc.wantCode || math.Abs(got-tc.want) > 1e-15*math.Abs(tc.want) {
			t.Errorf("CanonicalKey(%s %s) = (%q, %v), want (%q, %v)",
				tc.value, tc.code, code, got, tc.wantCode, tc.want)
		}
	}
}

var errSentinel = errors.New("sentinel")

// failingService devuelve un error propio desde CanonicalRat y hace fallar el
// test si alguien cae al camino float64.
type failingService struct {
	ucum.ExactService
	t *testing.T
}

func (s failingService) CanonicalRat(*big.Rat, string) (ucum.RatPair, error) {
	return ucum.RatPair{}, errSentinel
}

func (s failingService) Canonical(float64, string) (ucum.Pair, error) {
	s.t.Error("Canonical called: only ErrNotRational may fall back to float64")
	return ucum.Pair{}, nil
}

// Test 3: un error que no es ErrNotRational se devuelve, sin caer a float64.
func TestCanonicalKeyPropagatesErrors(t *testing.T) {
	_, svc := newComparator(t)
	c := fhir.NewComparatorWith(failingService{ExactService: svc, t: t})
	_, _, err := c.CanonicalKey(fhir.Quantity{Exact: big.NewRat(1, 1), Code: "mg"})
	if !errors.Is(err, errSentinel) {
		t.Errorf("CanonicalKey error = %v, want the service's own error", err)
	}
}

// Test 4: bordes de v4.3.0, fijados solo como regresión (rechazarlos es otro
// cambio, fuera del plan).
func TestCanonicalKeyEdges(t *testing.T) {
	c, _ := newComparator(t)
	code, v, err := c.CanonicalKey(fhir.Quantity{Value: math.NaN(), Code: "mg"})
	if err != nil || code != "g" || !math.IsNaN(v) {
		t.Errorf("CanonicalKey(NaN mg) = (%q, %v, %v), want (\"g\", NaN, nil)", code, v, err)
	}
	for _, sign := range []int{1, -1} {
		code, v, err = c.CanonicalKey(fhir.Quantity{Value: math.Inf(sign), Code: "mg"})
		if err != nil || code != "g" || !math.IsInf(v, sign) {
			t.Errorf("CanonicalKey(%v mg) = (%q, %v, %v), want (\"g\", %v, nil)", math.Inf(sign), code, v, err, math.Inf(sign))
		}
	}
	if _, _, err := c.CanonicalKey(fhir.Quantity{Exact: big.NewRat(1, 1), Code: "notaunit"}); err == nil {
		t.Error("CanonicalKey(notaunit) succeeded, want an error")
	}
	_, _, err = c.CanonicalKey(fhir.Quantity{Exact: big.NewRat(1, 1), Code: "mg", System: "http://snomed.info/sct"})
	if !errors.Is(err, fhir.ErrNotUCUMSystem) {
		t.Errorf("CanonicalKey(SNOMED system) error = %v, want ErrNotUCUMSystem", err)
	}
}

// variedDecimals son valores escritos como en un JSON de FHIR. Mezclan los
// que no tienen forma binaria finita (0.07, 0.1) con exponentes y enteros.
var variedDecimals = []string{
	"0.07", "0.01", "1.5", "123.456", "1e-7", "98.6", "3", "0.3",
	"37.5", "0.000123", "4.56e5", "7.25", "0.1", "2.2", "1e-3",
}

// linearCodes separa los códigos lineales. ConversionFactor(u, u) falla con
// ErrNotLinear para las unidades especiales de ucum-essence.xml (offset o
// escala logarítmica), que no se pueden escalar dentro de un término
// algebraico. Cualquier otro error hace fallar el test: nada se excluye en
// silencio.
func linearCodes(t testing.TB, svc ucum.ExactService, codes []string) (linear, special []string) {
	t.Helper()
	for _, u := range codes {
		_, err := svc.ConversionFactor(u, u)
		switch {
		case err == nil:
			linear = append(linear, u)
		case errors.Is(err, ucum.ErrNotLinear):
			special = append(special, u)
		default:
			t.Errorf("ConversionFactor(%q, %q): %v", u, u, err)
		}
	}
	return linear, special
}

var thousand = big.NewRat(1000, 1)

// checkScale es el oráculo 5: v u es la misma cantidad que v/1000 de
// 10*3.(u) y que v×1000 de 10*-3.(u). Los valores exactos son iguales, así
// que con un único redondeo la clave y el float64 coinciden bit a bit.
func checkScale(t *testing.T, c *fhir.Comparator, v *big.Rat, u string) bool {
	t.Helper()
	code, want, err := c.CanonicalKey(fhir.Quantity{Exact: v, Code: u})
	if err != nil {
		t.Errorf("CanonicalKey(%s %s): %v", v.RatString(), u, err)
		return false
	}
	ok := true
	for _, s := range []struct {
		value *big.Rat
		code  string
	}{
		{new(big.Rat).Quo(v, thousand), "10*3.(" + u + ")"},
		{new(big.Rat).Mul(v, thousand), "10*-3.(" + u + ")"},
	} {
		gotCode, got, err := c.CanonicalKey(fhir.Quantity{Exact: s.value, Code: s.code})
		if err != nil {
			t.Errorf("CanonicalKey(%s %s): %v", s.value.RatString(), s.code, err)
			ok = false
			continue
		}
		if gotCode != code || got != want {
			t.Errorf("CanonicalKey(%s %s) = (%q, %v), want (%q, %v) as for %s %s",
				s.value.RatString(), s.code, gotCode, got, code, want, v.RatString(), u)
			ok = false
		}
	}
	return ok
}

// checkCompare es el oráculo 6: la clave tiene que ordenar igual que Compare.
// Compare usa la vía exacta cuando las dos unidades son lineales, que es lo
// único que se le pasa.
func checkCompare(t *testing.T, c *fhir.Comparator, a, b fhir.Quantity) bool {
	t.Helper()
	cmp, err := c.Compare(a, b)
	if err != nil {
		t.Errorf("Compare(%s %s, %s %s): %v", a.Exact.RatString(), a.Code, b.Exact.RatString(), b.Code, err)
		return false
	}
	ka, va, errA := c.CanonicalKey(a)
	kb, vb, errB := c.CanonicalKey(b)
	if errA != nil || errB != nil {
		t.Errorf("CanonicalKey: %v, %v", errA, errB)
		return false
	}
	if ka != kb ||
		(cmp == 0 && va != vb) ||
		(cmp < 0 && va > vb) ||
		(cmp > 0 && va < vb) {
		t.Errorf("Compare(%s %s, %s %s) = %d, but keys are (%q, %v) and (%q, %v)",
			a.Exact.RatString(), a.Code, b.Exact.RatString(), b.Code, cmp, ka, va, kb, vb)
		return false
	}
	return true
}

// nearFactors mueven un valor por debajo y por encima de medio ulp de
// float64 (~1.1e-16 relativo), donde un doble redondeo puede invertir el
// orden.
var nearFactors = []*big.Rat{
	big.NewRat(1, 1),
	new(big.Rat).SetFrac(big.NewInt(1e16+1), big.NewInt(1e16)),
	new(big.Rat).SetFrac(big.NewInt(1e16-1), big.NewInt(1e16)),
	new(big.Rat).SetFrac(big.NewInt(1e17+1), big.NewInt(1e17)),
	new(big.Rat).SetFrac(big.NewInt(1e17-1), big.NewInt(1e17)),
}

// comparePairs arma pares entre unidades distintas con el mismo canónico: el
// primero con cada otro y cada uno con el siguiente, para no crecer como n².
// Con la misma unidad el doble redondeo es monótono y no discriminaría.
func comparePairs(t testing.TB, svc ucum.ExactService, linear []string) [][2]string {
	t.Helper()
	groups := map[string][]string{}
	for _, u := range linear {
		p, err := svc.CanonicalRat(big.NewRat(1, 1), u)
		if err != nil {
			t.Fatalf("CanonicalRat(1, %q): %v", u, err)
		}
		groups[p.Code] = append(groups[p.Code], u)
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var pairs [][2]string
	for _, k := range keys {
		g := groups[k]
		for i := 1; i < len(g); i++ {
			pairs = append(pairs, [2]string{g[0], g[i]})
			if i > 1 {
				pairs = append(pairs, [2]string{g[i-1], g[i]})
			}
		}
	}
	return pairs
}

// checkPair compara v u1 con el mismo valor en u2 (por el factor exacto) y con
// sus vecinos a medio ulp.
func checkPair(t *testing.T, c *fhir.Comparator, svc ucum.ExactService, v *big.Rat, u1, u2 string) bool {
	t.Helper()
	f, err := svc.ConversionFactor(u1, u2)
	if err != nil {
		t.Errorf("ConversionFactor(%q, %q): %v", u1, u2, err)
		return false
	}
	same := new(big.Rat).Mul(v, f)
	ok := true
	for _, n := range nearFactors {
		b := fhir.Quantity{Exact: new(big.Rat).Mul(same, n), Code: u2}
		ok = checkCompare(t, c, fhir.Quantity{Exact: v, Code: u1}, b) && ok
	}
	return ok
}

// stopAfter corta un subtest que ya acumuló demasiados errores, para que una
// regresión no imprima miles de líneas.
const stopAfter = 20

func TestCanonicalKeyScaleCommonUnits(t *testing.T) {
	c, svc := newComparator(t)
	linear, special := linearCodes(t, svc, fhir.CommonCodes())
	t.Logf("%d linear codes, %d special codes excluded: %v", len(linear), len(special), special)
	// Conteo del ValueSet embebido (fhir.CommonUnitsValueSetVersion): si la
	// librería clasifica distinto, la cobertura cambia y el test lo dice.
	if len(linear) != 825 || len(special) != 15 {
		t.Errorf("%d linear and %d special codes, want 825 and 15", len(linear), len(special))
	}
	fails := 0
	for _, u := range linear {
		for _, s := range variedDecimals {
			if !checkScale(t, c, mustRat(t, s), u) {
				fails++
				if fails > stopAfter {
					t.Fatal("too many failures")
				}
			}
		}
	}
}

func TestCanonicalKeyMatchesCompareCommonUnits(t *testing.T) {
	c, svc := newComparator(t)
	linear, _ := linearCodes(t, svc, fhir.CommonCodes())
	pairs := comparePairs(t, svc, linear)
	t.Logf("%d unit pairs × %d values × %d neighbors", len(pairs), len(variedDecimals), len(nearFactors))
	fails := 0
	for _, p := range pairs {
		for _, s := range variedDecimals {
			if !checkPair(t, c, svc, mustRat(t, s), p[0], p[1]) {
				fails++
				if fails > stopAfter {
					t.Fatal("too many failures")
				}
			}
		}
	}
}
