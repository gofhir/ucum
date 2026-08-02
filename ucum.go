// Package ucum provides UCUM (Unified Code for Units of Measure) services
// including validation, conversion, and canonical form computation.
//
// The engine, the grammar and the definitions live under internal/. This file is
// the whole public API: two interfaces, three value types, the error types in
// errors.go, and the constructors below.
package ucum

import (
	"io"
	"math/big"

	"github.com/gofhir/ucum/v4/internal/engine"
)

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

// The value types are defined in internal/engine, which produces them, and
// aliased here so that they are one type rather than two.
type (
	// Pair represents a numeric value with its UCUM unit code.
	Pair = engine.Pair

	// RatPair is a value with its UCUM unit code, held as an exact rational.
	RatPair = engine.RatPair

	// Definitions identifies the UCUM release a service was built from, as the
	// definitions themselves declare it.
	Definitions = engine.Definitions
)

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

// ExactService extends Service with exact rational arithmetic, for callers that
// need results free of float64 rounding — currency-style decimal storage,
// comparisons that must not depend on the last bits, or conversion factors that
// are cached and composed.
//
// It is additive: the float64 methods of Service behave the same and are still
// the right choice for the logarithmic and trigonometric scales, which have no
// exact rational form.
//
// The service returned by New and NewFromReader also satisfies this interface,
// so a type assertion covers a Service you already hold, including one built
// from custom definitions:
//
//	svc, err := ucum.NewFromReader(defs)
//	exact := svc.(ucum.ExactService)
type ExactService interface {
	Service

	// ConversionFactor returns the exact factor to multiply a value in `from`
	// by to obtain the value in `to`. It fails with ErrNotLinear if either unit
	// is special, and with an error if the units are not commensurable. The
	// result can be cached and composed by the caller.
	ConversionFactor(from, to string) (*big.Rat, error)

	// ConvertRat converts an exact value between units, including the affine
	// temperature scales. It fails with ErrNotRational when the mapping has no
	// exact rational form.
	ConvertRat(value *big.Rat, from, to string) (*big.Rat, error)

	// CanonicalRat returns the exact canonical (base-unit) form of a value.
	// It fails with ErrNotRational under the same conditions as ConvertRat.
	CanonicalRat(value *big.Rat, code string) (RatPair, error)
}

// New creates a Service using the embedded ucum-essence.xml definitions.
func New() (Service, error) {
	return engine.New(nil, false)
}

// NewFromReader creates a Service loading definitions from a custom source.
func NewFromReader(r io.Reader) (Service, error) {
	return engine.New(r, false)
}

// NewExact creates a Service with the exact rational API, using the embedded
// ucum-essence.xml definitions.
func NewExact() (ExactService, error) {
	return engine.New(nil, false)
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
	return engine.New(nil, true)
}

// NewCaseInsensitiveFromReader creates a case-insensitive Service loading
// definitions from a custom source.
func NewCaseInsensitiveFromReader(r io.Reader) (Service, error) {
	return engine.New(r, true)
}
