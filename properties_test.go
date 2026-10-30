package ucum_test

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
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

// Inspect preserves occurrences for consumer rules that cannot use reduced
// dimensions: eq and mol can be mathematically equal but denote different roles.
func TestInspectSymbols(t *testing.T) {
	svc, err := ucum.NewExact()
	if err != nil {
		t.Fatal(err)
	}
	inspector, ok := svc.(ucum.Inspector)
	if !ok {
		t.Fatal("NewExact() does not satisfy Inspector")
	}
	type part struct {
		code, prefix string
		exponent     int
	}
	for _, tt := range []struct {
		code string
		want []part
	}{
		{"mg/dL", []part{{"g", "m", 1}, {"L", "d", -1}}},
		{"mol.eq/eq/L", []part{{"mol", "", 1}, {"eq", "", 1}, {"eq", "", -1}, {"L", "", -1}}},
		{"mol.(eq/eq)/L", []part{{"mol", "", 1}, {"eq", "", 1}, {"eq", "", -1}, {"L", "", -1}}},
		{"g/(mol/L)", []part{{"g", "", 1}, {"mol", "", -1}, {"L", "", 1}}},
		{"/mol/L", []part{{"mol", "", -1}, {"L", "", -1}}},
		{"mol0/L", []part{{"mol", "", 0}, {"L", "", -1}}},
		{"10.mmol-2/{sample}", []part{{"mol", "m", -2}}},
		{"{cells}", nil},
		{"1", nil},
	} {
		t.Run(tt.code, func(t *testing.T) {
			symbols, err := inspector.Inspect(tt.code)
			if err != nil {
				t.Fatal(err)
			}
			got := make([]part, 0, len(symbols))
			for _, s := range symbols {
				got = append(got, part{s.Code, s.Prefix, s.Exponent})
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("Inspect(%q) = %+v, want %+v", tt.code, got, tt.want)
			}
		})
	}
}

func TestInspectDefinitions(t *testing.T) {
	svc, err := ucum.New()
	if err != nil {
		t.Fatal(err)
	}
	inspector, ok := svc.(ucum.Inspector)
	if !ok {
		t.Fatal("New() does not satisfy Inspector")
	}
	for _, tt := range []struct {
		code, definition         string
		base, special, arbitrary bool
	}{
		{"g", "", true, false, false},
		{"mol", "10*23", false, false, false},
		{"eq", "mol", false, false, false},
		{"osm", "mol", false, false, false},
		{"L", "l", false, false, false},
		{"l", "dm3", false, false, false},
		{"Cel", "", false, true, false},
		{"[IU]", "[iU]", false, false, true},
	} {
		t.Run(tt.code, func(t *testing.T) {
			symbols, err := inspector.Inspect(tt.code)
			if err != nil || len(symbols) != 1 {
				t.Fatalf("Inspect(%q) = %v, %v", tt.code, symbols, err)
			}
			s := symbols[0]
			if s.Definition != tt.definition || s.IsBase != tt.base || s.IsSpecial != tt.special || s.IsArbitrary != tt.arbitrary {
				t.Fatalf("Inspect(%q) = %+v, want definition=%q base=%t special=%t arbitrary=%t", tt.code, s, tt.definition, tt.base, tt.special, tt.arbitrary)
			}
			symbols[0].Code = "changed"
			symbols[0].Definition = "changed"
			again, err := inspector.Inspect(tt.code)
			if err != nil || len(again) != 1 || again[0] != s {
				t.Fatalf("mutating output changed a cached result: %v, %v", again, err)
			}
		})
	}

	const defs = `<root version="fixture">
<base-unit Code="g" CODE="G" dim="M"><name>gram</name><property>mass</property></base-unit>
<unit Code="[vial]" CODE="[VIAL]"><name>fixture vial</name><property>fixture</property><value Unit="g" value="2"/></unit>
</root>`
	custom, err := ucum.NewFromReader(strings.NewReader(defs))
	if err != nil {
		t.Fatal(err)
	}
	ci, ok := custom.(ucum.Inspector)
	if !ok {
		t.Fatal("NewFromReader() does not satisfy Inspector")
	}
	got, err := ci.Inspect("[vial]")
	if err != nil || len(got) != 1 || got[0].Definition != "g" {
		t.Fatalf("custom definition not preserved: %v, %v", got, err)
	}

	insensitive, err := ucum.NewCaseInsensitive()
	if err != nil {
		t.Fatal(err)
	}
	i, ok := insensitive.(ucum.Inspector)
	if !ok {
		t.Fatal("case-insensitive service does not satisfy Inspector")
	}
	got, err = i.Inspect("MG")
	if err != nil || len(got) != 1 || got[0].Code != "g" || got[0].Prefix != "m" {
		t.Fatalf("case-insensitive input not resolved to definition identity: %v, %v", got, err)
	}
}

func TestInspectErrorsAndLimits(t *testing.T) {
	svc, err := ucum.New()
	if err != nil {
		t.Fatal(err)
	}
	inspector, ok := svc.(ucum.Inspector)
	if !ok {
		t.Fatal("service does not satisfy Inspector")
	}
	for _, tt := range []struct {
		code  string
		cause error
	}{
		{"", nil},
		{"mol/", nil},
		{"not_a_unit", nil},
		{"m" + strconv.Itoa(ucum.MaxExponent+1), ucum.ErrExponentTooLarge},
		{strings.Repeat("(", ucum.MaxNestingDepth+1) + "m" + strings.Repeat(")", ucum.MaxNestingDepth+1), ucum.ErrCodeTooComplex},
		{strings.Repeat("m", ucum.MaxCodeLength+1), ucum.ErrCodeTooLong},
	} {
		symbols, err := inspector.Inspect(tt.code)
		var ve *ucum.ValidationError
		if symbols != nil || !errors.As(err, &ve) || (tt.cause != nil && !errors.Is(err, tt.cause)) {
			t.Errorf("Inspect invalid/over-limit input: got %v, %v; want nil and ValidationError wrapping %v", symbols, err, tt.cause)
		}
	}
	code := strings.Repeat("m.", (ucum.MaxCodeLength-1)/2) + "m"
	symbols, err := inspector.Inspect(code)
	if err != nil || len(symbols) != (len(code)+1)/2 {
		t.Fatalf("long valid input: %d symbols, %v", len(symbols), err)
	}
}

func TestInspectConcurrentOwnership(t *testing.T) {
	t.Parallel()
	svc, err := ucum.New()
	if err != nil {
		t.Fatal(err)
	}
	inspector, ok := svc.(ucum.Inspector)
	if !ok {
		t.Fatal("service does not satisfy Inspector")
	}
	const code = "mol.(eq/eq)/L"
	want, err := inspector.Inspect(code)
	if err != nil {
		t.Fatal(err)
	}
	for n := range 8 {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			t.Parallel()
			for range 32 {
				got, err := inspector.Inspect(code)
				if err != nil || !slices.Equal(got, want) {
					t.Fatalf("concurrent inspection: %v, %v; want %v", got, err, want)
				}
				got[0].Definition = "changed"
				got[0].Code = "changed"
			}
		})
	}
}

func ExampleInspector() {
	svc, err := ucum.New()
	if err != nil {
		panic(err)
	}
	inspector, ok := svc.(ucum.Inspector)
	if !ok {
		return
	}
	symbols, err := inspector.Inspect("mol.(eq/eq)/L")
	if err != nil {
		panic(err)
	}
	for _, symbol := range symbols {
		fmt.Printf("%s %d\n", symbol.Code, symbol.Exponent)
	}
	// Output:
	// mol 1
	// eq 1
	// eq -1
	// L -1
}
