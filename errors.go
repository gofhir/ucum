package ucum

import (
	"errors"
	"fmt"
)

// ErrDivisionByZero is returned when a unit expression divides by a zero
// factor, as in "m/0", or when a quantity with a zero value is used as a
// divisor.
//
// Such codes parse successfully — a zero factor is well-formed UCUM — so they
// are rejected during canonicalization instead, which means the error can come
// out of any method that canonicalizes. Match it with errors.Is; it survives
// the wrapping in ValidationError and ConversionError:
//
//	if errors.Is(err, ucum.ErrDivisionByZero) { ... }
var ErrDivisionByZero = errors.New("division by zero in unit expression")

// ValidationError indicates an invalid UCUM code.
type ValidationError struct {
	// Code is the code that failed.
	Code string

	// Message describes the failure.
	Message string

	// Offset is the byte position in Code at which the problem was found, or -1
	// when the failure has no single position — a well-formed code that measures
	// the wrong property, for instance.
	Offset int

	// Err is the underlying cause, if any. It is what errors.Is and errors.As
	// see through.
	Err error
}

func (e *ValidationError) Error() string {
	if e.Offset >= 0 {
		return fmt.Sprintf("invalid UCUM code %q at position %d: %s", e.Code, e.Offset, e.Message)
	}
	return fmt.Sprintf("invalid UCUM code %q: %s", e.Code, e.Message)
}

// Unwrap returns the underlying cause.
func (e *ValidationError) Unwrap() error { return e.Err }

// ConversionError indicates a failed unit conversion.
type ConversionError struct {
	// From and To are the codes of the conversion that failed.
	From string
	To   string

	// Message describes the failure.
	Message string

	// Err is the underlying cause, if any. It is what errors.Is and errors.As
	// see through.
	Err error
}

func (e *ConversionError) Error() string {
	return fmt.Sprintf("cannot convert %q to %q: %s", e.From, e.To, e.Message)
}

// Unwrap returns the underlying cause.
func (e *ConversionError) Unwrap() error { return e.Err }

// posError carries the position at which a code failed to lex or parse, so that
// ValidationError can report it as a number instead of leaving it buried in
// prose.
type posError struct {
	offset int
	msg    string
}

func (e *posError) Error() string { return e.msg }

// withPosition attaches a position to an error that does not already carry one.
// The innermost position wins: the lexer knows exactly where it stopped, while
// the parser only knows which token it was looking at.
func withPosition(err error, offset int) error {
	if err == nil {
		return nil
	}
	var pe *posError
	if errors.As(err, &pe) {
		return err
	}
	return &posError{offset: offset, msg: err.Error()}
}

// offsetOf returns the position an error carries, or -1 if it carries none.
func offsetOf(err error) int {
	var pe *posError
	if errors.As(err, &pe) {
		return pe.offset
	}
	return -1
}

// validationError wraps an internal error as a ValidationError for the code it
// came from, keeping the cause reachable through errors.Is. An error that is
// already a ValidationError is returned unchanged, so an inner code is not
// renamed by an outer call.
func validationError(code string, err error) error {
	if err == nil {
		return nil
	}
	var ve *ValidationError
	if errors.As(err, &ve) {
		return err
	}
	return &ValidationError{
		Code:    code,
		Message: err.Error(),
		Offset:  offsetOf(err),
		Err:     err,
	}
}

// conversionError wraps an internal error as a ConversionError between two
// codes. An error that is already a ConversionError is returned unchanged.
func conversionError(from, to string, err error) error {
	if err == nil {
		return nil
	}
	var ce *ConversionError
	if errors.As(err, &ce) {
		return err
	}
	return &ConversionError{From: from, To: to, Message: err.Error(), Err: err}
}
