package fhir_test

import (
	"errors"
	"math"
	"math/big"
	"testing"

	"github.com/gofhir/ucum/v3"
	"github.com/gofhir/ucum/v3/fhir"
)

// TestCommonUnitsAreValidUCUM is the conformance check that matters most here: a
// code FHIR publishes as commonly encountered must be one this engine accepts.
// It also catches a botched regeneration of the embedded file.
func TestCommonUnitsAreValidUCUM(t *testing.T) {
	svc, err := ucum.New()
	if err != nil {
		t.Fatal(err)
	}
	codes := fhir.CommonCodes()
	if len(codes) != fhir.CommonUnitsCodeCount {
		t.Errorf("ucum-common has %d distinct codes, want %d for value set version %s",
			len(codes), fhir.CommonUnitsCodeCount, fhir.CommonUnitsValueSetVersion)
	}
	var invalid []string
	for _, code := range codes {
		if err := svc.Validate(code); err != nil {
			invalid = append(invalid, code)
		}
	}
	if len(invalid) > 0 {
		t.Errorf("%d of %d ucum-common codes are rejected as invalid UCUM: %v",
			len(invalid), len(codes), invalid)
	}
}

// TestCommonUnitsAreCanonicalizable goes further than validity: every common code
// must also reduce to a canonical form, which is what a server needs to compare
// or index it.
func TestCommonUnitsAreCanonicalizable(t *testing.T) {
	svc, err := ucum.New()
	if err != nil {
		t.Fatal(err)
	}
	var failed []string
	for _, code := range fhir.CommonCodes() {
		if _, err := svc.Canonical(1, code); err != nil {
			failed = append(failed, code)
		}
	}
	if len(failed) > 0 {
		t.Errorf("%d ucum-common codes cannot be canonicalized: %v", len(failed), failed)
	}
}

func TestInCommonUnits(t *testing.T) {
	in := []string{"%", "mg/dL", "mmol/L", "10*3/uL", "mm[Hg]", "Cel", "[IU]/L", "/min"}
	for _, code := range in {
		if !fhir.InCommonUnits(code) {
			t.Errorf("InCommonUnits(%q) = false, want true", code)
		}
	}
	// Valid UCUM, but not in the common subset. Verified against the published
	// value set rather than guessed: [bdsk'U] for instance IS in it.
	out := []string{"B[10.nV]", "[degRe]", "[hp'_X]", "kOhm", "not-a-unit"}
	for _, code := range out {
		if fhir.InCommonUnits(code) {
			t.Errorf("InCommonUnits(%q) = true, want false", code)
		}
	}
}

func TestCommonDisplay(t *testing.T) {
	tests := map[string]string{
		"%":        "percent",
		"%[slope]": "percent of slope",
	}
	for code, want := range tests {
		got, ok := fhir.CommonDisplay(code)
		if !ok {
			t.Errorf("CommonDisplay(%q): not a member", code)
			continue
		}
		if got != want {
			t.Errorf("CommonDisplay(%q) = %q, want %q", code, got, want)
		}
	}
	if _, ok := fhir.CommonDisplay("kOhm"); ok {
		t.Error(`CommonDisplay("kOhm") reported membership, want false`)
	}
	// A repeated code keeps the first, more descriptive display.
	if got, ok := fhir.CommonDisplay("/[HPF]"); !ok || got != "per high power field" {
		t.Errorf(`CommonDisplay("/[HPF]") = (%q, %v), want ("per high power field", true)`, got, ok)
	}
}

// TestCalendarEquivalents pins the FHIRPath table, including which relationships
// are equality and which are only equivalence.
func TestCalendarEquivalents(t *testing.T) {
	tests := []struct {
		code    string
		keyword string
		equal   bool
	}{
		{"a", "year", false},
		{"mo", "month", false},
		{"wk", "week", false},
		{"d", "day", false},
		{"h", "hour", false},
		{"min", "minute", false},
		{"s", "second", true},
		{"ms", "millisecond", true},
	}
	for _, tt := range tests {
		eq, ok := fhir.CalendarEquivalentOf(tt.code)
		if !ok {
			t.Errorf("CalendarEquivalentOf(%q): not found", tt.code)
			continue
		}
		if eq.Keyword != tt.keyword {
			t.Errorf("CalendarEquivalentOf(%q).Keyword = %q, want %q", tt.code, eq.Keyword, tt.keyword)
		}
		if eq.Equal != tt.equal {
			t.Errorf("CalendarEquivalentOf(%q).Equal = %v, want %v", tt.code, eq.Equal, tt.equal)
		}
	}
	// UCUM has other time units; FHIRPath's table names only the eight above.
	for _, code := range []string{"a_j", "a_g", "a_t", "mo_j", "mo_s", "us", "ns"} {
		if _, ok := fhir.CalendarEquivalentOf(code); ok {
			t.Errorf("CalendarEquivalentOf(%q) reported a calendar equivalent, want none", code)
		}
	}
}

// TestAllowedInDateTimeArithmetic pins the FHIRPath rule that a definite
// duration above seconds is an error in date/time arithmetic.
func TestAllowedInDateTimeArithmetic(t *testing.T) {
	for _, code := range []string{"s", "ms"} {
		if !fhir.AllowedInDateTimeArithmetic(code) {
			t.Errorf("AllowedInDateTimeArithmetic(%q) = false, want true", code)
		}
	}
	for _, code := range []string{"a", "mo", "wk", "d", "h", "min", "kg", "m"} {
		if fhir.AllowedInDateTimeArithmetic(code) {
			t.Errorf("AllowedInDateTimeArithmetic(%q) = true, want false", code)
		}
	}
}

func TestMappedByFHIRQuantityConversion(t *testing.T) {
	// FHIR R5 maps six codes; FHIRPath's table has eight.
	mapped := map[string]string{
		"a": "year", "mo": "month", "d": "day",
		"h": "hour", "min": "minute", "s": "second",
	}
	for code, want := range mapped {
		got, ok := fhir.MappedByFHIRQuantityConversion(code)
		if !ok || got != want {
			t.Errorf("MappedByFHIRQuantityConversion(%q) = (%q, %v), want (%q, true)", code, got, ok, want)
		}
	}
	// wk and ms have calendar equivalents in FHIRPath but are not part of the
	// FHIR Quantity mapping.
	for _, code := range []string{"wk", "ms"} {
		if _, ok := fhir.MappedByFHIRQuantityConversion(code); ok {
			t.Errorf("MappedByFHIRQuantityConversion(%q) = true, want false", code)
		}
		if _, ok := fhir.CalendarEquivalentOf(code); !ok {
			t.Errorf("CalendarEquivalentOf(%q) = false, want true", code)
		}
	}
}

// TestUCUMMonthIsNotThirtyDays records the discrepancy documented on
// CalendarEquivalentOf: FHIR R5's prose says UCUM defines mo as 30 days, while
// UCUM defines it as a twelfth of a Julian year.
func TestUCUMMonthIsNotThirtyDays(t *testing.T) {
	svc, err := ucum.New()
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.Convert(1, "mo", "d")
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got-30.4375) > 1e-12 {
		t.Errorf(`Convert(1, "mo", "d") = %v, want 30.4375 (365.25/12)`, got)
	}
	if got == 30 {
		t.Error("UCUM month resolved to 30 days; the FHIR prose figure was adopted, which it should not be")
	}
}

func TestComparatorAcrossUnits(t *testing.T) {
	c, err := fhir.NewComparator()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		a, b fhir.Quantity
		want int
	}{
		{fhir.Quantity{Value: 1, Code: "L"}, fhir.Quantity{Value: 1000, Code: "mL"}, 0},
		{fhir.Quantity{Value: 1, Code: "L"}, fhir.Quantity{Value: 999, Code: "mL"}, 1},
		{fhir.Quantity{Value: 1, Code: "L"}, fhir.Quantity{Value: 1001, Code: "mL"}, -1},
		{fhir.Quantity{Value: 5, Code: "kg"}, fhir.Quantity{Value: 5000, Code: "g"}, 0},
		// The case most likely to be wrong elsewhere.
		{fhir.Quantity{Value: 100, Code: "[degF]"}, fhir.Quantity{Value: 50, Code: "Cel"}, -1},
		{fhir.Quantity{Value: 32, Code: "[degF]"}, fhir.Quantity{Value: 0, Code: "Cel"}, 0},
		{fhir.Quantity{Value: 212, Code: "[degF]"}, fhir.Quantity{Value: 100, Code: "Cel"}, 0},
		{fhir.Quantity{Value: 80, Code: "[degRe]"}, fhir.Quantity{Value: 100, Code: "Cel"}, 0},
	}
	for _, tt := range tests {
		got, err := c.Compare(tt.a, tt.b)
		if err != nil {
			t.Fatalf("Compare(%v, %v): %v", tt.a, tt.b, err)
		}
		if got != tt.want {
			t.Errorf("Compare(%v %s, %v %s) = %d, want %d",
				tt.a.Value, tt.a.Code, tt.b.Value, tt.b.Code, got, tt.want)
		}
	}
}

// TestComparatorNonRationalScale exercises the float64 fallback: pH has no exact
// rational form, so the comparison cannot be exact but must still work.
func TestComparatorNonRationalScale(t *testing.T) {
	c, err := fhir.NewComparator()
	if err != nil {
		t.Fatal(err)
	}
	acid := fhir.Quantity{Value: 3, Code: "[pH]"}
	base := fhir.Quantity{Value: 10, Code: "[pH]"}
	// A lower pH is a higher concentration, so it compares greater once
	// canonicalized into mol/L.
	got, err := c.Compare(acid, base)
	if err != nil {
		t.Fatal(err)
	}
	if got != 1 {
		t.Errorf("Compare(3 [pH], 10 [pH]) = %d, want 1 (lower pH is more concentrated)", got)
	}
}

func TestComparatorRejects(t *testing.T) {
	c, err := fhir.NewComparator()
	if err != nil {
		t.Fatal(err)
	}
	// Incommensurable.
	if _, err := c.Compare(fhir.Quantity{Value: 1, Code: "m"}, fhir.Quantity{Value: 1, Code: "s"}); err == nil {
		t.Error("Compare(1 m, 1 s) = nil error, want an error")
	}
	// Arbitrary units are commensurable with nothing but themselves.
	if _, err := c.Compare(fhir.Quantity{Value: 1, Code: "[IU]"}, fhir.Quantity{Value: 1, Code: "[iU]"}); err == nil {
		t.Error("Compare(1 [IU], 1 [iU]) = nil error, want an error")
	}
	// A non-UCUM system.
	snomed := fhir.Quantity{Value: 1, Code: "258773002", System: "http://snomed.info/sct"}
	_, err = c.Compare(snomed, fhir.Quantity{Value: 1, Code: "mL"})
	if !errors.Is(err, fhir.ErrNotUCUMSystem) {
		t.Errorf("Compare with a SNOMED system: err = %v, want ErrNotUCUMSystem", err)
	}
	// An explicit UCUM system is accepted.
	explicit := fhir.Quantity{Value: 1, Code: "L", System: fhir.UCUMSystem}
	if _, err := c.Compare(explicit, fhir.Quantity{Value: 1000, Code: "mL"}); err != nil {
		t.Errorf("Compare with an explicit UCUM system: %v", err)
	}
	// Non-finite values have no rational representation.
	if _, err := c.Compare(fhir.Quantity{Value: math.NaN(), Code: "L"}, fhir.Quantity{Value: 1, Code: "L"}); err == nil {
		t.Error("Compare(NaN L, 1 L) = nil error, want an error")
	}
}

func TestCanonicalKey(t *testing.T) {
	c, err := fhir.NewComparator()
	if err != nil {
		t.Fatal(err)
	}
	// Two quantities that should share an index key.
	k1, v1, err := c.CanonicalKey(fhir.Quantity{Value: 1, Code: "L"})
	if err != nil {
		t.Fatal(err)
	}
	k2, v2, err := c.CanonicalKey(fhir.Quantity{Value: 1000, Code: "mL"})
	if err != nil {
		t.Fatal(err)
	}
	if k1 != k2 {
		t.Errorf("CanonicalKey: L gives %q, mL gives %q; want the same key", k1, k2)
	}
	if v1 != v2 {
		t.Errorf("CanonicalKey: 1 L gives %v, 1000 mL gives %v; want equal values", v1, v2)
	}
	if k1 != "m3" {
		t.Errorf("CanonicalKey(1 L) = %q, want %q", k1, "m3")
	}
}

func TestComparatorWithCustomService(t *testing.T) {
	svc, err := ucum.New()
	if err != nil {
		t.Fatal(err)
	}
	exact, ok := svc.(ucum.ExactService)
	if !ok {
		t.Fatal("Service does not satisfy ExactService")
	}
	c := fhir.NewComparatorWith(exact)
	got, err := c.Compare(fhir.Quantity{Value: 1, Code: "L"}, fhir.Quantity{Value: 1000, Code: "mL"})
	if err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Errorf("Compare(1 L, 1000 mL) = %d, want 0", got)
	}
}

func TestComparableReportsWithoutConverting(t *testing.T) {
	c, err := fhir.NewComparator()
	if err != nil {
		t.Fatal(err)
	}
	ok, err := c.Comparable(fhir.Quantity{Value: 1, Code: "mg/dL"}, fhir.Quantity{Value: 1, Code: "g/L"})
	if err != nil || !ok {
		t.Errorf("Comparable(mg/dL, g/L) = (%v, %v), want (true, nil)", ok, err)
	}
	ok, err = c.Comparable(fhir.Quantity{Value: 1, Code: "cm2"}, fhir.Quantity{Value: 1, Code: "cm"})
	if err != nil || ok {
		t.Errorf("Comparable(cm2, cm) = (%v, %v), want (false, nil)", ok, err)
	}
}

// TestExactDecimalComparison is the reason Quantity has an Exact field. A FHIR
// decimal arrives as text, and the float64 nearest 0.01 is not 1/100, so an
// exact comparison of the float64 reports a difference that the source data does
// not have.
func TestExactDecimalComparison(t *testing.T) {
	c, err := fhir.NewComparator()
	if err != nil {
		t.Fatal(err)
	}
	mgdl := fhir.Quantity{Value: 1, Code: "mg/dL"}

	// Via float64: 0.01 is not exactly 1/100, and the comparison says so.
	viaFloat, err := c.Compare(mgdl, fhir.Quantity{Value: 0.01, Code: "g/L"})
	if err != nil {
		t.Fatal(err)
	}
	if viaFloat == 0 {
		t.Skip("float64 0.01 happened to compare equal; the point below still holds")
	}

	// Via the decimal itself: exactly equal, which is what the data says.
	exact, ok := new(big.Rat).SetString("0.01")
	if !ok {
		t.Fatal("bad literal")
	}
	viaExact, err := c.Compare(mgdl, fhir.Quantity{Exact: exact, Code: "g/L"})
	if err != nil {
		t.Fatal(err)
	}
	if viaExact != 0 {
		t.Errorf("Compare(1 mg/dL, 0.01 g/L) with an exact decimal = %d, want 0", viaExact)
	}
}

// TestExactOverridesValue: when both are set, Exact wins, so a caller cannot get
// a half-converted answer.
func TestExactOverridesValue(t *testing.T) {
	c, err := fhir.NewComparator()
	if err != nil {
		t.Fatal(err)
	}
	q := fhir.Quantity{Value: 999, Exact: big.NewRat(1, 1), Code: "L"}
	got, err := c.Compare(q, fhir.Quantity{Value: 1000, Code: "mL"})
	if err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Errorf("Compare with Exact=1 L against 1000 mL = %d, want 0 (Exact must win over Value)", got)
	}
	key, val, err := c.CanonicalKey(q)
	if err != nil {
		t.Fatal(err)
	}
	if key != "m3" || val != 0.001 {
		t.Errorf("CanonicalKey = (%q, %v), want (\"m3\", 0.001)", key, val)
	}
}

// TestCommonValueSetHasDuplicateCodes records a quirk of the published resource:
// 848 concepts, 840 distinct codes.
func TestCommonValueSetHasDuplicateCodes(t *testing.T) {
	if fhir.CommonUnitsCodeCount != 840 {
		t.Errorf("CommonUnitsCodeCount = %d, want 840", fhir.CommonUnitsCodeCount)
	}
	// Every repeated code is still a member exactly once.
	for _, code := range []string{"/[HPF]", "/[LPF]", "U/10*12", "[beth'U]", "[iU]", "[pptr]", "[todd'U]", "fmol/mg"} {
		if !fhir.InCommonUnits(code) {
			t.Errorf("InCommonUnits(%q) = false, want true", code)
		}
	}
	seen := map[string]bool{}
	for _, code := range fhir.CommonCodes() {
		if seen[code] {
			t.Errorf("CommonCodes returned %q twice", code)
		}
		seen[code] = true
	}
}

func TestIsDefiniteDuration(t *testing.T) {
	// The eight units FHIRPath pairs with a calendar keyword.
	for _, code := range []string{"a", "mo", "wk", "d", "h", "min", "s", "ms"} {
		if !fhir.IsDefiniteDuration(code) {
			t.Errorf("IsDefiniteDuration(%q) = false, want true", code)
		}
	}
	// UCUM has other time units; FHIRPath's table does not name them, and the
	// question is not "is this a unit of time".
	for _, code := range []string{"a_j", "a_g", "a_t", "mo_j", "mo_s", "us", "ns", "kg", "m", "not-a-unit"} {
		if fhir.IsDefiniteDuration(code) {
			t.Errorf("IsDefiniteDuration(%q) = true, want false", code)
		}
	}
}

func TestDecimalFloat64(t *testing.T) {
	d, err := fhir.ParseDecimal("1.50")
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Float64(); got != 1.5 {
		t.Errorf("ParseDecimal(\"1.50\").Float64() = %v, want 1.5", got)
	}
	// A value with no finite decimal form still converts, losing exactness —
	// which is why the doc comment steers callers to String and Rat.
	third := fhir.NewDecimal(big.NewRat(1, 3), 4)
	if got := third.Float64(); got < 0.3333 || got > 0.3334 {
		t.Errorf("NewDecimal(1/3).Float64() = %v, want approximately 0.3333", got)
	}
	// The zero Decimal is usable.
	var zero fhir.Decimal
	if got := zero.Float64(); got != 0 {
		t.Errorf("zero Decimal Float64() = %v, want 0", got)
	}
	if got := zero.String(); got != "0" {
		t.Errorf("zero Decimal String() = %q, want %q", got, "0")
	}
	if got := zero.Rat().Sign(); got != 0 {
		t.Errorf("zero Decimal Rat() sign = %d, want 0", got)
	}
}
