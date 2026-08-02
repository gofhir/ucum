package ucum

import (
	"fmt"
	"sort"
	"strings"
)

// composeTerm serializes an AST term back to a UCUM string.
func composeTerm(t *term) string {
	if t == nil {
		return ""
	}

	var sb strings.Builder
	composeTermTo(&sb, t, false)
	return sb.String()
}

// composeTermTo writes the UCUM string for a term into the builder. The
// rightOperand flag says whether this term sits on the right of an operator,
// which is where a nested group has to keep its parentheses.
func composeTermTo(sb *strings.Builder, t *term, rightOperand bool) {
	composeComponentTo(sb, t.comp, rightOperand)

	if t.term != nil {
		sb.WriteString(t.op.String())
		composeTermTo(sb, t.term, true)
	}
}

// composeComponentTo writes the UCUM string for a single component.
//
// A nested term that carries an operator is parenthesized when it is a right
// operand, and only then. UCUM operators are left-associative, so the left
// operand of a chain needs no brackets — "a/b/c" already means "(a/b)/c" — but
// the right one does: dropping them turns "kg/(s.m2)" into "kg/s.m2", which is
// kg.m2.s-1 rather than kg.m-2.s-1. Those are real codes; "mL/(kg.min)" is a
// pediatric dose rate and "mL/min/(173.10*-2.m2)" is in the FHIR value set.
func composeComponentTo(sb *strings.Builder, c component, rightOperand bool) {
	switch v := c.(type) {
	case *factor:
		fmt.Fprintf(sb, "%d", v.value)
	case *symbol:
		if v.prefix != nil {
			sb.WriteString(v.prefix.Code)
		}
		sb.WriteString(v.unit.Code)
		if v.exponent != 1 {
			fmt.Fprintf(sb, "%d", v.exponent)
		}
	case *term:
		// Redundant parentheses are looked through first, so that "((a.b))" is
		// recognized as carrying an operator and "((m))" as not.
		inner := unwrapTerm(v)
		parenthesize := rightOperand && inner.term != nil
		if parenthesize {
			sb.WriteString("(")
		}
		composeTermTo(sb, inner, false)
		if parenthesize {
			sb.WriteString(")")
		}
	}
}

// composeCanonicalUnits serializes canonical units to a UCUM string.
// Example: [{m,1},{s,-1}] produces "m.s-1".
func composeCanonicalUnits(c *canonical) string {
	if c == nil || len(c.units) == 0 {
		return "1"
	}

	var parts []string
	for _, u := range c.units {
		if u.exponent == 0 {
			continue
		}
		s := u.base.Code
		if u.exponent != 1 {
			s += fmt.Sprintf("%d", u.exponent)
		}
		parts = append(parts, s)
	}

	if len(parts) == 0 {
		return "1"
	}
	sort.Strings(parts)
	return strings.Join(parts, ".")
}
