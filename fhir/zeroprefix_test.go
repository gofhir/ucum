package fhir_test

import (
	"errors"
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

// Plan 016 §4.4: un prefijo de valor 0 visto desde CanonicalKey, Compare y
// ConvertDecimal. UCUM §27 ■2 («the scalar value by which the unit atom is
// multiplied») no excluye el 0, y §22 ■4 (x = f⁻¹(α x'), x' = f(x)/α) no tiene
// inversa con α = 0: Om-1 y el destino OCel son ErrDivisionByZero, sin pánico.
// El XML oficial no tiene ningún valor 0; el prefijo O se agrega aquí.

const zpXML = ` <prefix Code="O" CODE="ZZO">
  <name>zero prefix</name><printSymbol>O</printSymbol><value value="0">0</value>
 </prefix>
`

var (
	zpOnce sync.Once
	zpCmp  *fhir.Comparator
	errZP  error
)

func zpComparator(t testing.TB) *fhir.Comparator {
	t.Helper()
	zpOnce.Do(func() {
		raw, err := os.ReadFile("../internal/essence/ucum-essence.xml")
		if err != nil {
			errZP = err
			return
		}
		svc, err := ucum.NewFromReader(strings.NewReader(strings.Replace(string(raw), "</root>", zpXML+"</root>", 1)))
		if err != nil {
			errZP = err
			return
		}
		ex, ok := svc.(ucum.ExactService)
		if !ok {
			errZP = errors.New("service is not an ExactService")
			return
		}
		zpCmp = fhir.NewComparatorWith(ex)
	})
	if errZP != nil {
		t.Fatal(errZP)
	}
	return zpCmp
}

// zpCall corre f y convierte un pánico en un fallo del test.
func zpCall(t *testing.T, what string, f func() error) (err error, ok bool) { //nolint:revive,gocritic // el error del llamado va primero
	t.Helper()
	defer func() {
		if p := recover(); p != nil {
			t.Errorf("%s panicked: %v", what, p)
			err, ok = nil, false
		}
	}()
	return f(), true
}

func zpWantZeroDiv(t *testing.T, what string, f func() error) {
	t.Helper()
	if err, ok := zpCall(t, what, f); ok && !errors.Is(err, ucum.ErrDivisionByZero) {
		t.Errorf("%s: error = %v, want ErrDivisionByZero", what, err)
	}
}

func TestZeroPrefixFHIR(t *testing.T) {
	c := zpComparator(t)
	d, err := fhir.ParseDecimal("1.50")
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"Om-1", "Om-2", "OK-1", "/Om"} {
		q := fhir.Quantity{Value: 1, Code: code, System: fhir.UCUMSystem}
		zpWantZeroDiv(t, fmt.Sprintf("CanonicalKey(1 %s)", code), func() error {
			_, _, err := c.CanonicalKey(q)
			return err
		})
		other := fhir.Quantity{Value: 1, Code: "m", System: fhir.UCUMSystem}
		zpWantZeroDiv(t, fmt.Sprintf("Compare(1 %s, 1 m)", code), func() error {
			_, err := c.Compare(q, other)
			return err
		})
		zpWantZeroDiv(t, fmt.Sprintf("Compare(1 m, 1 %s)", code), func() error {
			_, err := c.Compare(other, q)
			return err
		})
		zpWantZeroDiv(t, fmt.Sprintf("ConvertDecimal(1.50 %s -> 1)", code), func() error {
			_, err := c.ConvertDecimal(d, code, "1")
			return err
		})
	}
	// OCel es un destino especial con α = 0: x' = f(x)/α no existe.
	zpWantZeroDiv(t, "ConvertDecimal(1.50 K -> OCel)", func() error {
		_, err := c.ConvertDecimal(d, "K", "OCel")
		return err
	})
	// Como origen, OCel existe: x = f⁻¹(α x') = f⁻¹(0) = 273.15 K para todo x'.
	q := fhir.Quantity{Value: 5, Code: "OCel", System: fhir.UCUMSystem}
	k := fhir.Quantity{Exact: big.NewRat(27315, 100), Code: "K", System: fhir.UCUMSystem}
	zpNoPanic(t, "CanonicalKey(5 OCel)", func() {
		code, v, err := c.CanonicalKey(q)
		if err != nil || code != "K" || math.Abs(v-273.15) > 1e-9 {
			t.Errorf("CanonicalKey(5 OCel) = (%q, %v, %v), want (\"K\", 273.15, nil)", code, v, err)
		}
	})
	zpNoPanic(t, "Compare(5 OCel, 273.15 K)", func() {
		if got, err := c.Compare(q, k); err != nil || got != 0 {
			t.Errorf("Compare(5 OCel, 273.15 K) = (%d, %v), want (0, nil)", got, err)
		}
	})
	zpNoPanic(t, "Compare(273.15 K, 5 OCel)", func() { _, _ = c.Compare(k, q) })
}

func zpNoPanic(t *testing.T, what string, f func()) {
	t.Helper()
	defer func() {
		if p := recover(); p != nil {
			t.Errorf("%s panicked: %v", what, p)
		}
	}()
	f()
}
