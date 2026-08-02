package ucum

import (
	"errors"
	"math/big"
	"testing"
)

// The error contract: every failure carries a typed error a caller can match on,
// and the sentinel behind it survives the wrapping.
//
// A zero divisor is well-formed UCUM — "m/0" parses — so it surfaces during
// canonicalization, which means it can come out of almost any method. Before
// this was pinned, only Convert wrapped it; the other five returned a bare
// unexported error, leaving callers nothing to compare against but the string.

// TestZeroDivisorIsMatchableEverywhere checks the sentinel on every path that
// can produce it.
func TestZeroDivisorIsMatchableEverywhere(t *testing.T) {
	svc := newTestService(t)
	ex, err := NewExact()
	if err != nil {
		t.Fatal(err)
	}

	calls := []struct {
		name string
		err  error
	}{
		{"Convert(m -> m/0)", secondOf(svc.Convert(1, "m", "m/0"))},
		{"Convert(m/0 -> m)", secondOf(svc.Convert(1, "m/0", "m"))},
		{"Canonical(m/0)", pairErr(svc.Canonical(1, "m/0"))},
		{"IsComparable(m/0, m)", boolErr(svc.IsComparable("m/0", "m"))},
		{"Multiply(m/0, s)", pairErr(svc.Multiply(Pair{Value: 1, Code: "m/0"}, Pair{Value: 1, Code: "s"}))},
		{"Divide(m/0, s)", pairErr(svc.Divide(Pair{Value: 1, Code: "m/0"}, Pair{Value: 1, Code: "s"}))},
		{"Divide by a zero value", pairErr(svc.Divide(Pair{Value: 1, Code: "m"}, Pair{Value: 0, Code: "s"}))},
		{"ValidateInProperty(m/0)", svc.ValidateInProperty("m/0", "length")},
		{"ConversionFactor(m, m/0)", ratErr(ex.ConversionFactor("m", "m/0"))},
		{"ConvertRat(m -> m/0)", ratErr(ex.ConvertRat(big.NewRat(1, 1), "m", "m/0"))},
		{"CanonicalRat(m/0)", ratPairErr(ex.CanonicalRat(big.NewRat(1, 1), "m/0"))},
	}

	for _, c := range calls {
		if c.err == nil {
			t.Errorf("%s: got nil error, want one", c.name)
			continue
		}
		if !errors.Is(c.err, ErrDivisionByZero) {
			t.Errorf("%s: errors.Is(err, ErrDivisionByZero) = false, err = %v", c.name, c.err)
		}
	}
}

// TestErrorsCarryTheirType checks that each method reports failures as the type
// its documentation promises: a bad code is a *ValidationError, and a failed
// conversion between two codes is a *ConversionError.
func TestErrorsCarryTheirType(t *testing.T) {
	svc := newTestService(t)
	ex, err := NewExact()
	if err != nil {
		t.Fatal(err)
	}

	validation := []struct {
		name string
		err  error
	}{
		{"Validate", svc.Validate("nope")},
		{"Canonical", pairErr(svc.Canonical(1, "nope"))},
		{"Analyze", analyzeErr(svc.Analyze("nope"))},
		{"IsComparable", boolErr(svc.IsComparable("nope", "m"))},
		{"Multiply", pairErr(svc.Multiply(Pair{Value: 1, Code: "nope"}, Pair{Value: 1, Code: "s"}))},
		{"Divide", pairErr(svc.Divide(Pair{Value: 1, Code: "m"}, Pair{Value: 1, Code: "nope"}))},
		{"ValidateInProperty", svc.ValidateInProperty("nope", "length")},
		{"CanonicalRat", ratPairErr(ex.CanonicalRat(big.NewRat(1, 1), "nope"))},
		{"Canonical, zero divisor", pairErr(svc.Canonical(1, "m/0"))},
	}
	for _, c := range validation {
		var ve *ValidationError
		if !errors.As(c.err, &ve) {
			t.Errorf("%s: errors.As(err, **ValidationError) = false, err = %v", c.name, c.err)
			continue
		}
		if ve.Code == "" {
			t.Errorf("%s: ValidationError.Code is empty, err = %v", c.name, c.err)
		}
	}

	conversion := []struct {
		name string
		err  error
	}{
		{"Convert, unknown unit", secondOf(svc.Convert(1, "nope", "m"))},
		{"Convert, incommensurable", secondOf(svc.Convert(1, "m", "s"))},
		{"Convert, zero divisor", secondOf(svc.Convert(1, "m", "m/0"))},
		{"ConvertRat", ratErr(ex.ConvertRat(big.NewRat(1, 1), "nope", "m"))},
		{"ConversionFactor", ratErr(ex.ConversionFactor("nope", "m"))},
	}
	for _, c := range conversion {
		var ce *ConversionError
		if !errors.As(c.err, &ce) {
			t.Errorf("%s: errors.As(err, **ConversionError) = false, err = %v", c.name, c.err)
			continue
		}
		if ce.From == "" || ce.To == "" {
			t.Errorf("%s: ConversionError is missing From/To, err = %v", c.name, c.err)
		}
	}
}

// TestValidationErrorReportsThePosition pins that Offset is filled in rather
// than left at -1. The lexer knows where it stopped, and a caller highlighting
// the bad character in a form field needs that number, not a prose message it
// would have to parse back.
func TestValidationErrorReportsThePosition(t *testing.T) {
	svc := newTestService(t)

	tests := []struct {
		code string
		want int
	}{
		{"m/", 2},      // ran out of input after the solidus
		{"m..s", 2},    // the second period has nothing before it
		{"kg/{ann", 3}, // unterminated annotation, reported where it opened
		{"m@s", 1},     // not a token character
	}
	for _, tt := range tests {
		err := svc.Validate(tt.code)
		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Errorf("Validate(%q): errors.As(err, **ValidationError) = false, err = %v", tt.code, err)
			continue
		}
		if ve.Offset != tt.want {
			t.Errorf("Validate(%q): Offset = %d, want %d (err = %v)", tt.code, ve.Offset, tt.want, err)
		}
	}

	// An error with no position keeps -1 rather than pointing at the start.
	err := svc.ValidateInProperty("m", "mass")
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("ValidateInProperty: errors.As = false, err = %v", err)
	}
	if ve.Offset != -1 {
		t.Errorf("ValidateInProperty: Offset = %d, want -1: the code is well-formed", ve.Offset)
	}
}

// Helpers that drop the value and keep the error, so the tables above stay
// readable.

func secondOf(_ float64, err error) error   { return err }
func pairErr(_ Pair, err error) error       { return err }
func boolErr(_ bool, err error) error       { return err }
func analyzeErr(_ string, err error) error  { return err }
func ratErr(_ *big.Rat, err error) error    { return err }
func ratPairErr(_ RatPair, err error) error { return err }
