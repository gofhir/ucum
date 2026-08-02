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

// TestAnalyzeKeepsGroups is the same rule in the public Analyze, whose output is
// read by a person. A description of "mL/(kg.min)" that reads as "mL/kg.min"
// names a different unit; the brackets are square so they cannot be confused
// with the parentheses Analyze already puts around every unit name.
func TestAnalyzeKeepsGroups(t *testing.T) {
	svc := newTestService(t)

	tests := []struct {
		code string
		want string
	}{
		{"mL/(kg.min)", "(milliliter) / [(kilogram) * (minute)]"},
		{"kg/(s.m2)", "(kilogram) / [(second) * (meter ^ 2)]"},
		{"(kg/s).m2", "(kilogram) / (second) * (meter ^ 2)"},
		{"kg.m/s2", "(kilogram) * (meter) / (second ^ 2)"},
		{"(m)", "(meter)"},
	}
	for _, tt := range tests {
		got, err := svc.Analyze(tt.code)
		if err != nil {
			t.Errorf("Analyze(%q): %v", tt.code, err)
			continue
		}
		if got != tt.want {
			t.Errorf("Analyze(%q) = %q, want %q", tt.code, got, tt.want)
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

		// A group on the right of an operator keeps its parentheses. Dropping
		// them changes the unit, because the operators are left-associative:
		// "kg/s.m2" is kg.m2.s-1, not kg.m-2.s-1. Found by FuzzComposeRoundTrip,
		// on codes taken from the FHIR ucum-common value set.
		{"kg/(s.m2)", "kg/(s.m2)"},
		{"mL/(kg.min)", "mL/(kg.min)"},
		{"mL/min/(173.10*-2.m2)", "mL/min/(173.10*-2.m2)"},
		{"m.(s/kg)", "m.(s/kg)"},

		// A group on the left needs none, since that is what left-associativity
		// already means.
		{"(kg/s).m2", "kg/s.m2"},
		{"(m/s)/kg", "m/s/kg"},

		// Redundant parentheses are dropped, and looked through when deciding
		// whether the group carries an operator at all.
		{"(m)", "m"},
		{"((m))", "m"},
		{"0/((1.A))", "0/(1.A)"},
		{"kg/((s.m2))", "kg/(s.m2)"},

		// Annotations carry no meaning and do not survive the round trip.
		{"mg/dL{lot17}", "mg/dL"},
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
	var mBase, sBase *baseUnit
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
