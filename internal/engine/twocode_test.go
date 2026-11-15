package engine

import (
	"errors"
	"math/big"
	"os"
	"strings"
	"testing"

	"github.com/gofhir/ucum/v4/internal/ucumerr"
)

// Plan 022: con dos códigos, la sintaxis de los dos se mira antes de expandir
// ninguno, como hace Convert. El orden de los errores es: análisis del primero,
// análisis del segundo, expansión del primero, expansión del segundo.
//
// El orden se prueba por la identidad del error: un primer código que solo
// falla al expandirse (m/0, Cel2) daría su error, y no el del segundo, si se
// expandiera antes de analizar el segundo. Los esperados salen de esa regla,
// no de correr el código.

// twoCodeCycleXML agrega al XML oficial dos unidades que se definen una a otra.
func twoCodeCycleXML(t testing.TB) string {
	t.Helper()
	raw, err := os.ReadFile(essenceFile)
	if err != nil {
		t.Fatal(err)
	}
	const extra = ` <unit Code="[cy_a]" CODE="[CY_A]" isMetric="no" class="dimless">
  <name>a</name><printSymbol>a</printSymbol><property>test</property>
  <value Unit="[cy_b]" UNIT="[CY_B]" value="1">a</value>
 </unit>
 <unit Code="[cy_b]" CODE="[CY_B]" isMetric="no" class="dimless">
  <name>b</name><printSymbol>b</printSymbol><property>test</property>
  <value Unit="[cy_a]" UNIT="[CY_A]" value="1">b</value>
 </unit>
`
	return strings.Replace(string(raw), "</root>", extra+"</root>", 1)
}

func newTwoCodeService(t testing.TB, insensitive bool) *Service {
	t.Helper()
	svc, err := New(strings.NewReader(twoCodeCycleXML(t)), insensitive)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

// twoCodeCase es un código que falla, con lo que identifica su error.
type twoCodeCase struct {
	code   string
	expand bool // falla solo al expandirse: pasa Validate
	is     func(error) bool
}

func hasText(s string) func(error) bool {
	return func(err error) bool { return strings.Contains(err.Error(), s) }
}

var twoCodeCases = []twoCodeCase{
	{"m/", false, hasText("unexpected end of expression")},
	{"xyz", false, hasText(`unknown unit "xyz"`)},
	{"", false, hasText("UCUM expression is empty")},
	{"m{", false, hasText("unterminated annotation")},
	{"m/0", true, func(err error) bool { return errors.Is(err, ucumerr.ErrDivisionByZero) }},
	{"Cel2", true, hasText("cannot carry an exponent")},
	{"[pH]/m", true, hasText("is not on a linear scale")},
	{"[cy_a]", true, hasText("definition cycle")},
}

// wantFirst dice cuál de los dos códigos nombra el error: el primero, salvo
// cuando el primero solo falla al expandirse y el segundo ya falla al
// analizarse.
func wantFirst(a, b twoCodeCase) bool { return !a.expand || b.expand }

type twoCodeEntry struct {
	name string
	call func(s *Service, a, b string) error
	// names: el error es un ValidationError cuyo Code es el código que falló.
	names bool
	// converts: el error es un ConversionError de a a b.
	converts bool
}

func twoCodeEntries() []twoCodeEntry {
	one := big.NewRat(1, 1)
	return []twoCodeEntry{
		{"Convert", func(s *Service, a, b string) error { _, err := s.Convert(1, a, b); return err }, false, true},
		{"IsComparable", func(s *Service, a, b string) error { _, err := s.IsComparable(a, b); return err }, true, false},
		{"Multiply", func(s *Service, a, b string) error {
			_, err := s.Multiply(Pair{1, a}, Pair{1, b})
			return err
		}, true, false},
		{"Divide", func(s *Service, a, b string) error {
			_, err := s.Divide(Pair{1, a}, Pair{1, b})
			return err
		}, true, false},
		{"ConversionFactor", func(s *Service, a, b string) error { _, err := s.ConversionFactor(a, b); return err }, false, true},
		{"ConvertRat", func(s *Service, a, b string) error { _, err := s.ConvertRat(one, a, b); return err }, false, true},
	}
}

func TestTwoCodeErrorOrder(t *testing.T) {
	svc := newTwoCodeService(t, false)
	for _, c := range twoCodeCases {
		if c.expand != (svc.Validate(c.code) == nil) {
			t.Fatalf("%q: expand = %v but Validate = %v", c.code, c.expand, svc.Validate(c.code))
		}
	}
	for _, e := range twoCodeEntries() {
		for _, a := range twoCodeCases {
			for _, b := range twoCodeCases {
				want := b
				if wantFirst(a, b) {
					want = a
				}
				err := e.call(svc, a.code, b.code)
				if err == nil {
					t.Errorf("%s(%q, %q) = nil, want the error of %q", e.name, a.code, b.code, want.code)
					continue
				}
				var ve *ucumerr.ValidationError
				if e.names && (!errors.As(err, &ve) || ve.Code != want.code) {
					t.Errorf("%s(%q, %q) = %v, want ValidationError.Code %q", e.name, a.code, b.code, err, want.code)
				}
				var ce *ucumerr.ConversionError
				if e.converts && (!errors.As(err, &ce) || ce.From != a.code || ce.To != b.code) {
					t.Errorf("%s(%q, %q) = %v, want a ConversionError from %q to %q", e.name, a.code, b.code, err, a.code, b.code)
				}
				for _, other := range twoCodeCases {
					if got := other.is(err); got != (other.code == want.code) {
						t.Errorf("%s(%q, %q): error %q identifies %q = %v, want the error of %q",
							e.name, a.code, b.code, err, other.code, got, want.code)
					}
				}
			}
		}
	}
}

// ConversionFactor decide ErrNotLinear con los AST, sin expandir el otro código.
func TestConversionFactorNotLinearWithoutExpanding(t *testing.T) {
	svc := newTwoCodeService(t, false)
	for _, c := range [][2]string{{"Cel", "m/0"}, {"m/0", "Cel"}, {"Cel", "[cy_a]"}, {"[cy_a]", "Cel"}} {
		_, err := svc.ConversionFactor(c[0], c[1])
		if !errors.Is(err, ErrNotLinear) {
			t.Errorf("ConversionFactor(%q, %q) = %v, want ErrNotLinear", c[0], c[1], err)
		}
	}
	// Con dos unidades especiales se informa el origen.
	for _, c := range [][2]string{{"Cel", "[pH]"}, {"[pH]", "Cel"}} {
		_, err := svc.ConversionFactor(c[0], c[1])
		if !errors.Is(err, ErrNotLinear) || !strings.Contains(err.Error(), `"`+c[0]+`"`) {
			t.Errorf("ConversionFactor(%q, %q) = %v, want ErrNotLinear naming %q", c[0], c[1], err, c[0])
		}
	}
}

// Multiply y Divide nombran en ValidationError.Code el código que falla al
// analizarse, aunque el primero falle al expandirse.
func TestCombineNamesTheCodeThatFailsToParse(t *testing.T) {
	svc := newTwoCodeService(t, false)
	for _, e := range twoCodeEntries()[2:4] {
		for _, c := range [][2]string{{"m/0", "m/"}, {"Cel2", "xyz"}, {"[pH]/m", "m{"}} {
			err := e.call(svc, c[0], c[1])
			var ve *ucumerr.ValidationError
			if !errors.As(err, &ve) || ve.Code != c[1] {
				t.Errorf("%s(%q, %q) = %v, want ValidationError.Code %q", e.name, c[0], c[1], err, c[1])
			}
		}
	}
}

// El control de nil de ConvertRat va antes que el análisis.
func TestConvertRatNilBeforeParse(t *testing.T) {
	svc := newTwoCodeService(t, false)
	if _, err := svc.ConvertRat(nil, "m/", "m"); !errors.Is(err, ErrNilValue) {
		t.Errorf("ConvertRat(nil, m/, m) = %v, want ErrNilValue", err)
	}
}

// El servicio que ignora mayúsculas sigue la misma regla.
func TestTwoCodeErrorOrderCaseInsensitive(t *testing.T) {
	svc := newTwoCodeService(t, true)
	_, err := svc.IsComparable("m/0", "M/")
	if err == nil || !strings.Contains(err.Error(), "unexpected end of expression") {
		t.Errorf("IsComparable(m/0, M/) = %v, want the parse error of M/", err)
	}
	_, err = svc.IsComparable("M/0", "m/0")
	if !errors.Is(err, ucumerr.ErrDivisionByZero) {
		t.Errorf("IsComparable(M/0, m/0) = %v, want division by zero", err)
	}
}

// Con un código válido, el error es el del que falla y lo nombra (Code, o From
// y To), también cuando falla al expandirse en el segundo lugar.
func TestTwoCodeNamesTheFailingCode(t *testing.T) {
	svc := newTwoCodeService(t, false)
	for _, e := range twoCodeEntries() {
		for _, c := range twoCodeCases {
			for _, pair := range [][2]string{{"m", c.code}, {c.code, "m"}} {
				err := e.call(svc, pair[0], pair[1])
				if err == nil || !c.is(err) {
					t.Errorf("%s(%q, %q) = %v, want the error of %q", e.name, pair[0], pair[1], err, c.code)
					continue
				}
				var ve *ucumerr.ValidationError
				if e.names && (!errors.As(err, &ve) || ve.Code != c.code) {
					t.Errorf("%s(%q, %q) = %v, want ValidationError.Code %q", e.name, pair[0], pair[1], err, c.code)
				}
				var ce *ucumerr.ConversionError
				if e.converts && (!errors.As(err, &ce) || ce.From != pair[0] || ce.To != pair[1]) {
					t.Errorf("%s(%q, %q) = %v, want a ConversionError from %q to %q", e.name, pair[0], pair[1], err, pair[0], pair[1])
				}
			}
		}
	}
}
