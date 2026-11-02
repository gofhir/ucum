package ucum_test

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"strings"
	"testing"

	"github.com/gofhir/ucum/v4"
	"github.com/gofhir/ucum/v4/internal/essence"
)

// Plan 012: una escala de temperatura en una multiplicación, una división o una conversión con una
// escala no racional entra a la vía float64 con su valor exacto, y el resultado
// se redondea una vez. Los esperados salen de big.Rat: (bin(v)·α + origen) ·
// tamaño del grado (UCUM §21-22), con el origen de la tabla de abajo, copiada de
// la especificación y no de special.go, y el tamaño leído del XML. El otro
// operando aporta su valor por la regla de 010/011: la potencia exacta si el
// exponente es entero y, si no, el binario del float64 que da la escala.

// offsetOrigin es el origen de cada escala de temperatura en sus propios
// grados, por nombre de función: §30■4 (Cel), §43■4 (degF) y §43■5 (degRe).
var offsetOrigin = map[string]*big.Rat{
	"Cel":   big.NewRat(27315, 100),
	"degF":  big.NewRat(45967, 100),
	"degRe": big.NewRat(21852, 100),
}

// offScale es una escala de temperatura con su prefijo.
type offScale struct {
	code   string
	alpha  *big.Rat // el factor del prefijo, o 1
	origin *big.Rat // en grados de la escala
	size   *big.Rat // el grado en kelvin, del XML
}

// exact es el valor en kelvin de v grados de la escala: (bin(v)·α + origen) ·
// tamaño.
func (sc offScale) exact(v float64) *big.Rat {
	r := new(big.Rat).SetFloat64(v)
	r.Mul(r, sc.alpha)
	r.Add(r, sc.origin)
	return r.Mul(r, sc.size)
}

// originValue es el v cuyo valor es 0 K, redondeado al float64 más cercano.
func (sc offScale) originValue() float64 {
	o := new(big.Rat).Quo(sc.origin, sc.alpha)
	f, _ := o.Neg(o).Float64()
	return f
}

// offScales arma las escalas desde ucum-essence.xml: el tamaño del grado es la
// referencia de su definición (Cel 1 K, [degF] 5/9 K, [degRe] 5/4 K).
func offScales(t *testing.T, s ucum.Service, codes []struct{ prefix, atom string }) []offScale {
	t.Helper()
	m, err := essence.Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]offScale, 0, len(codes))
	for _, c := range codes {
		atom := m.Lookup(c.atom)
		if atom == nil || atom.Value == nil || atom.Value.Function == nil {
			t.Fatalf("%s: not a special unit in the XML", c.atom)
		}
		fn := atom.Value.Function
		origin, ok := offsetOrigin[fn.Name]
		if !ok {
			t.Fatalf("%s: function %s is not a temperature scale", c.atom, fn.Name)
		}
		alpha := big.NewRat(1, 1)
		if c.prefix != "" {
			p := m.LookupPrefix(c.prefix)
			if p == nil {
				t.Fatalf("prefix %s not in the XML", c.prefix)
			}
			alpha = p.Value.Rat()
		}
		code := c.prefix + c.atom
		if err := s.Validate(code); err != nil {
			t.Fatalf("%s: %v", code, err)
		}
		size, err := s.(ucum.ExactService).CanonicalRat(fn.Value.Rat(), fn.Unit)
		if err != nil {
			t.Fatalf("CanonicalRat(%s %s): %v", fn.Value.Rat().RatString(), fn.Unit, err)
		}
		out = append(out, offScale{code: code, alpha: alpha, origin: origin, size: size.Value})
	}
	return out
}

// offsetScaleCodes son las escalas del test: Cel con prefijos y las dos no
// métricas.
var offsetScaleCodes = []struct{ prefix, atom string }{
	{"", "Cel"}, {"m", "Cel"}, {"c", "Cel"}, {"k", "Cel"}, {"M", "Cel"},
	{"", "[degF]"}, {"", "[degRe]"},
}

// offOther es el otro operando: una escala no racional con un valor, su
// contribución exacta antes del factor de la unidad y ese factor.
type offOther struct {
	name string
	pair ucum.Pair
	m    *big.Rat // lo que aporta la escala, por la regla de 010/011
	ref  *big.Rat // el factor de la referencia de la definición, del XML
}

// value es el valor canónico exacto del operando.
func (o offOther) value() *big.Rat { return new(big.Rat).Mul(o.m, o.ref) }

// offOthers son escalas no racionales de cada forma: logarítmicas con exponente
// entero y no entero, con factor de referencia distinto de 1, ln, tan y sqrt.
func offOthers(t *testing.T, s ucum.Service) []offOther {
	t.Helper()
	units := loadLogUnits(t)
	bin := func(f float64) *big.Rat { return new(big.Rat).SetFloat64(f) }
	var out []offOther
	// Exponente entero: la potencia exacta. Entero y no entero, en B, [pH] (con
	// el signo al revés), B[V] (lgTimes2) y B[kW] (factor ≠ 1).
	for _, code := range []string{"B", "[pH]", "B[V]", "B[kW]"} {
		u := unitByCode(t, units, code)
		ref := refFactor(t, s, u).Value
		for _, n := range []int{3, -2, 0, 17} {
			v, ok := exponentValue(u, n)
			if !ok {
				t.Fatalf("no float64 with exponent %d in %s", n, code)
			}
			out = append(out, offOther{name: fmt.Sprintf("%s n=%d", code, n), pair: ucum.Pair{Value: v, Code: code}, m: powRat(u.base, n), ref: ref})
		}
		// Un exponente no entero: el binario del float64 de math.Pow.
		for _, v := range []float64{2.5, -3.7} {
			e := v / u.divisor
			if u.negate {
				e = -e
			}
			out = append(out, offOther{name: fmt.Sprintf("%s v=%v", code, v), pair: ucum.Pair{Value: v, Code: code}, m: bin(math.Pow(float64(u.base), e)), ref: ref})
		}
	}
	m, err := essence.Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	refOf := func(atom string) *big.Rat {
		fn := m.Lookup(atom).Value.Function
		p, err := s.(ucum.ExactService).CanonicalRat(fn.Value.Rat(), fn.Unit)
		if err != nil {
			t.Fatalf("CanonicalRat(%s %s): %v", fn.Value.Rat().RatString(), fn.Unit, err)
		}
		return p.Value
	}
	// ln: e^v, la base de math.E (§46).
	for _, v := range []float64{1, -2.5} {
		out = append(out, offOther{name: fmt.Sprintf("Np v=%v", v), pair: ucum.Pair{Value: v, Code: "Np"}, m: bin(math.Pow(math.E, v)), ref: refOf("Np")})
	}
	// tan: f⁻¹(x) = arctan(x/100), en radianes (§44■4).
	for _, v := range []float64{7, -250} {
		out = append(out, offOther{name: fmt.Sprintf("[p'diop] v=%v", v), pair: ucum.Pair{Value: v, Code: "[p'diop]"}, m: bin(math.Atan(v / 100)), ref: refOf("[p'diop]")})
	}
	// sqrt: f⁻¹(x) = x² (§48■3).
	for _, v := range []float64{3, 1.1} {
		out = append(out, offOther{name: fmt.Sprintf("sqrt v=%v", v), pair: ucum.Pair{Value: v, Code: "[m/s2/Hz^(1/2)]"}, m: bin(v * v), ref: refOf("[m/s2/Hz^(1/2)]")})
	}
	return out
}

// offOps son las cuatro operaciones entre la temperatura T y el otro operando
// X: T·X, X·T, T/X y X/T, con el exacto de cada una.
func offOps(s ucum.Service, t, x ucum.Pair, tv, xv *big.Rat) []struct {
	name string
	got  func() (ucum.Pair, error)
	want *big.Rat // nil: el divisor es cero
} {
	quo := func(a, b *big.Rat) *big.Rat {
		if b.Sign() == 0 {
			return nil
		}
		return new(big.Rat).Quo(a, b)
	}
	return []struct {
		name string
		got  func() (ucum.Pair, error)
		want *big.Rat
	}{
		{"T·X", func() (ucum.Pair, error) { return s.Multiply(t, x) }, new(big.Rat).Mul(tv, xv)},
		{"X·T", func() (ucum.Pair, error) { return s.Multiply(x, t) }, new(big.Rat).Mul(tv, xv)},
		{"T/X", func() (ucum.Pair, error) { return s.Divide(t, x) }, quo(tv, xv)},
		{"X/T", func() (ucum.Pair, error) { return s.Divide(x, t) }, quo(xv, tv)},
	}
}

// offTemps son los valores de la temperatura: típicos, el origen y sus
// vecinos, valores grandes y 50 al azar cerca del origen y 16 en general.
func offTemps(sc offScale, rng *rand.Rand) []float64 {
	o := sc.originValue()
	vs := []float64{o, math.Nextafter(o, 0), math.Nextafter(o, math.Inf(-1)), 0, 20, 37, -40, 100, 1e-3, -1e-3, 1e5, -1e5, 1e300, -1e300}
	for i := 0; i < 50; i++ {
		vs = append(vs, (rng.Float64()*4-2)*math.Abs(o))
	}
	for i := 0; i < 16; i++ {
		vs = append(vs, rng.Float64()*1000-500)
	}
	return vs
}

// Test 1: la rejilla. Multiply y Divide de una escala de temperatura por una
// escala no racional dan el float64 más cercano al exacto, en los dos órdenes y
// con la temperatura en cada lado.
func TestOffsetProductIsNearest(t *testing.T) {
	s := newService(t)
	scales := offScales(t, s, offsetScaleCodes)
	others := offOthers(t, s)
	rng := rand.New(rand.NewSource(12))
	r := &report{t: t, name: "temperature products"}
	for _, sc := range scales {
		for _, v := range offTemps(sc, rng) {
			tv := sc.exact(v)
			tp := ucum.Pair{Value: v, Code: sc.code}
			for _, o := range others {
				for _, op := range offOps(s, tp, o.pair, tv, o.value()) {
					if op.want == nil {
						continue
					}
					want, ok := nearestOf(op.want)
					if !ok {
						continue
					}
					r.checked++
					got, err := op.got()
					if err != nil || got.Value != want {
						r.fail("%s with %v %s and %s: %v %s = (%v, %v), want %v", op.name, v, sc.code, o.name, o.pair.Value, o.pair.Code, got.Value, err, want)
					}
				}
			}
		}
	}
	r.done(50000)
}

// originNeighbors son el origen de la escala y sus vecinos a ±1 y ±2 ulp, en
// orden creciente.
func originNeighbors(sc offScale) []float64 {
	o := sc.originValue()
	inf := math.Inf(1)
	lo1, lo2 := math.Nextafter(o, -inf), math.Nextafter(math.Nextafter(o, -inf), -inf)
	hi1, hi2 := math.Nextafter(o, inf), math.Nextafter(math.Nextafter(o, inf), inf)
	return []float64{lo2, lo1, o, hi1, hi2}
}

// Test 2: el origen de cada escala (-273.15 Cel, -459.67 [degF], -218.52
// [degRe], -0.27315 kCel) y sus vecinos: el origen no da error, el vecino
// conserva sus cifras, el signo cambia en el cruce exacto y el resultado es
// monótono en v.
func TestOffsetOriginAndNeighbors(t *testing.T) {
	s := newService(t)
	scales := offScales(t, s, offsetScaleCodes)
	others := offOthers(t, s)
	r := &report{t: t, name: "origin neighbors"}
	for _, sc := range scales {
		vs := originNeighbors(sc)
		for _, o := range others {
			var prev float64
			for i, v := range vs {
				tv := sc.exact(v)
				tp := ucum.Pair{Value: v, Code: sc.code}
				want, ok := nearestOf(new(big.Rat).Mul(tv, o.value()))
				if !ok {
					continue
				}
				r.checked++
				got, err := s.Multiply(o.pair, tp)
				if err != nil || got.Value != want {
					r.fail("Multiply(%v %s, %v %s) = (%v, %v), want %v", o.pair.Value, o.pair.Code, v, sc.code, got.Value, err, want)
					continue
				}
				// El signo del resultado es el del exacto (el de T·X, con X
				// positivo; el de [p'diop] y sqrt con v < 0 lo cambia a X).
				if wantSign := tv.Sign() * o.value().Sign(); sign(got.Value) != wantSign {
					r.fail("Multiply(%v %s, %v %s) = %v, sign %d, want %d", o.pair.Value, o.pair.Code, v, sc.code, got.Value, sign(got.Value), wantSign)
				}
				// Monótono en v: no decreciente con X positivo, no creciente con
				// X negativo.
				if i > 0 {
					if o.value().Sign() > 0 && got.Value < prev || o.value().Sign() < 0 && got.Value > prev {
						r.fail("Multiply(%v %s, v) not monotonic at %v %s: %v after %v", o.pair.Value, o.pair.Code, v, sc.code, got.Value, prev)
					}
				}
				prev = got.Value
			}
		}
	}
	r.done(150)
	// Los tres del plan con el valor escrito a mano.
	for _, tc := range []struct {
		v    float64
		code string
	}{{-273.15, "Cel"}, {-459.67, "[degF]"}, {-218.52, "[degRe]"}, {-0.27315, "kCel"}} {
		if _, err := s.Multiply(ucum.Pair{Value: 3, Code: "B"}, ucum.Pair{Value: tc.v, Code: tc.code}); err != nil {
			t.Errorf("Multiply(3 B, %v %s) = %v, want a value", tc.v, tc.code, err)
		}
	}
}

func sign(f float64) int {
	switch {
	case f > 0:
		return 1
	case f < 0:
		return -1
	}
	return 0
}

// Test 3: un cero exacto con prefijo. -273150 mCel, -27315 cCel y -2731.5 dCel
// son exactamente 0 K: la multiplicación da 0 en los dos órdenes, y el divisor cero
// es ErrDivisionByZero.
func TestOffsetExactZero(t *testing.T) {
	s := newService(t)
	scales := offScales(t, s, []struct{ prefix, atom string }{{"m", "Cel"}, {"c", "Cel"}, {"d", "Cel"}})
	others := offOthers(t, s)
	zeros := []float64{-273150, -27315, -2731.5}
	r := &report{t: t, name: "exact zero"}
	for i, sc := range scales {
		v := zeros[i]
		if sc.exact(v).Sign() != 0 {
			t.Fatalf("oracle: %v %s is %s K, want exactly 0", v, sc.code, sc.exact(v).RatString())
		}
		z := ucum.Pair{Value: v, Code: sc.code}
		for _, o := range others {
			// Todo operando de la lista vale algo distinto de cero (0 B es 1).
			r.checked += 4
			if got, err := s.Multiply(z, o.pair); err != nil || got.Value != 0 {
				r.fail("Multiply(%v %s, %s) = (%v, %v), want 0", v, sc.code, o.name, got.Value, err)
			}
			if got, err := s.Multiply(o.pair, z); err != nil || got.Value != 0 {
				r.fail("Multiply(%s, %v %s) = (%v, %v), want 0", o.name, v, sc.code, got.Value, err)
			}
			if got, err := s.Divide(z, o.pair); err != nil || got.Value != 0 {
				r.fail("Divide(%v %s, %s) = (%v, %v), want 0", v, sc.code, o.name, got.Value, err)
			}
			if _, err := s.Divide(o.pair, z); !errors.Is(err, ucum.ErrDivisionByZero) {
				r.fail("Divide(%s, %v %s) = %v, want ErrDivisionByZero", o.name, v, sc.code, err)
			}
		}
	}
	r.done(100)
}

// Test 4: el rango. ErrOutOfRange solo donde el exacto no cabe; un subnormal
// distinto de cero se acepta; un operando que no cabe aunque el resultado sí
// (1.7e308 MCel es 1.7e314 K) da el resultado.
func TestOffsetRange(t *testing.T) {
	s := newService(t)
	sc := offScales(t, s, []struct{ prefix, atom string }{{"", "Cel"}, {"M", "Cel"}})
	cel, mega := sc[0], sc[1]
	pow := func(n int, tv *big.Rat) *big.Rat { return new(big.Rat).Mul(powRat(10, n), tv) }
	for _, tc := range []struct {
		name string
		n    float64
		sc   offScale
		v    float64
		want *big.Rat // nil: ErrOutOfRange
	}{
		// 10^-320 · 2.27e-14 = 2.3e-334, bajo el menor subnormal.
		{"-320 B · -273.15 Cel", -320, cel, -273.15, nil},
		// 10^-300 · 2.27e-14 = 2.3e-314: subnormal, aceptado.
		{"-300 B · -273.15 Cel", -300, cel, -273.15, pow(-300, cel.exact(-273.15))},
		{"-320 B · 1.7e308 MCel", -320, mega, 1.7e308, pow(-320, mega.exact(1.7e308))},
		{"-1 B · 1.7e308 MCel", -1, mega, 1.7e308, nil},
		{"-320 B · -1.7e308 MCel", -320, mega, -1.7e308, pow(-320, mega.exact(-1.7e308))},
	} {
		b := ucum.Pair{Value: tc.n, Code: "B"}
		temp := ucum.Pair{Value: tc.v, Code: tc.sc.code}
		for _, swap := range []bool{false, true} {
			a, c := b, temp
			if swap {
				a, c = c, a
			}
			got, err := s.Multiply(a, c)
			if tc.want == nil {
				if !errors.Is(err, ucum.ErrOutOfRange) {
					t.Errorf("Multiply %s (swap %v) = (%v, %v), want ErrOutOfRange", tc.name, swap, got.Value, err)
				}
				continue
			}
			want, ok := nearestOf(tc.want)
			if !ok {
				t.Fatalf("%s: oracle %s does not fit", tc.name, tc.want.FloatString(3))
			}
			if err != nil || got.Value != want {
				t.Errorf("Multiply %s (swap %v) = (%v, %v), want %v", tc.name, swap, got.Value, err, want)
			}
		}
	}
	// El mismo resultado por Divide, con el otro operando en el divisor.
	got, err := s.Divide(ucum.Pair{Value: 1.7e308, Code: "MCel"}, ucum.Pair{Value: 320, Code: "B"})
	want, _ := nearestOf(new(big.Rat).Mul(mega.exact(1.7e308), powRat(10, -320)))
	if err != nil || got.Value != want {
		t.Errorf("Divide(1.7e308 MCel, 320 B) = (%v, %v), want %v", got.Value, err, want)
	}
}

// Test 5: la temperatura como divisor. 3 B / -273.15 Cel = 10^3 / 2.27e-14.
func TestOffsetAsDivisor(t *testing.T) {
	s := newService(t)
	sc := offScales(t, s, []struct{ prefix, atom string }{{"", "Cel"}, {"", "[degF]"}, {"", "[degRe]"}})
	for _, c := range sc {
		v := c.originValue()
		for _, w := range []float64{v, math.Nextafter(v, 0), math.Nextafter(v, math.Inf(-1)), 20} {
			tv := c.exact(w)
			if tv.Sign() == 0 {
				continue
			}
			want, ok := nearestOf(new(big.Rat).Quo(powRat(10, 3), tv))
			if !ok {
				t.Fatalf("oracle for %v %s does not fit", w, c.code)
			}
			got, err := s.Divide(ucum.Pair{Value: 3, Code: "B"}, ucum.Pair{Value: w, Code: c.code})
			if err != nil || got.Value != want {
				t.Errorf("Divide(3 B, %v %s) = (%v, %v), want %v", w, c.code, got.Value, err, want)
			}
		}
	}
}

// Test 6: definiciones propias (NewFromReader): [lgK] = lg(1 K), una escala
// logarítmica de 1 K, hacia y desde las escalas de temperatura. El destino
// recibe el valor exacto, redondeado una vez, aunque el intermedio no quepa
// (10^-400 K) y sin el tope de 330 del log10.
func TestOffsetCustomLogScale(t *testing.T) {
	const custom = `<?xml version="1.0" encoding="ascii"?>
<root xmlns="http://unitsofmeasure.org/ucum-essence" version="9.9" revision="N/A"
      revision-date="2099-01-01">
 <prefix Code="m" CODE="M"><name>milli</name><printSymbol>m</printSymbol><value value="1e-3">1e-3</value></prefix>
 <prefix Code="Y" CODE="YA"><name>yotta</name><printSymbol>Y</printSymbol><value value="1e24">1e24</value></prefix>
 <prefix Code="n" CODE="N"><name>nano</name><printSymbol>n</printSymbol><value value="1e-9">1e-9</value></prefix>
 <prefix Code="x" CODE="X"><name>midpoint</name><printSymbol>x</printSymbol><value value="2.359296e-14">2.359296e-14</value></prefix>
 <base-unit Code="K" CODE="K" dim="C"><name>kelvin</name><printSymbol>K</printSymbol><property>temperature</property></base-unit>
 <unit Code="Cel" CODE="CEL" isMetric="yes" isSpecial="yes" class="si">
  <name>degree Celsius</name><printSymbol>C</printSymbol><property>temperature</property>
  <value Unit="cel(1 K)" UNIT="CEL(1 K)"><function name="Cel" value="1" Unit="K"/></value>
 </unit>
 <unit Code="[degF]" CODE="[DEGF]" isMetric="no" isSpecial="yes" class="heat">
  <name>degree Fahrenheit</name><printSymbol>F</printSymbol><property>temperature</property>
  <value Unit="degf(5 K/9)" UNIT="DEGF(5 K/9)"><function name="degF" value="5" Unit="K/9"/></value>
 </unit>
 <unit Code="[lgK]" CODE="[LGK]" isMetric="no" isSpecial="yes" class="heat">
  <name>lgK</name><printSymbol>lgK</printSymbol><property>temperature</property>
  <value Unit="lg(1 K)" UNIT="LG(1 K)"><function name="lg" value="1" Unit="K"/></value>
 </unit>
 <unit Code="[lnK]" CODE="[LNK]" isMetric="no" isSpecial="yes" class="heat">
  <name>lnK</name><printSymbol>lnK</printSymbol><property>temperature</property>
  <value Unit="ln(1 K)" UNIT="LN(1 K)"><function name="ln" value="1" Unit="K"/></value>
 </unit>
 <unit Code="[lnhK]" CODE="[LNHK]" isMetric="no" isSpecial="yes" class="heat">
  <name>lnhK</name><printSymbol>lnhK</printSymbol><property>temperature</property>
  <value Unit="ln(100 K)" UNIT="LN(100 K)"><function name="ln" value="100" Unit="K"/></value>
 </unit>
 <unit Code="[sqK]" CODE="[SQK]" isMetric="no" isSpecial="yes" class="heat">
  <name>sqK</name><printSymbol>sqK</printSymbol><property>temperature</property>
  <value Unit="sqrt(1 K)" UNIT="SQRT(1 K)"><function name="sqrt" value="1" Unit="K"/></value>
 </unit>
</root>`
	svc, err := ucum.NewFromReader(strings.NewReader(custom))
	if err != nil {
		t.Fatal(err)
	}
	def := newService(t)
	sc := offScales(t, def, []struct{ prefix, atom string }{{"", "Cel"}, {"m", "Cel"}, {"", "[degF]"}, {"Y", "Cel"}, {"n", "Cel"}})
	cel, mcel, degF, ycel, ncel := sc[0], sc[1], sc[2], sc[3], sc[4]
	// Un prefijo propio, que no está en el XML oficial: el mismo Cel con su α.
	xcel := cel
	xcel.alpha, _ = new(big.Rat).SetString("2.359296e-14")
	// kelvin(n) = 10^n K; en la escala sc: kelvin/tamaño/α - origen/α.
	from := func(k *big.Rat, c offScale) *big.Rat {
		r := new(big.Rat).Quo(k, c.size)
		r.Sub(r, c.origin)
		return r.Quo(r, c.alpha)
	}
	for _, tc := range []struct {
		v    float64
		to   string
		want *big.Rat // nil: ErrOutOfRange
	}{
		{2, "Cel", from(powRat(10, 2), cel)},
		{2, "mCel", from(powRat(10, 2), mcel)},
		{2, "[degF]", from(powRat(10, 2), degF)},
		{-400, "Cel", from(powRat(10, -400), cel)},
		{-400, "[degF]", from(powRat(10, -400), degF)},
		{-400, "mCel", from(powRat(10, -400), mcel)},
		{400, "Cel", nil},
		// Un intermedio sobre 10^330 cabe con un prefijo grande: 10^331 K es
		// 10^307 YCel. El tope de 330 no aplica hacia una escala de temperatura.
		{331, "YCel", from(powRat(10, 331), ycel)},
		// Bajo el tope de costo de 10^-5000 no hay exacto, pero el aporte es
		// menor que medio ulp del origen: -273.15 + ε redondea como -273.15.
		{-5001, "Cel", from(powRat(10, -5001), cel)},
		{-1e300, "Cel", from(new(big.Rat), cel)},
		// Con α = 10^-9, -273.15/α en float64 no es el más cercano a f(0)/α.
		{-5001, "nCel", from(powRat(10, -5001), ncel)},
		// Con α = 2.359296e-14, 273.15/α es 607·5^19, justo un punto medio
		// entre dos float64: decide el signo del intermedio, que es positivo.
		{-5001, "xCel", from(powRat(10, -5001), xcel)},
	} {
		got, err := svc.Convert(tc.v, "[lgK]", tc.to)
		if tc.want == nil {
			if !errors.Is(err, ucum.ErrOutOfRange) {
				t.Errorf("Convert(%v, [lgK], %q) = (%v, %v), want ErrOutOfRange", tc.v, tc.to, got, err)
			}
			continue
		}
		want, _ := nearestOf(tc.want)
		if err != nil || got != want {
			t.Errorf("Convert(%v, [lgK], %q) = (%v, %v), want %v", tc.v, tc.to, got, err, want)
		}
	}
	// ln (su base no es racional, sin exacto): bajo el rango normal el aporte es
	// menor que medio ulp del origen, así que el resultado es el origen.
	// Entre e^-708 (bajo el rango normal) y e^-760 (bajo 10^-330) el
	// intermedio es subnormal o menor: también da el origen.
	for _, to := range []struct {
		code string
		c    offScale
	}{{"Cel", cel}, {"[degF]", degF}, {"mCel", mcel}, {"nCel", ncel}} {
		want, _ := nearestOf(from(new(big.Rat), to.c))
		for _, v := range []float64{-709, -746, -750, -759, -760, -800} {
			if got, err := svc.Convert(v, "[lnK]", to.code); err != nil || got != want {
				t.Errorf("Convert(%v, [lnK], %q) = (%v, %v), want %v", v, to.code, got, err, want)
			}
		}
	}
	// Con factor 100, un e^v subnormal da un intermedio normal sin exacto: entra
	// como su binario y se redondea una vez, no como (m - 273.15)/α en float64.
	// El intermedio es menor que 1e-290 K, así que el más cercano es el origen.
	originN, _ := nearestOf(from(new(big.Rat), ncel))
	for _, v := range []float64{-709, -710, -712} {
		if got, err := svc.Convert(v, "[lnhK]", "nCel"); err != nil || got != originN {
			t.Errorf("Convert(%v, [lnhK], nCel) = (%v, %v), want %v", v, got, err, originN)
		}
	}
	// Un cero exacto (-273150 mCel es 0 K) hacia una raíz cuadrada da 0, no un
	// subdesborde: sqrt(0 K) = 0.
	if got, err := svc.Convert(-273150, "mCel", "[sqK]"); err != nil || got != 0 {
		t.Errorf("Convert(-273150, mCel, [sqK]) = (%v, %v), want 0", got, err)
	}
	// De la temperatura a [lgK]: log10 del exacto en kelvin; un exacto que no
	// es positivo no tiene logaritmo (ErrOutOfDomain), también el de [degF].
	closeTo := func(got, want float64) bool { return math.Abs(got-want) <= 2e-15*math.Max(1, math.Abs(want)) }
	for _, tc := range []struct {
		c offScale
		v float64
	}{
		{cel, -273.15}, {cel, -173.15}, {cel, -273.1499999999999}, {cel, 20},
		{degF, -459.67}, {degF, math.Nextafter(-459.67, math.Inf(-1))}, {degF, -459.6699999999999}, {degF, 68},
	} {
		tv := tc.c.exact(tc.v)
		got, err := svc.Convert(tc.v, tc.c.code, "[lgK]")
		if tv.Sign() <= 0 {
			if !errors.Is(err, ucum.ErrOutOfDomain) {
				t.Errorf("Convert(%v, %q, [lgK]) = (%v, %v), want ErrOutOfDomain (exact %s K)", tc.v, tc.c.code, got, err, tv.FloatString(20))
			}
			continue
		}
		f, _ := tv.Float64()
		if want := math.Log10(f); err != nil || !closeTo(got, want) {
			t.Errorf("Convert(%v, %q, [lgK]) = (%v, %v), want about %v", tc.v, tc.c.code, got, err, want)
		}
	}
}

// Test 7: lo que no cambia. Canonical y Convert de las escalas de temperatura
// (la vía racional, el más cercano al exacto) y una multiplicación de B por m.
func TestOffsetLeavesOtherPathsAlone(t *testing.T) {
	s := newService(t)
	scales := offScales(t, s, offsetScaleCodes)
	rng := rand.New(rand.NewSource(7))
	r := &report{t: t, name: "rational paths"}
	for _, sc := range scales {
		for _, v := range offTemps(sc, rng)[:60] {
			want, ok := nearestOf(sc.exact(v))
			if !ok {
				continue
			}
			r.checked++
			if got, err := s.Canonical(v, sc.code); err != nil || got.Value != want || got.Code != "K" {
				r.fail("Canonical(%v, %q) = (%v %q, %v), want %v K", v, sc.code, got.Value, got.Code, err, want)
			}
			// Convert a kelvin y a otra escala, también racionales.
			if got, err := s.Convert(v, sc.code, "K"); err != nil || got != want {
				r.fail("Convert(%v, %q, K) = (%v, %v), want %v", v, sc.code, got, err, want)
			}
		}
	}
	r.done(300)
	// B por m: sin temperatura, la potencia exacta por el valor.
	for _, tc := range []struct {
		n, m float64
	}{{3, 2}, {-2, 7}, {0, 0.1}, {17, 3}} {
		want, _ := nearestOf(new(big.Rat).Mul(powRat(10, int(tc.n)), new(big.Rat).SetFloat64(tc.m)))
		got, err := s.Multiply(ucum.Pair{Value: tc.n, Code: "B"}, ucum.Pair{Value: tc.m, Code: "m"})
		if err != nil || got.Value != want {
			t.Errorf("Multiply(%v B, %v m) = (%v, %v), want %v", tc.n, tc.m, got.Value, err, want)
		}
	}
}
