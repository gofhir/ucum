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
