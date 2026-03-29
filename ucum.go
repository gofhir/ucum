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
	Analyse(code string) (string, error)
	Multiply(v1, v2 Pair) (Pair, error)
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
