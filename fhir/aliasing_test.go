package fhir

import (
	"math/big"
	"testing"

	"github.com/gofhir/ucum/v4"
)

// Plan 007 (#56). Ninguna operación modifica un *big.Rat que recibe, ni
// devuelve uno que use la misma memoria que su entrada. El esperado es siempre
// la copia tomada antes de la llamada, nunca un valor calculado por el
// código.

// aliasingValues son negativos, cero, fracciones sin forma decimal finita y
// grandes.
func aliasingValues() []*big.Rat {
	big30, _ := new(big.Rat).SetString("1000000000000000000000000000000")
	return []*big.Rat{
		big.NewRat(37, 1), big.NewRat(-3, 2), new(big.Rat), big.NewRat(1, 3), big30,
	}
}

// aliasingPairs son pares comparables: lineales, especiales racionales sin y
// con prefijo (solo con prefijo el factor Alpha es distinto de 1), y una
// escala sin forma racional, que va por float64.
var aliasingPairs = [][2]string{
	{"mL", "L"}, {"mg/dL", "g/L"}, {"L", "L"},
	{"Cel", "K"}, {"[degF]", "Cel"}, {"mCel", "K"}, {"kCel", "Cel"},
	{"[pH]", "[pH]"},
}

// operands devuelve dos maneras de construir los operandos de un par de
// valores: con *big.Rat distintos y con el mismo en los dos. Cada método
// recibe operandos nuevos, así un error no se arrastra al siguiente.
func operands(av, bv *big.Rat, from, to string) []func() (a, b Quantity) {
	return []func() (Quantity, Quantity){
		func() (Quantity, Quantity) {
			return Quantity{Exact: new(big.Rat).Set(av), Code: from}, Quantity{Exact: new(big.Rat).Set(bv), Code: to}
		},
		func() (Quantity, Quantity) {
			shared := new(big.Rat).Set(av)
			return Quantity{Exact: shared, Code: from}, Quantity{Exact: shared, Code: to}
		},
	}
}

// unchanged falla si a o b ya no valen lo que valían.
func unchanged(t *testing.T, op string, a, b Quantity, wantA, wantB *big.Rat) {
	t.Helper()
	if a.Exact.Cmp(wantA) != 0 || b.Exact.Cmp(wantB) != 0 {
		t.Errorf("%s(%s %s, %s %s) changed its operands to %s and %s",
			op, wantA.RatString(), a.Code, wantB.RatString(), b.Code, a.Exact.RatString(), b.Exact.RatString())
	}
}

// Test 1: cada método de Comparator, con el servicio de este módulo.
func TestComparatorDoesNotMutateOperands(t *testing.T) {
	c, err := NewComparator()
	if err != nil {
		t.Fatal(err)
	}
	checkComparator(t, c)
}

func checkComparator(t *testing.T, c *Comparator) {
	t.Helper()
	n := 0
	for _, pair := range aliasingPairs {
		for _, av := range aliasingValues() {
			for _, bv := range aliasingValues() {
				for _, mk := range operands(av, bv, pair[0], pair[1]) {
					a, b := mk()
					wantA, wantB := new(big.Rat).Set(a.Exact), new(big.Rat).Set(b.Exact)
					_, _ = c.Compare(a, b)
					unchanged(t, "Compare", a, b, wantA, wantB)

					a, b = mk()
					_, _ = c.Comparable(a, b)
					unchanged(t, "Comparable", a, b, wantA, wantB)

					a, b = mk()
					_, _, _ = c.CanonicalKey(a)
					_, _, _ = c.CanonicalKey(b)
					unchanged(t, "CanonicalKey", a, b, wantA, wantB)

					for _, op := range []struct {
						name string
						f    func(Quantity, Quantity) (Quantity, error)
					}{{"Add", c.Add}, {"Sub", c.Sub}} {
						name := op.name
						a, b = mk()
						got, err := op.f(a, b)
						unchanged(t, name, a, b, wantA, wantB)
						if err == nil {
							// El resultado es de quien llama.
							got.Exact.Add(got.Exact, big.NewRat(7, 1))
							unchanged(t, name+" result", a, b, wantA, wantB)
						}
					}

					// Con precisión declarada y sin límite (0), que es lo que da
					// ParseDecimal de un entero.
					for _, figs := range []int{3, 0} {
						a, _ = mk()
						d := NewDecimal(a.Exact, figs)
						out, err := c.ConvertDecimal(d, pair[0], pair[1])
						if err == nil {
							want := out.Rat()
							r := out.Rat()
							r.Add(r, big.NewRat(7, 1))
							// Una segunda conversión: un servicio que reutiliza lo que
							// devolvió no puede cambiar el Decimal ya entregado.
							_, _ = c.ConvertDecimal(d, pair[0], pair[1])
							if out.Rat().Cmp(want) != 0 {
								t.Errorf("ConvertDecimal(%s %s): the result changed to %s after another call", wantA.RatString(), pair[0], out.Rat().RatString())
							}
						}
						if d.Rat().Cmp(wantA) != 0 {
							t.Errorf("ConvertDecimal(%s %s) changed its Decimal to %s", wantA.RatString(), pair[0], d.Rat().RatString())
						}
					}
					n++
				}
			}
		}
	}
	nv := len(aliasingValues())
	if want := len(aliasingPairs) * nv * nv * 2; n != want {
		t.Fatalf("%d cases, want %d", n, want)
	}
}

// scribbler es un ExactService que no cumple la interfaz: escribe sobre el
// valor que recibe, y reutiliza el *big.Rat que devolvió en ConvertRat,
// cambiándolo en la llamada siguiente.
type scribbler struct {
	ucum.ExactService
	last *big.Rat
}

func (s *scribbler) CanonicalRat(v *big.Rat, code string) (ucum.RatPair, error) {
	p, err := s.ExactService.CanonicalRat(v, code)
	v.Add(v, big.NewRat(1, 1))
	return p, err
}

func (s *scribbler) ConvertRat(v *big.Rat, from, to string) (*big.Rat, error) {
	r, err := s.ExactService.ConvertRat(v, from, to)
	v.Add(v, big.NewRat(1, 1))
	if s.last != nil {
		s.last.Add(s.last, big.NewRat(1, 1))
	}
	s.last = r
	return r, err
}

// Test 1, con un servicio que escribe sobre su entrada: Comparator no le
// pasa nunca el Exact de quien llama.
func TestComparatorProtectsOperandsFromService(t *testing.T) {
	svc, err := ucum.NewExact()
	if err != nil {
		t.Fatal(err)
	}
	checkComparator(t, NewComparatorWith(&scribbler{ExactService: svc}))
}

// Test 2: Decimal es un valor. Ni el *big.Rat de quien lo construye, ni el
// que devuelve Rat, ni sus operaciones lo cambian.
func TestDecimalDoesNotAlias(t *testing.T) {
	for _, v := range aliasingValues() {
		for _, figs := range []int{4, 0} {
			v := new(big.Rat).Set(v)
			checkNewDecimal(t, v, figs)
		}
	}

	ops := []struct {
		name string
		f    func(d, o Decimal) Decimal
	}{
		{"Mul", Decimal.Mul},
		{"Div", func(d, o Decimal) Decimal { q, _ := d.Div(o); return q }},
		{"Add", Decimal.Add},
		{"Sub", Decimal.Sub},
	}
	for _, x := range aliasingValues() {
		for _, y := range aliasingValues() {
			// Con precisión declarada y sin límite (0).
			for _, figs := range [][2]int{{3, 5}, {0, 0}} {
				d, o := NewDecimal(x, figs[0]), NewDecimal(y, figs[1])
				for _, op := range ops {
					for _, rhs := range []Decimal{o, d} { // operando distinto y el mismo
						wantD, wantO := d.Rat(), rhs.Rat()
						_ = op.f(d, rhs)
						if d.Rat().Cmp(wantD) != 0 || rhs.Rat().Cmp(wantO) != 0 {
							t.Errorf("%s(%s, %s) changed its operands to %s and %s",
								op.name, wantD.RatString(), wantO.RatString(), d.Rat().RatString(), rhs.Rat().RatString())
						}
					}
				}
			}
		}
	}
}

// checkNewDecimal: cambiar el *big.Rat pasado a NewDecimal, o el que devuelve
// Rat, no cambia el Decimal.
func checkNewDecimal(t *testing.T, v *big.Rat, figs int) {
	t.Helper()
	want := new(big.Rat).Set(v)
	d := NewDecimal(v, figs)
	v.Add(v, big.NewRat(7, 1))
	if d.Rat().Cmp(want) != 0 {
		t.Errorf("changing the value given to NewDecimal changed the Decimal: %s, want %s", d.Rat().RatString(), want.RatString())
	}
	r := d.Rat()
	r.Add(r, big.NewRat(7, 1))
	if d.Rat().Cmp(want) != 0 {
		t.Errorf("changing the value Rat returned changed the Decimal: %s, want %s", d.Rat().RatString(), want.RatString())
	}
}
