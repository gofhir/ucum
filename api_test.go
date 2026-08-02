package ucum_test

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/gofhir/ucum/v4"
)

// The root package is a thin wrapper: interfaces, constructors and the aliases
// that make the internal types public. What is worth testing here is that the
// wrapping holds — that the value New returns satisfies every interface the
// documentation claims, and that errors built three packages down still match
// through the aliases.

func TestNewSatisfiesEveryDocumentedInterface(t *testing.T) {
	svc, err := ucum.New()
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := svc.(ucum.ExactService); !ok {
		t.Error("New() does not satisfy ExactService, which the docs promise")
	}
	if _, ok := svc.(ucum.Identified); !ok {
		t.Error("New() does not satisfy Identified, which the docs promise")
	}

	// The same has to hold for every constructor, including the ones that read
	// custom definitions, since callers type-assert on what they hold.
	ci, err := ucum.NewCaseInsensitive()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ci.(ucum.ExactService); !ok {
		t.Error("NewCaseInsensitive() does not satisfy ExactService")
	}

	ex, err := ucum.NewExact()
	if err != nil {
		t.Fatal(err)
	}
	if defs := ex.(ucum.Identified).Definitions(); defs.Version == "" {
		t.Error("NewExact() reports no definitions version")
	}
}

// TestAliasedErrorsMatchThroughThePublicNames checks the re-export. The error
// comes from internal/ucumerr; a caller only ever names ucum.ValidationError, and
// an alias is what makes those the same type rather than two.
func TestAliasedErrorsMatchThroughThePublicNames(t *testing.T) {
	svc, err := ucum.New()
	if err != nil {
		t.Fatal(err)
	}

	var ve *ucum.ValidationError
	if !errors.As(svc.Validate("nope"), &ve) {
		t.Fatal("errors.As(**ucum.ValidationError) = false")
	}
	if ve.Code != "nope" || ve.Offset != 0 {
		t.Errorf("ValidationError = %+v, want Code \"nope\" at offset 0", ve)
	}

	var ce *ucum.ConversionError
	if _, err := svc.Convert(1, "m", "s"); !errors.As(err, &ce) {
		t.Error("errors.As(**ucum.ConversionError) = false")
	}

	if _, err := svc.Canonical(1, "m/0"); !errors.Is(err, ucum.ErrDivisionByZero) {
		t.Error("errors.Is(err, ucum.ErrDivisionByZero) = false")
	}
	if err := svc.Validate("m2000000000"); !errors.Is(err, ucum.ErrExponentTooLarge) {
		t.Error("errors.Is(err, ucum.ErrExponentTooLarge) = false")
	}

	ex, err := ucum.NewExact()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ex.ConversionFactor("Cel", "K"); !errors.Is(err, ucum.ErrNotLinear) {
		t.Error("errors.Is(err, ucum.ErrNotLinear) = false")
	}
	if _, err := ex.ConvertRat(big.NewRat(7, 1), "[pH]", "mol/l"); !errors.Is(err, ucum.ErrNotRational) {
		t.Error("errors.Is(err, ucum.ErrNotRational) = false")
	}
	if _, err := ex.ConvertRat(nil, "m", "m"); !errors.Is(err, ucum.ErrNilValue) {
		t.Error("errors.Is(err, ucum.ErrNilValue) = false")
	}
}

func TestNewFromReaderUsesTheGivenDefinitions(t *testing.T) {
	const custom = `<?xml version="1.0" encoding="ascii"?>
<root xmlns="http://unitsofmeasure.org/ucum-essence" version="9.9" revision="$Revision: 1$"
      revision-date="2099-01-01">
 <prefix Code="k" CODE="K"><name>kilo</name><printSymbol>k</printSymbol><value value="1e3">10<sup>3</sup></value></prefix>
 <base-unit Code="m" CODE="M" dim="L"><name>meter</name><printSymbol>m</printSymbol><property>length</property></base-unit>
</root>`

	svc, err := ucum.NewFromReader(strings.NewReader(custom))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := svc.Convert(1, "km", "m"); err != nil || got != 1000 {
		t.Errorf(`Convert(1, "km", "m") = %v, %v; want 1000, nil`, got, err)
	}
	if err := svc.Validate("mol"); err == nil {
		t.Error(`Validate("mol") = nil; the custom definitions do not declare it`)
	}
	if defs := svc.(ucum.Identified).Definitions(); defs.Version != "9.9" {
		t.Errorf("Definitions().Version = %q, want %q", defs.Version, "9.9")
	}
}

// Examples, which double as the documentation on pkg.go.dev.

func ExampleNew() {
	svc, err := ucum.New()
	if err != nil {
		panic(err)
	}

	fmt.Println(svc.Validate("mg/dL"))

	v, _ := svc.Convert(1, "mg/dL", "g/L")
	fmt.Println(v)

	can, _ := svc.Canonical(1, "mg/dL")
	fmt.Println(can.Value, can.Code)

	ok, _ := svc.IsComparable("mg/dL", "g/L")
	fmt.Println(ok)

	// Output:
	// <nil>
	// 0.01
	// 10 g.m-3
	// true
}

func ExampleService_Analyze() {
	svc, _ := ucum.New()

	fmt.Println(svc.Analyze("kg.m/s2"))
	fmt.Println(svc.Analyze("mL/(kg.min)"))

	// Output:
	// (kilogram) * (meter) / (second ^ 2) <nil>
	// (milliliter) / [(kilogram) * (minute)] <nil>
}

// Temperature is the case most likely to be wrong elsewhere: the scales are
// affine, so a conversion is not a multiplication.
func ExampleService_Convert_temperature() {
	svc, _ := ucum.New()

	f, _ := svc.Convert(37, "Cel", "[degF]")
	fmt.Printf("%.1f\n", f)

	// 100 °F is colder than 50 °C, and the canonical forms say so.
	hot, _ := svc.Canonical(50, "Cel")
	warm, _ := svc.Canonical(100, "[degF]")
	fmt.Println(warm.Value < hot.Value)

	// Output:
	// 98.6
	// true
}

// ExampleExactService shows the arithmetic that does not round.
func ExampleNewExact() {
	ex, _ := ucum.NewExact()

	factor, _ := ex.ConversionFactor("L", "mL")
	fmt.Println(factor.RatString())

	v, _ := ex.ConvertRat(big.NewRat(100, 1), "[degF]", "Cel")
	fmt.Println(v)

	// A logarithmic scale has no exact rational form, and says so rather than
	// rounding.
	_, err := ex.ConvertRat(big.NewRat(7, 1), "[pH]", "mol/l")
	fmt.Println(errors.Is(err, ucum.ErrNotRational))

	// Output:
	// 1000
	// 340/9
	// true
}

// Arbitrary units are commensurable with nothing, not even with each other.
func ExampleService_IsComparable() {
	svc, _ := ucum.New()

	fmt.Println(svc.IsComparable("mg/dL", "g/L"))
	fmt.Println(svc.IsComparable("[IU]", "mol"))
	fmt.Println(svc.IsComparable("[IU]", "[iU]"))

	// Output:
	// true <nil>
	// false <nil>
	// false <nil>
}
