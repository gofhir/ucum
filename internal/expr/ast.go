// Package expr is the UCUM grammar: the lexer, the recursive-descent parser and
// the composer that renders an AST back to a code.
//
// It resolves symbols against a model but knows nothing about canonicalizing or
// converting them. The AST it produces is exported because the canonicalizer
// walks it.
package expr

import (
	"github.com/gofhir/ucum/v4/internal/model"
)

// Operator represents a binary Operator in a UCUM expression.
type Operator int

// The two binary operators UCUM defines.
const (
	OpMultiplication Operator = iota
	OpDivision
)

func (o Operator) String() string {
	if o == OpDivision {
		return "/"
	}
	return "."
}

// Component is the interface for AST nodes.
type Component interface {
	isComponent()
}

// Term represents a binary operation: comp op term.
type Term struct {
	Comp Component
	Op   Operator
	Term *Term
}

func (Term) isComponent() {}

// Symbol represents a unit reference with optional prefix and exponent.
type Symbol struct {
	Unit     *model.Unit
	Prefix   *model.Prefix
	Exponent int
}

func (Symbol) isComponent() {}

// Factor represents a numeric literal.
type Factor struct {
	Value int
}

func (Factor) isComponent() {}

// Unwrap looks through terms that hold nothing but a single nested term,
// which is what redundant parentheses produce, and returns the innermost one.
// "((m))" and "m" both come back as the term holding the symbol m.
func Unwrap(t *Term) *Term {
	for t != nil && t.Term == nil {
		inner, ok := t.Comp.(*Term)
		if !ok {
			return t
		}
		t = inner
	}
	return t
}

// LoneSymbol returns the single symbol a term denotes, looking through any
// number of redundant parentheses, and nil if the term is anything else.
func LoneSymbol(t *Term) *Symbol {
	t = Unwrap(t)
	if t == nil || t.Term != nil {
		return nil
	}
	sym, ok := t.Comp.(*Symbol)
	if !ok {
		return nil
	}
	return sym
}
