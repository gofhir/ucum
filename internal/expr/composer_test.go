package expr

import (
	"testing"

	"github.com/gofhir/ucum/v4/internal/essence"
	"github.com/gofhir/ucum/v4/internal/model"
)

func TestComposerRoundTrip(t *testing.T) {
	defs, err := essence.Load(nil)
	if err != nil {
		t.Fatalf("essence.Load: %v", err)
	}
	p := NewParser(defs)

	// These codes should round-trip through parse -> compose -> re-parse.
	codes := []string{"m", "m/s", "kg.m/s2", "mg/dL", "%", "[lb_av]", "m2", "m-1"}
	for _, code := range codes {
		ast, err := p.Parse(code)
		if err != nil {
			t.Fatalf("parse(%q): %v", code, err)
		}
		result := Compose(ast)
		// Re-parse to verify validity.
		_, err = p.Parse(result)
		if err != nil {
			t.Errorf("compose(%q) = %q, fails re-parse: %v", code, result, err)
		}
	}
}

func TestComposerExactOutput(t *testing.T) {
	defs, err := essence.Load(nil)
	if err != nil {
		t.Fatalf("essence.Load: %v", err)
	}
	p := NewParser(defs)

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

		// A group on the right of an Operator keeps its parentheses. Dropping
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
		// whether the group carries an Operator at all.
		{"(m)", "m"},
		{"((m))", "m"},
		{"0/((1.A))", "0/(1.A)"},
		{"kg/((s.m2))", "kg/(s.m2)"},

		// Annotations carry no meaning and do not survive the round trip.
		{"mg/dL{lot17}", "mg/dL"},
	}

	for _, tt := range tests {
		ast, err := p.Parse(tt.input)
		if err != nil {
			t.Fatalf("parse(%q): %v", tt.input, err)
		}
		got := Compose(ast)
		if got != tt.want {
			t.Errorf("Compose(parse(%q)) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestComposeCanonicalUnits(t *testing.T) {
	defs, err := essence.Load(nil)
	if err != nil {
		t.Fatalf("essence.Load: %v", err)
	}

	// Find base units for testing.
	var mBase, sBase *model.BaseUnit
	for _, bu := range defs.BaseUnits {
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
		canon *model.Canonical
		want  string
	}{
		{
			name:  "nil canonical",
			canon: nil,
			want:  "1",
		},
		{
			name:  "no units",
			canon: &model.Canonical{Units: nil},
			want:  "1",
		},
		{
			name: "single base unit",
			canon: &model.Canonical{
				Units: []model.CanonicalUnit{{Base: mBase, Exponent: 1}},
			},
			want: "m",
		},
		{
			name: "velocity m.s-1",
			canon: &model.Canonical{
				Units: []model.CanonicalUnit{
					{Base: mBase, Exponent: 1},
					{Base: sBase, Exponent: -1},
				},
			},
			want: "m.s-1",
		},
		{
			name: "area m2",
			canon: &model.Canonical{
				Units: []model.CanonicalUnit{
					{Base: mBase, Exponent: 2},
				},
			},
			want: "m2",
		},
		{
			name: "skip zero exponent",
			canon: &model.Canonical{
				Units: []model.CanonicalUnit{
					{Base: mBase, Exponent: 1},
					{Base: sBase, Exponent: 0},
				},
			},
			want: "m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComposeCanonicalUnits(tt.canon)
			if got != tt.want {
				t.Errorf("ComposeCanonicalUnits() = %q, want %q", got, tt.want)
			}
		})
	}
}
