package ucum

import (
	"errors"
	"fmt"
)

// errDivisionByZero is returned when a unit expression divides by a zero
// factor, as in "m/0". Such codes parse successfully, so they have to be
// rejected during canonicalization.
var errDivisionByZero = errors.New("division by zero in unit expression")

// ValidationError indicates an invalid UCUM code.
type ValidationError struct {
	Code    string
	Message string
	Offset  int
}

func (e *ValidationError) Error() string {
	if e.Offset >= 0 {
		return fmt.Sprintf("invalid UCUM code %q at position %d: %s", e.Code, e.Offset, e.Message)
	}
	return fmt.Sprintf("invalid UCUM code %q: %s", e.Code, e.Message)
}

// ConversionError indicates a failed unit conversion.
type ConversionError struct {
	From    string
	To      string
	Message string
}

func (e *ConversionError) Error() string {
	return fmt.Sprintf("cannot convert %q to %q: %s", e.From, e.To, e.Message)
}
