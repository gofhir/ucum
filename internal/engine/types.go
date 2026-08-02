package engine

import "math/big"

// Pair represents a numeric value with its UCUM unit code.
type Pair struct {
	Value float64
	Code  string
}

// RatPair is a value with its UCUM unit code, held as an exact rational.
type RatPair struct {
	Value *big.Rat
	Code  string
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
