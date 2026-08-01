package fhir

// CalendarEquivalent describes how a UCUM time unit relates to the FHIRPath
// calendar duration keyword of the same magnitude.
type CalendarEquivalent struct {
	// Keyword is the FHIRPath calendar duration keyword, singular.
	Keyword string

	// Equal reports whether the relationship is equality rather than mere
	// equivalence. FHIRPath writes '=' for second and millisecond, whose length
	// is fixed, and '~' for everything above, whose length varies with the
	// calendar.
	Equal bool
}

// calendarEquivalents is the table in FHIRPath N1, "Time-valued Quantities":
//
//	year / years         'year'        ~ 1 'a'
//	month / months       'month'       ~ 1 'mo'
//	week / weeks         'week'        ~ 1 'wk'
//	day / days           'day'         ~ 1 'd'
//	hour / hours         'hour'        ~ 1 'h'
//	minute / minutes     'minute'      ~ 1 'min'
//	second / seconds     'second'      = 1 's'
//	millisecond / ms     'millisecond' = 1 'ms'
var calendarEquivalents = map[string]CalendarEquivalent{
	"a":   {Keyword: "year", Equal: false},
	"mo":  {Keyword: "month", Equal: false},
	"wk":  {Keyword: "week", Equal: false},
	"d":   {Keyword: "day", Equal: false},
	"h":   {Keyword: "hour", Equal: false},
	"min": {Keyword: "minute", Equal: false},
	"s":   {Keyword: "second", Equal: true},
	"ms":  {Keyword: "millisecond", Equal: true},
}

// CalendarEquivalentOf returns the FHIRPath calendar duration keyword
// corresponding to a UCUM time unit, and whether the correspondence is equality
// or only equivalence.
//
// The distinction is the whole point. A UCUM year is a definite duration of
// exactly 365.25 days; a calendar year is 365 or 366. So `1 'a'` and `1 year`
// are equivalent (~) but not equal (=), and code that treats them as
// interchangeable will be wrong by a day roughly one year in four. Only second
// and millisecond are genuinely equal.
//
// Note a discrepancy worth knowing about: FHIR R5 states that the UCUM
// definitions of a and mo are "365.25 and 30 days respectively". UCUM defines mo
// as mo_j, a twelfth of a Julian year, which is 30.4375 days — and that is what
// the parent package computes for Convert(1, "mo", "d"). The 30-day figure in
// the FHIR prose does not match UCUM, and this package does not adopt it: UCUM
// governs UCUM arithmetic.
func CalendarEquivalentOf(code string) (CalendarEquivalent, bool) {
	eq, ok := calendarEquivalents[code]
	return eq, ok
}

// fhirQuantityMapping is the table in FHIR R5, "Using FHIRPath with FHIR": the
// implicit conversion applied when a FHIR Quantity becomes a FHIRPath
// System.Quantity.
//
//	a -> year, mo -> month, d -> day, h -> hour, min -> minute, s -> second
//
// It is a subset of the FHIRPath table above — it omits wk and ms — so the two
// are kept separate rather than merged.
var fhirQuantityMapping = map[string]string{
	"a":   "year",
	"mo":  "month",
	"d":   "day",
	"h":   "hour",
	"min": "minute",
	"s":   "second",
}

// MappedByFHIRQuantityConversion reports whether FHIR implicitly converts this
// UCUM code to a calendar duration when a Quantity is evaluated as a FHIRPath
// System.Quantity, and to which keyword.
//
// This matters when reading FHIR data: a Quantity of 1 'a' does not stay a
// definite duration through that conversion, it becomes 1 'year'. The set is
// narrower than CalendarEquivalentOf's, which describes the FHIRPath type
// system rather than the FHIR mapping.
func MappedByFHIRQuantityConversion(code string) (string, bool) {
	keyword, ok := fhirQuantityMapping[code]
	return keyword, ok
}

// AllowedInDateTimeArithmetic reports whether a definite-duration UCUM unit may
// take part in FHIRPath date/time arithmetic.
//
// FHIRPath N1, "Date/Time Arithmetic", defines definite-duration arithmetic for
// seconds and below and calendar-based arithmetic for seconds and above, and
// says that if a definite-duration quantity above seconds appears in a date/time
// calculation "the evaluation will end and signal an error to the calling
// environment".
//
// So 's' and 'ms' are allowed, the coarser time units are not, and a unit that
// is not a duration at all is not applicable — this returns false for it, since
// it cannot lawfully appear in date/time arithmetic either.
func AllowedInDateTimeArithmetic(code string) bool {
	eq, ok := calendarEquivalents[code]
	return ok && eq.Equal
}

// IsDefiniteDuration reports whether code is one of the UCUM time units that
// FHIRPath pairs with a calendar duration keyword.
//
// It answers "is this a duration FHIRPath has an opinion about" rather than "is
// this a unit of time". UCUM has many more time units — a_j, a_g, mo_s and so on
// — and FHIRPath's table names only these eight.
func IsDefiniteDuration(code string) bool {
	_, ok := calendarEquivalents[code]
	return ok
}
