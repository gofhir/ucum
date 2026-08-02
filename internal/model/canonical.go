package model

import "github.com/gofhir/ucum/v4/internal/decimal"

// Canonical represents a value expressed in canonical (base) units.
type Canonical struct {
	Value decimal.Decimal
	Units []CanonicalUnit
}

// CanonicalUnit pairs a base unit with its exponent in a canonical form.
type CanonicalUnit struct {
	Base     *BaseUnit
	Exponent int
}
