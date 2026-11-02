package ucum_test

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"
	"testing"

	"github.com/gofhir/ucum/v4"
	"github.com/gofhir/ucum/v4/internal/essence"
)

// Plan 013: un valor con α·v < 0 en una escala de raíz cuadrada no existe. La
// imagen de sqrt es [0, ∞) (UCUM §47 ■4, tabla 21: «the square root with its
// respective inverse function, the square»), así que ErrOutOfDomain, no el
// cuadrado del valor. Los esperados salen de v² por el factor de la unidad,
// leído del XML, en big.Rat; la escala se busca en el XML por su función.

// sqrtScale es la escala de raíz cuadrada del XML con el factor que la lleva a
// su unidad canónica.
type sqrtScale struct {
	code   string   // el código de la unidad
	ref    string   // su unidad de referencia, sin el multiplicador
	factor *big.Rat // v² · factor es el valor canónico
	canon  string   // la unidad canónica
}

func findSqrt(t testing.TB) sqrtScale {
	t.Helper()
	m, err := essence.Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	var found *sqrtScale
	for _, du := range m.DefinedUnits {
		if !du.IsSpecial || du.Value.Function == nil || du.Value.Function.Name != "sqrt" {
			continue
		}
		if found != nil {
			t.Fatalf("more than one sqrt scale in the XML: %s and %s", found.code, du.Code)
		}
		fn := du.Value.Function
		svc, err := ucum.NewExact()
		if err != nil {
			t.Fatal(err)
		}
		p, err := svc.CanonicalRat(fn.Value.Rat(), fn.Unit)
		if err != nil {
			t.Fatalf("CanonicalRat(%s %s): %v", fn.Value.Rat().RatString(), fn.Unit, err)
		}
		found = &sqrtScale{code: du.Code, ref: fn.Unit, factor: p.Value, canon: p.Code}
	}
	if found == nil {
		t.Fatal("no sqrt scale in the XML")
	}
	return *found
}

// want es v²·factor redondeado una vez, o ok en false si no cabe en float64.
func (sc sqrtScale) want(v float64) (f float64, ok bool) {
	r := new(big.Rat).SetFloat64(v)
	r.Mul(r, r)
	r.Mul(r, sc.factor)
	f, _ = r.Float64()
	if math.IsInf(f, 0) || f == 0 && r.Sign() != 0 {
		return 0, false
	}
	return f, true
}

// negatives son los valores negativos de la prueba: normal, subnormal que
// eleva a cero, el menor subnormal, enormes que desbordan al cuadrado.
var negatives = []float64{-2, -1e-310, -5e-324, -1e300, -math.MaxFloat64, -1e-3}

func wantDomain(t *testing.T, what string, v float64, code string, err error) {
	t.Helper()
	if !errors.Is(err, ucum.ErrOutOfDomain) {
		t.Errorf("%s = %v, want ErrOutOfDomain", what, err)
		return
	}
	if errors.Is(err, ucum.ErrOutOfRange) {
		t.Errorf("%s = %v, also wraps ErrOutOfRange", what, err)
	}
	msg := err.Error()
	// Juntos, «valor unidad»: el prefijo del ValidationError ya nombra una
	// unidad, que puede ser la del otro operando.
	if !strings.Contains(msg, fmt.Sprint(v)+" "+code) {
		t.Errorf("%s: message %q does not name %v %s", what, msg, v, code)
	}
	// La API exacta tampoco da el valor (sqrt no es racional): el mensaje no
	// la ofrece, como el error de dominio del destino.
	if !strings.Contains(msg, "outside the domain") || strings.Contains(msg, "NewExact") {
		t.Errorf("%s: message %q, want the domain text and no NewExact hint", what, msg)
	}
}

// wantDomainOperand es wantDomain en Multiply y Divide: el ValidationError nombra la
// unidad del operando que no existe, de cualquier lado.
func wantDomainOperand(t *testing.T, what string, v float64, code string, err error) {
	t.Helper()
	wantDomain(t, what, v, code, err)
	var ve *ucum.ValidationError
	if errors.As(err, &ve) && ve.Code != code {
		t.Errorf("%s: ValidationError.Code = %q, want %q", what, ve.Code, code)
	} else if ve == nil {
		t.Errorf("%s = %v, want a *ValidationError", what, err)
	}
}

// Tests 1 y 2: cada operación, con el servicio sensible a mayúsculas y el que
// no lo es, y con la anotación.
func TestSqrtNegativeIsOutOfDomain(t *testing.T) {
	sc := findSqrt(t)
	ci, err := ucum.NewCaseInsensitive()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		svc  ucum.Service
		code string
	}{
		{"plain", newService(t), sc.code},
		{"annotated", newService(t), sc.code + "{x}"},
		{"case-insensitive", ci, strings.ToUpper(sc.code)},
	} {
		for _, v := range negatives {
			label := fmt.Sprintf("%s %v", tc.name, v)
			_, err := tc.svc.Canonical(v, tc.code)
			wantDomain(t, "Canonical("+label+")", v, tc.code, err)

			for _, to := range []string{tc.code, sc.ref} {
				_, err = tc.svc.Convert(v, tc.code, to)
				wantDomain(t, "Convert("+label+" to "+to+")", v, tc.code, err)
			}

			// Multiply y Divide, en los dos órdenes, con el otro operando finito,
			// cero y no finito. El dominio va antes que el cero y que el infinito.
			neg := ucum.Pair{Value: v, Code: tc.code}
			for _, other := range []ucum.Pair{
				{Value: 3, Code: "m"}, {Value: 0, Code: "m"}, {Value: math.Inf(1), Code: "m"},
				{Value: math.Inf(-1), Code: "m"}, {Value: math.NaN(), Code: "m"},
				{Value: 3, Code: "B"},
			} {
				ol := fmt.Sprintf("%s with %v %s", label, other.Value, other.Code)
				_, err = tc.svc.Multiply(neg, other)
				wantDomainOperand(t, "Multiply("+ol+")", v, tc.code, err)
				_, err = tc.svc.Multiply(other, neg)
				wantDomainOperand(t, "Multiply("+ol+", swapped)", v, tc.code, err)
				_, err = tc.svc.Divide(neg, other)
				wantDomainOperand(t, "Divide("+ol+")", v, tc.code, err)
				_, err = tc.svc.Divide(other, neg)
				wantDomainOperand(t, "Divide("+ol+", swapped)", v, tc.code, err)
			}
		}
	}
}

// El caso que más engaña: dividir 1 m por -2 S daba 0.25, y -2 S a sí misma
// perdía el signo.
func TestSqrtNegativeNoLongerLosesTheSign(t *testing.T) {
	sc := findSqrt(t)
	svc := newService(t)
	if got, err := svc.Convert(-2, sc.code, sc.code); err == nil {
		t.Errorf("Convert(-2, %s, %s) = %v, want ErrOutOfDomain", sc.code, sc.code, got)
	}
	if got, err := svc.Divide(ucum.Pair{Value: 1, Code: "m"}, ucum.Pair{Value: -2, Code: sc.code}); err == nil {
		t.Errorf("Divide(1 m, -2 %s) = %v, want ErrOutOfDomain", sc.code, got)
	}
	if got, err := svc.Divide(ucum.Pair{Value: -2, Code: sc.code}, ucum.Pair{Value: 0, Code: "m"}); !errors.Is(err, ucum.ErrOutOfDomain) {
		t.Errorf("Divide(-2 %s, 0 m) = (%v, %v), want ErrOutOfDomain before the zero divisor", sc.code, got, err)
	}
}

// Test 3: lo que no cambia, contra v² por el factor del XML.
func TestSqrtNonNegativeUnchanged(t *testing.T) {
	sc := findSqrt(t)
	svc := newService(t)
	negZero := math.Copysign(0, -1)
	values := []float64{0, negZero, 2, 1e-3, 1.5, 1e-160, 5e-324, 1e150, 1e154, 1e200, math.MaxFloat64, 1e-170}
	for _, v := range values {
		want, ok := sc.want(v)
		got, err := svc.Canonical(v, sc.code)
		switch {
		case ok && (err != nil || got.Value != want || got.Code != sc.canon):
			t.Errorf("Canonical(%v, %s) = (%v, %v), want %v %s", v, sc.code, got, err, want, sc.canon)
		case !ok && !errors.Is(err, ucum.ErrOutOfRange):
			t.Errorf("Canonical(%v, %s) = (%v, %v), want ErrOutOfRange", v, sc.code, got, err)
		}

		// A la unidad de referencia: el cuadrado exacto, redondeado una vez.
		if !ok {
			continue
		}
		if back, err := svc.Convert(v, sc.code, sc.ref); err != nil || back != want {
			t.Errorf("Convert(%v, %s, %s) = (%v, %v), want %v", v, sc.code, sc.ref, back, err, want)
		}
	}

	// Una entrada no finita pasa sin verificar (#55): -Inf da +Inf.
	for _, tc := range []struct{ in, want float64 }{
		{math.Inf(1), math.Inf(1)}, {math.Inf(-1), math.Inf(1)}, {math.NaN(), math.NaN()},
	} {
		got, err := svc.Canonical(tc.in, sc.code)
		if err != nil || got.Value != tc.want && !(math.IsNaN(got.Value) && math.IsNaN(tc.want)) {
			t.Errorf("Canonical(%v, %s) = (%v, %v), want %v", tc.in, sc.code, got, err, tc.want)
		}
	}

	// En una multiplicación, un operando no finito de la escala pasa también, y con él
	// el otro, finito.
	if got, err := svc.Multiply(ucum.Pair{Value: math.Inf(-1), Code: sc.code}, ucum.Pair{Value: 3, Code: "m"}); err != nil || !math.IsInf(got.Value, 1) {
		t.Errorf("Multiply(-Inf %s, 3 m) = (%v, %v), want +Inf", sc.code, got, err)
	}

	// Una multiplicación con un positivo: lo mismo que el valor exacto.
	got, err := svc.Multiply(ucum.Pair{Value: 2, Code: sc.code}, ucum.Pair{Value: 3, Code: "m"})
	want, _ := sc.want(2)
	if err != nil || got.Value != want*3 {
		t.Errorf("Multiply(2 %s, 3 m) = (%v, %v), want %v", sc.code, got, err, want*3)
	}
}

// Test 4: ninguna otra escala especial da error de dominio para un negativo
// hacia la escala canónica.
func TestOtherSpecialScalesAcceptNegatives(t *testing.T) {
	m, err := essence.Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	svc := newService(t)
	n := 0
	for _, du := range m.DefinedUnits {
		if !du.IsSpecial || du.Value.Function == nil || du.Value.Function.Name == "sqrt" {
			continue
		}
		n++
		for _, v := range []float64{-2, -273.15, -1e-3} {
			if _, err := svc.Canonical(v, du.Code); errors.Is(err, ucum.ErrOutOfDomain) {
				t.Errorf("Canonical(%v, %s) = %v: only a sqrt scale has a domain on this side", v, du.Code, err)
			}
		}
	}
	if n < 15 {
		t.Errorf("only %d special scales other than sqrt in the XML", n)
	}
}

// Test 5: definiciones propias. [sqK] es sqrt(1 K); un prefijo propio Q de
// valor -1 hace α·v < 0 con v > 0 (UCUM §22 ■4: x = f⁻¹(α x')), y el signo de
// v solo no decide. La ida y vuelta por esa escala vuelve al valor.
func TestSqrtOwnPrefixLooksAtAlphaTimesValue(t *testing.T) {
	const custom = `<?xml version="1.0" encoding="ascii"?>
<root xmlns="http://unitsofmeasure.org/ucum-essence" version="9.9" revision="N/A"
      revision-date="2099-01-01">
 <prefix Code="Q" CODE="Q"><name>negative</name><printSymbol>Q</printSymbol><value value="-1">-1</value></prefix>
 <prefix Code="h" CODE="H"><name>hecto</name><printSymbol>h</printSymbol><value value="100">100</value></prefix>
 <base-unit Code="K" CODE="K" dim="C"><name>kelvin</name><printSymbol>K</printSymbol><property>temperature</property></base-unit>
 <unit Code="Cel" CODE="CEL" isMetric="yes" isSpecial="yes" class="si">
  <name>degree Celsius</name><printSymbol>C</printSymbol><property>temperature</property>
  <value Unit="cel(1 K)" UNIT="CEL(1 K)"><function name="Cel" value="1" Unit="K"/></value>
 </unit>
 <unit Code="[sqK]" CODE="[SQK]" isMetric="yes" isSpecial="yes" class="heat">
  <name>sqK</name><printSymbol>sqK</printSymbol><property>temperature</property>
  <value Unit="sqrt(1 K)" UNIT="SQRT(1 K)"><function name="sqrt" value="1" Unit="K"/></value>
 </unit>
</root>`
	svc, err := ucum.NewFromReader(strings.NewReader(custom))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Canonical(2, "Q[sqK]"); !errors.Is(err, ucum.ErrOutOfDomain) {
		t.Errorf("Canonical(2, Q[sqK]) = %v, want ErrOutOfDomain: α·v = -2", err)
	}
	if got, err := svc.Canonical(-2, "Q[sqK]"); err != nil || got.Value != 4 || got.Code != "K" {
		t.Errorf("Canonical(-2, Q[sqK]) = (%v, %v), want 4 K", got, err)
	}
	if got, err := svc.Canonical(-2, "h[sqK]"); !errors.Is(err, ucum.ErrOutOfDomain) {
		t.Errorf("Canonical(-2, h[sqK]) = (%v, %v), want ErrOutOfDomain", got, err)
	}
	if got, err := svc.Canonical(2, "h[sqK]"); err != nil || got.Value != 40000 {
		t.Errorf("Canonical(2, h[sqK]) = (%v, %v), want 40000 K", got, err)
	}

	// Hacia otra escala especial: de [sqK] a Cel, y la ida y vuelta.
	if _, err := svc.Convert(-2, "[sqK]", "Cel"); !errors.Is(err, ucum.ErrOutOfDomain) {
		t.Errorf("Convert(-2, [sqK], Cel) = %v, want ErrOutOfDomain", err)
	}
	r, err := svc.Convert(2, "Cel", "Q[sqK]")
	if err != nil {
		t.Fatal(err)
	}
	// r² es 275.15 K: (2 + 273.15)·1, con el signo del prefijo.
	if r >= 0 || math.Abs(r*r-275.15) > 1e-9 {
		t.Errorf("Convert(2, Cel, Q[sqK]) = %v, want -sqrt(275.15)", r)
	}
	back, err := svc.Convert(r, "Q[sqK]", "Cel")
	if err != nil || math.Abs(back-2) > 1e-12 {
		t.Errorf("round trip back = (%v, %v), want 2", back, err)
	}
	// Y con el valor del mismo signo que α·v inverso, no.
	if _, err := svc.Convert(-r, "Q[sqK]", "Cel"); !errors.Is(err, ucum.ErrOutOfDomain) {
		t.Errorf("Convert(%v, Q[sqK], Cel) = %v, want ErrOutOfDomain", -r, err)
	}
}
