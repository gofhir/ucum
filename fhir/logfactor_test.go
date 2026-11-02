package fhir_test

import (
	"math/big"
	"testing"

	"github.com/gofhir/ucum/v4/fhir"
)

// Plan 011 (#75, opción B): un exponente entero en una escala logarítmica se
// redondea una vez con el factor de la unidad, así que 1 [pH] es igual a
// Exact 1/10 mol/L, el valor que define UCUM §22 (pH = -lg(mol/l)); y menor que
// un Value de 0.1, cuyo binario exacto es mayor que 1/10. Los esperados salen de
// big.Rat, no de la librería.
func TestLogFactorCompareAndKey(t *testing.T) {
	c, _ := newComparator(t)
	for _, tc := range []struct {
		pH      float64
		decimal string // la concentración 10^-pH mol/l como decimal
		value   float64
	}{
		{1, "0.1", 0.1},
		{2, "0.01", 0.01},
	} {
		exact := fhir.Quantity{Exact: mustRat(t, tc.decimal), Code: "mol/L"}
		byValue := fhir.Quantity{Value: tc.value, Code: "mol/L"}
		ph := fhir.Quantity{Value: tc.pH, Code: "[pH]"}

		if got, err := c.Compare(ph, exact); err != nil || got != 0 {
			t.Errorf("Compare(%v [pH], Exact %s mol/L) = (%d, %v), want 0", tc.pH, tc.decimal, got, err)
		}
		// El binario de 0.1 y de 0.01 está por encima del decimal; el esperado
		// sale de comparar los racionales.
		want := -new(big.Rat).SetFloat64(tc.value).Cmp(mustRat(t, tc.decimal))
		if got, err := c.Compare(ph, byValue); err != nil || got != want {
			t.Errorf("Compare(%v [pH], Value %v mol/L) = (%d, %v), want %d", tc.pH, tc.value, got, err, want)
		}
		if tc.pH == 1 && want != -1 {
			t.Errorf("the binary of 0.1 is not above 1/10: the plan's -1 does not apply")
		}

		kp, vp, err := c.CanonicalKey(ph)
		if err != nil {
			t.Fatal(err)
		}
		ke, ve, err := c.CanonicalKey(exact)
		if err != nil {
			t.Fatal(err)
		}
		if kp != ke || vp != ve {
			t.Errorf("CanonicalKey(%v [pH]) = (%q, %v), want that of Exact %s mol/L, (%q, %v)", tc.pH, kp, vp, tc.decimal, ke, ve)
		}
	}
}
