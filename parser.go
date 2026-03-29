package ucum

import (
	"fmt"
	"sort"
	"strings"
)

// parser is a recursive-descent parser for UCUM expressions. It converts a
// UCUM code string into an AST of term/symbol/factor nodes. Symbol resolution
// uses the model's prefixes and units.
//
// This is a port of Java's ExpressionParser.java from FHIR/Ucum-java.
type parser struct {
	model          *UcumModel
	sortedPrefixes []*Prefix // prefixes sorted longest-code-first
}

// newParser creates a parser backed by the given UcumModel.
func newParser(model *UcumModel) *parser {
	// Pre-sort prefixes by descending code length for deterministic
	// longest-match resolution.
	sorted := make([]*Prefix, len(model.Prefixes))
	copy(sorted, model.Prefixes)
	sort.Slice(sorted, func(i, j int) bool {
		return len(sorted[i].Code) > len(sorted[j].Code)
	})
	return &parser{model: model, sortedPrefixes: sorted}
}

// parse parses a UCUM expression string into an AST.
func (p *parser) parse(code string) (*term, error) {
	if code == "" {
		return nil, fmt.Errorf("UCUM expression is empty")
	}

	lex, err := newLexer(code)
	if err != nil {
		return nil, fmt.Errorf("parse %q: %w", code, err)
	}

	t, err := p.parseTerm(lex)
	if err != nil {
		return nil, fmt.Errorf("parse %q: %w", code, err)
	}

	if !lex.finished() {
		return nil, fmt.Errorf("parse %q: unexpected token %q at end of expression", code, lex.getToken())
	}

	return t, nil
}

// parseCompOrAnnotation parses either a component or an annotation (treated as
// factor(1)). It also consumes any trailing annotation after the component.
func (p *parser) parseCompOrAnnotation(lex *lexer) (component, error) {
	if lex.getType() == tokenAnnotation {
		if err := lex.consume(); err != nil {
			return nil, err
		}
		return &factor{value: 1}, nil
	}
	comp, err := p.parseComp(lex)
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
//	term = "/" compOrAnnotation [ ("/" | ".") compOrAnnotation ]*
//	     | compOrAnnotation [ ("/" | ".") compOrAnnotation ]*
//
// Operators are left-associative: a/b/c = (a/b)/c.
func (p *parser) parseTerm(lex *lexer) (*term, error) {
	var result *term

	// Leading "/" -> implicit factor(1) divided by the next comp.
	if lex.getType() == tokenSolidus {
		if err := lex.consume(); err != nil {
			return nil, err
		}
		right, err := p.parseCompOrAnnotation(lex)
		if err != nil {
			return nil, err
		}
		result = &term{
			comp: &factor{value: 1},
			op:   opDivision,
			term: &term{comp: right},
		}
	} else {
		comp, err := p.parseCompOrAnnotation(lex)
		if err != nil {
			return nil, err
		}
		result = &term{comp: comp}
	}

	// Iteratively parse operators for left-associativity.
	// a/b/c -> term(term(a, /, b), /, c)
	for lex.getType() == tokenSolidus || lex.getType() == tokenPeriod {
		var op operator
		if lex.getType() == tokenSolidus {
			op = opDivision
		} else {
			op = opMultiplication
		}
		if err := lex.consume(); err != nil {
			return nil, err
		}

		right, err := p.parseCompOrAnnotation(lex)
		if err != nil {
			return nil, err
		}

		result = &term{
			comp: result,
			op:   op,
			term: &term{comp: right},
		}
	}

	return result, nil
}

// parseComp parses:
//
//	comp = NUMBER
//	     | SYMBOL [NUMBER]
//	     | "(" term ")"
func (p *parser) parseComp(lex *lexer) (component, error) {
	switch lex.getType() {
	case tokenNumber:
		n, err := lex.getTokenAsInt()
		if err != nil {
			return nil, err
		}
		if err := lex.consume(); err != nil {
			return nil, err
		}
		return &factor{value: n}, nil

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
		t, err := p.parseTerm(lex)
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
func (p *parser) parseSymbol(lex *lexer) (*symbol, error) {
	tok := lex.getToken()
	if err := lex.consume(); err != nil {
		return nil, err
	}

	// If the next token is a bracket-symbol (e.g. "[H2O]"), try combining
	// with the current token. The lexer splits "cm[H2O]" into "cm" + "[H2O]"
	// but the unit code is "m[H2O]" with prefix "c".
	bracket := ""
	if lex.getType() == tokenSymbol && len(lex.getToken()) > 0 && lex.getToken()[0] == '[' {
		bracket = lex.getToken()
	}

	// Try prefix + metric unit resolution (longest prefix first).
	for _, pfx := range p.sortedPrefixes {
		if strings.HasPrefix(tok, pfx.Code) && len(pfx.Code) < len(tok) {
			remainder := tok[len(pfx.Code):]
			// Try with bracket suffix first.
			if bracket != "" {
				u := p.model.getUnit(remainder + bracket)
				// For bracket units (e.g. [IU], [iU]), allow metric prefixes
				// even if IsMetric is not set. The UCUM spec permits prefixes
				// on arbitrary units expressed with bracket notation, and the
				// Java reference parser does the same.
				if u != nil && (u.IsMetric || u.IsBase || strings.HasPrefix(remainder+bracket, "[")) {
					// Consume the bracket token.
					if err := lex.consume(); err != nil {
						return nil, err
					}
					exp, err := p.parseExponent(lex)
					if err != nil {
						return nil, err
					}
					return &symbol{unit: u, prefix: pfx, exponent: exp}, nil
				}
			}
			u := p.model.getUnit(remainder)
			if u != nil && (u.IsMetric || u.IsBase) {
				exp, err := p.parseExponent(lex)
				if err != nil {
					return nil, err
				}
				return &symbol{unit: u, prefix: pfx, exponent: exp}, nil
			}
		}
	}

	// If the entire token equals a prefix code and there's a bracket unit
	// following (e.g. token "m" + bracket "[IU]" -> prefix "m" + unit "[IU]"),
	// try matching the bracket alone as the unit with the token as prefix.
	if bracket != "" {
		for _, pfx := range p.sortedPrefixes {
			if tok == pfx.Code {
				u := p.model.getUnit(bracket)
				if u != nil && (u.IsMetric || u.IsBase || strings.HasPrefix(bracket, "[")) {
					if err := lex.consume(); err != nil {
						return nil, err
					}
					exp, err := p.parseExponent(lex)
					if err != nil {
						return nil, err
					}
					return &symbol{unit: u, prefix: pfx, exponent: exp}, nil
				}
			}
		}
	}

	// No prefix match; try full symbol with bracket suffix.
	if bracket != "" {
		u := p.model.getUnit(tok + bracket)
		if u != nil {
			if err := lex.consume(); err != nil {
				return nil, err
			}
			exp, err := p.parseExponent(lex)
			if err != nil {
				return nil, err
			}
			return &symbol{unit: u, exponent: exp}, nil
		}
	}

	// No prefix match; look up the full symbol as a unit.
	u := p.model.getUnit(tok)
	if u != nil {
		exp, err := p.parseExponent(lex)
		if err != nil {
			return nil, err
		}
		return &symbol{unit: u, exponent: exp}, nil
	}

	return nil, fmt.Errorf("unknown unit %q", tok)
}

// parseExponent checks if the next token is a number and, if so, consumes it
// as an exponent. Returns 1 if there is no exponent.
func (p *parser) parseExponent(lex *lexer) (int, error) {
	if lex.getType() == tokenNumber {
		n, err := lex.getTokenAsInt()
		if err != nil {
			return 0, err
		}
		if err := lex.consume(); err != nil {
			return 0, err
		}
		return n, nil
	}
	return 1, nil
}
