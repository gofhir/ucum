package ucum_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/gofhir/ucum/v4"
)

// TestPropertiesListsWhatValidateInPropertyAccepts closes the loop between the
// two: without an enumeration a caller has to guess the exact spelling of a
// property, and a wrong guess is indistinguishable from a unit that does not
// measure it.
func TestPropertiesListsWhatValidateInPropertyAccepts(t *testing.T) {
	svc, err := ucum.New()
	if err != nil {
		t.Fatal(err)
	}
	lister, ok := svc.(ucum.PropertyLister)
	if !ok {
		t.Fatal("New() does not satisfy PropertyLister")
	}

	props := lister.Properties()
	if len(props) < 90 {
		t.Errorf("Properties() returned %d entries, want the ~101 the definitions declare", len(props))
	}
	if !slices.IsSorted(props) {
		t.Error("Properties() is not sorted")
	}
	for i := 1; i < len(props); i++ {
		if props[i] == props[i-1] {
			t.Errorf("Properties() repeats %q", props[i])
			break
		}
	}

	// The ones a caller is most likely to reach for.
	for _, want := range []string{"length", "mass", "force", "pressure", "acceleration", "mass concentration"} {
		if !slices.Contains(props, want) {
			t.Errorf("Properties() does not list %q", want)
		}
	}

	// Every name it returns is one ValidateInProperty accepts: it must never
	// report "unknown property" for a name that came from here.
	for _, p := range props {
		err := svc.ValidateInProperty("m", p)
		if err != nil && strings.Contains(err.Error(), "unknown property") {
			t.Errorf("Properties() returned %q but ValidateInProperty calls it unknown", p)
		}
	}

	// And a name that is not in the list is reported as unknown rather than as
	// a mismatch, so the two failures stay distinguishable.
	err = svc.ValidateInProperty("m", "no such property")
	if err == nil || !strings.Contains(err.Error(), "unknown property") {
		t.Errorf("ValidateInProperty with an unknown property = %v, want an \"unknown property\" error", err)
	}

	// The slice is the caller's: mutating it must not affect the next call.
	if len(props) > 0 {
		props[0] = "clobbered"
		if again := lister.Properties(); again[0] == "clobbered" {
			t.Error("Properties() hands out its internal slice")
		}
	}
}

// TestValidateCanonicalUnits covers the free function, which is Java's
// validateCanonicalUnits: does this code reduce to that canonical form?
func TestValidateCanonicalUnits(t *testing.T) {
	svc, err := ucum.New()
	if err != nil {
		t.Fatal(err)
	}

	ok := []struct{ code, canonical string }{
		{"m", "m"},
		{"km", "m"},
		{"mg/dL", "g.m-3"},
		{"N", "g.m.s-2"},
		{"Cel", "K"},
		{"1", "1"},
		{"%", "1"},
	}
	for _, tt := range ok {
		if err := ucum.ValidateCanonicalUnits(svc, tt.code, tt.canonical); err != nil {
			t.Errorf("ValidateCanonicalUnits(%q, %q) = %v, want nil", tt.code, tt.canonical, err)
		}
	}

	bad := []struct{ code, canonical string }{
		{"m", "s"},
		{"m", "m2"},
		{"mg/dL", "g"},
	}
	for _, tt := range bad {
		err := ucum.ValidateCanonicalUnits(svc, tt.code, tt.canonical)
		var ve *ucum.ValidationError
		if !errors.As(err, &ve) {
			t.Errorf("ValidateCanonicalUnits(%q, %q) = %v, want a *ValidationError", tt.code, tt.canonical, err)
		}
	}

	// An invalid code fails as an invalid code, not as a mismatch.
	err = ucum.ValidateCanonicalUnits(svc, "nope", "m")
	var ve *ucum.ValidationError
	if !errors.As(err, &ve) || !strings.Contains(err.Error(), "unknown unit") {
		t.Errorf("ValidateCanonicalUnits with a bad code = %v, want the parse error", err)
	}

	// A canonical form that is itself not canonical is a caller mistake worth
	// naming: "kg" is a valid code but never a canonical form, since the
	// canonical mass unit is the gram.
	if err := ucum.ValidateCanonicalUnits(svc, "kg", "kg"); err == nil {
		t.Error(`ValidateCanonicalUnits("kg", "kg") = nil, want an error: the canonical form of kg is g`)
	}
}
