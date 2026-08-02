package ucum

import (
	"github.com/gofhir/ucum/v4/internal/engine"
	"github.com/gofhir/ucum/v4/internal/ucumerr"
)

// The error types and sentinels live in internal/ucumerr so that the lexer, the
// parser and the engine can construct them without importing this package, which
// would be a cycle. They are re-exported here because they are part of the public
// API, and the re-export is exact:
//
//   - The types are aliases, not new types, so errors.As(err, **ucum.ValidationError)
//     matches an error a lower layer built.
//   - The sentinels are the same interface values, so errors.Is matches whichever
//     name a caller reaches for.
type (
	// ValidationError indicates an invalid UCUM code. Offset is the byte
	// position in Code at which the problem was found, or -1 when the failure
	// has no single position.
	ValidationError = ucumerr.ValidationError

	// ConversionError indicates a failed unit conversion between two codes.
	ConversionError = ucumerr.ConversionError
)

var (
	// ErrDivisionByZero is returned when a unit expression divides by a zero
	// factor, as in "m/0", or when a quantity with a zero value is used as a
	// divisor.
	//
	// Such codes parse successfully — a zero factor is well-formed UCUM — so
	// they are rejected during canonicalization instead, which means the error
	// can come out of any method that canonicalizes. Match it with errors.Is; it
	// survives the wrapping in ValidationError and ConversionError:
	//
	//	if errors.Is(err, ucum.ErrDivisionByZero) { ... }
	ErrDivisionByZero = ucumerr.ErrDivisionByZero

	// ErrCodeTooLong is returned for a code longer than MaxCodeLength.
	ErrCodeTooLong = ucumerr.ErrCodeTooLong

	// ErrCodeTooComplex is returned for a code nested deeper than
	// MaxNestingDepth.
	ErrCodeTooComplex = ucumerr.ErrCodeTooComplex

	// ErrExponentTooLarge is returned for an exponent whose magnitude exceeds
	// MaxExponent.
	ErrExponentTooLarge = ucumerr.ErrExponentTooLarge

	// ErrNotLinear is returned by ConversionFactor when one of the units sits on
	// a non-ratio scale. Between Cel and [degF] the relation is affine
	// (Cel = ([degF] - 32) * 5/9), so no single multiplicative factor describes
	// it; use ConvertRat instead.
	ErrNotLinear = engine.ErrNotLinear

	// ErrNotRational is returned by ConvertRat and CanonicalRat when a special
	// unit's mapping is not a rational function — logarithmic ([pH], B, Np,
	// bit_s, [hp'_X]), trigonometric ([p'diop], %[slope]) or square root. Their
	// results are irrational in general, so they cannot be represented exactly
	// as a *big.Rat. Use Convert or Canonical for those.
	ErrNotRational = engine.ErrNotRational

	// ErrNilValue is returned when a *big.Rat argument is nil.
	ErrNilValue = engine.ErrNilValue
)

// MaxCacheEntries is how many parsed codes a service keeps per cache generation.
//
// A service caches the codes it is given so that repeating one is cheap, and what
// it is given is not under its control: an annotation is free text, so
// "mg/dL{lot17}" is a valid code and there are unboundedly many of them. The
// bound is generous — a deployment sees tens of distinct codes, not thousands —
// and eviction costs a reparse, not an error.
const MaxCacheEntries = engine.MaxCacheEntries

// Bounds on the size of a code this package will parse.
//
// UCUM states no such bounds, because it describes a notation rather than an
// implementation. An implementation that takes codes from the network needs them
// anyway: without them a short string is enough to exhaust the process. "m2e9"
// spent minutes building an integer of billions of digits, and two hundred nested
// parentheses crashed canonicalization with a stack overflow, which recover
// cannot catch.
//
// They are set far above anything real. The longest code in the official suite is
// 17 bytes, the longest in the FHIR ucum-common value set is 21, the deepest
// nesting in either is one, and no definition in ucum-essence.xml uses an
// exponent outside [-4, 4].
const (
	// MaxCodeLength is the longest code accepted, in bytes. It also bounds the
	// depth of an expression written without parentheses, since each level costs
	// at least one byte.
	MaxCodeLength = ucumerr.MaxCodeLength

	// MaxNestingDepth is the deepest parenthesis nesting accepted.
	MaxNestingDepth = ucumerr.MaxNestingDepth

	// MaxExponent is the largest exponent magnitude accepted.
	MaxExponent = ucumerr.MaxExponent
)
