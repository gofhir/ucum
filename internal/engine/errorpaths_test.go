package engine

import (
	"errors"
	"math/big"
	"strings"
	"testing"

	"github.com/gofhir/ucum/v4/internal/ucumerr"
)

// The failure paths of the public API had no tests: they were reached only when
// something upstream went wrong, which no test made happen. What follows checks
// that each one propagates an error naming the code at fault, rather than
// checking that a line executes.

const badCode = "definitely-not-a-unit"

// TestErrorPathsNameTheOffendingCode: with two unit arguments, the error has to
// say which one was wrong, or a caller cannot act on it.
func TestErrorPathsNameTheOffendingCode(t *testing.T) {
	svc := newTestService(t)
	one := big.NewRat(1, 1)

	cases := []struct {
		name string
		call func() error
	}{
		{"Convert bad source", func() error { _, err := svc.Convert(1, badCode, "m"); return err }},
		{"Convert bad target", func() error { _, err := svc.Convert(1, "m", badCode); return err }},
		{"ConvertRat bad source", func() error { _, err := svc.ConvertRat(one, badCode, "m"); return err }},
		{"ConvertRat bad target", func() error { _, err := svc.ConvertRat(one, "m", badCode); return err }},
		{"ConversionFactor bad source", func() error { _, err := svc.ConversionFactor(badCode, "m"); return err }},
		{"ConversionFactor bad target", func() error { _, err := svc.ConversionFactor("m", badCode); return err }},
		{"CanonicalRat", func() error { _, err := svc.CanonicalRat(one, badCode); return err }},
		{"Canonical", func() error { _, err := svc.Canonical(1, badCode); return err }},
		{"Analyze", func() error { _, err := svc.Analyze(badCode); return err }},
		{"IsComparable first", func() error { _, err := svc.IsComparable(badCode, "m"); return err }},
		{"IsComparable second", func() error { _, err := svc.IsComparable("m", badCode); return err }},
		{"Multiply first", func() error { _, err := svc.Multiply(Pair{1, badCode}, Pair{1, "m"}); return err }},
		{"Multiply second", func() error { _, err := svc.Multiply(Pair{1, "m"}, Pair{1, badCode}); return err }},
		{"Divide first", func() error { _, err := svc.Divide(Pair{1, badCode}, Pair{1, "m"}); return err }},
		{"Divide second", func() error { _, err := svc.Divide(Pair{1, "m"}, Pair{1, badCode}); return err }},
		{"ValidateInProperty", func() error { return svc.ValidateInProperty(badCode, "length") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if err == nil {
				t.Fatal("no error for an invalid code")
			}
			if !strings.Contains(err.Error(), badCode) {
				t.Errorf("error does not name the offending code: %v", err)
			}
		})
	}
}

// TestErrorPathsOnValidButUncanonicalizableCodes: "m/0" parses, so these paths
// fail during canonicalization rather than parsing — a different branch.
func TestErrorPathsOnValidButUncanonicalizableCodes(t *testing.T) {
	svc := newTestService(t)
	one := big.NewRat(1, 1)

	if err := svc.Validate("m/0"); err != nil {
		t.Fatalf(`Validate("m/0") should succeed: it is well-formed UCUM, got %v`, err)
	}

	cases := map[string]func() error{
		"Canonical":          func() error { _, err := svc.Canonical(1, "m/0"); return err },
		"Convert source":     func() error { _, err := svc.Convert(1, "m/0", "m"); return err },
		"Convert target":     func() error { _, err := svc.Convert(1, "m", "m/0"); return err },
		"ConvertRat source":  func() error { _, err := svc.ConvertRat(one, "m/0", "m"); return err },
		"ConvertRat target":  func() error { _, err := svc.ConvertRat(one, "m", "m/0"); return err },
		"ConversionFactor":   func() error { _, err := svc.ConversionFactor("m/0", "m"); return err },
		"CanonicalRat":       func() error { _, err := svc.CanonicalRat(one, "m/0"); return err },
		"IsComparable":       func() error { _, err := svc.IsComparable("m/0", "m"); return err },
		"Multiply":           func() error { _, err := svc.Multiply(Pair{1, "m/0"}, Pair{1, "m"}); return err },
		"Divide":             func() error { _, err := svc.Divide(Pair{1, "m"}, Pair{1, "m/0"}); return err },
		"ValidateInProperty": func() error { return svc.ValidateInProperty("m/0", "length") },
	}
	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panicked on a zero divisor: %v", r)
				}
			}()
			if err := call(); err == nil {
				t.Error("no error for a code that cannot be canonicalized")
			}
		})
	}
}

// TestConvertRatRejectsIncomparableAndZero covers the two guards that sit between
// parsing and arithmetic.
func TestConvertRatRejectsIncomparableAndZero(t *testing.T) {
	svc := newTestService(t)
	one := big.NewRat(1, 1)

	_, err := svc.ConvertRat(one, "m", "s")
	if err == nil {
		t.Fatal(`ConvertRat(1, "m", "s") = nil error, want an error`)
	}
	var ce *ucumerr.ConversionError
	if !errors.As(err, &ce) {
		t.Errorf("error is %T, want *ucumerr.ConversionError", err)
	}
	if !strings.Contains(err.Error(), "not comparable") {
		t.Errorf("error does not say the units are not comparable: %v", err)
	}

	if _, err := svc.ConvertRat(one, "1", "0"); err == nil {
		t.Error(`ConvertRat(1, "1", "0") = nil error, want an error for a zero divisor`)
	}
	if _, err := svc.ConversionFactor("1", "0"); err == nil {
		t.Error(`ConversionFactor("1", "0") = nil error, want an error for a zero divisor`)
	}
}

// TestValidateInPropertyOnUnknownProperty covers the branch where the code is
// fine and the property is not.
func TestValidateInPropertyOnUnknownProperty(t *testing.T) {
	svc := newTestService(t)
	err := svc.ValidateInProperty("m", "no such property at all")
	if err == nil {
		t.Fatal("no error for an unknown property")
	}
	if !strings.Contains(err.Error(), "unknown property") {
		t.Errorf("error should say the property is unknown, got: %v", err)
	}
	// A compound expression with an unknown property takes a different branch.
	if err := svc.ValidateInProperty("m/s", "no such property at all"); err == nil {
		t.Error("no error for an unknown property on a compound expression")
	}
}

// TestUnsupportedSpecialFunction covers the construction-time guard: definitions
// naming a function this package does not implement must fail loudly rather than
// silently misconvert a unit.
func TestUnsupportedSpecialFunction(t *testing.T) {
	const defs = `<?xml version="1.0" encoding="ascii"?>
<root xmlns="http://unitsofmeasure.org/ucum-essence" version="1.0" revision="test"
      revision-date="2099-01-01">
   <base-unit Code="K" CODE="K" dim="C"><name>kelvin</name>
      <printSymbol>K</printSymbol><property>temperature</property></base-unit>
   <unit Code="Xx" CODE="XX" isMetric="yes" isSpecial="yes" class="test">
      <name>invented scale</name><printSymbol>Xx</printSymbol>
      <property>temperature</property>
      <value Unit="invented(1 K)" UNIT="INVENTED(1 K)">
         <function name="inventedFunction" value="1" Unit="K"/>
      </value></unit>
</root>`

	_, err := New(strings.NewReader(defs), false)
	if err == nil {
		t.Fatal("NewFromReader accepted definitions naming an unimplemented function")
	}
	for _, want := range []string{"Xx", "inventedFunction"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should name %q, got: %v", want, err)
		}
	}
}

// TestSpecialUnitWithoutFunction covers the other construction guard: a unit
// flagged special whose definition carries no function at all.
func TestSpecialUnitWithoutFunction(t *testing.T) {
	const defs = `<?xml version="1.0" encoding="ascii"?>
<root xmlns="http://unitsofmeasure.org/ucum-essence" version="1.0" revision="test"
      revision-date="2099-01-01">
   <base-unit Code="K" CODE="K" dim="C"><name>kelvin</name>
      <printSymbol>K</printSymbol><property>temperature</property></base-unit>
   <unit Code="Yy" CODE="YY" isMetric="yes" isSpecial="yes" class="test">
      <name>special without a function</name><printSymbol>Yy</printSymbol>
      <property>temperature</property>
      <value Unit="K" UNIT="K" value="1">1</value></unit>
</root>`

	_, err := New(strings.NewReader(defs), false)
	if err == nil {
		t.Fatal("NewFromReader accepted a special unit with no function definition")
	}
	if !strings.Contains(err.Error(), "Yy") {
		t.Errorf("error should name the unit, got: %v", err)
	}
}
