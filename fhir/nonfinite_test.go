package fhir_test

import (
	"errors"
	"math"
	"math/big"
	"strconv"
	"strings"
	"testing"

	"github.com/gofhir/ucum/v4"
	"github.com/gofhir/ucum/v4/fhir"
)

// Plan 008 (#55). Una Quantity cuyo valor no es un número finito de float64 no
// tiene clave, ni se compara, ni se suma. FHIR define decimal como «A rational
// number with implicit precision» y pide «Do not use an IEEE type floating
// point type» (StructureDefinition-decimal, R4, R4B y R5). Los esperados
// salen de esa regla (§2 del plan), no de correr el código.

var (
	nonFinite = []float64{math.NaN(), math.Inf(1), math.Inf(-1)}

	// Lineales, especiales racionales sin y con prefijo, sin forma racional y
	// arbitrarias.
	nonFiniteUnits = []string{"mg", "mL", "Cel", "mCel", "[pH]", "B", "[IU]"}
)

// Test 1: un Value no finito, sin Exact, no tiene clave.
func TestCanonicalKeyRejectsNonFinite(t *testing.T) {
	c, _ := newComparator(t)
	for _, u := range nonFiniteUnits {
		for _, v := range nonFinite {
			code, got, err := c.CanonicalKey(fhir.Quantity{Value: v, Code: u})
			if !errors.Is(err, fhir.ErrNotFinite) || code != "" || got != 0 {
				t.Errorf("CanonicalKey(%v %s) = (%q, %v, %v), want fhir.ErrNotFinite", v, u, code, got, err)
			}
		}
	}
}

// Test 2: Exact manda sobre Value (godoc de Quantity.Exact).
func TestCanonicalKeyExactOverridesNonFiniteValue(t *testing.T) {
	c, _ := newComparator(t)
	want, _, err := c.CanonicalKey(fhir.Quantity{Exact: big.NewRat(37, 1), Code: "mg"})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range nonFinite {
		code, got, err := c.CanonicalKey(fhir.Quantity{Value: v, Exact: big.NewRat(37, 1), Code: "mg"})
		if err != nil || code != want || got != 0.037 {
			t.Errorf("CanonicalKey(Value %v, Exact 37 mg) = (%q, %v, %v), want (%q, 0.037, nil)", v, code, got, err, want)
		}
	}
}

// Test 3: en una escala sin forma racional, un Exact demasiado grande para
// float64 se lee como el mayor finito, y la regla de rango de #59 se aplica al
// resultado. 10^(±1e400) no cabe en float64: ErrOutOfRange. Antes el Exact
// redondeaba a ±Inf, la API base lo dejaba pasar, y Compare llamaba iguales a
// 1e400 B y 1e401 B.
func TestExactOutOfRangeOnFloatPath(t *testing.T) {
	c, _ := newComparator(t)
	for _, tc := range []struct{ u, e string }{
		{"[pH]", "1e400"}, {"[pH]", "-1e400"}, {"B", "1e400"}, {"B", "-1e400"},
		// atan(1e-402) y (1e-400)² no caben: un Exact diminuto no se lee como 0.
		{"[p'diop]", "1e-400"}, {"[p'diop]", "-1e-400"}, {"%[slope]", "1e-400"},
		{"[m/s2/Hz^(1/2)]", "1e-400"},
	} {
		u, e := tc.u, tc.e
		{
			q := fhir.Quantity{Exact: mustRat(t, e), Code: u}
			_, got, err := c.CanonicalKey(q)
			var ve *ucum.ValidationError
			if !errors.Is(err, ucum.ErrOutOfRange) || !errors.As(err, &ve) {
				t.Errorf("CanonicalKey(%s %s) = (%v, %v), want ucum.ErrOutOfRange in a ValidationError", e, u, got, err)
			} else if !strings.Contains(err.Error(), new(big.Float).SetRat(mustRat(t, e)).Text('g', 6)) {
				// El mensaje nombra el valor de quien llama, no el float64 en que se leyó.
				t.Errorf("CanonicalKey(%s %s) = %v, want the message to name %s", e, u, err, e)
			}
			for _, other := range []fhir.Quantity{q, {Value: 1, Code: u}} {
				if n, err := c.Compare(q, other); !errors.Is(err, ucum.ErrOutOfRange) {
					t.Errorf("Compare(%s %s, %v) = (%d, %v), want ucum.ErrOutOfRange", e, u, other, n, err)
				}
				if n, err := c.Compare(other, q); !errors.Is(err, ucum.ErrOutOfRange) {
					t.Errorf("Compare(%v, %s %s) = (%d, %v), want ucum.ErrOutOfRange", other, e, u, n, err)
				}
			}
		}
	}
	if n, err := c.Compare(fhir.Quantity{Exact: mustRat(t, "1e400"), Code: "B"}, fhir.Quantity{Exact: mustRat(t, "1e401"), Code: "B"}); err == nil {
		t.Errorf("Compare(1e400 B, 1e401 B) = %d, nil; the two values differ", n)
	}

	// El error de la vía racional lleva el código y ninguna posición.
	_, _, err := c.CanonicalKey(fhir.Quantity{Exact: mustRat(t, "1e400"), Code: "m"})
	var ve *ucum.ValidationError
	if !errors.As(err, &ve) || ve.Code != "m" || ve.Offset != -1 || !errors.Is(err, ucum.ErrOutOfRange) {
		t.Errorf("CanonicalKey(1e400 m) = %v, want a ValidationError for \"m\" at no position wrapping ucum.ErrOutOfRange", err)
	}
}

// Test 3b: lo que sí tiene clave en la vía float64. Un Exact que redondea a 0
// tiene la clave de 0: f es continua, y f(1e-400) redondea al mismo float64
// que f(0). Un Exact enorme en una escala que se satura tiene la clave del
// límite: atan(1e400/100) redondea a π/2, igual que atan(1e300/100).
func TestFloatPathKeysThatFit(t *testing.T) {
	c, _ := newComparator(t)
	for _, tc := range []struct{ code, small, ref string }{
		{"[pH]", "1e-400", "0"},
		{"[pH]", "-1e-400", "0"},
		{"B", "1e-400", "0"},
		{"[p'diop]", "1e400", "1e300"},
		{"[p'diop]", "-1e400", "-1e300"},
	} {
		wantCode, want, err := c.CanonicalKey(fhir.Quantity{Exact: mustRat(t, tc.ref), Code: tc.code})
		if err != nil {
			t.Fatalf("CanonicalKey(%s %s) = %v", tc.ref, tc.code, err)
		}
		code, got, err := c.CanonicalKey(fhir.Quantity{Exact: mustRat(t, tc.small), Code: tc.code})
		if err != nil || code != wantCode || got != want {
			t.Errorf("CanonicalKey(%s %s) = (%q, %v, %v), want the key of %s: (%q, %v)", tc.small, tc.code, code, got, err, tc.ref, wantCode, want)
		}
	}
}

// Test 4: primero el sistema, después el valor, después el código y las
// unidades.
func TestNonFiniteErrorOrder(t *testing.T) {
	c, _ := newComparator(t)
	nan := math.NaN()
	other := fhir.Quantity{System: "http://example.org", Value: nan, Code: "mg"}
	if _, _, err := c.CanonicalKey(other); !errors.Is(err, fhir.ErrNotUCUMSystem) {
		t.Errorf("CanonicalKey(NaN, other system) = %v, want fhir.ErrNotUCUMSystem", err)
	}
	one := fhir.Quantity{Value: 1, Code: "mg"}
	if _, err := c.Compare(other, one); !errors.Is(err, fhir.ErrNotUCUMSystem) {
		t.Errorf("Compare(NaN, other system) = %v, want fhir.ErrNotUCUMSystem", err)
	}
	if _, err := c.Add(other, one); !errors.Is(err, fhir.ErrNotUCUMSystem) {
		t.Errorf("Add(NaN, other system) = %v, want fhir.ErrNotUCUMSystem", err)
	}
	if _, err := c.Sub(one, other); !errors.Is(err, fhir.ErrNotUCUMSystem) {
		t.Errorf("Sub(1, NaN with other system) = %v, want fhir.ErrNotUCUMSystem", err)
	}

	// Un Exact enorme no adelanta un error de rango a uno del código, de
	// ningún lado, y no cambia el de unidades no comparables.
	huge := mustRat(t, "1e400")
	for _, pair := range [][2]fhir.Quantity{
		{{Value: 1, Code: "[pH]"}, {Exact: huge, Code: "xyz"}},
		{{Exact: huge, Code: "xyz"}, {Value: 1, Code: "[pH]"}},
		{{Exact: huge, Code: "mg"}, {Value: 1, Code: "[pH]"}},
		{{Value: 1, Code: "[pH]"}, {Exact: huge, Code: "mg"}},
	} {
		if _, err := c.Compare(pair[0], pair[1]); err == nil || errors.Is(err, ucum.ErrOutOfRange) {
			t.Errorf("Compare(%s %s, %s %s) = %v, want the error of the code or of the units, not ucum.ErrOutOfRange",
				describe(pair[0]), pair[0].Code, describe(pair[1]), pair[1].Code, err)
		}
	}

	bad := fhir.Quantity{Value: nan, Code: "xyz"}
	if _, _, err := c.CanonicalKey(bad); !errors.Is(err, fhir.ErrNotFinite) {
		t.Errorf("CanonicalKey(NaN xyz) = %v, want fhir.ErrNotFinite before the code is read", err)
	}
	for _, pair := range [][2]fhir.Quantity{
		{bad, {Value: 1, Code: "m"}},
		{{Value: nan, Code: "mg"}, {Value: 1, Code: "m"}},    // no comparables
		{{Value: nan, Code: "Cel"}, {Value: 1, Code: "Cel"}}, // escala afín
	} {
		if _, err := c.Compare(pair[0], pair[1]); !errors.Is(err, fhir.ErrNotFinite) {
			t.Errorf("Compare(%v, %v) = %v, want fhir.ErrNotFinite", pair[0], pair[1], err)
		}
		if _, err := c.Add(pair[0], pair[1]); !errors.Is(err, fhir.ErrNotFinite) {
			t.Errorf("Add(%v, %v) = %v, want fhir.ErrNotFinite", pair[0], pair[1], err)
		}
		if _, err := c.Sub(pair[0], pair[1]); !errors.Is(err, fhir.ErrNotFinite) {
			t.Errorf("Sub(%v, %v) = %v, want fhir.ErrNotFinite", pair[0], pair[1], err)
		}
	}
}

// Test 5: Compare, Add y Sub envuelven el sentinela, con el no finito a cada
// lado, y el texto no cambia.
func TestNonFiniteSentinel(t *testing.T) {
	c, _ := newComparator(t)
	ops := []struct {
		name string
		f    func(a, b fhir.Quantity) error
	}{
		{"Compare", func(a, b fhir.Quantity) error { _, err := c.Compare(a, b); return err }},
		{"Add", func(a, b fhir.Quantity) error { _, err := c.Add(a, b); return err }},
		{"Sub", func(a, b fhir.Quantity) error { _, err := c.Sub(a, b); return err }},
	}
	one := fhir.Quantity{Value: 1, Code: "mg"}
	for _, v := range nonFinite {
		q := fhir.Quantity{Value: v, Code: "mg"}
		for _, op := range ops {
			for _, args := range [][2]fhir.Quantity{{q, one}, {one, q}} {
				err := op.f(args[0], args[1])
				if !errors.Is(err, fhir.ErrNotFinite) || err.Error() != "quantity value is not finite" {
					t.Errorf("%s(%v, %v) = %v, want fhir.ErrNotFinite with its text unchanged", op.name, args[0].Value, args[1].Value, err)
				}
			}
		}
	}
}

// Test 6: si CanonicalKey no da error, el valor es finito; si Compare da 0 sin
// error y las dos cantidades tienen clave, las claves son iguales. Una clave
// puede faltar donde Compare sí responde: en una escala racional Compare es
// exacto, y 1e400 mL es igual a 1e400 mL aunque no quepa en float64.
func TestCanonicalKeyFiniteInvariant(t *testing.T) {
	c, _ := newComparator(t)
	finite := []float64{0, -1.5, 1e300}
	exacts := []string{"1/3", "1e400", "-1e400", "1e-400", "-1e-400"}
	qs := make([]fhir.Quantity, 0, len(nonFiniteUnits)*(len(finite)+len(nonFinite)+len(exacts)))
	for _, u := range nonFiniteUnits {
		for _, v := range append(finite, nonFinite...) {
			qs = append(qs, fhir.Quantity{Value: v, Code: u})
		}
		for _, e := range exacts {
			qs = append(qs, fhir.Quantity{Exact: mustRat(t, e), Code: u})
		}
	}
	for _, q := range qs {
		code, v, err := c.CanonicalKey(q)
		if err == nil && (math.IsNaN(v) || math.IsInf(v, 0)) {
			t.Errorf("CanonicalKey(%v %s) = (%q, %v), nil: a key with a value that is not finite", describe(q), q.Code, code, v)
		}
	}
	for _, a := range qs {
		for _, b := range qs {
			if n, err := c.Compare(a, b); err != nil || n != 0 {
				continue
			}
			ka, va, erra := c.CanonicalKey(a)
			kb, vb, errb := c.CanonicalKey(b)
			if erra != nil || errb != nil {
				continue
			}
			if ka != kb || va != vb {
				t.Errorf("Compare(%s %s, %s %s) = 0, nil, but their keys are (%q, %v) and (%q, %v)",
					describe(a), a.Code, describe(b), b.Code, ka, va, kb, vb)
			}
		}
	}
}

// Test 8: la API base no cambia (#59 decidió que deja pasar lo no finito).
func TestBaseAPIPassesNonFiniteThrough(t *testing.T) {
	svc, err := ucum.New()
	if err != nil {
		t.Fatal(err)
	}
	p, err := svc.Canonical(math.NaN(), "mg")
	if err != nil || !math.IsNaN(p.Value) {
		t.Errorf("Canonical(NaN, mg) = (%v, %v), want NaN and no error", p.Value, err)
	}
}

// describe muestra el valor de una cantidad en pocas cifras.
func describe(q fhir.Quantity) string {
	if q.Exact != nil {
		return new(big.Float).SetRat(q.Exact).Text('g', 6)
	}
	return strconv.FormatFloat(q.Value, 'g', 6, 64)
}

// Test 3c: un par que mezcla una escala sin forma racional con una unidad
// lineal con Exact. La lineal se lleva a float64 desde su forma exacta, con la
// regla de CanonicalKey, y no desde el Exact redondeado. Los esperados salen de
// la aritmética: n B es 10^n, y el factor de % es 1/100 y el de 10*30, 10^30.
func TestCompareMixedPairs(t *testing.T) {
	c, _ := newComparator(t)
	for _, tc := range []struct {
		a, b fhir.Quantity
		want int
	}{
		// 10^2 = 100 (exacto en float64) contra 1e-28 · 10^30 = 100. No se usa
		// -300 B: la API base da 10^-300 a 3 ulp (math.Pow), un defecto aparte
		// anotado en el plan.
		{fhir.Quantity{Value: 2, Code: "B"}, fhir.Quantity{Exact: mustRat(t, "1e-28"), Code: "10*30"}, 0},
		// 10^308 contra 1e300 / 100 = 10^298.
		{fhir.Quantity{Value: 308, Code: "B"}, fhir.Quantity{Exact: mustRat(t, "1e300"), Code: "%"}, 1},
		{fhir.Quantity{Exact: mustRat(t, "1e300"), Code: "%"}, fhir.Quantity{Value: 308, Code: "B"}, -1},
	} {
		got, err := c.Compare(tc.a, tc.b)
		if err != nil || got != tc.want {
			t.Errorf("Compare(%s %s, %s %s) = (%d, %v), want %d", describe(tc.a), tc.a.Code, describe(tc.b), tc.b.Code, got, err, tc.want)
		}
	}
	// 1e400 / 100 = 10^398 no cabe en float64, de ningún lado.
	huge := fhir.Quantity{Exact: mustRat(t, "1e400"), Code: "%"}
	for _, pair := range [][2]fhir.Quantity{{{Value: 308, Code: "B"}, huge}, {huge, {Value: 308, Code: "B"}}} {
		if got, err := c.Compare(pair[0], pair[1]); !errors.Is(err, ucum.ErrOutOfRange) {
			t.Errorf("Compare(%s %s, %s %s) = (%d, %v), want ucum.ErrOutOfRange", describe(pair[0]), pair[0].Code, describe(pair[1]), pair[1].Code, got, err)
		}
	}
}

// Test 4b: el error de unidades no comparables nombra los códigos de quien
// llama, en la vía racional y en la de float64.
func TestNotComparableNamesCallerCodes(t *testing.T) {
	c, _ := newComparator(t)
	for _, tc := range []struct{ a, b, want string }{
		{"[in_i]", "g", "[in_i] vs g"},
		{"B", "m", "B vs m"},
		{"m", "[pH]", "m vs [pH]"},
	} {
		_, err := c.Compare(fhir.Quantity{Value: 1, Code: tc.a}, fhir.Quantity{Value: 1, Code: tc.b})
		if err == nil || !strings.Contains(err.Error(), "not comparable: "+tc.want) {
			t.Errorf("Compare(1 %s, 1 %s) = %v, want \"not comparable: %s\"", tc.a, tc.b, err, tc.want)
		}
	}
}
