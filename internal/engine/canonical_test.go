package engine

import (
	"testing"

	"github.com/gofhir/ucum/v4/internal/expr"
)

// These tests cover the canonicalization path used in production
// (service.canonicalizeTerm). They replace the tests of the parallel
// implementation that used to live in converter.go.

func TestCanonicalizeUnits(t *testing.T) {
	svc, err := New(nil, false)
	if err != nil {
		t.Fatal(err)
	}

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
		{"mm[Hg]", "g.m-1.s-2"},
		{"mol/L", "m-3"},
		{"Cel", "K"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			can, err := svc.getCanonical(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if got := expr.ComposeCanonicalUnits(can); got != tt.want {
				t.Errorf("canonical(%q) units = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCanonicalizeValues(t *testing.T) {
	svc, err := New(nil, false)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		input string
		want  string // exact canonical factor, as a rational
	}{
		{"km", "1000"},  // 1 km = 1000 m
		{"kg", "1000"},  // 1 kg = 1000 g
		{"m", "1"},      // base unit
		{"L", "1/1000"}, // 1 L = 0.001 m3
		{"mL", "1/1000000"},
		{"cm", "1/100"},
		{"%", "1/100"},
		{"h", "3600"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			can, err := svc.getCanonical(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if got := can.Value.Rat().RatString(); got != tt.want {
				t.Errorf("canonical(%q) value = %s, want %s", tt.input, got, tt.want)
			}
		})
	}
}

func TestCanonicalizeRejectsUnknownUnit(t *testing.T) {
	svc, err := New(nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.getCanonical("definitely-not-a-unit"); err == nil {
		t.Error("getCanonical of an unknown unit = nil error, want an error")
	}
}
