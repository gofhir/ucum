package expr

import (
	"testing"

	"github.com/gofhir/ucum/v4/internal/essence"
)

func TestParserValid(t *testing.T) {
	model, err := essence.Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	p := NewParser(model)

	valid := []string{
		"m", "kg", "m/s", "mg/dL", "10*3/uL", "m.s-1", "m2",
		"kg.m/s2", "%", "[lb_av]", "cm[H2O]", "mol/L", "mm[Hg]",
		"/m", "{score}", "m{annotation}", "1",
	}
	for _, code := range valid {
		_, err := p.Parse(code)
		if err != nil {
			t.Errorf("parse(%q) error: %v", code, err)
		}
	}
}

func TestParserInvalid(t *testing.T) {
	model, err := essence.Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	p := NewParser(model)

	invalid := []string{"xyz", "m/", ""}
	for _, code := range invalid {
		_, err := p.Parse(code)
		if err == nil {
			t.Errorf("parse(%q) should fail", code)
		}
	}
}

func TestParserSymbolResolution(t *testing.T) {
	model, err := essence.Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	p := NewParser(model)

	// "km" should resolve to prefix "k" + unit "m"
	ast, err := p.Parse("km")
	if err != nil {
		t.Fatal(err)
	}
	sym, ok := ast.Comp.(*Symbol)
	if !ok {
		t.Fatal("expected symbol Component")
	}
	if sym.Prefix == nil || sym.Prefix.Code != "k" {
		t.Error("expected prefix k")
	}
	if sym.Unit == nil || sym.Unit.Code != "m" {
		t.Error("expected unit m")
	}
}

func TestParserExponent(t *testing.T) {
	model, err := essence.Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	p := NewParser(model)

	ast, err := p.Parse("m2")
	if err != nil {
		t.Fatal(err)
	}
	sym := ast.Comp.(*Symbol)
	if sym.Exponent != 2 {
		t.Errorf("exponent = %d, want 2", sym.Exponent)
	}

	ast, err = p.Parse("m-2")
	if err != nil {
		t.Fatal(err)
	}
	sym = ast.Comp.(*Symbol)
	if sym.Exponent != -2 {
		t.Errorf("exponent = %d, want -2", sym.Exponent)
	}
}
