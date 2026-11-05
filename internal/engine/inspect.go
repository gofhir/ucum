package engine

import (
	"github.com/gofhir/ucum/v4/internal/expr"
	"github.com/gofhir/ucum/v4/internal/ucumerr"
)

// Symbol describes one resolved unit occurrence. Codes use the case-sensitive
// vocabulary of the loaded definitions, even for case-insensitive input.
type Symbol struct {
	Code   string // The atom code, without its prefix or exponent.
	Prefix string // The resolved prefix code, or empty for none.

	// Exponent includes the sign contributed by enclosing divisions. Separate
	// occurrences remain separate, including zero powers and canceling pairs.
	Exponent int

	// Definition is the unit expression defining an ordinary derived atom,
	// without its numeric multiplier. It is empty for base and special units.
	// This is a declaration, not a canonical form or a conversion coefficient.
	Definition string

	IsBase      bool
	IsSpecial   bool
	IsArbitrary bool
}

// Inspect returns a fresh snapshot of resolved unit occurrences in source
// order. It follows Validate's syntax and resource limits, returning no partial
// result on error. It does not canonicalize, expand definitions, combine terms,
// or determine whether a numeric conversion or a consumer's policy is valid.
// Numeric factors and annotations contribute no symbols; the result therefore
// cannot reconstruct the original expression. No mutable cache data is exposed.
func (s *Service) Inspect(code string) ([]Symbol, error) {
	t, err := s.parseCached(code)
	if err != nil {
		return nil, ucumerr.Validation(code, err)
	}
	return appendSymbols(nil, t, 1), nil
}

func appendSymbols(out []Symbol, t *expr.Term, sign int) []Symbol {
	for t != nil {
		switch c := t.Comp.(type) {
		case *expr.Term:
			out = appendSymbols(out, c, sign)
		case *expr.Symbol:
			symbol := Symbol{
				Code: c.Unit.Code, Exponent: sign * c.Exponent,
				IsBase: c.Unit.IsBase, IsSpecial: c.Unit.IsSpecial,
				IsArbitrary: c.Unit.IsArbitrary,
			}
			if c.Unit.Value != nil && !c.Unit.IsSpecial {
				symbol.Definition = c.Unit.Value.Unit
			}
			if c.Prefix != nil {
				symbol.Prefix = c.Prefix.Code
			}
			out = append(out, symbol)
		}
		if t.Op == expr.OpDivision {
			sign = -sign
		}
		t = t.Term
	}
	return out
}
