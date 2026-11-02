package fhir_test

import (
	"errors"
	"math/big"
	"strings"
	"testing"

	"github.com/gofhir/ucum/v4"
	"github.com/gofhir/ucum/v4/fhir"
	"github.com/gofhir/ucum/v4/internal/essence"
)

// Plan 013: un valor con α·v < 0 en una escala de raíz cuadrada no existe
// (UCUM §47 ■4: la imagen de sqrt es [0, ∞)). Comparator lo dice con
// ErrOutOfDomain por Value y por Exact, sin tapar un error de código o de
// unidades.

// sqrtCode es la escala de raíz cuadrada del XML, buscada por su función.
func sqrtCode(t testing.TB) string {
	t.Helper()
	m, err := essence.Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, du := range m.DefinedUnits {
		if du.IsSpecial && du.Value.Function != nil && du.Value.Function.Name == "sqrt" {
			return du.Code
		}
	}
	t.Fatal("no sqrt scale in the XML")
	return ""
}

func TestSqrtNegativeComparator(t *testing.T) {
	c, _ := newComparator(t)
	s := sqrtCode(t)
	for _, q := range []fhir.Quantity{
		{Value: -2, Code: s},
		{Exact: big.NewRat(-2, 1), Code: s},
		{Value: -1e-310, Code: s},
		{Exact: mustRat(t, "-1e-310"), Code: s},
		{Exact: mustRat(t, "-1e300"), Code: s},
		{Exact: mustRat(t, "-1e-400"), Code: s},
		{Exact: mustRat(t, "-1e400"), Code: s},
	} {
		code, v, err := c.CanonicalKey(q)
		if !errors.Is(err, ucum.ErrOutOfDomain) || errors.Is(err, ucum.ErrOutOfRange) {
			t.Errorf("CanonicalKey(%+v) = (%q, %v, %v), want ErrOutOfDomain", q, code, v, err)
		}
		// 4 m2/s4/Hz es el cuadrado de 2 [m/s2/Hz^(1/2)]: antes eran iguales.
		other := fhir.Quantity{Value: 4, Code: "m2/s4/Hz"}
		for _, pair := range [][2]fhir.Quantity{{q, other}, {other, q}, {q, q}} {
			got, err := c.Compare(pair[0], pair[1])
			if !errors.Is(err, ucum.ErrOutOfDomain) || errors.Is(err, ucum.ErrOutOfRange) {
				t.Errorf("Compare(%+v, %+v) = (%d, %v), want ErrOutOfDomain", pair[0], pair[1], got, err)
			}
		}
	}
}

// Un error de dominio nombra el Exact de quien llama, no el float64 en que se
// leyó (-1e-400 se lee como -5e-324).
func TestSqrtDomainErrorNamesExact(t *testing.T) {
	c, _ := newComparator(t)
	s := sqrtCode(t)
	_, _, err := c.CanonicalKey(fhir.Quantity{Exact: mustRat(t, "-1e-400"), Code: s})
	if err == nil || !strings.Contains(err.Error(), "-1e-400") || !strings.Contains(err.Error(), s) ||
		!strings.Contains(err.Error(), "outside the domain") {
		t.Errorf("CanonicalKey(-1e-400 %s) = %v, want a domain message naming -1e-400 and the unit", s, err)
	}
	// Como el error de rango: un ValidationError con el código y sin posición.
	var ve *ucum.ValidationError
	if !errors.As(err, &ve) || ve.Code != s || ve.Offset != -1 || !errors.Is(err, ucum.ErrOutOfDomain) {
		t.Errorf("CanonicalKey(-1e-400 %s) = %v, want a ValidationError for %q at no position wrapping ucum.ErrOutOfDomain", s, err, s)
	}
	_, err = c.Compare(fhir.Quantity{Exact: mustRat(t, "-1e-400"), Code: s}, fhir.Quantity{Value: 1, Code: "m2/s4/Hz"})
	if err == nil || !strings.Contains(err.Error(), "-1e-400") {
		t.Errorf("Compare(-1e-400 %s, ...) = %v, want a message naming -1e-400", s, err)
	}
	// Con los dos lados fuera del dominio, el error es el del primero, como
	// con dos errores de rango.
	_, err = c.Compare(fhir.Quantity{Value: -2, Code: s}, fhir.Quantity{Value: -3, Code: s})
	if !errors.Is(err, ucum.ErrOutOfDomain) || !strings.Contains(err.Error(), "-2 ") || strings.Contains(err.Error(), "-3 ") {
		t.Errorf("Compare(-2 %s, -3 %s) = %v, want the domain error of -2", s, s, err)
	}
}

// El orden de errores de Compare no cambia: código, unidades, valor. Un error
// de dominio no tapa «not comparable» ni un código inválido, en ningún orden.
func TestSqrtDomainDoesNotHideEarlierErrors(t *testing.T) {
	c, _ := newComparator(t)
	neg := fhir.Quantity{Value: -2, Code: sqrtCode(t)}

	kg := fhir.Quantity{Value: 1, Code: "kg"}
	for _, pair := range [][2]fhir.Quantity{{neg, kg}, {kg, neg}} {
		_, err := c.Compare(pair[0], pair[1])
		if err == nil || errors.Is(err, ucum.ErrOutOfDomain) || !strings.Contains(err.Error(), "not comparable") {
			t.Errorf("Compare(%s, %s) = %v, want \"not comparable\"", pair[0].Code, pair[1].Code, err)
		}
	}

	bad := fhir.Quantity{Value: 1, Code: "[foo]"}
	for _, pair := range [][2]fhir.Quantity{{neg, bad}, {bad, neg}} {
		_, err := c.Compare(pair[0], pair[1])
		if err == nil || errors.Is(err, ucum.ErrOutOfDomain) {
			t.Errorf("Compare(%s, %s) = %v, want the code error, not ErrOutOfDomain", pair[0].Code, pair[1].Code, err)
		}
	}
}

// Los valores que sí existen siguen comparando como antes.
func TestSqrtNonNegativeComparatorUnchanged(t *testing.T) {
	c, _ := newComparator(t)
	s := sqrtCode(t)
	for _, tc := range []struct {
		a, b fhir.Quantity
		want int
	}{
		{fhir.Quantity{Value: 2, Code: s}, fhir.Quantity{Value: 4, Code: "m2/s4/Hz"}, 0},
		{fhir.Quantity{Value: 2, Code: s}, fhir.Quantity{Value: 5, Code: "m2/s4/Hz"}, -1},
		{fhir.Quantity{Value: 0, Code: s}, fhir.Quantity{Value: 0, Code: "m2/s4/Hz"}, 0},
	} {
		got, err := c.Compare(tc.a, tc.b)
		if err != nil || got != tc.want {
			t.Errorf("Compare(%+v, %+v) = (%d, %v), want %d", tc.a, tc.b, got, err, tc.want)
		}
	}
}
