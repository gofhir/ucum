package ucum

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// UCUM states no bound on the length of a code, the depth of its parentheses or
// the size of an exponent, because it describes a notation rather than an
// implementation. An implementation that takes codes from the network needs
// bounds anyway: without them a short string is enough to exhaust the process.
//
// The bounds here are set far above anything real. The longest code in the
// official suite is 17 bytes, the longest in the FHIR ucum-common value set is
// 21, the deepest nesting in either is one, and no definition in
// ucum-essence.xml uses an exponent outside [-4, 4].

// TestLimitsRejectRatherThanExhaust pins that each bound produces an error, and
// promptly.
func TestLimitsRejectRatherThanExhaust(t *testing.T) {
	svc := newTestService(t)

	tests := []struct {
		name string
		code string
		want error
	}{
		{
			// Before the bound, this returned in about 5 minutes of big.Rat
			// multiplication, having built an integer of billions of digits.
			name: "an exponent large enough to be a denial of service",
			code: "m2000000000",
			want: ErrExponentTooLarge,
		},
		{
			name: "an exponent just past the bound",
			code: "m1001",
			want: ErrExponentTooLarge,
		},
		{
			name: "a negative exponent past the bound",
			code: "m-1001",
			want: ErrExponentTooLarge,
		},
		{
			// Before the bound, Canonical on this crashed the process with
			// "fatal error: stack overflow", which recover() cannot catch.
			name: "nesting deep enough to overflow the stack",
			code: strings.Repeat("(", 200) + "m" + strings.Repeat(")", 200),
			want: ErrCodeTooComplex,
		},
		{
			name: "a code long enough to nest deeply without parentheses",
			code: strings.Repeat("m.", 2000) + "m",
			want: ErrCodeTooLong,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			done := make(chan error, 1)
			go func() { done <- svc.Validate(tt.code) }()

			select {
			case err := <-done:
				if !errors.Is(err, tt.want) {
					t.Errorf("Validate: err = %v, want %v", err, tt.want)
				}
				var ve *ValidationError
				if !errors.As(err, &ve) {
					t.Errorf("Validate: errors.As(err, **ValidationError) = false, err = %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Validate did not return within 5s: the bound is not being applied")
			}

			// Canonical is the path that actually recursed and allocated, so it
			// has to refuse too rather than relying on the caller validating
			// first.
			if _, err := svc.Canonical(1, tt.code); !errors.Is(err, tt.want) {
				t.Errorf("Canonical: err = %v, want %v", err, tt.want)
			}
		})
	}
}

// TestLimitsAcceptEverythingReal checks the bounds leave a wide margin over the
// codes that actually occur, including a long annotation, which is the one place
// a real code can grow.
func TestLimitsAcceptEverythingReal(t *testing.T) {
	svc := newTestService(t)

	accepted := []string{
		"4.[pi].10*-7.N/A2",                    // longest in the official suite
		"mL/min/(173.10*-2.m2)",                // longest in the FHIR value set
		"m1000",                                // exactly at the exponent bound
		"m-1000",                               // and its negative
		"kg{" + strings.Repeat("a", 900) + "}", // a 900-byte annotation
		strings.Repeat("(", 50) + "m" + strings.Repeat(")", 50), // half the depth bound
	}
	for _, code := range accepted {
		if err := svc.Validate(code); err != nil {
			t.Errorf("Validate(%.40q...): %v", code, err)
		}
	}
}

// TestExponentiationIsSubQuadratic pins the algorithm rather than a wall-clock
// figure: raising to the bound must not cost the bound in multiplications.
// Iterative multiplication made Canonical(1, "m1000000") take 312ms and scaled
// linearly from there.
func TestExponentiationIsSubQuadratic(t *testing.T) {
	small := decimalFromString2(t, "1.0000001")

	// Square-and-multiply does about log2(n) multiplications, so a thousandfold
	// increase in the exponent must not cost a thousandfold in time. Compare
	// shapes rather than absolute times, which vary with the machine.
	start := time.Now()
	for i := 0; i < 100; i++ {
		_ = small.pow(1000)
	}
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Errorf("100 × pow(1000) took %v, want well under 2s: the exponentiation is still linear", elapsed)
	}

	// And it stays correct: pow is used for every prefixed unit.
	two := decimalFromInt(2)
	if got := two.pow(10); got.String() != "1024" {
		t.Errorf("2^10 = %s, want 1024", got)
	}
	if got := two.pow(-3); got.String() != "0.1250000000" {
		t.Errorf("2^-3 = %s, want 0.125", got)
	}
	if got := two.pow(0); got.String() != "1" {
		t.Errorf("2^0 = %s, want 1", got)
	}
	if got := decimalFromInt(-3).pow(3); got.String() != "-27" {
		t.Errorf("(-3)^3 = %s, want -27", got)
	}
}

func decimalFromString2(t *testing.T, s string) decimal {
	t.Helper()
	d, err := decimalFromString(s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
