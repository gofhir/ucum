package engine

import (
	"fmt"
	"strings"

	"github.com/gofhir/ucum/v4/internal/expr"
	"github.com/gofhir/ucum/v4/internal/ucumerr"
)

// ValidateInProperty and the machinery behind it: deciding whether a code
// measures a named quantity.

// ValidateInProperty validates the code and checks that it measures the given
// property, case-insensitively.
//
// An atomic unit is checked against the property UCUM declares for it, and that
// is the whole answer for it.
//
// A compound expression has no declared property, so it is checked
// dimensionally: its canonical form must match that of a unit which does declare
// the property. That is the best the definitions allow, and it is not airtight —
// UCUM gives 15 canonical forms to more than one property, so "m.s-1" tells
// velocity from acceleration but "1" cannot tell "amount of substance" from
// "fraction". Atomic codes never take that path, which is why they are handled
// separately rather than folded into the same comparison.
func (s *Service) ValidateInProperty(code, property string) error {
	if err := s.Validate(code); err != nil {
		return err
	}
	t, can, err := s.canonicalParts(code)
	if err != nil {
		return ucumerr.Validation(code, err)
	}

	// An atomic unit carries its property in the definitions, so that is the
	// answer — no dimensional fallback. Falling back would accept mol as a
	// "fraction", since mol is dimensionless in UCUM and so shares its canonical
	// form.
	if declared, ok := declaredProperty(t); ok {
		if strings.EqualFold(declared, property) {
			return nil
		}
		if _, err := s.canonicalFormsOfProperty(property); err != nil {
			return &ucumerr.ValidationError{Code: code, Message: err.Error(), Offset: -1}
		}
		return &ucumerr.ValidationError{
			Code:    code,
			Message: fmt.Sprintf("unit %q does not measure %q (it measures %q)", code, property, declared),
			Offset:  -1,
		}
	}

	forms, err := s.canonicalFormsOfProperty(property)
	if err != nil {
		return &ucumerr.ValidationError{Code: code, Message: err.Error(), Offset: -1}
	}
	if forms[expr.ComposeCanonicalUnits(can)] {
		return nil
	}

	return &ucumerr.ValidationError{
		Code:    code,
		Message: fmt.Sprintf("unit %q does not measure %q (its canonical form is %s)", code, property, expr.ComposeCanonicalUnits(can)),
		Offset:  -1,
	}
}

// declaredProperty returns the property UCUM declares for a term that is a
// single unit with exponent 1. A prefix does not change the property, but an
// exponent does (m is length, m2 is area), so exponents are excluded.
//
// Redundant parentheses are looked through, for the same reason as in
// specialUseForTerm: "(mol)" is the same code as "mol". Without that, a
// parenthesized atom fell to the dimensional comparison and "(mol)" was accepted
// as a "fraction" — mol is dimensionless in UCUM, so it shares that canonical
// form — while "mol" was correctly refused.
func declaredProperty(t *expr.Term) (string, bool) {
	sym := expr.LoneSymbol(t)
	if sym == nil || sym.Exponent != 1 || sym.Unit == nil {
		return "", false
	}
	if sym.Unit.Property == "" {
		return "", false
	}
	return sym.Unit.Property, true
}

// canonicalFormsOfProperty returns the canonical forms of the units declaring a
// property. Results are memoized: resolving one property canonicalizes only its
// own units, and never more than once.
func (s *Service) canonicalFormsOfProperty(property string) (map[string]bool, error) {
	key := strings.ToLower(strings.TrimSpace(property))
	if v, ok := s.propertyForms.Load(key); ok {
		forms, ok := v.(map[string]bool)
		if !ok {
			return nil, fmt.Errorf("ucum: unexpected property cache entry type %T", v)
		}
		return forms, nil
	}

	codes, ok := s.codesByProperty[key]
	if !ok {
		return nil, fmt.Errorf("unknown property %q", property)
	}

	forms := make(map[string]bool)
	for _, code := range codes {
		can, err := s.canonicalOfDefinition(code)
		if err != nil {
			// A definition this package cannot canonicalize contributes nothing;
			// the remaining units of the property still describe it.
			continue
		}
		forms[expr.ComposeCanonicalUnits(can)] = true
	}
	s.propertyForms.Store(key, forms)
	return forms, nil
}
