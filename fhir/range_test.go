package fhir_test

import (
	"errors"
	"math/big"
	"testing"

	"github.com/gofhir/ucum/v4"
	"github.com/gofhir/ucum/v4/fhir"
)

// Resultados fuera de float64 (#59, plan docs/plans/003-float-range.md).

// TestCanonicalKeyOutOfRange: la misma cantidad da el mismo error por Value
// y por Exact. 1e-300 ym es 10^-324 m, que redondea a cero; 1000 B es
// 10^1000 (§46■3).
func TestCanonicalKeyOutOfRange(t *testing.T) {
	c, _ := newComparator(t)
	for _, q := range []fhir.Quantity{
		{Value: 1e-300, Code: "ym"},
		{Exact: mustRat(t, "1e-300"), Code: "ym"},
		{Exact: mustRat(t, "-1e-400"), Code: "m"},
		{Exact: mustRat(t, "1e400"), Code: "m"},
		{Value: 1000, Code: "B"},
		{Exact: big.NewRat(1000, 1), Code: "B"},
	} {
		code, v, err := c.CanonicalKey(q)
		if !errors.Is(err, ucum.ErrOutOfRange) {
			t.Errorf("CanonicalKey(%+v) = (%q, %v, %v), want ucum.ErrOutOfRange", q, code, v, err)
		}
	}
}

// TestCompareOutOfRange: dos cantidades fuera de float64 no son iguales
// porque las dos den +Inf.
func TestCompareOutOfRange(t *testing.T) {
	c, _ := newComparator(t)
	_, err := c.Compare(fhir.Quantity{Value: 1000, Code: "B"}, fhir.Quantity{Value: 400, Code: "B"})
	if !errors.Is(err, ucum.ErrOutOfRange) {
		t.Errorf("Compare(1000 B, 400 B) error = %v, want ucum.ErrOutOfRange", err)
	}
}
