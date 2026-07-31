package ucum

import (
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"
	"sync"
)

// service is the concrete implementation of Service.
type service struct {
	model  *Model
	parser *parser
	cache  sync.Map // map[string]*term
}

// newService creates a fully wired service.
func newService(r io.Reader) (*service, error) {
	m, err := loadDefinitions(r)
	if err != nil {
		return nil, fmt.Errorf("ucum: load definitions: %w", err)
	}
	return &service{
		model:  m,
		parser: newParser(m),
	}, nil
}

// parseCached parses a UCUM code, caching the result.
func (s *service) parseCached(code string) (*term, error) {
	if v, ok := s.cache.Load(code); ok {
		t, ok := v.(*term)
		if !ok {
			return nil, fmt.Errorf("ucum: unexpected cache entry type %T", v)
		}
		return t, nil
	}
	t, err := s.parser.parse(code)
	if err != nil {
		return nil, err
	}
	s.cache.Store(code, t)
	return t, nil
}

// Validate checks if the given code is a valid UCUM expression.
func (s *service) Validate(code string) error {
	_, err := s.parseCached(code)
	if err != nil {
		return &ValidationError{Code: code, Message: err.Error(), Offset: -1}
	}
	return nil
}

// ValidateInProperty validates the code and checks that its canonical form
// has the expected property (dimension).
func (s *service) ValidateInProperty(code, property string) error {
	if err := s.Validate(code); err != nil {
		return err
	}
	can, err := s.getCanonical(code)
	if err != nil {
		return err
	}
	// Determine the property from canonical base units.
	p := canonicalProperty(can, s.model)
	if !strings.EqualFold(p, property) {
		return &ValidationError{
			Code:    code,
			Message: fmt.Sprintf("unit %q has property %q, expected %q", code, p, property),
			Offset:  -1,
		}
	}
	return nil
}

// Canonical returns the canonical (base-unit) form of a value+code pair.
func (s *service) Canonical(value float64, code string) (Pair, error) {
	v, can, err := s.canonicalScalar(value, code)
	if err != nil {
		return Pair{}, err
	}
	return Pair{Value: v, Code: composeCanonicalUnits(can)}, nil
}

// canonicalScalar maps value onto the canonical scale of code and scales it by
// the canonical factor, rounding once.
func (s *service) canonicalScalar(value float64, code string) (float64, *canonical, error) {
	t, can, err := s.canonicalParts(code)
	if err != nil {
		return 0, nil, err
	}

	// Preferred path: map and scale entirely in exact arithmetic.
	if rv := ratFromFloat(value); rv != nil {
		mapped, err := toCanonicalRat(t, code, rv)
		switch {
		case err == nil:
			out, _ := mapped.Mul(mapped, can.value.rat()).Float64()
			return out, can, nil
		case !errors.Is(err, ErrNotRational):
			return 0, nil, err
		}
	}

	// Non-rational scale or non-finite value: use the float64 handler.
	mapped, err := s.canonicalMapped(value, code)
	if err != nil {
		return 0, nil, err
	}
	return mulExact(mapped, can.value.rat()), can, nil
}

// canonicalMapped maps value onto the canonical scale without applying the
// multiplicative factor, so a caller combining several operands can keep the
// factors exact and round only once at the end.
//
// Special units sit on non-ratio scales, so the multiplicative factor alone
// does not describe them: the handler has to map the value first (Cel adds an
// offset, [pH] exponentiates, B[V] takes a power). Skipping it would silently
// return the raw input value, making canonical forms non-comparable.
func (s *service) canonicalMapped(value float64, code string) (float64, error) {
	t, _, err := s.canonicalParts(code)
	if err != nil {
		return 0, err
	}
	if h := specialHandlerForTerm(t); h != nil {
		value = h.toCanonical(value)
	}
	return value, nil
}

// canonicalParts parses a code and canonicalizes it, returning both the AST
// (needed to detect special units) and the canonical form.
func (s *service) canonicalParts(code string) (*term, *canonical, error) {
	t, err := s.parseCached(code)
	if err != nil {
		return nil, nil, err
	}
	can, err := s.canonicalizeTerm(t)
	if err != nil {
		return nil, nil, err
	}
	return t, can, nil
}

// Convert converts a value from one unit to another.
func (s *service) Convert(value float64, from, to string) (float64, error) {
	srcTerm, err := s.parseCached(from)
	if err != nil {
		return 0, &ConversionError{From: from, To: to, Message: err.Error()}
	}
	dstTerm, err := s.parseCached(to)
	if err != nil {
		return 0, &ConversionError{From: from, To: to, Message: err.Error()}
	}

	srcCan, err := s.getCanonical(from)
	if err != nil {
		return 0, &ConversionError{From: from, To: to, Message: err.Error()}
	}
	dstCan, err := s.getCanonical(to)
	if err != nil {
		return 0, &ConversionError{From: from, To: to, Message: err.Error()}
	}

	// Check comparability: canonical unit strings must match.
	if err := requireComparable(from, to, srcCan, dstCan); err != nil {
		return 0, err
	}

	// A zero destination factor ("0", "m/0" cancels to it) would divide by zero.
	if dstCan.value.isZero() {
		return 0, &ConversionError{From: from, To: to, Message: errDivisionByZero.Error()}
	}

	parts := convertParts{
		srcTerm: srcTerm, dstTerm: dstTerm,
		srcCan: srcCan, dstCan: dstCan,
		from: from, to: to,
	}

	// Preferred path: run the whole conversion in exact rational arithmetic and
	// round once at the end. Combining canonical factors that were each already
	// rounded to float64 loses exactness even when the true result is
	// representable — L -> mL came out as 1000.0000000000001 because 1/1000 and
	// 1/1000000 are not binary fractions while their exact quotient is 1000 —
	// and the affine temperature handlers compounded it further.
	if rv := ratFromFloat(value); rv != nil {
		exact, err := convertRatCore(rv, parts)
		if err == nil {
			out, _ := exact.Float64()
			return out, nil
		}
		if !errors.Is(err, ErrNotRational) {
			return 0, err
		}
		// Non-rational scale: fall through to the float64 handlers below.
	}

	result := value

	// Step 1: If source is special, convert value to canonical first.
	if srcHandler := specialHandlerForTerm(srcTerm); srcHandler != nil {
		result = srcHandler.toCanonical(result)
	}

	// Step 2: Multiplicative conversion, exact factor with a single rounding.
	result = mulExact(result, new(big.Rat).Quo(srcCan.value.rat(), dstCan.value.rat()))

	// Step 3: If dest is special, convert from canonical.
	if dstHandler := specialHandlerForTerm(dstTerm); dstHandler != nil {
		result = dstHandler.fromCanonical(result)
	}

	return result, nil
}

// IsComparable returns true if the two unit codes have the same canonical units.
func (s *service) IsComparable(code1, code2 string) (bool, error) {
	can1, err := s.getCanonical(code1)
	if err != nil {
		return false, err
	}
	can2, err := s.getCanonical(code2)
	if err != nil {
		return false, err
	}
	return composeCanonicalUnits(can1) == composeCanonicalUnits(can2), nil
}

// Analyze returns a human-readable description of the unit expression.
func (s *service) Analyze(code string) (string, error) {
	t, err := s.parseCached(code)
	if err != nil {
		return "", err
	}
	return analyseTermHuman(t), nil
}

// Multiply multiplies two value/unit pairs.
func (s *service) Multiply(v1, v2 Pair) (Pair, error) {
	t1, can1, err := s.canonicalParts(v1.Code)
	if err != nil {
		return Pair{}, err
	}
	t2, can2, err := s.canonicalParts(v2.Code)
	if err != nil {
		return Pair{}, err
	}
	code := composeCanonicalUnits(mergeCanonicalUnits(can1, can2))
	factor := new(big.Rat).Mul(can1.value.rat(), can2.value.rat())

	// Preferred path: both operands mapped and combined in exact arithmetic,
	// with a single rounding at the end.
	exact, err := s.multiplyRat(t1, t2, v1, v2)
	switch {
	case err == nil && exact != nil:
		out, _ := exact.Mul(exact, factor).Float64()
		return Pair{Value: out, Code: code}, nil
	case err != nil && !errors.Is(err, ErrNotRational):
		return Pair{}, err
	}

	// Non-rational scale or non-finite value: use the float64 handlers.
	val1, err := s.canonicalMapped(v1.Value, v1.Code)
	if err != nil {
		return Pair{}, err
	}
	val2, err := s.canonicalMapped(v2.Value, v2.Code)
	if err != nil {
		return Pair{}, err
	}
	return Pair{Value: mulExact(val1*val2, factor), Code: code}, nil
}

// multiplyRat maps both operands onto their canonical scales exactly and returns
// their product, without the canonical factors. It returns a nil result when an
// operand is not finite, and ErrNotRational when a scale has no rational form.
func (s *service) multiplyRat(t1, t2 *term, v1, v2 Pair) (*big.Rat, error) {
	r1, r2 := ratFromFloat(v1.Value), ratFromFloat(v2.Value)
	if r1 == nil || r2 == nil {
		return nil, nil
	}
	m1, err := toCanonicalRat(t1, v1.Code, r1)
	if err != nil {
		return nil, err
	}
	m2, err := toCanonicalRat(t2, v2.Code, r2)
	if err != nil {
		return nil, err
	}
	return m1.Mul(m1, m2), nil
}

// Canonical conversion (converter logic).

// getCanonical computes the canonical form of a UCUM code.
func (s *service) getCanonical(code string) (*canonical, error) {
	t, err := s.parseCached(code)
	if err != nil {
		return nil, err
	}
	return s.canonicalizeTerm(t)
}

// canonicalizeTerm recursively converts a term AST into canonical form.
func (s *service) canonicalizeTerm(t *term) (*canonical, error) {
	if t == nil {
		return &canonical{value: decimalFromInt(1)}, nil
	}

	left, err := s.canonicalizeComponent(t.comp)
	if err != nil {
		return nil, err
	}

	if t.term == nil {
		return left, nil
	}

	right, err := s.canonicalizeTerm(t.term)
	if err != nil {
		return nil, err
	}

	switch t.op {
	case opMultiplication:
		return multiplyCanonicals(left, right), nil
	case opDivision:
		return divideCanonicals(left, right)
	default:
		return nil, fmt.Errorf("unknown operator %d", t.op)
	}
}

// canonicalizeComponent converts a single component to canonical form.
func (s *service) canonicalizeComponent(c component) (*canonical, error) {
	switch v := c.(type) {
	case *factor:
		return &canonical{value: decimalFromInt(int64(v.value))}, nil
	case *symbol:
		return s.canonicalizeSymbol(v)
	case *term:
		return s.canonicalizeTerm(v)
	default:
		return nil, fmt.Errorf("unexpected component type %T", c)
	}
}

// canonicalizeSymbol converts a symbol to its canonical form by recursively
// expanding the unit's definition.
func (s *service) canonicalizeSymbol(sym *symbol) (*canonical, error) {
	u := sym.unit

	// Start with the prefix value (or 1 if no prefix).
	prefixVal := decimalFromInt(1)
	if sym.prefix != nil {
		prefixVal = sym.prefix.Value
	}

	if u.IsBase {
		// Base unit: canonical is itself.
		bu := s.findBaseUnit(u.Code)
		if bu == nil {
			return nil, fmt.Errorf("base unit %q not found", u.Code)
		}
		val := prefixVal.pow(sym.exponent)
		return &canonical{
			value: val,
			units: []canonicalUnit{{base: bu, exponent: sym.exponent}},
		}, nil
	}

	// Defined unit: expand through its value expression.
	if u.Value == nil {
		return nil, fmt.Errorf("unit %q has no value definition", u.Code)
	}

	// For special units, use the handler's unit expression for canonical units,
	// but the numeric multiplier is 1 (the handler does the real conversion).
	unitExpr := u.Value.Unit
	unitValue := u.Value.Value

	if u.IsSpecial {
		h, ok := specialHandlers[u.Code]
		if !ok {
			return nil, fmt.Errorf("no handler for special unit %q", u.Code)
		}
		unitExpr = h.units()
		unitValue = decimalFromInt(1)
	}

	// Parse and canonicalize the unit's value expression.
	if unitExpr == "" || unitExpr == "1" {
		// Dimensionless unit.
		val := prefixVal.mul(unitValue).pow(sym.exponent)
		return &canonical{value: val}, nil
	}

	inner, err := s.parseCached(unitExpr)
	if err != nil {
		return nil, fmt.Errorf("expand unit %q: %w", u.Code, err)
	}

	can, err := s.canonicalizeTerm(inner)
	if err != nil {
		return nil, fmt.Errorf("expand unit %q: %w", u.Code, err)
	}

	// Multiply by the unit's numeric value and the prefix.
	can.value = prefixVal.mul(unitValue).mul(can.value)

	// Apply exponent.
	if sym.exponent != 1 {
		can.value = can.value.pow(sym.exponent)
		for i := range can.units {
			can.units[i].exponent *= sym.exponent
		}
	}

	return can, nil
}

// findBaseUnit looks up a BaseUnit by code.
func (s *service) findBaseUnit(code string) *BaseUnit {
	for _, bu := range s.model.BaseUnits {
		if bu.Code == code {
			return bu
		}
	}
	return nil
}

// Canonical arithmetic helpers.

func multiplyCanonicals(left, right *canonical) *canonical {
	result := &canonical{
		value: left.value.mul(right.value),
		units: mergeUnitLists(left.units, right.units, 1),
	}
	return result
}

func divideCanonicals(left, right *canonical) (*canonical, error) {
	// A zero factor is syntactically valid ("m/0"), so the divisor has to be
	// checked here rather than left to big.Rat, which panics.
	if right.value.isZero() {
		return nil, errDivisionByZero
	}
	result := &canonical{
		value: left.value.div(right.value),
		units: mergeUnitLists(left.units, right.units, -1),
	}
	return result, nil
}

// mergeUnitLists merges two canonical unit lists. The sign parameter is
// applied to the exponents of the right list (+1 for multiply, -1 for divide).
func mergeUnitLists(left, right []canonicalUnit, sign int) []canonicalUnit {
	result := make([]canonicalUnit, len(left))
	copy(result, left)

	for _, ru := range right {
		found := false
		for i := range result {
			if result[i].base.Code == ru.base.Code {
				result[i].exponent += ru.exponent * sign
				found = true
				break
			}
		}
		if !found {
			result = append(result, canonicalUnit{
				base:     ru.base,
				exponent: ru.exponent * sign,
			})
		}
	}

	// Remove zero-exponent units.
	filtered := result[:0]
	for _, u := range result {
		if u.exponent != 0 {
			filtered = append(filtered, u)
		}
	}
	return filtered
}

// mergeCanonicalUnits creates a new canonical with merged unit lists (multiply).
func mergeCanonicalUnits(a, b *canonical) *canonical {
	return &canonical{
		value: decimalFromInt(1),
		units: mergeUnitLists(a.units, b.units, 1),
	}
}

// Property resolution.

// canonicalProperty determines the property from canonical base units.
func canonicalProperty(can *canonical, _ *Model) string {
	if len(can.units) == 0 {
		return "dimensionless"
	}
	// If single base unit with exponent 1, return its property directly.
	if len(can.units) == 1 && can.units[0].exponent == 1 {
		return can.units[0].base.Property
	}
	// For compound units, build a dimension string.
	return composeCanonicalUnits(can)
}

// Human-readable analysis.

// analyseTermHuman returns a human-readable description of a term.
func analyseTermHuman(t *term) string {
	if t == nil {
		return ""
	}

	var sb strings.Builder
	analyseComponentHuman(&sb, t.comp)

	if t.term != nil {
		sb.WriteString(t.op.String())
		analyseTermHumanTo(&sb, t.term)
	}

	return sb.String()
}

func analyseTermHumanTo(sb *strings.Builder, t *term) {
	analyseComponentHuman(sb, t.comp)
	if t.term != nil {
		sb.WriteString(t.op.String())
		analyseTermHumanTo(sb, t.term)
	}
}

func analyseComponentHuman(sb *strings.Builder, c component) {
	switch v := c.(type) {
	case *factor:
		fmt.Fprintf(sb, "%d", v.value)
	case *symbol:
		if v.prefix != nil {
			sb.WriteString(v.prefix.Name)
		}
		sb.WriteString(v.unit.Name)
		if v.exponent != 1 {
			fmt.Fprintf(sb, "%d", v.exponent)
		}
	case *term:
		analyseTermHumanTo(sb, v)
	}
}

// Special unit detection.

// specialHandlerForTerm returns a specialHandler if the term is a single
// symbol whose unit is special (no operators, no exponents other than 1).
func specialHandlerForTerm(t *term) specialHandler {
	if t == nil || t.term != nil {
		return nil
	}
	sym, ok := t.comp.(*symbol)
	if !ok {
		return nil
	}
	if !sym.unit.IsSpecial {
		return nil
	}
	if sym.exponent != 1 {
		return nil
	}
	h, ok := specialHandlers[sym.unit.Code]
	if !ok {
		return nil
	}
	return h
}
