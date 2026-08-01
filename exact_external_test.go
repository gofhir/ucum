package ucum_test

// This file compiles as an external package on purpose: it exercises the exact
// API exactly as a consumer would, so anything it cannot reach is not really
// public.

import (
	"errors"
	"math/big"
	"testing"

	"github.com/gofhir/ucum/v3"
)

func TestExternalExactConversion(t *testing.T) {
	svc, err := ucum.NewExact()
	if err != nil {
		t.Fatal(err)
	}

	// A cached, composable factor.
	factor, err := svc.ConversionFactor("L", "mL")
	if err != nil {
		t.Fatal(err)
	}
	if factor.RatString() != "1000" {
		t.Errorf("ConversionFactor(L, mL) = %s, want 1000", factor.RatString())
	}

	// A lab value carried without rounding: 5.4 mmol/L into umol/L.
	value, ok := new(big.Rat).SetString("5.4")
	if !ok {
		t.Fatal("bad literal")
	}
	got, err := svc.ConvertRat(value, "mmol/L", "umol/L")
	if err != nil {
		t.Fatal(err)
	}
	if got.RatString() != "5400" {
		t.Errorf("ConvertRat(5.4, mmol/L, umol/L) = %s, want 5400", got.RatString())
	}

	// Comparing temperatures across scales, exactly.
	f100, _ := new(big.Rat).SetString("100")
	c50, _ := new(big.Rat).SetString("50")
	inCel, err := svc.ConvertRat(f100, "[degF]", "Cel")
	if err != nil {
		t.Fatal(err)
	}
	if inCel.Cmp(c50) >= 0 {
		t.Errorf("100 [degF] = %s Cel, which should be below 50", inCel.RatString())
	}
}

func TestExternalExactErrorsAreDistinguishable(t *testing.T) {
	svc, err := ucum.NewExact()
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.ConversionFactor("Cel", "K"); !errors.Is(err, ucum.ErrNotLinear) {
		t.Errorf("ConversionFactor(Cel, K) error = %v, want ucum.ErrNotLinear", err)
	}
	if _, err := svc.ConvertRat(big.NewRat(1, 1), "[pH]", "mol/L"); !errors.Is(err, ucum.ErrNotRational) {
		t.Errorf("ConvertRat(1, [pH], mol/L) error = %v, want ucum.ErrNotRational", err)
	}
	// The non-rational scales remain available through the float64 API.
	if _, err := svc.Convert(7, "[pH]", "mol/L"); err != nil {
		t.Errorf("Convert(7, [pH], mol/L) = %v, want it to succeed", err)
	}
}

// TestExternalServiceAssertsToExactService documents the migration path for
// code that already holds a Service from New().
func TestExternalServiceAssertsToExactService(t *testing.T) {
	svc, err := ucum.New()
	if err != nil {
		t.Fatal(err)
	}
	exact, ok := svc.(ucum.ExactService)
	if !ok {
		t.Fatal("Service from New() does not satisfy ExactService")
	}
	p, err := exact.CanonicalRat(big.NewRat(1, 1), "Cel")
	if err != nil {
		t.Fatal(err)
	}
	if p.Value.RatString() != "5483/20" || p.Code != "K" {
		t.Errorf("CanonicalRat(1, Cel) = %s %s, want 5483/20 K", p.Value.RatString(), p.Code)
	}
}
