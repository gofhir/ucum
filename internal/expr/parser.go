// Portions of this file are ported from ExpressionParser.java in FHIR/Ucum-java,
// which is licensed under the BSD 3-Clause License:
//
//	Copyright (c) 2006+, Health Intersections Pty Ltd
//	All rights reserved.
//
// See LICENSE for the full text and NOTICE for the provenance of every
// third-party Component.

package expr

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gofhir/ucum/v4/internal/model"

	"github.com/gofhir/ucum/v4/internal/ucumerr"
)

// Parser is a recursive-descent parser for UCUM expressions. It converts a
// UCUM code string into an AST of term/symbol/factor nodes. Symbol resolution
// uses the model's prefixes and units.
//
// This is a port of Java's ExpressionParser.java from FHIR/Ucum-java.
type Parser struct {
	model          *model.Model
	sortedPrefixes []*model.Prefix // prefixes sorted longest-code-first
	insensitive    bool            // resolve against the case-insensitive vocabulary
}

// NewParser creates a parser backed by the given model.Model, resolving codes in
// the case-sensitive vocabulary.
func NewParser(m *model.Model) *Parser {
	return NewParserFor(m, false)
}

// NewParserFor creates a parser for one of UCUM's two vocabularies. They are
// never mixed: the specification calls them incompatible, and the same string can
// mean different units in each — "G" is Gauss case-sensitively and gram
// case-insensitively.
func NewParserFor(m *model.Model, insensitive bool) *Parser {
	p := &Parser{model: m, insensitive: insensitive}

	// Pre-sort prefixes by descending code length for deterministic
	// longest-match resolution. The lengths differ between vocabularies — giga is
	// "G" in one and "GA" in the other — so the order follows the active one.
	sorted := make([]*model.Prefix, len(m.Prefixes))
	copy(sorted, m.Prefixes)
	sort.Slice(sorted, func(i, j int) bool {
		return len(p.prefixCode(sorted[i])) > len(p.prefixCode(sorted[j]))
	})
	p.sortedPrefixes = sorted
	return p
}

// lookupUnit resolves a unit code in the parser's vocabulary.
func (p *Parser) lookupUnit(code string) *model.Unit {
	if p.insensitive {
		return p.model.LookupCI(code)
	}
	return p.model.Lookup(code)
}

// prefixCode returns a prefix's spelling in the parser's vocabulary.
func (p *Parser) prefixCode(pfx *model.Prefix) string {
	if p.insensitive {
		return pfx.CodeCI
	}
	return pfx.Code
}

// startsWithPrefix reports whether tok begins with the prefix's code, ignoring
// case in the case-insensitive vocabulary, where case carries no meaning.
func (p *Parser) startsWithPrefix(tok string, pfx *model.Prefix) bool {
	code := p.prefixCode(pfx)
	if code == "" || len(tok) <= len(code) {
		return false
	}
	if p.insensitive {
		return strings.EqualFold(tok[:len(code)], code)
	}
	return strings.HasPrefix(tok, code)
}

// isPrefixCode reports whether tok is exactly the prefix's code.
func (p *Parser) isPrefixCode(tok string, pfx *model.Prefix) bool {
	code := p.prefixCode(pfx)
	if code == "" {
		return false
	}
	if p.insensitive {
		return strings.EqualFold(tok, code)
	}
	return tok == code
}

// Parse parses a UCUM expression string into an AST.
func (p *Parser) Parse(code string) (*Term, error) {
	if code == "" {
		return nil, fmt.Errorf("UCUM expression is empty")
	}
	if len(code) > ucumerr.MaxCodeLength {
		return nil, fmt.Errorf("parse: %w: %d bytes, the limit is %d",
			ucumerr.ErrCodeTooLong, len(code), ucumerr.MaxCodeLength)
	}

	lex, err := newLexer(code)
	if err != nil {
		return nil, fmt.Errorf("parse %q: %w", code, err)
	}

	// The lexer reports its own position; a parser error is attributed to the
	// token the parser was looking at when it gave up.
	t, err := p.parseTerm(lex, 0)
	if err != nil {
		return nil, fmt.Errorf("parse %q: %w", code, ucumerr.WithPosition(err, lex.position()))
	}

	if !lex.finished() {
		return nil, fmt.Errorf("parse %q: %w", code, ucumerr.At(lex.position(), "unexpected token %q at end of expression", lex.getToken()))
	}

	return t, nil
}

// parseCompOrAnnotation parses either a Component or an annotation (treated as
// factor(1)). It also consumes any trailing annotation after the Component.
func (p *Parser) parseCompOrAnnotation(lex *lexer, depth int) (Component, error) {
	if lex.getType() == tokenAnnotation {
		if err := lex.consume(); err != nil {
			return nil, err
		}
		return &Factor{Value: 1}, nil
	}
	comp, err := p.parseComp(lex, depth)
	if err != nil {
		return nil, err
	}
	// Consume optional trailing annotation (e.g. "m{annotation}").
	if lex.getType() == tokenAnnotation {
		if err := lex.consume(); err != nil {
			return nil, err
		}
	}
	return comp, nil
}

// parseTerm parses:
//
//	Term = "/" compOrAnnotation [ ("/" | ".") compOrAnnotation ]*
//	     | compOrAnnotation [ ("/" | ".") compOrAnnotation ]*
//
// Operators are left-associative: a/b/c = (a/b)/c.
func (p *Parser) parseTerm(lex *lexer, depth int) (*Term, error) {
	// Every level of nesting is a frame in this parser and another in every
	// walk over the AST it produces. Canonicalization recurses over both, and a
	// Go stack overflow is a fatal error rather than a panic, so the bound has
	// to be here, before the AST exists.
	if depth > ucumerr.MaxNestingDepth {
		return nil, fmt.Errorf("%w: more than %d levels", ucumerr.ErrCodeTooComplex, ucumerr.MaxNestingDepth)
	}

	var result *Term

	// Leading "/" -> implicit factor(1) divided by the next comp.
	if lex.getType() == tokenSolidus {
		if err := lex.consume(); err != nil {
			return nil, err
		}
		right, err := p.parseCompOrAnnotation(lex, depth)
		if err != nil {
			return nil, err
		}
		result = &Term{
			Comp: &Factor{Value: 1},
			Op:   OpDivision,
			Term: &Term{Comp: right},
		}
	} else {
		comp, err := p.parseCompOrAnnotation(lex, depth)
		if err != nil {
			return nil, err
		}
		result = &Term{Comp: comp}
	}

	// Iteratively parse operators for left-associativity.
	// a/b/c -> term(term(a, /, b), /, c)
	for lex.getType() == tokenSolidus || lex.getType() == tokenPeriod {
		var op Operator
		if lex.getType() == tokenSolidus {
			op = OpDivision
		} else {
			op = OpMultiplication
		}
		if err := lex.consume(); err != nil {
			return nil, err
		}

		right, err := p.parseCompOrAnnotation(lex, depth)
		if err != nil {
			return nil, err
		}

		result = &Term{
			Comp: result,
			Op:   op,
			Term: &Term{Comp: right},
		}
	}

	return result, nil
}

// parseComp parses:
//
//	comp = NUMBER
//	     | SYMBOL [NUMBER]
//	     | "(" term ")"
func (p *Parser) parseComp(lex *lexer, depth int) (Component, error) {
	switch lex.getType() {
	case tokenNumber:
		n, err := lex.getTokenAsInt()
		if err != nil {
			return nil, err
		}
		if err := lex.consume(); err != nil {
			return nil, err
		}
		return &Factor{Value: n}, nil

	case tokenSymbol:
		sym, err := p.parseSymbol(lex)
		if err != nil {
			return nil, err
		}
		return sym, nil

	case tokenOpen:
		if err := lex.consume(); err != nil {
			return nil, err
		}
		t, err := p.parseTerm(lex, depth+1)
		if err != nil {
			return nil, err
		}
		if lex.getType() != tokenClose {
			return nil, fmt.Errorf("expected ')' but got %s", lex.getType())
		}
		if err := lex.consume(); err != nil {
			return nil, err
		}
		return t, nil

	case tokenNone:
		return nil, fmt.Errorf("unexpected end of expression")

	default:
		return nil, fmt.Errorf("unexpected token %q (%s)", lex.getToken(), lex.getType())
	}
}

// parseSymbol resolves a symbol token into a symbol AST node with optional
// prefix and exponent.
func (p *Parser) parseSymbol(lex *lexer) (*Symbol, error) {
	tok := lex.getToken()
	// Where the symbol starts, kept because resolving it consumes the token and
	// an "unknown unit" error has to point at the unit, not past it.
	start := lex.position()
	if err := lex.consume(); err != nil {
		return nil, err
	}

	// If the next token is a bracket-symbol (e.g. "[H2O]"), try combining
	// with the current token. The lexer splits "cm[H2O]" into "cm" + "[H2O]"
	// but the unit code is "m[H2O]" with prefix "c".
	bracket := ""
	if lex.getType() == tokenSymbol && lex.getToken() != "" && lex.getToken()[0] == '[' {
		bracket = lex.getToken()
	}

	// Try prefix + metric unit resolution (longest prefix first).
	if sym, err := p.resolveWithPrefix(lex, tok, bracket); sym != nil || err != nil {
		return sym, err
	}

	// If the entire token equals a prefix code and there's a bracket unit
	// following, try matching the bracket alone as the unit with the token
	// as prefix.
	if sym, err := p.resolveExactPrefixBracket(lex, tok, bracket); sym != nil || err != nil {
		return sym, err
	}

	// No prefix match; try full symbol with bracket suffix.
	if sym, err := p.resolveFullWithBracket(lex, tok, bracket); sym != nil || err != nil {
		return sym, err
	}

	// No prefix match; look up the full symbol as a unit.
	u := p.lookupUnit(tok)
	if u != nil {
		exp, err := p.parseExponent(lex)
		if err != nil {
			return nil, err
		}
		return &Symbol{Unit: u, Exponent: exp}, nil
	}

	return nil, ucumerr.At(start, "unknown unit %q", tok)
}

// resolveWithPrefix tries to match a prefix from the token and resolve the
// remainder as a unit, optionally combined with a bracket suffix.
func (p *Parser) resolveWithPrefix(lex *lexer, tok, bracket string) (*Symbol, error) {
	for _, pfx := range p.sortedPrefixes {
		if !p.startsWithPrefix(tok, pfx) {
			continue
		}
		remainder := tok[len(p.prefixCode(pfx)):]

		// Try with bracket suffix first.
		if bracket != "" {
			u := p.lookupUnit(remainder + bracket)
			// UCUM §11 ■1: only metric unit atoms may be combined with a
			// prefix. Bracket notation makes no difference — [IU] and [iU] take
			// prefixes because they are declared isMetric="yes", while [ft_i]
			// and [lb_av] are not and cannot.
			if u != nil && (u.IsMetric || u.IsBase) {
				if err := lex.consume(); err != nil {
					return nil, err
				}
				exp, err := p.parseExponent(lex)
				if err != nil {
					return nil, err
				}
				return &Symbol{Unit: u, Prefix: pfx, Exponent: exp}, nil
			}
		}

		u := p.lookupUnit(remainder)
		if u != nil && (u.IsMetric || u.IsBase) {
			exp, err := p.parseExponent(lex)
			if err != nil {
				return nil, err
			}
			return &Symbol{Unit: u, Prefix: pfx, Exponent: exp}, nil
		}
	}
	return nil, nil
}

// resolveExactPrefixBracket handles the case where the entire token is a prefix
// code and the bracket token is the unit (e.g. token "m" + bracket "[IU]").
func (p *Parser) resolveExactPrefixBracket(lex *lexer, tok, bracket string) (*Symbol, error) {
	if bracket == "" {
		return nil, nil
	}
	for _, pfx := range p.sortedPrefixes {
		if !p.isPrefixCode(tok, pfx) {
			continue
		}
		u := p.lookupUnit(bracket)
		// UCUM §11 ■1, as above.
		if u != nil && (u.IsMetric || u.IsBase) {
			if err := lex.consume(); err != nil {
				return nil, err
			}
			exp, err := p.parseExponent(lex)
			if err != nil {
				return nil, err
			}
			return &Symbol{Unit: u, Prefix: pfx, Exponent: exp}, nil
		}
	}
	return nil, nil
}

// resolveFullWithBracket tries to resolve the full token combined with a
// bracket suffix as a single unit code.
func (p *Parser) resolveFullWithBracket(lex *lexer, tok, bracket string) (*Symbol, error) {
	if bracket == "" {
		return nil, nil
	}
	u := p.lookupUnit(tok + bracket)
	if u == nil {
		return nil, nil
	}
	if err := lex.consume(); err != nil {
		return nil, err
	}
	exp, err := p.parseExponent(lex)
	if err != nil {
		return nil, err
	}
	return &Symbol{Unit: u, Exponent: exp}, nil
}

// parseExponent checks if the next token is a number and, if so, consumes it
// as an exponent. Returns 1 if there is no exponent.
func (p *Parser) parseExponent(lex *lexer) (int, error) {
	if lex.getType() != tokenNumber {
		return 1, nil
	}
	n, err := lex.getTokenAsInt()
	if err != nil {
		return 0, err
	}
	// The value a unit raises to this power grows with it — "k2000000000" is a
	// power of ten with billions of digits — so it is bounded here rather than
	// left to exhaust memory later.
	if n > ucumerr.MaxExponent || n < -ucumerr.MaxExponent {
		return 0, ucumerr.AtCause(lex.position(), ucumerr.ErrExponentTooLarge,
			"%v: %d, the limit is ±%d", ucumerr.ErrExponentTooLarge, n, ucumerr.MaxExponent)
	}
	if err := lex.consume(); err != nil {
		return 0, err
	}
	return n, nil
}
