package fhir_test

import (
	"fmt"
	"math"
	"math/big"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/gofhir/ucum/v4"
	"github.com/gofhir/ucum/v4/fhir"
)

// Plan 015 (§4.5): un factor canónico negativo en la vía log10, visto desde
// CanonicalKey y Compare. [tiny] = 1e-310 rad, [ntiny] = -1 [tiny] y
// [nlnB] = ln(-1 [tiny]) (UCUM §18 ■5: «real number scalar», sin signo; §22 ■4:
// x = f⁻¹(α x')): el canónico de v [nlnB] es -1e-310·e^v rad, que para v
// sobre 709.78 pasa por la vía log10. El esperado sale de big.Float (exp por
// serie, 300 bits), nunca de la librería.

const nfXML = ` <unit Code="[tiny]" CODE="[TINY]" isMetric="no" class="dimless">
  <name>tiny</name><printSymbol>tiny</printSymbol><property>test</property>
  <value Unit="rad" UNIT="RAD" value="1e-310">1e-310</value>
 </unit>
 <unit Code="[ntiny]" CODE="[NTINY]" isMetric="no" class="dimless">
  <name>ntiny</name><printSymbol>ntiny</printSymbol><property>test</property>
  <value Unit="[tiny]" UNIT="[TINY]" value="-1">-1</value>
 </unit>
 <unit Code="[nlnB]" CODE="[NLNB]" isMetric="no" isSpecial="yes" class="levels">
  <name>nlnB</name><printSymbol>nlnB</printSymbol><property>test</property>
  <value Unit="ln(1 [ntiny])" UNIT="LN(1 [NTINY])"><function name="ln" value="1" Unit="[ntiny]"/></value>
 </unit>

 <unit Code="[zero]" CODE="[ZERO]" isMetric="no" class="dimless">
  <name>zero</name><printSymbol>zero</printSymbol><property>test</property>
  <value Unit="m" UNIT="M" value="0">0</value>
 </unit>
 <unit Code="[zln]" CODE="[ZLN]" isMetric="no" isSpecial="yes" class="levels">
  <name>zln</name><printSymbol>zln</printSymbol><property>test</property>
  <value Unit="ln(1 [zero])" UNIT="LN(1 [ZERO])"><function name="ln" value="1" Unit="[zero]"/></value>
 </unit>
`

var (
	nfOnce sync.Once
	nfCmp  *fhir.Comparator
	errNF  error
)

func nfComparator(t testing.TB) *fhir.Comparator {
	t.Helper()
	nfOnce.Do(func() {
		raw, err := os.ReadFile("../internal/essence/ucum-essence.xml")
		if err != nil {
			errNF = err
			return
		}
		svc, err := ucum.NewFromReader(strings.NewReader(strings.Replace(string(raw), "</root>", nfXML+"</root>", 1)))
		if err != nil {
			errNF = err
			return
		}
		nfCmp = fhir.NewComparatorWith(svc.(ucum.ExactService))
	})
	if errNF != nil {
		t.Fatal(errNF)
	}
	return nfCmp
}

// nfCanon es -1e-310·e^v, en big.Float de 300 bits: e^v = (e^(v/2^12))^(2^12)
// con la serie de Taylor del argumento reducido.
func nfCanon(v float64) float64 {
	const prec = 364
	r := new(big.Float).SetPrec(prec).Quo(new(big.Float).SetPrec(prec).SetFloat64(v), new(big.Float).SetPrec(prec).SetInt64(1<<12))
	sum := new(big.Float).SetPrec(prec).SetInt64(1)
	term := new(big.Float).SetPrec(prec).SetInt64(1)
	eps := new(big.Float).SetPrec(prec).SetMantExp(big.NewFloat(1), -prec-8)
	for n := int64(1); n < 2000; n++ {
		term.Mul(term, r)
		term.Quo(term, new(big.Float).SetPrec(prec).SetInt64(n))
		sum.Add(sum, term)
		if new(big.Float).Abs(term).Cmp(eps) < 0 {
			break
		}
	}
	for i := 0; i < 12; i++ {
		sum.Mul(sum, sum)
	}
	tiny, _ := new(big.Rat).SetString("1e-310")
	sum.Mul(sum, new(big.Float).SetPrec(prec).SetRat(tiny))
	sum.Neg(sum)
	f, _ := sum.Float64()
	return f
}

// nfGrid cruza el borde entre la vía directa y la log10 (e^v desborda sobre
// 709.78).
var nfGrid = []float64{0, 100, 700, 705, 709.7, 709.78, 709.79, 710, 712, 714, 714.25, 714.3}

// CanonicalKey lleva el signo. La cota relativa es la de la vía log10 de
// fromLog10 (|l|·2^-52·ln 10 con l = v/ln 10 ≈ 310), 4 veces: unos 6e-13
// relativos como máximo. El signo se exige exacto.
func TestNegCanonicalKey(t *testing.T) {
	c := nfComparator(t)
	for _, v := range nfGrid {
		for _, q := range []fhir.Quantity{{Value: v, Code: "[nlnB]"}, {Exact: new(big.Rat).SetFloat64(v), Code: "[nlnB]"}} {
			code, got, err := c.CanonicalKey(q)
			want := nfCanon(v)
			what := fmt.Sprintf("CanonicalKey(%v [nlnB], exact %v)", v, q.Exact != nil)
			if err != nil || code != "rad" {
				t.Errorf("%s = (%q, %v, %v), want rad", what, code, got, err)
				continue
			}
			if math.Signbit(got) != math.Signbit(want) {
				t.Errorf("%s = %v, want %v: wrong sign", what, got, want)
				continue
			}
			if tol := math.Abs(want) * 4 * (v/math.Ln10 + 1) * 0x1p-52 * math.Ln10; math.Abs(got-want) > tol {
				t.Errorf("%s = %v, want %v (tolerance %.3g)", what, got, want, tol)
			}
		}
	}
}

// Compare no invierte el orden entre la vía directa y la log10: más v es un
// canónico más negativo (e^v crece), así que v mayor es menor. Contra 0 rad
// todo valor de la escala es negativo; entre vecinos de la rejilla, estricto.
func TestNegCompareOrder(t *testing.T) {
	c := nfComparator(t)
	zero := fhir.Quantity{Value: 0, Code: "rad"}
	for i, v := range nfGrid {
		a := fhir.Quantity{Value: v, Code: "[nlnB]"}
		if got, err := c.Compare(a, zero); err != nil || got != -1 {
			t.Errorf("Compare(%v [nlnB], 0 rad) = (%d, %v), want -1", v, got, err)
		}
		if got, err := c.Compare(zero, a); err != nil || got != 1 {
			t.Errorf("Compare(0 rad, %v [nlnB]) = (%d, %v), want 1", v, got, err)
		}
		if i == 0 {
			continue
		}
		prev := fhir.Quantity{Value: nfGrid[i-1], Code: "[nlnB]"}
		if got, err := c.Compare(a, prev); err != nil || got != -1 {
			t.Errorf("Compare(%v [nlnB], %v [nlnB]) = (%d, %v), want -1", v, nfGrid[i-1], got, err)
		}
		if got, err := c.Compare(prev, a); err != nil || got != 1 {
			t.Errorf("Compare(%v [nlnB], %v [nlnB]) = (%d, %v), want 1", nfGrid[i-1], v, got, err)
		}
	}
	// El caso del plan, y contra un lineal: -1.566 rad está sobre -2 rad y bajo
	// -1 rad.
	a := fhir.Quantity{Value: 714.25, Code: "[nlnB]"}
	for _, tc := range []struct {
		b    fhir.Quantity
		want int
	}{
		{fhir.Quantity{Value: 700, Code: "[nlnB]"}, -1},
		{fhir.Quantity{Value: -1, Code: "rad"}, -1},
		{fhir.Quantity{Value: -2, Code: "rad"}, 1},
		{fhir.Quantity{Value: 1, Code: "rad"}, -1},
		{fhir.Quantity{Exact: big.NewRat(-1, 1), Code: "rad"}, -1},
	} {
		if got, err := c.Compare(a, tc.b); err != nil || got != tc.want {
			t.Errorf("Compare(714.25 [nlnB], %v %s) = (%d, %v), want %d", tc.b.Value, tc.b.Code, got, err, tc.want)
		}
	}
}

// Ronda 3, hallazgo 1: con una referencia de valor 0 el canónico es 0 m a ambos
// lados de la frontera entre la vía directa y la log10 (709 para ln), y dos
// ceros comparan igual.
func TestNegZeroReferenceKeyAndCompare(t *testing.T) {
	c := nfComparator(t)
	for _, v := range []float64{1, -800, 800, 1e300} {
		code, got, err := c.CanonicalKey(fhir.Quantity{Value: v, Code: "[zln]"})
		if err != nil || code != "m" || got != 0 {
			t.Errorf("CanonicalKey(%v [zln]) = (%q, %v, %v), want (m, 0)", v, code, got, err)
		}
		for _, b := range []fhir.Quantity{{Value: 0, Code: "m"}, {Value: 1, Code: "[zln]"}, {Value: 800, Code: "[zln]"}} {
			if cmp, err := c.Compare(fhir.Quantity{Value: v, Code: "[zln]"}, b); err != nil || cmp != 0 {
				t.Errorf("Compare(%v [zln], %v %s) = (%d, %v), want 0", v, b.Value, b.Code, cmp, err)
			}
		}
	}
}
