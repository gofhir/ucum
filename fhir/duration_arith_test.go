package fhir_test

import (
	"testing"

	"github.com/gofhir/ucum/v4/fhir"
)

// TestAllowedInDateTimeArithmeticMatchesTheSuite pins the function to the
// official conformance suite rather than to the specification's prose, which
// disagree.
//
// FHIRPath's prose says that "if a definite-quantity duration above seconds
// appears in a date/time arithmetic calculation, the evaluation will end and
// signal an error", and its list of accepted units names UCUM codes only for
// seconds and milliseconds. Read literally, that rejects 'wk', 'd', 'h' and
// 'min'.
//
// The conformance suite says otherwise. From FHIR/fhir-test-cases,
// r4/fhirpath/tests-fhir-r4.xml, where an expression without invalid= must
// evaluate without error:
//
//	<expression>@1973-12-25 + 1 'd'</expression>
//	<expression>@1973-12-25 + 1 'wk'</expression>
//	<expression>@1973-12-25T00:00:00.000+10:00 + 1 'min'</expression>
//	<expression>@1973-12-25T00:00:00.000+10:00 + 1 'h'</expression>
//	<expression>@1973-12-25T00:00:00.000+10:00 + 1 's'</expression>
//	<expression>@1973-12-25T00:00:00.000+10:00 + 10 'ms'</expression>
//	<expression invalid="execution">@1973-12-25 + 1 'mo'</expression>
//	<expression invalid="execution">@1973-12-25 + 1 'a'</expression>
//
// Only 'a' and 'mo' are rejected, and the reason is not magnitude: those are the
// two units whose calendar length varies, so a UCUM year of exactly 365.25 days
// is not a calendar year. A UCUM week is exactly seven days and so is a calendar
// week, which is why 'wk' is allowed even though it is "above seconds".
func TestAllowedInDateTimeArithmeticMatchesTheSuite(t *testing.T) {
	allowed := []string{"wk", "d", "h", "min", "s", "ms"}
	for _, code := range allowed {
		if !fhir.AllowedInDateTimeArithmetic(code) {
			t.Errorf("AllowedInDateTimeArithmetic(%q) = false, want true: the suite evaluates it without error", code)
		}
	}
	// The two whose calendar length varies.
	for _, code := range []string{"a", "mo"} {
		if fhir.AllowedInDateTimeArithmetic(code) {
			t.Errorf("AllowedInDateTimeArithmetic(%q) = true, want false: the suite marks it invalid=\"execution\"", code)
		}
	}
	// Units that are not durations at all cannot appear either.
	for _, code := range []string{"kg", "m", "Cel", "a_j", "mo_s", "us", "not-a-unit"} {
		if fhir.AllowedInDateTimeArithmetic(code) {
			t.Errorf("AllowedInDateTimeArithmetic(%q) = true, want false", code)
		}
	}
}

// TestCalendarEquivalenceIsSeparateFromArithmetic: the equivalence table and the
// arithmetic rule answer different questions, and conflating them is what made
// the function wrong.
//
// The table states how a UCUM quantity relates to a calendar keyword, and marks
// everything above seconds as equivalent (~) rather than equal (=). That says
// nothing about whether the unit may be added to a date, which the suite settles
// separately.
func TestCalendarEquivalenceIsSeparateFromArithmetic(t *testing.T) {
	// 'wk' is only equivalent to a calendar week, yet is valid in arithmetic.
	eq, ok := fhir.CalendarEquivalentOf("wk")
	if !ok {
		t.Fatal("CalendarEquivalentOf(\"wk\") not found")
	}
	if eq.Equal {
		t.Error(`CalendarEquivalentOf("wk").Equal = true; the specification's table marks it ~, not =`)
	}
	if !fhir.AllowedInDateTimeArithmetic("wk") {
		t.Error(`"wk" is equivalent-but-not-equal and still valid in date arithmetic; the two questions are separate`)
	}
	// 'a' is equally "only equivalent", and is not valid in arithmetic.
	if fhir.AllowedInDateTimeArithmetic("a") {
		t.Error(`"a" must not be valid in date arithmetic`)
	}
}
