package engine

import (
	"errors"
	"fmt"
	"math"

	"github.com/gofhir/ucum/v4/internal/decimal"
	"github.com/gofhir/ucum/v4/internal/expr"
	"github.com/gofhir/ucum/v4/internal/model"
	"github.com/gofhir/ucum/v4/internal/special"
	"github.com/gofhir/ucum/v4/internal/ucumerr"
)

// Canonicalization: reducing a parsed code to base units and a scalar factor.
//
// This is where a unit is expanded through its definition, where an arbitrary
// unit is given its own dimension, and where a special unit's reference quantity
// is read out of the definitions.

// cycleError es el error de una definición que vuelve a sí misma. Es un tipo
// propio para que cada nivel de la expansión lo deje pasar sin envolverlo otra
// vez: el mensaje nombra la unidad y no repite la cadena entera.
type cycleError struct{ code string }

// exponentError es el error de un exponente canónico que no cabe en un int.
// Como cycleError, wrapExpand no lo envuelve otra vez: en una cadena de cientos
// de definiciones el mensaje no repite «expand unit» por nivel. Es
// ucumerr.ErrExponentTooLarge para errors.Is. El primer wrapExpand anota la
// unidad cuya definición desbordó, como cycleError.
type exponentError struct{ code string }

func (e *exponentError) Error() string {
	msg := ucumerr.ErrExponentTooLarge.Error() + ": a canonical exponent does not fit in an int"
	if e.code == "" {
		return msg
	}
	return fmt.Sprintf("expand unit %q: %s", e.code, msg)
}

func (*exponentError) Unwrap() error { return ucumerr.ErrExponentTooLarge }

func (e *cycleError) Error() string {
	return fmt.Sprintf("unit %q is part of or leads to a definition cycle", e.code)
}

// wrapExpand envuelve el error de expandir la definición de code, salvo el de
// un ciclo, que ya viene completo.
func wrapExpand(code string, err error) error {
	var ce *cycleError
	var ee *exponentError
	if errors.As(err, &ce) {
		return err
	}
	if errors.As(err, &ee) {
		// Cada desborde crea su propio exponentError: anotarlo no toca el de
		// otra llamada.
		if ee.code == "" {
			ee.code = code
		}
		return err
	}
	return fmt.Errorf("expand unit %q: %w", code, err)
}

// canonicalOfDefinition canonicalizes a code that came from the definitions
// rather than from a caller, so it resolves case-sensitively whatever vocabulary
// the service was built for.
func (s *Service) canonicalOfDefinition(code string) (*model.Canonical, error) {
	t, err := s.parseDefinition(code)
	if err != nil {
		return nil, err
	}
	return s.canonicalizeTerm(t, s.specialContextOf(t), 0)
}

// getCanonical computes the canonical form of a UCUM code.
func (s *Service) getCanonical(code string) (*model.Canonical, error) {
	t, err := s.parseCached(code)
	if err != nil {
		return nil, err
	}
	return s.canonicalizeTerm(t, s.specialContextOf(t), 0)
}

// canonicalizeTerm recursively converts a term AST into canonical form.
func (s *Service) canonicalizeTerm(t *expr.Term, ctx specialContext, depth int) (*model.Canonical, error) {
	if t == nil {
		return &model.Canonical{Value: decimal.FromInt(1)}, nil
	}

	left, err := s.canonicalizeComponent(t.Comp, ctx, depth)
	if err != nil {
		return nil, err
	}

	if t.Term == nil {
		return left, nil
	}

	right, err := s.canonicalizeTerm(t.Term, ctx, depth)
	if err != nil {
		return nil, err
	}

	switch t.Op {
	case expr.OpMultiplication:
		return multiplyCanonicals(left, right)
	case expr.OpDivision:
		return divideCanonicals(left, right)
	default:
		return nil, fmt.Errorf("unknown operator %d", t.Op)
	}
}

// canonicalizeComponent converts a single expr.Component to canonical form.
func (s *Service) canonicalizeComponent(c expr.Component, ctx specialContext, depth int) (*model.Canonical, error) {
	switch v := c.(type) {
	case *expr.Factor:
		return &model.Canonical{Value: decimal.FromInt(int64(v.Value))}, nil
	case *expr.Symbol:
		return s.canonicalizeSymbol(v, ctx, depth)
	case *expr.Term:
		return s.canonicalizeTerm(v, ctx, depth)
	default:
		return nil, fmt.Errorf("unexpected component type %T", c)
	}
}

// canonicalizeSymbol converts a symbol to its canonical form by recursively
// expanding the unit's definition.
func (s *Service) canonicalizeSymbol(sym *expr.Symbol, ctx specialContext, depth int) (*model.Canonical, error) {
	u := sym.Unit

	// Start with the prefix value (or 1 if no prefix).
	prefixVal := prefixValue(sym)

	if u.IsBase {
		// Base unit: canonical is itself.
		return s.baseCanonical(u, prefixVal, sym.Exponent)
	}

	// Arbitrary unit: its own dimension, never reducible to a number. Expanding
	// it through its definition would make [IU] compare equal to [iU], to mol
	// and to 1, and convert between them with a meaningless factor.
	if u.IsArbitrary {
		bu := s.arbitraryBases[u.Code]
		if bu == nil {
			return nil, fmt.Errorf("arbitrary unit %q not found", u.Code)
		}
		val, err := powValue(prefixVal, sym.Exponent)
		if err != nil {
			return nil, err
		}
		return &model.Canonical{
			Value: val,
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
		return dimensionlessCanonical(prefixVal.Mul(unitValue), sym.Exponent)
	}

	// The reference expression of a special unit contains no special units of its
	// own, so it canonicalizes in the ordinary way.
	can, err := s.expandDefinition(u.Code, unitExpr, depth)
	if err != nil {
		return nil, wrapExpand(u.Code, err)
	}

	// Multiply by the unit's numeric value and the prefix.
	can.Value = prefixVal.Mul(unitValue).Mul(can.Value)

	// Apply exponent.
	if err := raiseCanonical(can, sym.Exponent); err != nil {
		return nil, err
	}

	return can, nil
}

// expandDefinition is the canonical form of the definition expression text of
// the unit code, expanded once per service. The result depends only on the text (always
// specialStandalone, always in the case-sensitive vocabulary), so a definition
// reached by many paths is not expanded once per path.
//
// The cache holds its own copy and every caller gets another: callers mutate
// what they receive (Value and the exponents of Units), and a shared Units
// slice would carry one caller's exponent into the next. Errors are not stored:
// a cycle never completes, and an error stops the expansion at the first path.
func (s *Service) expandDefinition(code, text string, depth int) (*model.Canonical, error) {
	if v, ok := s.defCanonical.Load(text); ok {
		if c, isCan := v.(*model.Canonical); isCan {
			return c.Clone(), nil
		}
	}
	// Una definición que vuelve a sí misma ([pi] definido en [pi], o [pi2] y [pi]
	// una en la otra) nunca termina de expandirse: sin esta guarda desborda la
	// pila, y recover no atrapa ese error. Una cadena sin ciclo expande cada
	// unidad definida a lo más una vez, así que no pasa de maxDepth niveles; la
	// profundidad se lleva por valor y no asigna memoria. La guarda va después
	// del caché: un acierto no baja, y un subárbol sano ya expandido no cuenta
	// para la profundidad, así que el error nombra una unidad del ciclo y no
	// depende del orden de las llamadas. Un ciclo nunca se guarda.
	if depth >= s.maxDepth {
		return nil, &cycleError{code: code}
	}
	inner, err := s.parseDefinition(text)
	if err != nil {
		return nil, err
	}
	can, err := s.canonicalizeTerm(inner, specialStandalone, depth+1)
	if err != nil {
		return nil, err
	}
	s.defCanonical.Store(text, can.Clone())
	return can, nil
}

// baseCanonical is the canonical form of a base unit under a prefix value and
// an exponent: the unit itself.
func (s *Service) baseCanonical(u *model.Unit, prefixVal decimal.Decimal, n int) (*model.Canonical, error) {
	bu := s.findBaseUnit(u.Code)
	if bu == nil {
		return nil, fmt.Errorf("base unit %q not found", u.Code)
	}
	val, err := powValue(prefixVal, n)
	if err != nil {
		return nil, err
	}
	return &model.Canonical{
		Value: val,
		Units: []model.CanonicalUnit{{Base: bu, Exponent: n}},
	}, nil
}

// dimensionlessCanonical is the canonical form of v^n with no base units.
func dimensionlessCanonical(v decimal.Decimal, n int) (*model.Canonical, error) {
	val, err := powValue(v, n)
	if err != nil {
		return nil, err
	}
	return &model.Canonical{Value: val}, nil
}

// raiseCanonical raises a canonical form to the power n in place.
func raiseCanonical(can *model.Canonical, n int) error {
	if n == 1 {
		return nil
	}
	// Los exponentes nuevos se comprueban antes de la potencia del valor y antes
	// de tocar can.Units: un exponente que no cabe no paga la potencia, y el
	// error no deja la forma canónica a medio cambiar. Comprobar y luego
	// multiplicar evita un slice auxiliar por llamada.
	for _, u := range can.Units {
		if _, err := mulExponent(u.Exponent, n); err != nil {
			return err
		}
	}
	val, err := powValue(can.Value, n)
	if err != nil {
		return err
	}
	can.Value = val
	for i := range can.Units {
		can.Units[i].Exponent *= n
	}
	return nil
}

// mulExponent returns a*b, or an error that is ucumerr.ErrExponentTooLarge if
// it does not fit in an int. UCUM §19 ■1 lets an implementation not represent
// every dimension: a dimension that does not fit is an error, never a
// wrapped-around exponent.
func mulExponent(a, b int) (int, error) {
	if b == 0 {
		return 0, nil
	}
	// c/b devuelve a salvo desborde; el único caso que esa prueba no ve es
	// MinInt·-1, que en Go da MinInt y MinInt/-1 da otra vez MinInt.
	c := a * b
	if (a == math.MinInt && b == -1) || c/b != a {
		return 0, &exponentError{}
	}
	return c, nil
}

// addExponent returns a+b, or an error that is ucumerr.ErrExponentTooLarge if
// it does not fit in an int (UCUM §19 ■1, as in mulExponent).
func addExponent(a, b int) (int, error) {
	c := a + b
	if (b > 0 && c < a) || (b < 0 && c > a) {
		return 0, &exponentError{}
	}
	return c, nil
}

// powValue raises v to the integer power n. A zero value with a negative
// exponent is a division by zero, which decimal.Decimal.Pow would turn into a
// panic: it comes back as ucumerr.ErrDivisionByZero instead.
func powValue(v decimal.Decimal, n int) (decimal.Decimal, error) {
	if n < 0 && v.IsZero() {
		return decimal.Decimal{}, ucumerr.ErrDivisionByZero
	}
	return v.Pow(n), nil
}

// findBaseUnit looks up a model.BaseUnit by code.
func (s *Service) findBaseUnit(code string) *model.BaseUnit {
	return s.baseByCode[code]
}
func multiplyCanonicals(left, right *model.Canonical) (*model.Canonical, error) {
	units, err := mergeUnitLists(left.Units, right.Units, 1)
	if err != nil {
		return nil, err
	}
	return &model.Canonical{Value: left.Value.Mul(right.Value), Units: units}, nil
}
func divideCanonicals(left, right *model.Canonical) (*model.Canonical, error) {
	// A zero factor is syntactically valid ("m/0"), so the divisor has to be
	// checked here rather than left to big.Rat, which panics.
	if right.Value.IsZero() {
		return nil, ucumerr.ErrDivisionByZero
	}
	units, err := mergeUnitLists(left.Units, right.Units, -1)
	if err != nil {
		return nil, err
	}
	return &model.Canonical{Value: left.Value.Div(right.Value), Units: units}, nil
}

// mergeUnitLists merges two canonical unit lists. The sign parameter is
// applied to the exponents of the right list (+1 for multiply, -1 for divide).
// An exponent that does not fit in an int is ucumerr.ErrExponentTooLarge.
func mergeUnitLists(left, right []model.CanonicalUnit, sign int) ([]model.CanonicalUnit, error) {
	result := make([]model.CanonicalUnit, len(left))
	copy(result, left)

	for _, ru := range right {
		signed, err := mulExponent(ru.Exponent, sign)
		if err != nil {
			return nil, err
		}
		at := -1
		for i := range result {
			if result[i].Base.Code == ru.Base.Code {
				at = i
				break
			}
		}
		if at < 0 {
			result = append(result, model.CanonicalUnit{Base: ru.Base, Exponent: signed})
			continue
		}
		sum, err := addExponent(result[at].Exponent, signed)
		if err != nil {
			return nil, err
		}
		result[at].Exponent = sum
	}

	// Remove zero-exponent units.
	filtered := result[:0]
	for _, u := range result {
		if u.Exponent != 0 {
			filtered = append(filtered, u)
		}
	}
	return filtered, nil
}

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
	sym, h := s.loneSpecial(t)
	if sym == nil {
		return nil
	}
	return &special.Use{Handler: h, Alpha: prefixValue(sym)}
}

// loneSpecial returns the symbol and handler of a term that denotes a special
// unit, under the rules of specialUseForTerm, and nil otherwise. It allocates
// nothing, so a guard on every conversion can use it.
func (s *Service) loneSpecial(t *expr.Term) (*expr.Symbol, special.Handler) {
	sym := expr.LoneSymbol(t)
	if sym == nil || !sym.Unit.IsSpecial || sym.Exponent != 1 {
		return nil, nil
	}
	h, ok := s.handlers[sym.Unit.Code]
	if !ok {
		return nil, nil
	}
	return sym, h
}

// prefixValue returns the scale factor of a symbol's prefix, or 1 if it has
// none.
func prefixValue(sym *expr.Symbol) decimal.Decimal {
	if sym.Prefix == nil {
		return decimal.FromInt(1)
	}
	return sym.Prefix.Value
}
