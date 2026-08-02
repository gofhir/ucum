package expr

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gofhir/ucum/v4/internal/model"
)

// Compose serializes an AST term back to a UCUM string.
func Compose(t *Term) string {
	if t == nil {
		return ""
	}

	var sb strings.Builder
	composeTermTo(&sb, t, false)
	return sb.String()
}

// composeTermTo writes the UCUM string for a term into the builder. The
// rightOperand flag says whether this term sits on the right of an Operator,
// which is where a nested group has to keep its parentheses.
func composeTermTo(sb *strings.Builder, t *Term, rightOperand bool) {
	composeComponentTo(sb, t.Comp, rightOperand)

	if t.Term != nil {
		sb.WriteString(t.Op.String())
		composeTermTo(sb, t.Term, true)
	}
}

// composeComponentTo writes the UCUM string for a single Component.
//
// A nested term that carries an Operator is parenthesized when it is a right
// operand, and only then. UCUM operators are left-associative, so the left
// operand of a chain needs no brackets — "a/b/c" already means "(a/b)/c" — but
// the right one does: dropping them turns "kg/(s.m2)" into "kg/s.m2", which is
// kg.m2.s-1 rather than kg.m-2.s-1. Those are real codes; "mL/(kg.min)" is a
// pediatric dose rate and "mL/min/(173.10*-2.m2)" is in the FHIR value set.
func composeComponentTo(sb *strings.Builder, c Component, rightOperand bool) {
	switch v := c.(type) {
	case *Factor:
		fmt.Fprintf(sb, "%d", v.Value)
	case *Symbol:
		if v.Prefix != nil {
			sb.WriteString(v.Prefix.Code)
		}
		sb.WriteString(v.Unit.Code)
		if v.Exponent != 1 {
			fmt.Fprintf(sb, "%d", v.Exponent)
		}
	case *Term:
		// Redundant parentheses are looked through first, so that "((a.b))" is
		// recognized as carrying an Operator and "((m))" as not.
		inner := Unwrap(v)
		parenthesize := rightOperand && inner.Term != nil
		if parenthesize {
			sb.WriteString("(")
		}
		composeTermTo(sb, inner, false)
		if parenthesize {
			sb.WriteString(")")
		}
	}
}

// ComposeCanonicalUnits serializes canonical units to a UCUM string.
// Example: [{m,1},{s,-1}] produces "m.s-1".
func ComposeCanonicalUnits(c *model.Canonical) string {
	if c == nil || len(c.Units) == 0 {
		return "1"
	}

	var parts []string
	for _, u := range c.Units {
		if u.Exponent == 0 {
			continue
		}
		s := u.Base.Code
		if u.Exponent != 1 {
			s += fmt.Sprintf("%d", u.Exponent)
		}
		parts = append(parts, s)
	}

	if len(parts) == 0 {
		return "1"
	}
	sort.Strings(parts)
	return strings.Join(parts, ".")
}
