package ucum_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/gofhir/ucum/v4"
)

// Resultados fuera de float64 en la API pública (#59, plan
// docs/plans/003-float-range.md).

// TestRangeSentinelsArePublic: los sentinels se pueden comparar con el
// nombre público.
func TestRangeSentinelsArePublic(t *testing.T) {
	svc, err := ucum.New()
	if err != nil {
		t.Fatal(err)
	}
	// §46■3: 1000 B es 10^1000.
	if _, err := svc.Canonical(1000, "B"); !errors.Is(err, ucum.ErrOutOfRange) {
		t.Errorf("Canonical(1000 B) error = %v, want ucum.ErrOutOfRange", err)
	}
	// §46■3: lg 0 no existe.
	if _, err := svc.Convert(0, "1", "B"); !errors.Is(err, ucum.ErrOutOfDomain) {
		t.Errorf("Convert(0, 1, B) error = %v, want ucum.ErrOutOfDomain", err)
	}
}

// TestValidateCanonicalUnitsOutOfRange: la pregunta es de dimensión y no
// depende de que 1 en la unidad quepa en float64 (Ym13 es 10^312 m13).
func TestValidateCanonicalUnitsOutOfRange(t *testing.T) {
	svc, err := ucum.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ code, canonical string }{
		{"Ym13", "m13"},
		{"ym14", "m14"},
	} {
		if err := ucum.ValidateCanonicalUnits(svc, tt.code, tt.canonical); err != nil {
			t.Errorf("ValidateCanonicalUnits(%q, %q) = %v, want nil", tt.code, tt.canonical, err)
		}
	}
	for _, tt := range []struct{ code, canonical string }{
		{"Ym13", "m12"},  // otra dimensión
		{"Ym13", "km13"}, // misma dimensión, pero no es una forma canónica
		{"Ym13", "s"},
		{"Ym13", "Ym13"}, // tampoco cabe, pero el error es que no es canónica
	} {
		var ve *ucum.ValidationError
		err := ucum.ValidateCanonicalUnits(svc, tt.code, tt.canonical)
		if !errors.As(err, &ve) || errors.Is(err, ucum.ErrOutOfRange) || !strings.Contains(err.Error(), "does not reduce") {
			t.Errorf("ValidateCanonicalUnits(%q, %q) = %v, want a *ValidationError saying it does not reduce", tt.code, tt.canonical, err)
		}
	}
}
