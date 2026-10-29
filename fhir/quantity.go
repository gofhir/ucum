package fhir

import (
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/gofhir/ucum/v4"
)

// Quantity is the part of a FHIR Quantity this package can reason about.
//
// System is optional: an empty System is taken to mean UCUMSystem, since that is
// the only system these operations apply to. Setting it to anything else is an
// error rather than a silent assumption — a Quantity coded in SNOMED or in a
// local system has no UCUM code to canonicalize.
type Quantity struct {
	Value  float64
	Code   string
	System string

	// Exact overrides Value when non-nil, and is used exactly as given.
	//
	// It exists because a FHIR decimal is serialized as text and a float64
	// cannot hold most of them: the JSON value 0.01 becomes the float64 nearest
	// 1/100, which is not 1/100. An exact comparison notices, so
	// Compare(1 mg/dL, 0.01 g/L) reports "not equal" for values a reader would
	// call equal. Carrying the decimal itself avoids the question:
	//
	//	v, _ := new(big.Rat).SetString("0.01")
	//	q := fhir.Quantity{Exact: v, Code: "g/L"}
	//
	// Prefer this wherever the value came from FHIR JSON rather than from a
	// computation.
	//
	// No Comparator method modifies it, whatever service the Comparator was
	// built over, and a result never shares it.
	Exact *big.Rat
}

// rat returns the quantity's value as an exact rational, preferring Exact. It
// returns nil when Value is not finite, since that has no rational form.
func (q Quantity) rat() *big.Rat {
	if q.Exact != nil {
		return new(big.Rat).Set(q.Exact)
	}
	return new(big.Rat).SetFloat64(q.Value)
}

// float devuelve el valor como float64, para las escalas sin forma racional.
// Un Exact que no cabe se lee como el float64 finito y distinto de cero más
// cercano, con su signo: el mayor si es enorme, el menor subnormal si es
// diminuto, y no ±Inf ni 0. Así la API base lo trata como la entrada finita y
// distinta de cero que es, y aplica sus reglas de rango al resultado (#59):
// 1e400 B y 1e-400 [p'diop] dan ErrOutOfRange, porque 10^(1e400) y
// atan(1e-402) no caben; 1e400 [p'diop] da π/2 y 1e-400 [pH] la clave de 0,
// que son sus valores correctamente redondeados.
func (q Quantity) float() float64 {
	if q.Exact == nil {
		return q.Value
	}
	f, _ := q.Exact.Float64()
	switch {
	case math.IsInf(f, 0):
		return math.Copysign(math.MaxFloat64, f)
	case f == 0 && q.Exact.Sign() != 0:
		return math.Copysign(math.SmallestNonzeroFloat64, float64(q.Exact.Sign()))
	}
	return f
}

// outOfRange es el error de un Exact cuya forma canónica no cabe en float64.
// Lo comparten la vía racional y la de float64.
func (q Quantity) outOfRange() error {
	return &ucum.ValidationError{
		Code:    q.Code,
		Message: fmt.Sprintf("%s %s: %v", new(big.Float).SetRat(q.Exact).Text('g', 6), q.Code, ucum.ErrOutOfRange),
		Offset:  -1,
		Err:     ucum.ErrOutOfRange,
	}
}

// checkFinite rechaza una cantidad cuyo valor no es un número finito: un Value
// NaN o ±Inf sin Exact. Corre después de checkSystems y antes de leer ningún
// código, así el error es el mismo con cualquier unidad.
func checkFinite(qs ...Quantity) error {
	for _, q := range qs {
		if q.Exact == nil && (math.IsNaN(q.Value) || math.IsInf(q.Value, 0)) {
			return ErrNotFinite
		}
	}
	return nil
}

// usesUCUM reports whether the quantity's system is UCUM, treating the empty
// string as UCUM.
func (q Quantity) usesUCUM() bool {
	return q.System == "" || q.System == UCUMSystem
}

// ErrNotUCUMSystem is returned when a Quantity carries a system other than
// UCUMSystem, in which case its code is not a UCUM code.
var ErrNotUCUMSystem = errors.New("quantity system is not " + UCUMSystem)

// ErrNotFinite is returned when a Quantity has no Exact and its Value is NaN or
// ±Inf. FHIR defines a decimal as a rational number and has no such values, so
// such a quantity has no canonical key and compares or adds with nothing.
// Exact, when set, is used instead and is always finite.
var ErrNotFinite = errors.New("quantity value is not finite")

// Comparator compares FHIR Quantity values by canonicalizing them, which is what
// a server has to do to answer a quantity search with gt, lt, ge or le, and what
// any comparison across units requires.
//
// It holds a UCUM service, so build one and share it: the service is safe for
// concurrent use and caches parsed expressions.
type Comparator struct {
	svc ucum.ExactService
}

// NewComparator builds a Comparator over the embedded UCUM definitions.
func NewComparator() (*Comparator, error) {
	svc, err := ucum.NewExact()
	if err != nil {
		return nil, err
	}
	return &Comparator{svc: svc}, nil
}

// NewComparatorWith builds a Comparator over a service the caller already holds,
// including one built from custom definitions via ucum.NewFromReader.
func NewComparatorWith(svc ucum.ExactService) *Comparator {
	return &Comparator{svc: svc}
}

// Compare returns -1, 0 or 1 as a is less than, equal to, or greater than b,
// after converting both onto a common scale. It fails if the units are not
// comparable.
//
// The comparison is exact wherever the units allow it: both operands go through
// the exact rational API, so it does not turn on the last bits of a float64.
// That matters for the boundary cases a search comparator lands on — 1 L versus
// 1000 mL compares equal, where dividing rounded factors could make it not.
//
// For units on a scale with no exact rational form — pH, bel, prism diopter —
// the comparison falls back to float64, since no exact answer exists. Each
// operand is then rounded to float64 as CanonicalKey rounds it, so Compare
// returns 0 exactly when the two keys are equal. An operand on such a scale
// whose Exact does not fit in a float64 is read as the nearest finite, non-zero
// one, so the range rules of Canonical apply to it as to any finite value:
// 1e400 B and 1e401 B are ucum.ErrOutOfRange, where both would round to +Inf
// and compare equal. An operand on a linear scale is rounded once from its
// exact canonical value.
//
// A Value of NaN or ±Inf with no Exact is ErrNotFinite, checked before the
// units are read.
//
// Temperature is handled correctly, which is the case most likely to be wrong
// elsewhere: 100 [degF] compares below 50 Cel.
func (c *Comparator) Compare(a, b Quantity) (int, error) {
	if err := checkSystems(a, b); err != nil {
		return 0, err
	}
	if err := checkFinite(a, b); err != nil {
		return 0, err
	}
	ra, rb, err := c.canonicalPair(a, b)
	if err != nil {
		return 0, err
	}
	if ra != nil {
		return ra.Cmp(rb), nil
	}

	// Non-rational scale: compare in float64. Cada operando se lleva a float64
	// con la regla de CanonicalKey, así una unidad lineal con Exact se redondea
	// una sola vez desde su forma exacta aunque el otro operando no la tenga, y
	// Compare da 0 solo entre cantidades con la misma clave.
	ka, va, errA := c.key(a)
	kb, vb, errB := c.key(b)
	if errA != nil || errB != nil {
		return 0, c.firstError(a, b, errA, errB)
	}
	if ka != kb {
		return 0, notComparable(a, b)
	}
	switch {
	case va < vb:
		return -1, nil
	case va > vb:
		return 1, nil
	default:
		return 0, nil
	}
}

// canonicalPair maps both quantities onto their canonical scales exactly. It
// returns nil rationals, and no error, when a scale has no exact rational form,
// which tells Compare to use the float64 path.
func (c *Comparator) canonicalPair(a, b Quantity) (ra, rb *big.Rat, err error) {
	// checkFinite ya rechazó un Value no finito, el único caso en que rat()
	// devuelve nil.
	va, vb := a.rat(), b.rat()

	ca, ok, err := c.canonicalRat(va, a.Code)
	if err != nil || !ok {
		return nil, nil, err
	}
	cb, ok, err := c.canonicalRat(vb, b.Code)
	if err != nil || !ok {
		return nil, nil, err
	}
	if ca.Code != cb.Code {
		return nil, nil, notComparable(a, b)
	}
	return ca.Value, cb.Value, nil
}

// notComparable nombra los códigos de quien llama, en las dos vías de Compare.
func notComparable(a, b Quantity) error {
	return fmt.Errorf("quantities are not comparable: %s vs %s", a.Code, b.Code)
}

// firstError elige el error de Compare en la vía float64 con el orden de la
// vía racional: un error de código antes que uno de unidades, y uno de
// unidades antes que uno de rango, de cualquier lado. IsComparable solo corre
// cuando hay un error de rango, para no canonicalizar dos veces en el caso
// común.
func (c *Comparator) firstError(a, b Quantity, errA, errB error) error {
	for _, err := range []error{errA, errB} {
		if err != nil && !errors.Is(err, ucum.ErrOutOfRange) {
			return err
		}
	}
	ok, err := c.svc.IsComparable(a.Code, b.Code)
	if err != nil {
		return err
	}
	if !ok {
		return notComparable(a, b)
	}
	if errA != nil {
		return errA
	}
	return errB
}

// canonicalRat es la regla que comparten Compare y CanonicalKey: la forma
// canónica exacta de v en code, o ok en false, sin error, cuando la escala no
// tiene forma racional (ErrNotRational) y quien llama tiene que usar float64.
// Quien llama pasa una copia: v puede ir a un servicio ajeno.
func (c *Comparator) canonicalRat(v *big.Rat, code string) (p ucum.RatPair, ok bool, err error) {
	p, err = c.svc.CanonicalRat(v, code)
	if errors.Is(err, ucum.ErrNotRational) {
		return ucum.RatPair{}, false, nil
	}
	if err != nil {
		return ucum.RatPair{}, false, err
	}
	return p, true, nil
}

// canonicalFloat es la vía float64 de Compare y CanonicalKey. Un error de rango
// nombra el Exact de quien llama, no el float64 en que se leyó.
func (c *Comparator) canonicalFloat(q Quantity) (ucum.Pair, error) {
	p, err := c.svc.Canonical(q.float(), q.Code)
	if q.Exact != nil && errors.Is(err, ucum.ErrOutOfRange) {
		return ucum.Pair{}, q.outOfRange()
	}
	return p, err
}

// ConvertDecimal converts a value between units, preserving the precision its
// source declared.
//
// The conversion itself is exact — it runs through the rational API — and the
// result carries the same significant figures as the input. That follows from
// how precision propagates: a unit conversion multiplies by an exactly known
// factor, and multiplying by an exact quantity neither adds nor removes
// significant figures.
//
//	d, _ := fhir.ParseDecimal("1.50")            // 3 significant figures
//	out, _ := c.ConvertDecimal(d, "g/L", "mg/dL")
//	out.String()                                 // "150", still 3 figures
//
// It fails for the scales with no exact rational form — pH, bel, prism diopter —
// where the result would be an approximation and claiming the input's precision
// for it would be a lie. Use Convert on the service for those.
func (c *Comparator) ConvertDecimal(d Decimal, from, to string) (Decimal, error) {
	out, err := c.svc.ConvertRat(d.Rat(), from, to)
	if err != nil {
		return Decimal{}, err
	}
	return NewDecimal(out, d.SignificantFigures()), nil
}

// Add returns a + b, in the units of a.
//
// FHIRPath defines arithmetic over quantities and says that "implementations
// that do support units shall do so as specified by [UCUM]", so the right
// operand is converted to the unit of the left one before the addition. The
// conversion and the sum are both exact, which is the point: ten quantities of
// 0.1 L add up to exactly 1 L, where ten float64 tenths do not.
//
// It fails with ErrNotFinite if either Value is NaN or ±Inf with no Exact,
// before anything else about the units; with ucum.ErrNotLinear if either
// quantity sits on a non-ratio scale — see the note on Sub — and with an error
// if the units are not comparable.
func (c *Comparator) Add(a, b Quantity) (Quantity, error) {
	return c.additive(a, b, true)
}

// Sub returns a - b, in the units of a, under the same rules as Add.
//
// Both refuse the non-ratio scales rather than returning a number. Adding two
// points on an affine scale gives an answer that depends on the unit it is
// carried out in: 20 Cel + 20 Cel is 40 Cel, while the same sum done in kelvin
// is 586.3 K, which is 313.15 Cel. Neither is more correct than the other, so
// there is nothing to return. The same holds for the logarithmic scales, where
// adding two bels is not adding their values at all.
//
// A temperature *difference* is a different quantity, and UCUM has no separate
// unit for it. Where a rate or a gradient is meant, name the compound unit and
// convert it — Convert(1, "Cel/min", "K/min") — which is the reading in which
// the offset cancels.
func (c *Comparator) Sub(a, b Quantity) (Quantity, error) {
	return c.additive(a, b, false)
}

// additive is the shared body of Add and Sub.
func (c *Comparator) additive(a, b Quantity, add bool) (Quantity, error) {
	if err := checkSystems(a, b); err != nil {
		return Quantity{}, err
	}
	if err := checkFinite(a, b); err != nil {
		return Quantity{}, err
	}

	// ConversionFactor does three jobs here: it rejects a unit on a non-ratio
	// scale with ErrNotLinear, which is exactly the case that has no answer; it
	// rejects incommensurable units; and it returns the exact factor for
	// everything else.
	factor, err := c.svc.ConversionFactor(b.Code, a.Code)
	if err != nil {
		return Quantity{}, err
	}

	av, bv := a.rat(), b.rat()
	bv.Mul(bv, factor)
	if add {
		av.Add(av, bv)
	} else {
		av.Sub(av, bv)
	}
	return Quantity{Exact: av, Code: a.Code, System: a.System}, nil
}

// Comparable reports whether two quantities can be compared at all, that is,
// whether their units share a canonical form.
func (c *Comparator) Comparable(a, b Quantity) (bool, error) {
	if err := checkSystems(a, b); err != nil {
		return false, err
	}
	return c.svc.IsComparable(a.Code, b.Code)
}

// checkSystems rejects quantities that are not coded in UCUM.
func checkSystems(qs ...Quantity) error {
	for _, q := range qs {
		if !q.usesUCUM() {
			return fmt.Errorf("%q: %w", q.System, ErrNotUCUMSystem)
		}
	}
	return nil
}

// CanonicalKey returns the canonical unit code of a quantity, which is the key a
// server can index quantity values under so that a search comparator only has to
// compare numbers.
//
// Two quantities with the same key are comparable; the value to store alongside
// it is the second return, the quantity restated in that unit.
//
// The value is computed exactly and rounded to float64 once, so Exact is used
// as given: 0.07 mg/dL gives 0.7 g.m-3, not 0.7000000000000001. Units on a scale
// with no exact rational form — pH, bel, prism diopter — go through float64, as
// in Compare, so there Exact is rounded to float64 first, an Exact that does not
// fit being read as the nearest finite, non-zero float64.
//
// A Value of NaN or ±Inf with no Exact has no key: it is ErrNotFinite, as in
// Compare, so an index never stores a value that is not a number.
func (c *Comparator) CanonicalKey(q Quantity) (code string, value float64, err error) {
	if err := checkSystems(q); err != nil {
		return "", 0, err
	}
	if err := checkFinite(q); err != nil {
		return "", 0, err
	}
	return c.key(q)
}

// key es la clave de una cantidad ya verificada (sistema y finitud): la de
// CanonicalKey, y la que usa Compare en la vía float64.
func (c *Comparator) key(q Quantity) (code string, value float64, err error) {
	// Solo Exact necesita la vía exacta: Canonical ya convierte un Value a
	// racional y redondea una vez, y así una escala no racional no se
	// canonicaliza dos veces. Se pasa una copia, como en Compare: el servicio
	// puede ser cualquier ExactService (NewComparatorWith), y el Exact de quien
	// llama no se toca (#56).
	if q.Exact != nil {
		p, ok, err := c.canonicalRat(q.rat(), q.Code)
		if err != nil {
			return "", 0, err
		}
		if ok {
			// La misma regla que Canonical por el camino de Value (#59): un
			// valor que no cabe en float64, o que no es cero y redondea a
			// cero, es ucum.ErrOutOfRange.
			f, _ := p.Value.Float64()
			if math.IsInf(f, 0) || f == 0 && p.Value.Sign() != 0 {
				return "", 0, q.outOfRange()
			}
			return p.Code, f, nil
		}
	}
	p, err := c.canonicalFloat(q)
	if err != nil {
		return "", 0, err
	}
	return p.Code, p.Value, nil
}
