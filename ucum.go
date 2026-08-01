// Package ucum provides UCUM (Unified Code for Units of Measure) services
// including validation, conversion, and canonical form computation.
package ucum

import "io"

// Service is the main interface for UCUM operations.
type Service interface {
	Validate(code string) error
	ValidateInProperty(code, property string) error
	Canonical(value float64, code string) (Pair, error)
	Convert(value float64, from, to string) (float64, error)
	IsComparable(code1, code2 string) (bool, error)
	Analyze(code string) (string, error)
	Multiply(v1, v2 Pair) (Pair, error)
	Divide(v1, v2 Pair) (Pair, error)
}

// Definitions identifies the UCUM release a service was built from, as the
// definitions themselves declare it.
type Definitions struct {
	// Version is the UCUM version, such as "2.2".
	Version string

	// Revision is the revision string. The published definitions have carried
	// "N/A" here since UCUM 2.1, so do not rely on it being meaningful.
	Revision string

	// RevisionDate is the release date, such as "2024-06-17".
	RevisionDate string
}

// Identified is implemented by a service that can report which UCUM release it
// was built from. The values returned by New, NewFromReader and NewExact all
// satisfy it:
//
//	svc, _ := ucum.New()
//	defs := svc.(ucum.Identified).Definitions()
//	defs.Version   // "2.2"
//
// It matters for a consumer that has to state which release it implements — a
// FHIR server declaring the version of the UCUM code system it supports, for
// instance — and for one loading definitions through NewFromReader, which cannot
// otherwise tell what it just loaded.
//
// It is a separate interface rather than a method on Service so that adding it
// costs no import-path change: a method on Service would break every
// implementation of that interface and force a new major version on everyone.
type Identified interface {
	Definitions() Definitions
}

// Pair represents a numeric value with its UCUM unit code.
type Pair struct {
	Value float64
	Code  string
}

// New creates a Service using the embedded ucum-essence.xml definitions.
func New() (Service, error) {
	return newService(nil)
}

// NewFromReader creates a Service loading definitions from a custom source.
func NewFromReader(r io.Reader) (Service, error) {
	return newService(r)
}

// NewCaseInsensitive creates a Service that resolves codes in UCUM's
// case-insensitive vocabulary.
//
// UCUM defines a case-insensitive variant of every terminal symbol, "to be used
// when there is a risk of upper and lower case to be confused", and states that
// "case insensitive symbols are incompatible to the case sensitive symbols". They
// are two parallel vocabularies, and a service resolves in one of them only —
// mixing would make codes ambiguous, since "G" is Gauss case-sensitively and gram
// case-insensitively.
//
// Within this variant case carries no meaning, so "MG/DL", "mg/dl" and "Mg/Dl"
// are the same code. Use it for data from a system that cannot preserve case;
// FHIR uses the case-sensitive form, so New is the right choice there.
//
// Canonical forms are reported in case-sensitive codes in both variants, so that
// a canonical form is a stable comparison key regardless of which vocabulary
// produced it.
func NewCaseInsensitive() (Service, error) {
	return newServiceFor(nil, true)
}

// NewCaseInsensitiveFromReader creates a case-insensitive Service loading
// definitions from a custom source.
func NewCaseInsensitiveFromReader(r io.Reader) (Service, error) {
	return newServiceFor(r, true)
}
