package ucum

import (
	"strings"
	"testing"
)

// TestDefinitionsAreSelfConsistent is the equivalent of validateUCUM() in the
// Java reference: it checks that the shipped definitions can actually be used,
// rather than assuming so.
//
// It matters because the documented way to take up a new UCUM release is to
// replace ucum-essence.xml wholesale — its license forbids editing it — and this
// is what would catch a unit that stopped resolving after such a replacement.
func TestDefinitionsAreSelfConsistent(t *testing.T) {
	svc := newTestService(t)
	s, ok := svc.(*service)
	if !ok {
		t.Fatal("unexpected service type")
	}

	if len(s.model.BaseUnits) != 7 {
		t.Errorf("the model has %d base units, want 7", len(s.model.BaseUnits))
	}
	if len(s.model.Prefixes) == 0 || len(s.model.DefinedUnits) == 0 {
		t.Fatalf("the model looks empty: %d prefixes, %d defined units",
			len(s.model.Prefixes), len(s.model.DefinedUnits))
	}

	// Every base unit resolves to itself.
	for _, bu := range s.model.BaseUnits {
		if err := svc.Validate(bu.Code); err != nil {
			t.Errorf("base unit %q does not validate: %v", bu.Code, err)
			continue
		}
		p, err := svc.Canonical(1, bu.Code)
		if err != nil {
			t.Errorf("base unit %q does not canonicalize: %v", bu.Code, err)
			continue
		}
		if p.Code != bu.Code {
			t.Errorf("base unit %q canonicalizes to %q, want itself", bu.Code, p.Code)
		}
	}

	// Every defined unit validates, canonicalizes, and reports a property.
	for _, du := range s.model.DefinedUnits {
		if err := svc.Validate(du.Code); err != nil {
			t.Errorf("defined unit %q does not validate: %v", du.Code, err)
			continue
		}
		if _, err := svc.Canonical(1, du.Code); err != nil {
			t.Errorf("defined unit %q does not canonicalize: %v", du.Code, err)
		}
		if du.Property == "" {
			t.Errorf("defined unit %q declares no property", du.Code)
		}
		if du.Name == "" {
			t.Errorf("defined unit %q declares no name", du.Code)
		}
	}

	// Every prefix applies to a metric unit and scales it.
	for _, p := range s.model.Prefixes {
		code := p.Code + "m"
		if err := svc.Validate(code); err != nil {
			t.Errorf("prefix %q does not apply to a metric unit (%q): %v", p.Code, code, err)
			continue
		}
		factor, err := svc.Convert(1, code, "m")
		if err != nil {
			t.Errorf("Convert(1, %q, \"m\"): %v", code, err)
			continue
		}
		want, _ := p.Value.rat().Float64()
		if factor != want {
			t.Errorf("prefix %q scales by %v, want %v", p.Code, factor, want)
		}
	}

	// Every unit is reachable by code, and every code is unique.
	seen := make(map[string]bool)
	for _, du := range s.model.DefinedUnits {
		if seen[du.Code] {
			t.Errorf("duplicate unit code %q in the definitions", du.Code)
		}
		seen[du.Code] = true
		if s.model.getUnit(du.Code) == nil {
			t.Errorf("unit %q is not reachable through getUnit", du.Code)
		}
	}
	for _, p := range s.model.Prefixes {
		if s.model.getPrefix(p.Code) == nil {
			t.Errorf("prefix %q is not reachable through getPrefix", p.Code)
		}
	}
}

// TestDefinitionsIdentification covers the Identified interface, and pins the
// release this copy of ucum-essence.xml declares. A version bump is then a
// deliberate change to this test rather than a silent one.
func TestDefinitionsIdentification(t *testing.T) {
	svc := newTestService(t)
	id, ok := svc.(Identified)
	if !ok {
		t.Fatal("the service does not satisfy Identified")
	}
	defs := id.Definitions()
	if defs.Version != "2.2" {
		t.Errorf("Definitions().Version = %q, want %q", defs.Version, "2.2")
	}
	if defs.RevisionDate != "2024-06-17" {
		t.Errorf("Definitions().RevisionDate = %q, want %q", defs.RevisionDate, "2024-06-17")
	}
	// The published definitions have carried "N/A" here since UCUM 2.1.
	if defs.Revision == "" {
		t.Error("Definitions().Revision is empty; the attribute was not parsed")
	}
}

// TestNewFromReaderLoadsCustomDefinitions covers the constructor that had no
// test, including that its service reports the loaded release rather than the
// embedded one.
func TestNewFromReaderLoadsCustomDefinitions(t *testing.T) {
	const defs = `<?xml version="1.0" encoding="ascii"?>
<root xmlns="http://unitsofmeasure.org/ucum-essence" version="9.9" revision="test"
      revision-date="2099-01-01">
   <prefix Code="k" CODE="K"><name>kilo</name><printSymbol>k</printSymbol>
      <value value="1e3">1000</value></prefix>
   <base-unit Code="m" CODE="M" dim="L"><name>meter</name>
      <printSymbol>m</printSymbol><property>length</property></base-unit>
   <unit Code="km2" CODE="KM2" isMetric="yes" class="test">
      <name>square kilometer</name><printSymbol>km2</printSymbol>
      <property>area</property>
      <value Unit="km2" UNIT="KM2" value="1">1</value></unit>
</root>`

	svc, err := NewFromReader(strings.NewReader(defs))
	if err != nil {
		t.Fatalf("NewFromReader: %v", err)
	}

	if err := svc.Validate("km"); err != nil {
		t.Errorf(`Validate("km") with custom definitions: %v`, err)
	}
	if err := svc.Validate("mol"); err == nil {
		t.Error(`Validate("mol") should fail: the custom definitions do not declare it`)
	}
	got, err := svc.Convert(1, "km", "m")
	if err != nil {
		t.Fatalf(`Convert(1, "km", "m"): %v`, err)
	}
	if got != 1000 {
		t.Errorf(`Convert(1, "km", "m") = %v, want 1000`, got)
	}

	// The identification comes from the loaded definitions, not the embedded ones.
	id, ok := svc.(Identified)
	if !ok {
		t.Fatal("the service does not satisfy Identified")
	}
	if defs := id.Definitions(); defs.Version != "9.9" || defs.RevisionDate != "2099-01-01" {
		t.Errorf("Definitions() = %+v, want version 9.9 dated 2099-01-01", defs)
	}

	// A service from custom definitions still satisfies ExactService.
	if _, ok := svc.(ExactService); !ok {
		t.Error("a service from NewFromReader does not satisfy ExactService")
	}
}

func TestNewFromReaderRejectsBadInput(t *testing.T) {
	if _, err := NewFromReader(strings.NewReader("not xml at all")); err == nil {
		t.Error("NewFromReader with junk = nil error, want an error")
	}
	if _, err := NewFromReader(strings.NewReader("")); err == nil {
		t.Error("NewFromReader with empty input = nil error, want an error")
	}
}

// TestErrorMessages covers the Error methods, which had no direct test even
// though their text is what a caller sees.
func TestErrorMessages(t *testing.T) {
	ve := &ValidationError{Code: "m/", Message: "unexpected end", Offset: 2}
	if got := ve.Error(); !strings.Contains(got, "m/") || !strings.Contains(got, "position 2") {
		t.Errorf("ValidationError.Error() = %q, want it to name the code and the offset", got)
	}
	veNoOffset := &ValidationError{Code: "zz", Message: "unknown unit", Offset: -1}
	if got := veNoOffset.Error(); strings.Contains(got, "position") {
		t.Errorf("ValidationError.Error() with no offset = %q, should not mention a position", got)
	}
	ce := &ConversionError{From: "m", To: "s", Message: "not comparable"}
	got := ce.Error()
	for _, want := range []string{"m", "s", "not comparable"} {
		if !strings.Contains(got, want) {
			t.Errorf("ConversionError.Error() = %q, want it to contain %q", got, want)
		}
	}
}
