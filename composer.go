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
	composeTermTo(&sb, t)
	return sb.String()
}

// composeTermTo writes the UCUM string for a term into the builder.
func composeTermTo(sb *strings.Builder, t *term) {
	composeComponentTo(sb, t.comp)

	if t.term != nil {
		sb.WriteString(t.op.String())
		composeTermTo(sb, t.term)
	}
}

// composeComponentTo writes the UCUM string for a single component.
func composeComponentTo(sb *strings.Builder, c component) {
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
		composeTermTo(sb, v)
	}
}

// composeCanonicalUnits serializes canonical units to a UCUM string.
// Example: [{m,1},{s,-1}] -> "m.s-1"
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
