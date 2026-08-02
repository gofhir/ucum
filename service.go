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
	model *ucumModel

	// parser resolves codes supplied by a caller, in whichever of UCUM's two
	// vocabularies this service was built for.
	parser *parser
	cache  *termCache

	// defParser resolves the expressions inside the definitions, which are always
	// written in case-sensitive codes: the year is defined as "a_j", not as its
	// case-insensitive spelling "ANN_J". Expanding a definition therefore never
	// uses the caller's vocabulary.
	//
	// defCache is unbounded, unlike cache, and can be: its keys come from the
	// definitions, so there are as many as ucum-essence.xml has unit expressions
	// and no caller can add to them.
	defParser *parser
	defCache  sync.Map // map[string]*term

	// baseByCode indexes the seven base units, and arbitraryBases gives every
	// arbitrary unit its own dimension. Both are populated once at construction
	// and read-only afterwards.
	baseByCode     map[string]*baseUnit
	arbitraryBases map[string]*baseUnit

	// codesByProperty maps a lower-cased property name to the codes of the units
	// that declare it. Built at construction; the canonical forms behind them
	// are resolved lazily and memoised in propertyForms.
	codesByProperty map[string][]string
	propertyForms   sync.Map // map[string]map[string]bool

	// handlers holds one conversion per special unit, built from the function
	// each definition names. Read-only after construction.
	handlers map[string]specialHandler
}

// newService creates a fully wired service resolving the case-sensitive
// vocabulary.
func newService(r io.Reader) (*service, error) {
	return newServiceFor(r, false)
}

// newServiceFor creates a service resolving one of UCUM's two vocabularies.
func newServiceFor(r io.Reader, insensitive bool) (*service, error) {
	m, err := loadDefinitions(r)
	if err != nil {
		return nil, fmt.Errorf("ucum: load definitions: %w", err)
	}
	bases := make(map[string]*baseUnit, len(m.BaseUnits))
	for _, bu := range m.BaseUnits {
		bases[bu.Code] = bu
	}

	// UCUM makes arbitrary units commensurable with nothing, so each one gets a
	// synthetic base unit standing for its own dimension.
	arbitrary := make(map[string]*baseUnit)
	for _, du := range m.DefinedUnits {
		if du.IsArbitrary {
			arbitrary[du.Code] = &baseUnit{
				Code:     du.Code,
				Name:     du.Name,
				Property: du.Property,
			}
		}
	}

	// Index the declared properties. Canonicalizing every unit here would slow
	// construction down for a feature most callers never use, so only the codes
	// are collected now.
	codesByProperty := make(map[string][]string)
	addProperty := func(prop, code string) {
		if prop == "" {
			return
		}
		key := strings.ToLower(prop)
		codesByProperty[key] = append(codesByProperty[key], code)
	}
	for _, bu := range m.BaseUnits {
		addProperty(bu.Property, bu.Code)
	}
	for _, du := range m.DefinedUnits {
		addProperty(du.Property, du.Code)
	}

	handlers, err := buildSpecialHandlers(m)
	if err != nil {
		return nil, fmt.Errorf("ucum: %w", err)
	}

	return &service{
		model:           m,
		parser:          newParserFor(m, insensitive),
		cache:           newTermCache(),
		defParser:       newParser(m),
		baseByCode:      bases,
		arbitraryBases:  arbitrary,
		codesByProperty: codesByProperty,
		handlers:        handlers,
	}, nil
}

// parseCached parses a UCUM code, caching the result.
func (s *service) parseCached(code string) (*term, error) {
	if t, ok := s.cache.load(code); ok {
		return t, nil
	}
	t, err := s.parser.parse(code)
	if err != nil {
		return nil, err
	}
	s.cache.store(code, t)
	return t, nil
}

// Definitions reports which UCUM release these definitions declare themselves
// to be.
func (s *service) Definitions() Definitions {
	return Definitions{
		Version:      s.model.Version,
		Revision:     s.model.Revision,
		RevisionDate: s.model.RevisionDate,
	}
}

// parseDefinition parses an expression taken from the definitions, always in the
// case-sensitive vocabulary, and caches the result separately from caller input.
func (s *service) parseDefinition(expr string) (*term, error) {
	if v, ok := s.defCache.Load(expr); ok {
		t, ok := v.(*term)
		if !ok {
			return nil, fmt.Errorf("ucum: unexpected cache entry type %T", v)
		}
		return t, nil
	}
	t, err := s.defParser.parse(expr)
	if err != nil {
		return nil, err
	}
	s.defCache.Store(expr, t)
	return t, nil
}

// Validate checks if the given code is a valid UCUM expression.
func (s *service) Validate(code string) error {
	_, err := s.parseCached(code)
	return validationError(code, err)
}

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
func (s *service) ValidateInProperty(code, property string) error {
	if err := s.Validate(code); err != nil {
		return err
	}
	t, can, err := s.canonicalParts(code)
	if err != nil {
		return validationError(code, err)
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
			return &ValidationError{Code: code, Message: err.Error(), Offset: -1}
		}
		return &ValidationError{
			Code:    code,
			Message: fmt.Sprintf("unit %q does not measure %q (it measures %q)", code, property, declared),
			Offset:  -1,
		}
	}

	forms, err := s.canonicalFormsOfProperty(property)
	if err != nil {
		return &ValidationError{Code: code, Message: err.Error(), Offset: -1}
	}
	if forms[composeCanonicalUnits(can)] {
		return nil
	}

	return &ValidationError{
		Code:    code,
		Message: fmt.Sprintf("unit %q does not measure %q (its canonical form is %s)", code, property, composeCanonicalUnits(can)),
		Offset:  -1,
	}
}

// declaredProperty returns the property UCUM declares for a term that is a
// single unit with exponent 1. A prefix does not change the property, but an
// exponent does (m is length, m2 is area), so exponents are excluded.
//
// Redundant parentheses are looked through, for the same reason as in
// specialUseForTerm: "(mol)" is the same code as "mol". Without that, a
// parenthesised atom fell to the dimensional comparison and "(mol)" was accepted
// as a "fraction" — mol is dimensionless in UCUM, so it shares that canonical
// form — while "mol" was correctly refused.
func declaredProperty(t *term) (string, bool) {
	sym := loneSymbol(t)
	if sym == nil || sym.exponent != 1 || sym.unit == nil {
		return "", false
	}
	if sym.unit.Property == "" {
		return "", false
	}
	return sym.unit.Property, true
}

// canonicalFormsOfProperty returns the canonical forms of the units declaring a
// property. Results are memoised: resolving one property canonicalizes only its
// own units, and never more than once.
func (s *service) canonicalFormsOfProperty(property string) (map[string]bool, error) {
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
		forms[composeCanonicalUnits(can)] = true
	}
	s.propertyForms.Store(key, forms)
	return forms, nil
}

// Canonical returns the canonical (base-unit) form of a value+code pair.
func (s *service) Canonical(value float64, code string) (Pair, error) {
	v, can, err := s.canonicalScalar(value, code)
	if err != nil {
		return Pair{}, validationError(code, err)
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
		mapped, err := s.toCanonicalRat(t, code, rv)
		switch {
		case err == nil:
			out, _ := mapped.Mul(mapped, can.value.rat()).Float64()
			return out, can, nil
		case !errors.Is(err, ErrNotRational):
			return 0, nil, err
		}
	}

	// Non-rational scale or non-finite value: use the float64 handler.
	return mulExact(s.mapFloat(t, value), can.value.rat()), can, nil
}

// mapFloat maps value onto the canonical scale of an already-resolved term,
// using the float64 handler. It serves the scales that have no exact rational
// form; everything else goes through toCanonicalRat.
//
// Special units sit on non-ratio scales, so the multiplicative factor alone
// does not describe them: the handler has to map the value first (Cel adds an
// offset, [pH] exponentiates, B[V] takes a power). Skipping it would silently
// return the raw input value, making canonical forms non-comparable.
func (s *service) mapFloat(t *term, value float64) float64 {
	if use := s.specialUseForTerm(t); use != nil {
		return use.toCanonical(value)
	}
	return value
}

// specialContext says how a special unit in a term is to be read.
//
// Standalone it denotes a point on its own scale, and its handler applies the
// whole mapping including any offset: 1 Cel is 274.15 K. Inside an algebraic term
// it denotes a difference — a gradient of 1 [degF]/min is a rate of change, not a
// temperature — so the offset cancels while the scale, which the definitions
// give, still applies.
type specialContext int

const (
	specialStandalone specialContext = iota
	specialAsDelta
)

// specialContextOf reports how special units in this term are to be read. A term
// that is a lone special symbol is standalone; anything else puts its units into
// an algebraic relationship.
func (s *service) specialContextOf(t *term) specialContext {
	if s.specialUseForTerm(t) != nil {
		return specialStandalone
	}
	return specialAsDelta
}

// canonicalParts parses a code and canonicalizes it, returning both the AST
// (needed to detect special units) and the canonical form.
func (s *service) canonicalParts(code string) (*term, *canonical, error) {
	t, err := s.parseCached(code)
	if err != nil {
		return nil, nil, err
	}
	can, err := s.canonicalizeTerm(t, s.specialContextOf(t))
	if err != nil {
		return nil, nil, err
	}
	return t, can, nil
}

// Convert converts a value from one unit to another.
func (s *service) Convert(value float64, from, to string) (float64, error) {
	srcTerm, err := s.parseCached(from)
	if err != nil {
		return 0, conversionError(from, to, err)
	}
	dstTerm, err := s.parseCached(to)
	if err != nil {
		return 0, conversionError(from, to, err)
	}

	srcCan, err := s.getCanonical(from)
	if err != nil {
		return 0, conversionError(from, to, err)
	}
	dstCan, err := s.getCanonical(to)
	if err != nil {
		return 0, conversionError(from, to, err)
	}

	// Check comparability: canonical unit strings must match.
	if err := requireComparable(from, to, srcCan, dstCan); err != nil {
		return 0, err
	}

	// A zero destination factor ("0", "m/0" cancels to it) would divide by zero.
	if dstCan.value.isZero() {
		return 0, conversionError(from, to, ErrDivisionByZero)
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
		exact, err := s.convertRatCore(rv, parts)
		if err == nil {
			out, _ := exact.Float64()
			return out, nil
		}
		if !errors.Is(err, ErrNotRational) {
			return 0, conversionError(from, to, err)
		}
		// Non-rational scale: fall through to the float64 handlers below.
	}

	result := value

	// Step 1: If source is special, convert value to canonical first.
	if srcUse := s.specialUseForTerm(srcTerm); srcUse != nil {
		result = srcUse.toCanonical(result)
	}

	// Step 2: Multiplicative conversion, exact factor with a single rounding.
	result = mulExact(result, new(big.Rat).Quo(srcCan.value.rat(), dstCan.value.rat()))

	// Step 3: If dest is special, convert from canonical.
	if dstUse := s.specialUseForTerm(dstTerm); dstUse != nil {
		result = dstUse.fromCanonical(result)
	}

	return result, nil
}

// IsComparable returns true if the two unit codes have the same canonical units.
func (s *service) IsComparable(code1, code2 string) (bool, error) {
	can1, err := s.getCanonical(code1)
	if err != nil {
		return false, validationError(code1, err)
	}
	can2, err := s.getCanonical(code2)
	if err != nil {
		return false, validationError(code2, err)
	}
	return composeCanonicalUnits(can1) == composeCanonicalUnits(can2), nil
}

// Analyze returns a human-readable description of the unit expression, in the
// display format of the official UCUM test suite: each unit parenthesised with
// its full name, exponents written as " ^ n", and operators as " * " and " / ".
//
// An empty expression describes the unity, matching the suite, even though
// Validate rejects it as a code.
func (s *service) Analyze(code string) (string, error) {
	if strings.TrimSpace(code) == "" {
		return unityDisplayName, nil
	}
	t, err := s.parseCached(code)
	if err != nil {
		return "", validationError(code, err)
	}
	return displayName(t), nil
}

// Multiply multiplies two value/unit pairs.
func (s *service) Multiply(v1, v2 Pair) (Pair, error) {
	return s.combine(v1, v2, opMultiplication)
}

// Divide divides two value/unit pairs.
func (s *service) Divide(v1, v2 Pair) (Pair, error) {
	return s.combine(v1, v2, opDivision)
}

// combine is the shared body of Multiply and Divide. Both map each operand onto
// its canonical scale, combine the operands, combine the canonical factors and
// round once; they differ in the operator and in division having to reject a
// zero divisor.
func (s *service) combine(v1, v2 Pair, op operator) (Pair, error) {
	t1, can1, err := s.canonicalParts(v1.Code)
	if err != nil {
		return Pair{}, validationError(v1.Code, err)
	}
	t2, can2, err := s.canonicalParts(v2.Code)
	if err != nil {
		return Pair{}, validationError(v2.Code, err)
	}

	div := op == opDivision
	if div && can2.value.isZero() {
		return Pair{}, validationError(v2.Code, ErrDivisionByZero)
	}

	sign := 1
	factor := can1.value.rat()
	if div {
		sign = -1
		factor.Quo(factor, can2.value.rat())
	} else {
		factor.Mul(factor, can2.value.rat())
	}
	code := composeCanonicalUnits(&canonical{units: mergeUnitLists(can1.units, can2.units, sign)})

	// Preferred path: both operands mapped and combined in exact arithmetic,
	// with a single rounding at the end.
	m1, m2, err := s.mappedRats(t1, t2, v1, v2)
	switch {
	case err == nil && m1 != nil:
		if div {
			// Defensive: a special operand can only map to exactly zero if its
			// input is exactly the bottom of its scale, which float64 cannot
			// represent for Cel or [degF]. Guarded so big.Rat.Quo cannot panic.
			if m2.Sign() == 0 {
				return Pair{}, zeroDivisorValue(v2)
			}
			m1.Quo(m1, m2)
		} else {
			m1.Mul(m1, m2)
		}
		out, _ := m1.Mul(m1, factor).Float64()
		return Pair{Value: out, Code: code}, nil
	case err != nil && !errors.Is(err, ErrNotRational):
		return Pair{}, validationError(v1.Code, err)
	}

	// Non-rational scale or non-finite value: use the float64 handlers.
	val1, val2 := s.mapFloat(t1, v1.Value), s.mapFloat(t2, v2.Value)
	combined := val1 * val2
	if div {
		if val2 == 0 {
			return Pair{}, zeroDivisorValue(v2)
		}
		combined = val1 / val2
	}
	return Pair{Value: mulExact(combined, factor), Code: code}, nil
}

// zeroDivisorValue reports a divisor whose *value* is zero, as opposed to a unit
// expression that cancels to zero. Both are ErrDivisionByZero, since a caller
// asking "was this a division by zero?" means the same question either way, but
// the message says which one happened.
func zeroDivisorValue(divisor Pair) error {
	return &ValidationError{
		Code:    divisor.Code,
		Message: fmt.Sprintf("the divisor has a value of zero (%v %s)", divisor.Value, divisor.Code),
		Offset:  -1,
		Err:     ErrDivisionByZero,
	}
}

// mappedRats maps both operands onto their canonical scales exactly, without the
// canonical factors. It returns nil results when an operand is not finite, and
// ErrNotRational when a scale has no rational form.
func (s *service) mappedRats(t1, t2 *term, v1, v2 Pair) (m1, m2 *big.Rat, err error) {
	r1, r2 := ratFromFloat(v1.Value), ratFromFloat(v2.Value)
	if r1 == nil || r2 == nil {
		return nil, nil, nil
	}
	m1, err = s.toCanonicalRat(t1, v1.Code, r1)
	if err != nil {
		return nil, nil, err
	}
	m2, err = s.toCanonicalRat(t2, v2.Code, r2)
	if err != nil {
		return nil, nil, err
	}
	return m1, m2, nil
}

// canonicalOfDefinition canonicalizes a code that came from the definitions
// rather than from a caller, so it resolves case-sensitively whatever vocabulary
// the service was built for.
func (s *service) canonicalOfDefinition(code string) (*canonical, error) {
	t, err := s.parseDefinition(code)
	if err != nil {
		return nil, err
	}
	return s.canonicalizeTerm(t, s.specialContextOf(t))
}

// getCanonical computes the canonical form of a UCUM code.
func (s *service) getCanonical(code string) (*canonical, error) {
	t, err := s.parseCached(code)
	if err != nil {
		return nil, err
	}
	return s.canonicalizeTerm(t, s.specialContextOf(t))
}

// canonicalizeTerm recursively converts a term AST into canonical form.
func (s *service) canonicalizeTerm(t *term, ctx specialContext) (*canonical, error) {
	if t == nil {
		return &canonical{value: decimalFromInt(1)}, nil
	}

	left, err := s.canonicalizeComponent(t.comp, ctx)
	if err != nil {
		return nil, err
	}

	if t.term == nil {
		return left, nil
	}

	right, err := s.canonicalizeTerm(t.term, ctx)
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
func (s *service) canonicalizeComponent(c component, ctx specialContext) (*canonical, error) {
	switch v := c.(type) {
	case *factor:
		return &canonical{value: decimalFromInt(int64(v.value))}, nil
	case *symbol:
		return s.canonicalizeSymbol(v, ctx)
	case *term:
		return s.canonicalizeTerm(v, ctx)
	default:
		return nil, fmt.Errorf("unexpected component type %T", c)
	}
}

// canonicalizeSymbol converts a symbol to its canonical form by recursively
// expanding the unit's definition.
func (s *service) canonicalizeSymbol(sym *symbol, ctx specialContext) (*canonical, error) {
	u := sym.unit

	// Start with the prefix value (or 1 if no prefix).
	prefixVal := prefixValue(sym)

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

	// Arbitrary unit: its own dimension, never reducible to a number. Expanding
	// it through its definition would make [IU] compare equal to [iU], to mol
	// and to 1, and convert between them with a meaningless factor.
	if u.IsArbitrary {
		bu := s.arbitraryBases[u.Code]
		if bu == nil {
			return nil, fmt.Errorf("arbitrary unit %q not found", u.Code)
		}
		return &canonical{
			value: prefixVal.pow(sym.exponent),
			units: []canonicalUnit{{base: bu, exponent: sym.exponent}},
		}, nil
	}

	// Defined unit: expand through its value expression.
	if u.Value == nil {
		return nil, fmt.Errorf("unit %q has no value definition", u.Code)
	}

	unitExpr := u.Value.unit
	unitValue := u.Value.Value

	if u.IsSpecial {
		h, ok := s.handlers[u.Code]
		if !ok {
			return nil, fmt.Errorf("no handler for special unit %q", u.Code)
		}
		use := specialUse{handler: h, alpha: prefixVal}

		// A special unit's factor is the canonical form of the reference quantity
		// its definition names: Cel is cel(1 K) so the factor is 1, degF is
		// degf(5 K/9) so it is 5/9. Reading it from the definitions is what makes
		// a gradient in [degF] differ from one in Cel — hardcoding 1 for every
		// special unit made them identical, and wrong by a factor of 1.8.
		unitExpr = u.Value.Function.Reference()
		unitValue = decimalFromInt(1)

		// A handler that produces its result in a fixed unit is scaled by that
		// unit, not by the declared reference: atan yields radians even when the
		// definition names deg.
		if native, ok := use.nativeUnit(); ok {
			unitExpr = native
		}

		switch ctx {
		case specialStandalone:
			// The prefix belongs to the argument of the conversion function, not
			// to the canonical factor: UCUM §22.4 defines a scaled special unit
			// as x = f_s-1(α x'). specialUse applies α there, so it must not be
			// applied a second time here — doing both is what moved the origin of
			// the scale with the prefix, making 0 mCel 0.27315 K instead of
			// 273.15 K, and 1 dB a factor of 1 instead of 10^0.1.
			prefixVal = decimalFromInt(1)
		case specialAsDelta:
			// In an algebraic term the unit denotes a difference: the offset
			// cancels, the scale does not. A prefix scales that difference in the
			// ordinary way — Δ(αx) = αΔx — so prefixVal stays as it is.
			if sym.exponent != 1 {
				return nil, fmt.Errorf("special unit %q cannot carry an exponent (%d): it denotes a point on its own scale",
					u.Code, sym.exponent)
			}
			if !use.isLinear() {
				return nil, fmt.Errorf("special unit %q is not on a linear scale, so it cannot appear in an algebraic term",
					u.Code)
			}
		}
	}

	// Parse and canonicalize the unit's value expression.
	if unitExpr == "" || unitExpr == "1" {
		// Dimensionless unit.
		val := prefixVal.mul(unitValue).pow(sym.exponent)
		return &canonical{value: val}, nil
	}

	inner, err := s.parseDefinition(unitExpr)
	if err != nil {
		return nil, fmt.Errorf("expand unit %q: %w", u.Code, err)
	}

	// The reference expression of a special unit contains no special units of its
	// own, so it canonicalizes in the ordinary way.
	can, err := s.canonicalizeTerm(inner, specialStandalone)
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

// findBaseUnit looks up a baseUnit by code.
func (s *service) findBaseUnit(code string) *baseUnit {
	return s.baseByCode[code]
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
		return nil, ErrDivisionByZero
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

// Human-readable analysis.

// unityDisplayName is what the official suite expects for an empty expression.
const unityDisplayName = "(unity)"

// displayName renders a term in the display format of the official UCUM test
// suite: every unit is parenthesised with its full name, a prefix is
// concatenated onto that name ("mm" is "(millimeter)"), an exponent other than
// 1 is written inside the parentheses as " ^ n", numeric factors appear bare,
// and the operators are " * " and " / ".
func displayName(t *term) string {
	if t == nil {
		return unityDisplayName
	}
	var sb strings.Builder
	displayTermTo(&sb, t)
	return sb.String()
}

func displayTermTo(sb *strings.Builder, t *term) {
	displayComponentTo(sb, t.comp)
	if t.term != nil {
		if t.op == opDivision {
			sb.WriteString(" / ")
		} else {
			sb.WriteString(" * ")
		}
		displayTermTo(sb, t.term)
	}
}

func displayComponentTo(sb *strings.Builder, c component) {
	switch v := c.(type) {
	case *factor:
		fmt.Fprintf(sb, "%d", v.value)
	case *symbol:
		sb.WriteString("(")
		if v.prefix != nil {
			sb.WriteString(v.prefix.Name)
		}
		sb.WriteString(v.unit.Name)
		if v.exponent != 1 {
			fmt.Fprintf(sb, " ^ %d", v.exponent)
		}
		sb.WriteString(")")
	case *term:
		// Rendered without extra parentheses: the AST does not distinguish a
		// group written in the source from the parser's own nesting, and adding
		// them here would bracket every operator. composeTerm makes the same
		// choice, so the two renderings stay consistent.
		displayTermTo(sb, v)
	}
}

// Special unit detection.

// specialUseForTerm returns the special unit a term denotes, together with the
// scale factor of its prefix, and nil if the term is not a lone special symbol.
// Anything else puts the unit into an algebraic relationship, where it denotes a
// difference and the handler's offset does not apply.
//
// Redundant parentheses are looked through first. UCUM §22.1-2 says a special
// unit "cannot take part in any algebraic operations", and a parenthesis is not
// an operation: "(Cel)" is the same code as "Cel" and has to mean the same
// thing. The AST cannot tell a group written in the source from the parser's own
// nesting, so without this "(Cel)" would fall through to the difference reading
// and lose the 273.15 offset silently.
func (s *service) specialUseForTerm(t *term) *specialUse {
	sym := loneSymbol(t)
	if sym == nil || !sym.unit.IsSpecial || sym.exponent != 1 {
		return nil
	}
	h, ok := s.handlers[sym.unit.Code]
	if !ok {
		return nil
	}
	return &specialUse{handler: h, alpha: prefixValue(sym)}
}

// loneSymbol returns the single symbol a term denotes, looking through any
// number of redundant parentheses, and nil if the term is anything else.
func loneSymbol(t *term) *symbol {
	for t != nil && t.term == nil {
		switch comp := t.comp.(type) {
		case *symbol:
			return comp
		case *term:
			t = comp
		default:
			return nil
		}
	}
	return nil
}

// prefixValue returns the scale factor of a symbol's prefix, or 1 if it has
// none.
func prefixValue(sym *symbol) decimal {
	if sym.prefix == nil {
		return decimalFromInt(1)
	}
	return sym.prefix.Value
}
