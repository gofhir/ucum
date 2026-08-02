package engine

import (
	"fmt"
	"strings"

	"github.com/gofhir/ucum/v4/internal/expr"
	"github.com/gofhir/ucum/v4/internal/ucumerr"
)

// Analyze and the display format of the official UCUM test suite.

// Analyze returns a human-readable description of the unit expression, in the
// display format of the official UCUM test suite: each unit parenthesized with
// its full name, exponents written as " ^ n", and operators as " * " and " / ".
//
// An empty expression describes the unity, matching the suite, even though
// Validate rejects it as a code.
func (s *Service) Analyze(code string) (string, error) {
	if strings.TrimSpace(code) == "" {
		return unityDisplayName, nil
	}
	t, err := s.parseCached(code)
	if err != nil {
		return "", ucumerr.Validation(code, err)
	}
	return displayName(t), nil
}

// displayName renders a term in the display format of the official UCUM test
// suite: every unit is parenthesized with its full name, a prefix is
// concatenated onto that name ("mm" is "(millimeter)"), an exponent other than
// 1 is written inside the parentheses as " ^ n", numeric factors appear bare,
// and the operators are " * " and " / ".
func displayName(t *expr.Term) string {
	if t == nil {
		return unityDisplayName
	}
	var sb strings.Builder
	displayTermTo(&sb, t, false)
	return sb.String()
}
func displayTermTo(sb *strings.Builder, t *expr.Term, rightOperand bool) {
	displayComponentTo(sb, t.Comp, rightOperand)
	if t.Term != nil {
		if t.Op == expr.OpDivision {
			sb.WriteString(" / ")
		} else {
			sb.WriteString(" * ")
		}
		displayTermTo(sb, t.Term, true)
	}
}
func displayComponentTo(sb *strings.Builder, c expr.Component, rightOperand bool) {
	switch v := c.(type) {
	case *expr.Factor:
		fmt.Fprintf(sb, "%d", v.Value)
	case *expr.Symbol:
		sb.WriteString("(")
		if v.Prefix != nil {
			sb.WriteString(v.Prefix.Name)
		}
		sb.WriteString(v.Unit.Name)
		if v.Exponent != 1 {
			fmt.Fprintf(sb, " ^ %d", v.Exponent)
		}
		sb.WriteString(")")
	case *expr.Term:
		// A group on the right of an expr.Operator keeps its brackets, for the reason
		// given in composeComponentTo: without them the description of
		// "mL/(kg.min)" reads as "mL/kg.min", which denotes a different unit.
		// The left operand of a chain needs none, since the operators are
		// left-associative.
		// Redundant parentheses are looked through first, so that "((a.b))" is
		// recognized as carrying an expr.Operator and "((m))" as not.
		inner := expr.Unwrap(v)
		parenthesize := rightOperand && inner.Term != nil
		if parenthesize {
			sb.WriteString("[")
		}
		displayTermTo(sb, inner, false)
		if parenthesize {
			sb.WriteString("]")
		}
	}
}
