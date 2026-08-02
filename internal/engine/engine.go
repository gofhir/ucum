// Package engine canonicalizes and converts UCUM codes.
//
// It is where the arithmetic lives: expanding a unit through its definition,
// reducing an expression to base units, applying the conversion a special unit
// performs, and doing all of it in exact rational arithmetic so that a float64
// result is rounded once at the end.
//
// The root package wraps it as Service and ExactService and adds nothing but the
// interfaces and constructors.
package engine

import (
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"
	"sync"

	"github.com/gofhir/ucum/v4/internal/decimal"
	"github.com/gofhir/ucum/v4/internal/essence"
	"github.com/gofhir/ucum/v4/internal/expr"
	"github.com/gofhir/ucum/v4/internal/model"
	"github.com/gofhir/ucum/v4/internal/special"

	"github.com/gofhir/ucum/v4/internal/ucumerr"
)

// Service is the concrete implementation of Service.
type Service struct {
	model *model.Model

	// parser resolves codes supplied by a caller, in whichever of UCUM's two
	// vocabularies this service was built for.
	parser *expr.Parser
	cache  *termCache

	// defParser resolves the expressions inside the definitions, which are always
	// written in case-sensitive codes: the year is defined as "a_j", not as its
	// case-insensitive spelling "ANN_J". Expanding a definition therefore never
	// uses the caller's vocabulary.
	//
	// defCache is unbounded, unlike cache, and can be: its keys come from the
	// definitions, so there are as many as ucum-essence.xml has unit expressions
	// and no caller can add to them.
	defParser *expr.Parser
	defCache  sync.Map // map[string]*expr.Term

	// baseByCode indexes the seven base units, and arbitraryBases gives every
	// arbitrary unit its own dimension. Both are populated once at construction
	// and read-only afterwards.
	baseByCode     map[string]*model.BaseUnit
	arbitraryBases map[string]*model.BaseUnit

	// codesByProperty maps a lower-cased property name to the codes of the units
	// that declare it. Built at construction; the canonical forms behind them
	// are resolved lazily and memoized in propertyForms.
	codesByProperty map[string][]string
	propertyForms   sync.Map // map[string]map[string]bool

	// handlers holds one conversion per special unit, built from the function
	// each definition names. Read-only after construction.
	handlers map[string]special.Handler
}

// New creates a fully wired service.
//
// It reads the definitions from r, or the embedded ucum-essence.xml if r is nil,
// and resolves codes in one of UCUM's two vocabularies: the case-sensitive one,
// which is what FHIR uses, or the case-insensitive one.
func New(r io.Reader, insensitive bool) (*Service, error) {
	m, err := essence.Load(r)
	if err != nil {
		return nil, fmt.Errorf("ucum: load definitions: %w", err)
	}
	bases := make(map[string]*model.BaseUnit, len(m.BaseUnits))
	for _, bu := range m.BaseUnits {
		bases[bu.Code] = bu
	}

	// UCUM makes arbitrary units commensurable with nothing, so each one gets a
	// synthetic base unit standing for its own dimension.
	arbitrary := make(map[string]*model.BaseUnit)
	for _, du := range m.DefinedUnits {
		if du.IsArbitrary {
			arbitrary[du.Code] = &model.BaseUnit{
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

	handlers, err := special.BuildHandlers(m)
	if err != nil {
		return nil, fmt.Errorf("ucum: %w", err)
	}

	return &Service{
		model:           m,
		parser:          expr.NewParserFor(m, insensitive),
		cache:           newTermCache(),
		defParser:       expr.NewParser(m),
		baseByCode:      bases,
		arbitraryBases:  arbitrary,
		codesByProperty: codesByProperty,
		handlers:        handlers,
	}, nil
}

// parseCached parses a UCUM code, caching the result.
func (s *Service) parseCached(code string) (*expr.Term, error) {
	if t, ok := s.cache.load(code); ok {
		return t, nil
	}
	t, err := s.parser.Parse(code)
	if err != nil {
		return nil, err
	}
	s.cache.store(code, t)
	return t, nil
}

// Definitions reports which UCUM release these definitions declare themselves
// to be.
func (s *Service) Definitions() Definitions {
	return Definitions{
		Version:      s.model.Version,
		Revision:     s.model.Revision,
		RevisionDate: s.model.RevisionDate,
	}
}

// parseDefinition parses an expression taken from the definitions, always in the
// case-sensitive vocabulary, and caches the result separately from caller input.
func (s *Service) parseDefinition(text string) (*expr.Term, error) {
	if v, ok := s.defCache.Load(text); ok {
		t, ok := v.(*expr.Term)
		if !ok {
			return nil, fmt.Errorf("ucum: unexpected cache entry type %T", v)
		}
		return t, nil
	}
	t, err := s.defParser.Parse(text)
	if err != nil {
		return nil, err
	}
	s.defCache.Store(text, t)
	return t, nil
}

// Validate checks if the given code is a valid UCUM expression.
func (s *Service) Validate(code string) error {
	_, err := s.parseCached(code)
	return ucumerr.Validation(code, err)
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

// Canonical returns the canonical (base-unit) form of a value+code pair.
func (s *Service) Canonical(value float64, code string) (Pair, error) {
	v, can, err := s.canonicalScalar(value, code)
	if err != nil {
		return Pair{}, ucumerr.Validation(code, err)
	}
	return Pair{Value: v, Code: expr.ComposeCanonicalUnits(can)}, nil
}

// canonicalScalar maps value onto the canonical scale of code and scales it by
// the canonical factor, rounding once.
func (s *Service) canonicalScalar(value float64, code string) (float64, *model.Canonical, error) {
	t, can, err := s.canonicalParts(code)
	if err != nil {
		return 0, nil, err
	}

	// Preferred path: map and scale entirely in exact arithmetic.
	if rv := decimal.RatFromFloat(value); rv != nil {
		mapped, err := s.toCanonicalRat(t, code, rv)
		switch {
		case err == nil:
			out, _ := mapped.Mul(mapped, can.Value.Rat()).Float64()
			return out, can, nil
		case !errors.Is(err, ErrNotRational):
			return 0, nil, err
		}
	}

	// Non-rational scale or non-finite value: use the float64 handler.
	return decimal.MulExact(s.mapFloat(t, value), can.Value.Rat()), can, nil
}

// mapFloat maps value onto the canonical scale of an already-resolved expr.Term,
// using the float64 handler. It serves the scales that have no exact rational
// form; everything else goes through toCanonicalRat.
//
// Special units sit on non-ratio scales, so the multiplicative factor alone
// does not describe them: the handler has to map the value first (Cel adds an
// offset, [pH] exponentiates, B[V] takes a power). Skipping it would silently
// return the raw input value, making canonical forms non-comparable.
func (s *Service) mapFloat(t *expr.Term, value float64) float64 {
	if use := s.specialUseForTerm(t); use != nil {
		return use.ToCanonical(value)
	}
	return value
}

// specialContext says how a special unit in a term is to be read.
//
// Standalone it denotes a point on its own scale, and its handler applies the
// whole mapping including any offset: 1 Cel is 274.15 K. Inside an algebraic expr.Term
// it denotes a difference — a gradient of 1 [degF]/min is a rate of change, not a
// temperature — so the offset cancels while the scale, which the definitions
// give, still applies.
type specialContext int

const (
	specialStandalone specialContext = iota
	specialAsDelta
)

// specialContextOf reports how special units in this expr.Term are to be read. A expr.Term
// that is a lone special symbol is standalone; anything else puts its units into
// an algebraic relationship.
func (s *Service) specialContextOf(t *expr.Term) specialContext {
	if s.specialUseForTerm(t) != nil {
		return specialStandalone
	}
	return specialAsDelta
}

// canonicalParts parses a code and canonicalizes it, returning both the AST
// (needed to detect special units) and the canonical form.
func (s *Service) canonicalParts(code string) (*expr.Term, *model.Canonical, error) {
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
func (s *Service) Convert(value float64, from, to string) (float64, error) {
	srcTerm, err := s.parseCached(from)
	if err != nil {
		return 0, ucumerr.Conversion(from, to, err)
	}
	dstTerm, err := s.parseCached(to)
	if err != nil {
		return 0, ucumerr.Conversion(from, to, err)
	}

	srcCan, err := s.getCanonical(from)
	if err != nil {
		return 0, ucumerr.Conversion(from, to, err)
	}
	dstCan, err := s.getCanonical(to)
	if err != nil {
		return 0, ucumerr.Conversion(from, to, err)
	}

	// Check comparability: canonical unit strings must match.
	if err := requireComparable(from, to, srcCan, dstCan); err != nil {
		return 0, err
	}

	// A zero destination factor ("0", "m/0" cancels to it) would divide by zero.
	if dstCan.Value.IsZero() {
		return 0, ucumerr.Conversion(from, to, ucumerr.ErrDivisionByZero)
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
	if rv := decimal.RatFromFloat(value); rv != nil {
		exact, err := s.convertRatCore(rv, parts)
		if err == nil {
			out, _ := exact.Float64()
			return out, nil
		}
		if !errors.Is(err, ErrNotRational) {
			return 0, ucumerr.Conversion(from, to, err)
		}
		// Non-rational scale: fall through to the float64 handlers below.
	}

	result := value

	// Step 1: If source is special, convert value to canonical first.
	if srcUse := s.specialUseForTerm(srcTerm); srcUse != nil {
		result = srcUse.ToCanonical(result)
	}

	// Step 2: Multiplicative conversion, exact factor with a single rounding.
	result = decimal.MulExact(result, new(big.Rat).Quo(srcCan.Value.Rat(), dstCan.Value.Rat()))

	// Step 3: If dest is special, convert from canonical.
	if dstUse := s.specialUseForTerm(dstTerm); dstUse != nil {
		result = dstUse.FromCanonical(result)
	}

	return result, nil
}

// IsComparable returns true if the two unit codes have the same canonical units.
func (s *Service) IsComparable(code1, code2 string) (bool, error) {
	can1, err := s.getCanonical(code1)
	if err != nil {
		return false, ucumerr.Validation(code1, err)
	}
	can2, err := s.getCanonical(code2)
	if err != nil {
		return false, ucumerr.Validation(code2, err)
	}
	return expr.ComposeCanonicalUnits(can1) == expr.ComposeCanonicalUnits(can2), nil
}

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

// Multiply multiplies two value/unit pairs.
func (s *Service) Multiply(v1, v2 Pair) (Pair, error) {
	return s.combine(v1, v2, expr.OpMultiplication)
}

// Divide divides two value/unit pairs.
func (s *Service) Divide(v1, v2 Pair) (Pair, error) {
	return s.combine(v1, v2, expr.OpDivision)
}

// combine is the shared body of Multiply and Divide. Both map each operand onto
// its canonical scale, combine the operands, combine the canonical factors and
// round once; they differ in the operator and in division having to reject a
// zero divisor.
func (s *Service) combine(v1, v2 Pair, op expr.Operator) (Pair, error) {
	t1, can1, err := s.canonicalParts(v1.Code)
	if err != nil {
		return Pair{}, ucumerr.Validation(v1.Code, err)
	}
	t2, can2, err := s.canonicalParts(v2.Code)
	if err != nil {
		return Pair{}, ucumerr.Validation(v2.Code, err)
	}

	div := op == expr.OpDivision
	if div && can2.Value.IsZero() {
		return Pair{}, ucumerr.Validation(v2.Code, ucumerr.ErrDivisionByZero)
	}

	sign := 1
	factor := can1.Value.Rat()
	if div {
		sign = -1
		factor.Quo(factor, can2.Value.Rat())
	} else {
		factor.Mul(factor, can2.Value.Rat())
	}
	code := expr.ComposeCanonicalUnits(&model.Canonical{Units: mergeUnitLists(can1.Units, can2.Units, sign)})

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
		return Pair{}, ucumerr.Validation(v1.Code, err)
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
	return Pair{Value: decimal.MulExact(combined, factor), Code: code}, nil
}

// zeroDivisorValue reports a divisor whose *value* is zero, as opposed to a unit
// expression that cancels to zero. Both are ucumerr.ErrDivisionByZero, since a caller
// asking "was this a division by zero?" means the same question either way, but
// the message says which one happened.
func zeroDivisorValue(divisor Pair) error {
	return &ucumerr.ValidationError{
		Code:    divisor.Code,
		Message: fmt.Sprintf("the divisor has a value of zero (%v %s)", divisor.Value, divisor.Code),
		Offset:  -1,
		Err:     ucumerr.ErrDivisionByZero,
	}
}

// mappedRats maps both operands onto their canonical scales exactly, without the
// canonical factors. It returns nil results when an operand is not finite, and
// ErrNotRational when a scale has no rational form.
func (s *Service) mappedRats(t1, t2 *expr.Term, v1, v2 Pair) (m1, m2 *big.Rat, err error) {
	r1, r2 := decimal.RatFromFloat(v1.Value), decimal.RatFromFloat(v2.Value)
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
func (s *Service) canonicalOfDefinition(code string) (*model.Canonical, error) {
	t, err := s.parseDefinition(code)
	if err != nil {
		return nil, err
	}
	return s.canonicalizeTerm(t, s.specialContextOf(t))
}

// getCanonical computes the canonical form of a UCUM code.
func (s *Service) getCanonical(code string) (*model.Canonical, error) {
	t, err := s.parseCached(code)
	if err != nil {
		return nil, err
	}
	return s.canonicalizeTerm(t, s.specialContextOf(t))
}

// canonicalizeTerm recursively converts a term AST into canonical form.
func (s *Service) canonicalizeTerm(t *expr.Term, ctx specialContext) (*model.Canonical, error) {
	if t == nil {
		return &model.Canonical{Value: decimal.FromInt(1)}, nil
	}

	left, err := s.canonicalizeComponent(t.Comp, ctx)
	if err != nil {
		return nil, err
	}

	if t.Term == nil {
		return left, nil
	}

	right, err := s.canonicalizeTerm(t.Term, ctx)
	if err != nil {
		return nil, err
	}

	switch t.Op {
	case expr.OpMultiplication:
		return multiplyCanonicals(left, right), nil
	case expr.OpDivision:
		return divideCanonicals(left, right)
	default:
		return nil, fmt.Errorf("unknown operator %d", t.Op)
	}
}

// canonicalizeComponent converts a single expr.Component to canonical form.
func (s *Service) canonicalizeComponent(c expr.Component, ctx specialContext) (*model.Canonical, error) {
	switch v := c.(type) {
	case *expr.Factor:
		return &model.Canonical{Value: decimal.FromInt(int64(v.Value))}, nil
	case *expr.Symbol:
		return s.canonicalizeSymbol(v, ctx)
	case *expr.Term:
		return s.canonicalizeTerm(v, ctx)
	default:
		return nil, fmt.Errorf("unexpected component type %T", c)
	}
}

// canonicalizeSymbol converts a symbol to its canonical form by recursively
// expanding the unit's definition.
func (s *Service) canonicalizeSymbol(sym *expr.Symbol, ctx specialContext) (*model.Canonical, error) {
	u := sym.Unit

	// Start with the prefix value (or 1 if no prefix).
	prefixVal := prefixValue(sym)

	if u.IsBase {
		// Base unit: canonical is itself.
		bu := s.findBaseUnit(u.Code)
		if bu == nil {
			return nil, fmt.Errorf("base unit %q not found", u.Code)
		}
		val := prefixVal.Pow(sym.Exponent)
		return &model.Canonical{
			Value: val,
			Units: []model.CanonicalUnit{{Base: bu, Exponent: sym.Exponent}},
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
		return &model.Canonical{
			Value: prefixVal.Pow(sym.Exponent),
			Units: []model.CanonicalUnit{{Base: bu, Exponent: sym.Exponent}},
		}, nil
	}

	// Defined unit: expand through its value expression.
	if u.Value == nil {
		return nil, fmt.Errorf("unit %q has no value definition", u.Code)
	}

	unitExpr := u.Value.Unit
	unitValue := u.Value.Value

	if u.IsSpecial {
		h, ok := s.handlers[u.Code]
		if !ok {
			return nil, fmt.Errorf("no handler for special unit %q", u.Code)
		}
		use := special.Use{Handler: h, Alpha: prefixVal}

		// A special unit's factor is the canonical form of the reference quantity
		// its definition names: Cel is cel(1 K) so the factor is 1, degF is
		// degf(5 K/9) so it is 5/9. Reading it from the definitions is what makes
		// a gradient in [degF] differ from one in Cel — hardcoding 1 for every
		// special unit made them identical, and wrong by a factor of 1.8.
		unitExpr = u.Value.Function.Reference()
		unitValue = decimal.FromInt(1)

		// A handler that produces its result in a fixed unit is scaled by that
		// unit, not by the declared reference: atan yields radians even when the
		// definition names deg.
		if native, ok := use.NativeUnit(); ok {
			unitExpr = native
		}

		switch ctx {
		case specialStandalone:
			// The prefix belongs to the argument of the conversion function, not
			// to the canonical factor: UCUM §22.4 defines a scaled special unit
			// as x = f_s-1(α x'). special.Use applies α there, so it must not be
			// applied a second time here — doing both is what moved the origin of
			// the scale with the prefix, making 0 mCel 0.27315 K instead of
			// 273.15 K, and 1 dB a factor of 1 instead of 10^0.1.
			prefixVal = decimal.FromInt(1)
		case specialAsDelta:
			// In an algebraic expr.Term the unit denotes a difference: the offset
			// cancels, the scale does not. A prefix scales that difference in the
			// ordinary way — Δ(αx) = αΔx — so prefixVal stays as it is.
			if sym.Exponent != 1 {
				return nil, fmt.Errorf("special unit %q cannot carry an exponent (%d): it denotes a point on its own scale",
					u.Code, sym.Exponent)
			}
			if !use.IsLinear() {
				return nil, fmt.Errorf("special unit %q is not on a linear scale, so it cannot appear in an algebraic expr.Term",
					u.Code)
			}
		}
	}

	// Parse and canonicalize the unit's value expression.
	if unitExpr == "" || unitExpr == "1" {
		// Dimensionless unit.
		val := prefixVal.Mul(unitValue).Pow(sym.Exponent)
		return &model.Canonical{Value: val}, nil
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
	can.Value = prefixVal.Mul(unitValue).Mul(can.Value)

	// Apply exponent.
	if sym.Exponent != 1 {
		can.Value = can.Value.Pow(sym.Exponent)
		for i := range can.Units {
			can.Units[i].Exponent *= sym.Exponent
		}
	}

	return can, nil
}

// findBaseUnit looks up a model.BaseUnit by code.
func (s *Service) findBaseUnit(code string) *model.BaseUnit {
	return s.baseByCode[code]
}

// Canonical arithmetic helpers.

func multiplyCanonicals(left, right *model.Canonical) *model.Canonical {
	result := &model.Canonical{
		Value: left.Value.Mul(right.Value),
		Units: mergeUnitLists(left.Units, right.Units, 1),
	}
	return result
}

func divideCanonicals(left, right *model.Canonical) (*model.Canonical, error) {
	// A zero factor is syntactically valid ("m/0"), so the divisor has to be
	// checked here rather than left to big.Rat, which panics.
	if right.Value.IsZero() {
		return nil, ucumerr.ErrDivisionByZero
	}
	result := &model.Canonical{
		Value: left.Value.Div(right.Value),
		Units: mergeUnitLists(left.Units, right.Units, -1),
	}
	return result, nil
}

// mergeUnitLists merges two canonical unit lists. The sign parameter is
// applied to the exponents of the right list (+1 for multiply, -1 for divide).
func mergeUnitLists(left, right []model.CanonicalUnit, sign int) []model.CanonicalUnit {
	result := make([]model.CanonicalUnit, len(left))
	copy(result, left)

	for _, ru := range right {
		found := false
		for i := range result {
			if result[i].Base.Code == ru.Base.Code {
				result[i].Exponent += ru.Exponent * sign
				found = true
				break
			}
		}
		if !found {
			result = append(result, model.CanonicalUnit{
				Base:     ru.Base,
				Exponent: ru.Exponent * sign,
			})
		}
	}

	// Remove zero-exponent units.
	filtered := result[:0]
	for _, u := range result {
		if u.Exponent != 0 {
			filtered = append(filtered, u)
		}
	}
	return filtered
}

// Human-readable analysis.

// unityDisplayName is what the official suite expects for an empty expression.
const unityDisplayName = "(unity)"

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
func (s *Service) specialUseForTerm(t *expr.Term) *special.Use {
	sym := expr.LoneSymbol(t)
	if sym == nil || !sym.Unit.IsSpecial || sym.Exponent != 1 {
		return nil
	}
	h, ok := s.handlers[sym.Unit.Code]
	if !ok {
		return nil
	}
	return &special.Use{Handler: h, Alpha: prefixValue(sym)}
}

// prefixValue returns the scale factor of a symbol's prefix, or 1 if it has
// none.
func prefixValue(sym *expr.Symbol) decimal.Decimal {
	if sym.Prefix == nil {
		return decimal.FromInt(1)
	}
	return sym.Prefix.Value
}
