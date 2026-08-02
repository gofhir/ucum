package engine

import (
	"errors"
	"math"
	"math/big"
	"testing"

	"github.com/gofhir/ucum/v4/internal/expr"
	"github.com/gofhir/ucum/v4/internal/ucumerr"
)

// The lexer and parser are hand-written and take strings from the network, so
// they get fuzzed. The properties below are the ones worth stating: a code
// either parses or errors, and whatever parses must survive everything the
// service will then do to it.

// seedCorpus returns the codes to start from: every case in the official suite,
// plus shapes that have caused trouble.
func seedCorpus(t *testing.T) []string {
	t.Helper()
	suite := loadTestSuite(t)

	codes := make([]string, 0, 40+len(suite.Validation.Cases)+2*len(suite.Conversion.Cases))
	codes = append(codes,
		"", "1", "m", "/s", "m/s2", "kg.m/s2", "10*3/uL", "mg/dL{lot17}",
		"4.[pi].10*-7.N/A2", "mL/min/(173.10*-2.m2)",
		"Cel", "(Cel)", "mCel", "dB", "[pH]", "Cel/min", "[degF]/h",
		"m/0", "0", "0/0", "m0", "m-0", "10*", "10^", "%", "[pi]", "{}", "m{}",
		"[IU]", "[iU]", "k[IU]", "k[ft_i]", "m1000", "m-1000",
		"(((m)))", "m.(s/(kg))", "m{a}", "{a}", "2.3", "m+2",
	)
	for _, c := range suite.Validation.Cases {
		codes = append(codes, c.Unit)
	}
	for _, c := range suite.Conversion.Cases {
		codes = append(codes, c.SrcUnit, c.DstUnit)
	}
	return codes
}

// FuzzValidate checks that no input crashes the parser, and that a code the
// parser accepts is one the rest of the service can handle: canonicalization
// must not panic, must not hang, and must agree with itself.
func FuzzValidate(f *testing.F) {
	for _, code := range seedCorpus(&testing.T{}) {
		f.Add(code)
	}
	svc, err := New(nil, false)
	if err != nil {
		f.Fatal(err)
	}
	ex, err := New(nil, false)
	if err != nil {
		f.Fatal(err)
	}

	f.Fuzz(func(t *testing.T, code string) {
		err := svc.Validate(code)
		if err != nil {
			// Every rejection is a *ucumerr.ValidationError naming the code. Nothing
			// else may come out of Validate.
			var ve *ucumerr.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("Validate(%q) returned %T, want *ucumerr.ValidationError: %v", code, err, err)
			}
			if ve.Offset < -1 || ve.Offset > len(code) {
				t.Fatalf("Validate(%q): Offset %d is outside the code", code, ve.Offset)
			}
			return
		}

		// Accepted. Canonicalization may still fail — "m/0" is well-formed and
		// not canonicalizable — but only with an error, never a panic.
		pair, err := svc.Canonical(1, code)
		if err != nil {
			var ve *ucumerr.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("Canonical(1, %q) returned %T, want *ucumerr.ValidationError: %v", code, err, err)
			}
			return
		}
		if pair.Code == "" {
			t.Fatalf("Canonical(1, %q) returned an empty canonical code", code)
		}

		// A unit is always comparable with itself, and converting to itself is
		// the identity. These are the invariants a canonicalizer can break
		// without anyone noticing.
		//
		// The one exception is a unit whose factor is zero — "0" and "m/0" are
		// well-formed codes — where the conversion divides by that factor. There
		// is no identity to preserve on a degenerate scale, so ucumerr.ErrDivisionByZero
		// is the right answer rather than a broken invariant.
		ok, err := svc.IsComparable(code, code)
		if err != nil {
			if errors.Is(err, ucumerr.ErrDivisionByZero) {
				return
			}
			t.Fatalf("IsComparable(%q, %q): %v", code, code, err)
		}
		if !ok {
			t.Fatalf("IsComparable(%q, %q) = false: a unit must be comparable with itself", code, code)
		}

		got, err := svc.Convert(1, code, code)
		if err != nil {
			if errors.Is(err, ucumerr.ErrDivisionByZero) {
				return
			}
			t.Fatalf("Convert(1, %q, %q): %v", code, code, err)
		}
		// The identity only survives float64 where the scale has an exact
		// rational form. On the logarithmic and trigonometric scales the round
		// trip goes through math.Pow and math.Log, and an extreme prefix
		// destroys the value on the way: a zetta-bel canonicalizes to 10^(10^21),
		// which overflows to +Inf, and an atto-bel to 10^(10^-18), which rounds
		// to exactly 1 and comes back as 0. Those are the limits of the type, not
		// of the conversion, and the exact API refuses those scales outright
		// rather than returning a rounded answer dressed up as an exact one.
		if _, err := ex.ConvertRat(oneRat(), code, code); errors.Is(err, ErrNotRational) {
			return
		}
		if math.Abs(got-1) > 1e-9 {
			t.Fatalf("Convert(1, %q, %q) = %v, want 1: converting to itself is the identity", code, code, got)
		}
	})
}

// oneRat returns a fresh exact 1, since the exact API takes ownership of nothing
// but also promises nothing about reuse.
func oneRat() *big.Rat { return big.NewRat(1, 1) }

// FuzzComposeRoundTrip checks that a parsed code renders back to something that
// parses to the same canonical form. A composer that drops a prefix or an
// exponent would show up here and nowhere else.
func FuzzComposeRoundTrip(f *testing.F) {
	for _, code := range seedCorpus(&testing.T{}) {
		f.Add(code)
	}
	svc, err := New(nil, false)
	if err != nil {
		f.Fatal(err)
	}

	f.Fuzz(func(t *testing.T, code string) {
		t1, err := svc.parseCached(code)
		if err != nil {
			return
		}
		rendered := expr.Compose(t1)
		if rendered == "" {
			return
		}

		t2, err := svc.parseCached(rendered)
		if err != nil {
			t.Fatalf("expr.Compose(parse(%q)) = %q, which does not parse: %v", code, rendered, err)
		}

		// The rendering is not always byte-identical — annotations are dropped,
		// since they carry no meaning — but it must denote the same unit.
		c1, err1 := svc.canonicalizeTerm(t1, svc.specialContextOf(t1))
		c2, err2 := svc.canonicalizeTerm(t2, svc.specialContextOf(t2))
		if err1 != nil || err2 != nil {
			return
		}
		if u1, u2 := expr.ComposeCanonicalUnits(c1), expr.ComposeCanonicalUnits(c2); u1 != u2 {
			t.Fatalf("round trip changed the units: %q -> %q, canonical %q vs %q", code, rendered, u1, u2)
		}
		if !c1.Value.Equal(c2.Value) {
			t.Fatalf("round trip changed the value: %q -> %q, %s vs %s", code, rendered, c1.Value, c2.Value)
		}
	})
}
