package ucum

import (
	"fmt"
	"sort"
)

// converter walks a parsed UCUM AST and produces a canonical form consisting of
// base units and a scalar value. This is a port of Java's Converter.java from
// FHIR/Ucum-java.
//
// The converter only handles the multiplicative part of the conversion. For
// special units with offsets (Cel, degF), it extracts the base units and scale
// factor but does NOT apply the offset — that is done by the service layer.
type converter struct {
	model    *UcumModel
	handlers map[string]SpecialHandler
	parser   *parser

	// baseUnitMap provides O(1) lookup for BaseUnit by code.
	baseUnitMap map[string]*BaseUnit
}

// newConverter creates a converter backed by the given model and special handlers.
func newConverter(model *UcumModel, handlers map[string]SpecialHandler) *converter {
	bm := make(map[string]*BaseUnit, len(model.BaseUnits))
	for _, bu := range model.BaseUnits {
		bm[bu.Code] = bu
	}
	return &converter{
		model:       model,
		handlers:    handlers,
		parser:      newParser(model),
		baseUnitMap: bm,
	}
}

// convert normalizes a parsed term into canonical form.
func (c *converter) convert(t *term) (*canonical, error) {
	return c.normaliseTerm(t)
}

// normaliseTerm walks the right-recursive linked list of term nodes.
// For each component it produces a canonical, then combines using the operator.
func (c *converter) normaliseTerm(t *term) (*canonical, error) {
	if t == nil {
		return &canonical{value: decimalFromInt(1)}, nil
	}

	left, err := c.normaliseComponent(t.comp)
	if err != nil {
		return nil, err
	}

	if t.term == nil {
		return left, nil
	}

	right, err := c.normaliseTerm(t.term)
	if err != nil {
		return nil, err
	}

	if t.op == opDivision {
		left.value = left.value.div(right.value)
		negateUnits(right)
	} else {
		left.value = left.value.mul(right.value)
	}

	left.units = append(left.units, right.units...)
	left.units = collate(left.units)
	return left, nil
}

// normaliseComponent dispatches on the concrete component type.
func (c *converter) normaliseComponent(comp component) (*canonical, error) {
	switch v := comp.(type) {
	case *factor:
		return &canonical{value: decimalFromInt(int64(v.value))}, nil
	case *term:
		return c.normaliseTerm(v)
	case *symbol:
		return c.normaliseSymbol(v)
	default:
		return nil, fmt.Errorf("unknown component type %T", comp)
	}
}

// normaliseSymbol resolves a symbol to its canonical base units.
func (c *converter) normaliseSymbol(sym *symbol) (*canonical, error) {
	u := sym.unit

	if u.IsBase {
		bu := c.baseUnitMap[u.Code]
		if bu == nil {
			return nil, fmt.Errorf("base unit %q not found in model", u.Code)
		}
		can := &canonical{
			value: decimalFromInt(1),
			units: []canonicalUnit{{base: bu, exponent: sym.exponent}},
		}
		return c.applyPrefix(can, sym)
	}

	// Special unit: use handler's units expression for the base unit decomposition.
	if u.IsSpecial {
		handler, ok := c.handlers[u.Code]
		if !ok {
			return nil, fmt.Errorf("no handler for special unit %q", u.Code)
		}

		// Parse and convert the handler's unit expression to get canonical base units.
		t, err := c.parser.parse(handler.Units())
		if err != nil {
			return nil, fmt.Errorf("parse special units %q for %q: %w", handler.Units(), u.Code, err)
		}
		can, err := c.normaliseTerm(t)
		if err != nil {
			return nil, fmt.Errorf("normalise special units for %q: %w", u.Code, err)
		}

		// Multiply by the definition's numeric value (scale factor).
		if u.Value != nil {
			can.value = can.value.mul(u.Value.Value)
		}

		// Apply exponent.
		if sym.exponent != 1 {
			can.value = can.value.pow(sym.exponent)
			for i := range can.units {
				can.units[i].exponent *= sym.exponent
			}
		}

		return c.applyPrefix(can, sym)
	}

	// Regular defined unit: expand by re-parsing and converting its definition.
	can, err := c.expandDefinedUnit(u)
	if err != nil {
		return nil, err
	}

	// Apply exponent.
	if sym.exponent != 1 {
		can.value = can.value.pow(sym.exponent)
		for i := range can.units {
			can.units[i].exponent *= sym.exponent
		}
	}

	return c.applyPrefix(can, sym)
}

// expandDefinedUnit re-parses a defined unit's value expression and converts it
// recursively, multiplying by the definition's numeric value.
func (c *converter) expandDefinedUnit(u *Unit) (*canonical, error) {
	if u.Value == nil {
		return nil, fmt.Errorf("defined unit %q has no value", u.Code)
	}

	// The unit expression might be empty (e.g., dimensionless).
	unitExpr := u.Value.Unit
	if unitExpr == "" || unitExpr == "1" {
		return &canonical{value: u.Value.Value}, nil
	}

	t, err := c.parser.parse(unitExpr)
	if err != nil {
		return nil, fmt.Errorf("parse definition of %q (%q): %w", u.Code, unitExpr, err)
	}

	can, err := c.normaliseTerm(t)
	if err != nil {
		return nil, fmt.Errorf("normalise definition of %q: %w", u.Code, err)
	}

	// Multiply by the definition's numeric value.
	can.value = can.value.mul(u.Value.Value)
	return can, nil
}

// applyPrefix multiplies or divides the canonical value by the prefix raised to
// the symbol's exponent.
func (c *converter) applyPrefix(can *canonical, sym *symbol) (*canonical, error) {
	if sym.prefix == nil {
		return can, nil
	}
	prefixVal := sym.prefix.Value.pow(sym.exponent)
	can.value = can.value.mul(prefixVal)
	return can, nil
}

// negateUnits flips the sign of every unit's exponent.
func negateUnits(can *canonical) {
	for i := range can.units {
		can.units[i].exponent = -can.units[i].exponent
	}
}

// collate merges canonical units with the same base (summing exponents),
// removes zero-exponent entries, and sorts alphabetically by base unit code.
func collate(units []canonicalUnit) []canonicalUnit {
	// Merge by base code.
	type entry struct {
		base     *BaseUnit
		exponent int
	}
	m := make(map[string]*entry, len(units))
	var order []string
	for _, u := range units {
		if e, ok := m[u.base.Code]; ok {
			e.exponent += u.exponent
		} else {
			m[u.base.Code] = &entry{base: u.base, exponent: u.exponent}
			order = append(order, u.base.Code)
		}
	}

	// Sort alphabetically.
	sort.Strings(order)

	// Build result, dropping zero-exponent entries.
	result := make([]canonicalUnit, 0, len(order))
	for _, code := range order {
		e := m[code]
		if e.exponent != 0 {
			result = append(result, canonicalUnit{base: e.base, exponent: e.exponent})
		}
	}
	return result
}
