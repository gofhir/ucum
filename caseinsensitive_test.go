package ucum

import "testing"

// UCUM defines a case-insensitive variant of every terminal symbol, "to be used
// when there is a risk of upper and lower case to be confused", and states that
// "case insensitive symbols are incompatible to the case sensitive symbols".
// They are therefore two parallel vocabularies, selected by the constructor, and
// never mixed.
func TestCaseInsensitiveVocabulary(t *testing.T) {
	ci, err := NewCaseInsensitive()
	if err != nil {
		t.Fatal(err)
	}

	// The case-insensitive spelling of everyday codes.
	for _, code := range []string{"MG/DL", "MOL/L", "CEL", "MM", "KM", "10*3/UL", "[DEGF]", "MMOL/L"} {
		if err := ci.Validate(code); err != nil {
			t.Errorf("case-insensitive Validate(%q) = %v, want nil", code, err)
		}
	}

	// Case does not matter within the variant: the same codes in any mixture.
	for _, code := range []string{"mg/dl", "Mg/Dl", "mOl/l", "cel", "10*3/ul"} {
		if err := ci.Validate(code); err != nil {
			t.Errorf("case-insensitive Validate(%q) = %v, want nil", code, err)
		}
	}

	// Conversions work on the case-insensitive vocabulary.
	tests := []struct {
		value    float64
		from, to string
		want     float64
	}{
		{1, "MOL/L", "MMOL/L", 1000},
		{1, "L", "ML", 1000},
		{1, "MG/DL", "G/L", 0.01},
		{0, "CEL", "K", 273.15},
		{32, "[DEGF]", "CEL", 0},
	}
	for _, tt := range tests {
		got, err := ci.Convert(tt.value, tt.from, tt.to)
		if err != nil {
			t.Errorf("case-insensitive Convert(%v, %q, %q): %v", tt.value, tt.from, tt.to, err)
			continue
		}
		if diff := got - tt.want; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("case-insensitive Convert(%v, %q, %q) = %v, want %v",
				tt.value, tt.from, tt.to, got, tt.want)
		}
	}
}

// TestVocabulariesAreIncompatible is the point of keeping them apart: the same
// string means different things, so a service must resolve in one vocabulary only.
func TestVocabulariesAreIncompatible(t *testing.T) {
	cs := newTestService(t)
	ci, err := NewCaseInsensitive()
	if err != nil {
		t.Fatal(err)
	}

	// "G" is Gauss case-sensitively and gram case-insensitively.
	csG, err := cs.Canonical(1, "G")
	if err != nil {
		t.Fatal(err)
	}
	ciG, err := ci.Canonical(1, "G")
	if err != nil {
		t.Fatal(err)
	}
	if csG.Code == ciG.Code {
		t.Errorf(`"G" canonicalizes to %q in both vocabularies; it should be Gauss case-sensitively and gram case-insensitively`, csG.Code)
	}
	if ciG.Code != "g" {
		t.Errorf(`case-insensitive "G" canonicalizes to %q, want the gram`, ciG.Code)
	}

	// A case-sensitive code that is not a case-insensitive one, and the reverse.
	if err := cs.Validate("MOL"); err == nil {
		t.Error(`case-sensitive Validate("MOL") = nil, want an error: MOL is the case-insensitive spelling`)
	}
	if err := ci.Validate("GS"); err != nil {
		t.Errorf(`case-insensitive Validate("GS") = %v, want nil: GS is Gauss in that vocabulary`, err)
	}
}

// TestCaseInsensitiveKeepsTheMetricRule: §11 applies in both vocabularies, which
// is what keeps the Table 25 conflicts from becoming ambiguities.
func TestCaseInsensitiveKeepsTheMetricRule(t *testing.T) {
	ci, err := NewCaseInsensitive()
	if err != nil {
		t.Fatal(err)
	}
	// [FT_I] is the case-insensitive foot, and is not metric.
	if err := ci.Validate("K[FT_I]"); err == nil {
		t.Error(`case-insensitive Validate("K[FT_I]") = nil, want an error: the atom is not metric`)
	}
	// Table 25: CD is the candela, not centi-day.
	p, err := ci.Canonical(1, "CD")
	if err != nil {
		t.Fatal(err)
	}
	if p.Code != "cd" {
		t.Errorf(`case-insensitive Canonical(1, "CD") = %q, want the candela`, p.Code)
	}
}

// TestCaseInsensitiveVocabularyIsComplete is the counterpart of the
// case-sensitive self-check: every atom's case-insensitive spelling has to
// resolve in that vocabulary, or the variant is only partly usable.
func TestCaseInsensitiveVocabularyIsComplete(t *testing.T) {
	ci, err := NewCaseInsensitive()
	if err != nil {
		t.Fatal(err)
	}
	s, ok := ci.(*service)
	if !ok {
		t.Fatal("unexpected service type")
	}

	var badValidate, badCanonical []string
	for _, du := range s.model.DefinedUnits {
		if du.CodeCI == "" {
			t.Errorf("defined unit %q declares no case-insensitive code", du.Code)
			continue
		}
		if err := ci.Validate(du.CodeCI); err != nil {
			badValidate = append(badValidate, du.CodeCI)
			continue
		}
		if _, err := ci.Canonical(1, du.CodeCI); err != nil {
			badCanonical = append(badCanonical, du.CodeCI)
		}
	}
	if len(badValidate) > 0 {
		t.Errorf("%d case-insensitive codes do not validate: %v", len(badValidate), badValidate)
	}
	if len(badCanonical) > 0 {
		t.Errorf("%d case-insensitive codes do not canonicalize: %v", len(badCanonical), badCanonical)
	}

	for _, bu := range s.model.BaseUnits {
		if err := ci.Validate(bu.CodeCI); err != nil {
			t.Errorf("base unit %q (case-insensitive %q) does not validate: %v", bu.Code, bu.CodeCI, err)
		}
	}
	// Every prefix applies to a metric atom and scales it, in this vocabulary too.
	for _, p := range s.model.Prefixes {
		code := p.CodeCI + "M" // the meter is "M" case-insensitively
		if err := ci.Validate(code); err != nil {
			t.Errorf("prefix %q does not apply in the case-insensitive vocabulary (%q): %v",
				p.CodeCI, code, err)
			continue
		}
		got, err := ci.Convert(1, code, "M")
		if err != nil {
			t.Errorf("Convert(1, %q, \"M\"): %v", code, err)
			continue
		}
		want, _ := p.Value.rat().Float64()
		if got != want {
			t.Errorf("prefix %q scales by %v, want %v", p.CodeCI, got, want)
		}
	}
}

// TestCaseInsensitiveCollisions documents the two places where the variant cannot
// tell two case-sensitive atoms apart, which the specification implies by calling
// it the greatest common denominator.
func TestCaseInsensitiveCollisions(t *testing.T) {
	ci, err := NewCaseInsensitive()
	if err != nil {
		t.Fatal(err)
	}
	// "l" and "L" are both the liter, so collapsing them loses nothing.
	if err := ci.Validate("L"); err != nil {
		t.Errorf(`case-insensitive Validate("L") = %v, want nil`, err)
	}
	// "[iU]" and "[IU]" are distinct arbitrary units case-sensitively, and share
	// one case-insensitive code, so the variant resolves only one of them.
	if err := ci.Validate("[IU]"); err != nil {
		t.Errorf(`case-insensitive Validate("[IU]") = %v, want nil`, err)
	}
	cs := newTestService(t)
	if _, err := cs.Convert(1, "[iU]", "[IU]"); err == nil {
		t.Error("case-sensitively [iU] and [IU] should not be comparable; they are separate arbitrary units")
	}
}

// TestCaseInsensitiveServiceIsFullyFeatured: the variant is a vocabulary choice,
// not a reduced service.
func TestCaseInsensitiveServiceIsFullyFeatured(t *testing.T) {
	ci, err := NewCaseInsensitive()
	if err != nil {
		t.Fatal(err)
	}
	exact, ok := ci.(ExactService)
	if !ok {
		t.Fatal("a case-insensitive service does not satisfy ExactService")
	}
	f, err := exact.ConversionFactor("L", "ML")
	if err != nil {
		t.Fatal(err)
	}
	if f.RatString() != "1000" {
		t.Errorf(`ConversionFactor("L", "ML") = %s, want 1000`, f.RatString())
	}
	if _, ok := ci.(Identified); !ok {
		t.Error("a case-insensitive service does not satisfy Identified")
	}
	// Arbitrary units keep their own dimension here too.
	if _, err := ci.Convert(1, "[ARB'U]", "[IU]"); err == nil {
		t.Error("arbitrary units should not be comparable in the case-insensitive vocabulary either")
	}
}
