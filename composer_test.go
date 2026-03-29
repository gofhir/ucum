package ucum

import "testing"

func TestComposerRoundTrip(t *testing.T) {
	model, err := loadDefinitions(nil)
	if err != nil {
		t.Fatalf("loadDefinitions: %v", err)
	}
	p := newParser(model)

	// These codes should round-trip through parse -> compose -> re-parse.
	codes := []string{"m", "m/s", "kg.m/s2", "mg/dL", "%", "[lb_av]", "m2", "m-1"}
	for _, code := range codes {
		ast, err := p.parse(code)
		if err != nil {
			t.Fatalf("parse(%q): %v", code, err)
		}
		result := composeTerm(ast)
		// Re-parse to verify validity.
		_, err = p.parse(result)
		if err != nil {
			t.Errorf("compose(%q) = %q, fails re-parse: %v", code, result, err)
		}
	}
}

func TestComposerExactOutput(t *testing.T) {
	model, err := loadDefinitions(nil)
	if err != nil {
		t.Fatalf("loadDefinitions: %v", err)
	}
	p := newParser(model)

	tests := []struct {
		input string
		want  string
	}{
		{"m", "m"},
		{"m2", "m2"},
		{"m-1", "m-1"},
		{"m/s", "m/s"},
		{"kg.m/s2", "kg.m/s2"},
		{"%", "%"},
		{"[lb_av]", "[lb_av]"},
		{"mg/dL", "mg/dL"},
	}

	for _, tt := range tests {
		ast, err := p.parse(tt.input)
		if err != nil {
			t.Fatalf("parse(%q): %v", tt.input, err)
		}
		got := composeTerm(ast)
		if got != tt.want {
			t.Errorf("composeTerm(parse(%q)) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestComposeCanonicalUnits(t *testing.T) {
	model, err := loadDefinitions(nil)
	if err != nil {
		t.Fatalf("loadDefinitions: %v", err)
	}

	// Find base units for testing.
	var mBase, sBase *BaseUnit
	for _, bu := range model.BaseUnits {
		switch bu.Code {
		case "m":
			mBase = bu
		case "s":
			sBase = bu
		}
	}
	if mBase == nil || sBase == nil {
		t.Fatal("could not find base units m and s")
	}

	tests := []struct {
		name  string
		canon *canonical
		want  string
	}{
		{
			name:  "nil canonical",
			canon: nil,
			want:  "1",
		},
		{
			name:  "no units",
			canon: &canonical{units: nil},
			want:  "1",
		},
		{
			name: "single base unit",
			canon: &canonical{
				units: []canonicalUnit{{base: mBase, exponent: 1}},
			},
			want: "m",
		},
		{
			name: "velocity m.s-1",
			canon: &canonical{
				units: []canonicalUnit{
					{base: mBase, exponent: 1},
					{base: sBase, exponent: -1},
				},
			},
			want: "m.s-1",
		},
		{
			name: "area m2",
			canon: &canonical{
				units: []canonicalUnit{
					{base: mBase, exponent: 2},
				},
			},
			want: "m2",
		},
		{
			name: "skip zero exponent",
			canon: &canonical{
				units: []canonicalUnit{
					{base: mBase, exponent: 1},
					{base: sBase, exponent: 0},
				},
			},
			want: "m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := composeCanonicalUnits(tt.canon)
			if got != tt.want {
				t.Errorf("composeCanonicalUnits() = %q, want %q", got, tt.want)
			}
		})
	}
}
