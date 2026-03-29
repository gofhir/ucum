package ucum

import "testing"

func TestConverterCanonicalUnits(t *testing.T) {
	model, err := loadDefinitions(nil)
	if err != nil {
		t.Fatal(err)
	}
	p := newParser(model)
	c := newConverter(model, specialHandlers)

	tests := []struct {
		input string
		want  string // canonical unit string
	}{
		{"m", "m"},
		{"km", "m"},      // prefix k * m
		{"m/s", "m.s-1"}, // division
		{"m2", "m2"},     // exponent
		{"1", "1"},       // dimensionless
		{"kg", "g"},      // kg = k * g, canonical is g
		{"%", "1"},       // percent is dimensionless
		{"L", "m3"},      // liter = dm3 = 0.001 m3
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			ast, err := p.parse(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			can, err := c.convert(ast)
			if err != nil {
				t.Fatal(err)
			}
			got := composeCanonicalUnits(can)
			if got != tt.want {
				t.Errorf("canonical(%q) units = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestConverterCanonicalValue(t *testing.T) {
	model, err := loadDefinitions(nil)
	if err != nil {
		t.Fatal(err)
	}
	p := newParser(model)
	c := newConverter(model, specialHandlers)

	// 1 km = 1000 m
	ast, err := p.parse("km")
	if err != nil {
		t.Fatal(err)
	}
	can, err := c.convert(ast)
	if err != nil {
		t.Fatal(err)
	}
	if can.value.float64() != 1000 {
		t.Errorf("canonical(km) value = %v, want 1000", can.value.float64())
	}

	// 1 kg = 1000 g
	ast, err = p.parse("kg")
	if err != nil {
		t.Fatal(err)
	}
	can, err = c.convert(ast)
	if err != nil {
		t.Fatal(err)
	}
	if can.value.float64() != 1000 {
		t.Errorf("canonical(kg) value = %v, want 1000", can.value.float64())
	}
}
