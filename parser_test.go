package ucum

import "testing"

func TestParserValid(t *testing.T) {
	model, err := loadDefinitions(nil)
	if err != nil {
		t.Fatal(err)
	}
	p := newParser(model)

	valid := []string{
		"m", "kg", "m/s", "mg/dL", "10*3/uL", "m.s-1", "m2",
		"kg.m/s2", "%", "[lb_av]", "cm[H2O]", "mol/L", "mm[Hg]",
		"/m", "{score}", "m{annotation}", "1",
	}
	for _, code := range valid {
		_, err := p.parse(code)
		if err != nil {
			t.Errorf("parse(%q) error: %v", code, err)
		}
	}
}

func TestParserInvalid(t *testing.T) {
	model, err := loadDefinitions(nil)
	if err != nil {
		t.Fatal(err)
	}
	p := newParser(model)

	invalid := []string{"xyz", "m/", ""}
	for _, code := range invalid {
		_, err := p.parse(code)
		if err == nil {
			t.Errorf("parse(%q) should fail", code)
		}
	}
}

func TestParserSymbolResolution(t *testing.T) {
	model, err := loadDefinitions(nil)
	if err != nil {
		t.Fatal(err)
	}
	p := newParser(model)

	// "km" should resolve to prefix "k" + unit "m"
	ast, err := p.parse("km")
	if err != nil {
		t.Fatal(err)
	}
	sym, ok := ast.comp.(*symbol)
	if !ok {
		t.Fatal("expected symbol component")
	}
	if sym.prefix == nil || sym.prefix.Code != "k" {
		t.Error("expected prefix k")
	}
	if sym.unit == nil || sym.unit.Code != "m" {
		t.Error("expected unit m")
	}
}

func TestParserExponent(t *testing.T) {
	model, err := loadDefinitions(nil)
	if err != nil {
		t.Fatal(err)
	}
	p := newParser(model)

	ast, err := p.parse("m2")
	if err != nil {
		t.Fatal(err)
	}
	sym := ast.comp.(*symbol)
	if sym.exponent != 2 {
		t.Errorf("exponent = %d, want 2", sym.exponent)
	}

	ast, err = p.parse("m-2")
	if err != nil {
		t.Fatal(err)
	}
	sym = ast.comp.(*symbol)
	if sym.exponent != -2 {
		t.Errorf("exponent = %d, want -2", sym.exponent)
	}
}

// TestPrefixRequiresMetricAtom covers UCUM §11 ■1, "Only metric unit atoms may
// be combined with a prefix".
func TestPrefixRequiresMetricAtom(t *testing.T) {
	svc := newTestService(t)

	// Non-metric atoms: a prefixed form is not a valid code.
	// Every atom below is declared isMetric="no" in the definitions.
	rejected := []string{
		"k[ft_i]", "m[lb_av]", "k[in_i]", "c[pi]", "k[oz_av]", "m[yd_i]",
	}
	for _, code := range rejected {
		if err := svc.Validate(code); err == nil {
			t.Errorf("Validate(%q) = nil, want an error: the atom is not metric", code)
		}
	}

	// Metric atoms, base units, and bracket units that are metric: still fine.
	accepted := []string{
		"mm", "kg", "kL", "cm3", "us", "nmol",
		"k[IU]", "m[IU]", "k[iU]", // [IU] and [iU] are isMetric="yes"
		"mCel",                        // §22 ■3 allows a prefix on a special unit
		"m[H2O]", "cm[H2O]", "mm[Hg]", // metric bracket units
		"[ft_i]", "[lb_av]", "[in_i]", "[pi]", // unprefixed, always valid
	}
	for _, code := range accepted {
		if err := svc.Validate(code); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", code, err)
		}
	}
}

// TestPrefixedNonMetricDoesNotConvert: the codes above must not sneak through
// into conversion either, which is what makes this more than a validation nicety.
func TestPrefixedNonMetricDoesNotConvert(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.Convert(1, "k[ft_i]", "m"); err == nil {
		t.Error(`Convert(1, "k[ft_i]", "m") = nil error, want an error`)
	}
	if _, err := svc.Canonical(1, "k[ft_i]"); err == nil {
		t.Error(`Canonical(1, "k[ft_i]") = nil error, want an error`)
	}
}
