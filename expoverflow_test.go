package ucum_test

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"
	"sync"
	"testing"

	"github.com/gofhir/ucum/v4"
	"github.com/gofhir/ucum/v4/internal/essence"
)

// Plan 018: el exponente canónico que desborda int es un error, no otra
// dimensión. UCUM §9 ■1 dice solo «The exponent is an integer number» y no le
// pone tope; §18 ■3-■4 lo definen por álgebra (u/v = u·v⁻¹, uⁿ = Π u), así que
// el exponente de una cadena de definiciones es la suma y la multiplicación de los
// exponentes de sus niveles. §19 ■1 permite a una implementación no
// representar el conjunto infinito de ellas («need not be able to
// represent the infinite set of dimensions»); lo que no permite es dar otra
// dimensión en lugar de la pedida. Hoy 2^63 pasa a -2^63 y 2^64 a 0.
//
// Las cadenas son [ovF_i] = plantilla([ovF_{i+1}]) con valor 1 en cada nivel y
// la hoja en 1 m (no km: el valor crecería antes que el exponente, §9 de UCUM,
// y la prueba colgaría; el valor es el frente 019). [ovF_k] tiene 64-k niveles,
// así que una sola cadena de 64 sirve para 62, 63 y 64 niveles:
//
//	s: X2     e(i) = 2·e(i+1)       [ovs_2] = m^(2^62), [ovs_1] = 2^63, [ovs_0] = 2^64
//	d: X.X    e(i) = e(i+1)+e(i+1)  igual que s por suma
//	v: /X2    e(i) = -2·e(i+1)      [ovv_2] = (-2)^62 = 2^62; [ovv_1] pide 2·2^62 antes de negar
//	n: X-2    e(i) = -2·e(i+1)      [ovn_2] = 2^62; [ovn_1] = -2^63 = math.MinInt (cabe);
//	                                [ovn_0] = (-2)·(-2^63) = 2^64
//	u: X-1    e(i) = -e(i+1)        64 niveles dan +1: nunca desborda
//
// Los esperados salen de la aritmética: 2^62 = 4611686018427387904,
// 2^62-1 = 4611686018427387903, 2^62+1 = 4611686018427387905,
// 2^62+2^61 = 6917529027641081856, -2^63 = math.MinInt = -9223372036854775808
// y -2^63+1 = -9223372036854775807. Nada sale de correr la librería.

const (
	ovLevels = 64
	ovTwo62  = "4611686018427387904"
	ovMinInt = "-9223372036854775808"
)

func ovChain(family, tmpl string) string {
	var b strings.Builder
	for i := 0; i <= ovLevels; i++ {
		unit := "m"
		if i < ovLevels {
			unit = strings.ReplaceAll(tmpl, "X", fmt.Sprintf("[ov%s_%d]", family, i+1))
		}
		fmt.Fprintf(&b, ` <unit Code="[ov%[1]s_%[2]d]" CODE="[OV%[3]s_%[2]d]" isMetric="no" class="dimless">
  <name>ov</name><printSymbol>ov</printSymbol><property>test</property>
  <value Unit="%[4]s" UNIT="%[5]s" value="1">ov</value>
 </unit>
`, family, i, strings.ToUpper(family), unit, strings.ToUpper(unit))
	}
	return b.String()
}

func ovExtra() string {
	return ovChain("s", "X2") + ovChain("d", "X.X") + ovChain("v", "/X2") +
		ovChain("n", "X-2") + ovChain("u", "X-1")
}

var (
	ovOnce sync.Once
	ovXML  string
	ovCS   ucum.Service
	ovCI   ucum.Service
	errOv  error
)

func ovServices(t testing.TB) (cs, ci ucum.Service) {
	t.Helper()
	ovOnce.Do(func() {
		ovXML = dagOfficialXML(t, ovExtra())
		if ovCS, errOv = ucum.NewFromReader(strings.NewReader(ovXML)); errOv != nil {
			return
		}
		ovCI, errOv = ucum.NewCaseInsensitiveFromReader(strings.NewReader(ovXML))
	})
	if errOv != nil {
		t.Fatalf("building the services: %v", errOv)
	}
	return ovCS, ovCI
}

// ovCase: want == "" significa ErrExponentTooLarge; si no, el código canónico.
type ovCase struct {
	name, code, want string
}

func ovCases() []ovCase {
	return []ovCase{
		// 4.1: 62 niveles caben, 63 y 64 no.
		{"square 62 levels", "[ovs_2]", "m" + ovTwo62},
		{"square 63 levels", "[ovs_1]", ""},
		{"square 64 levels", "[ovs_0]", ""},
		{"product 62 levels", "[ovd_2]", "m" + ovTwo62},
		{"product 63 levels", "[ovd_1]", ""},
		{"product 64 levels", "[ovd_0]", ""},
		{"inverse square 62 levels", "[ovv_2]", "m" + ovTwo62},
		{"inverse square 63 levels", "[ovv_1]", ""},
		{"inverse square 64 levels", "[ovv_0]", ""},
		{"negative square 62 levels", "[ovn_2]", "m" + ovTwo62},
		{"negative square 64 levels", "[ovn_0]", ""},
		{"alternating sign 64 levels", "[ovu_0]", "m"},

		// -2^63 = math.MinInt cabe en int: es un resultado válido.
		{"negative square 63 levels reaches MinInt", "[ovn_1]", "m" + ovMinInt},
		{"MinInt plus one", "[ovn_1].m", "m-9223372036854775807"},
		{"two negative quotients reach MinInt", "/[ovs_2]/[ovs_2]", "m" + ovMinInt},
		// ... pero su opuesto (2^63), su doble y su potencia no.
		{"quotient negates MinInt", "/[ovn_1]", ""},
		{"quotient by MinInt", "m/[ovn_1]", ""},
		{"MinInt to the -1", "[ovn_1]-1", ""},
		{"MinInt squared", "[ovn_1]2", ""},
		{"MinInt times MinInt", "[ovn_1].[ovn_1]", ""},
		{"MinInt minus one", "[ovn_1]/m", ""},
		{"below MinInt by quotients", "/[ovs_2]/[ovs_2]/m", ""},

		// Operaciones del llamador sobre un exponente válido de 2^62.
		{"caller squares 2^62", "[ovs_2]2", ""},
		{"caller multiplies 2^62 by itself", "[ovs_2].[ovs_2]", ""},
		{"caller multiplies by m", "[ovs_2].m", "m4611686018427387905"},
		{"caller divides by m", "[ovs_2]/m", "m4611686018427387903"},
		{"caller multiplies 2^62 by 2^61", "[ovs_2].[ovs_3]", "m6917529027641081856"},
		{"caller divides 2^62 by itself", "[ovs_2]/[ovs_2]", "1"},
		{"caller squares 63-level product", "[ovd_1]2", ""},
	}
}

func ovCode(ci bool, code string) string {
	if ci {
		return strings.ToUpper(code)
	}
	return code
}

// 4.1, 4.3: los tres caminos exactos y el de float64 coinciden con la
// aritmética, en los dos servicios.
func TestExpOverflowCanonicalAPIs(t *testing.T) {
	cs, ci := ovServices(t)
	for _, isCI := range []bool{false, true} {
		svc := cs
		label := "case-sensitive"
		if isCI {
			svc, label = ci, "case-insensitive"
		}
		ex, ok := svc.(ucum.ExactService)
		if !ok {
			t.Fatalf("%s service is not an ExactService", label)
		}
		for _, c := range ovCases() {
			code := ovCode(isCI, c.code)
			t.Run(label+"/"+c.name, func(t *testing.T) {
				p, err := svc.Canonical(1, code)
				rp, rerr := ex.CanonicalRat(big.NewRat(1, 1), code)
				_, cerr := ex.ConversionFactor(code, code)
				_, verr := svc.Convert(1, code, code)
				same, serr := svc.IsComparable(code, code)
				if c.want == "" {
					for api, e := range map[string]error{
						"Canonical": err, "CanonicalRat": rerr, "ConversionFactor": cerr,
						"Convert": verr, "IsComparable": serr,
					} {
						if !errors.Is(e, ucum.ErrExponentTooLarge) {
							t.Errorf("%s(%q) error = %v, want ErrExponentTooLarge", api, code, e)
						}
					}
					if cmp, e := svc.IsComparable(code, "1"); !errors.Is(e, ucum.ErrExponentTooLarge) {
						t.Errorf("IsComparable(%q, \"1\") = %v, %v, want ErrExponentTooLarge", code, cmp, e)
					}
					return
				}
				if err != nil || rerr != nil || cerr != nil || verr != nil || serr != nil {
					t.Fatalf("%q: errors %v %v %v %v %v, want none", code, err, rerr, cerr, verr, serr)
				}
				if !strings.EqualFold(p.Code, c.want) || !strings.EqualFold(rp.Code, c.want) {
					t.Errorf("%q: canonical code %q / %q, want %q", code, p.Code, rp.Code, c.want)
				}
				if p.Value != 1 || rp.Value.Cmp(big.NewRat(1, 1)) != 0 {
					t.Errorf("%q: canonical value %v / %v, want 1", code, p.Value, rp.Value)
				}
				if !same {
					t.Errorf("IsComparable(%q, %q) = false", code, code)
				}
			})
		}
	}
}

// 4.3: con 64 niveles el exponente daba vuelta hasta 0 y la unidad resultaba
// comparable con «1»: otra dimensión, no la pedida.
func TestExpOverflowIsComparableWithDimensionlessIsAnError(t *testing.T) {
	cs, ci := ovServices(t)
	for _, f := range []string{"s", "d", "v", "n"} {
		code := "[ov" + f + "_0]"
		for label, svc := range map[string]ucum.Service{"cs": cs, "ci": ci} {
			c := code
			if label == "ci" {
				c = strings.ToUpper(c)
			}
			ok, err := svc.IsComparable(c, "1")
			if !errors.Is(err, ucum.ErrExponentTooLarge) || ok {
				t.Errorf("%s IsComparable(%q, \"1\") = %v, %v, want false, ErrExponentTooLarge", label, c, ok, err)
			}
			if err := svc.ValidateInProperty(c, "test"); !errors.Is(err, ucum.ErrExponentTooLarge) {
				t.Errorf("%s ValidateInProperty(%q) = %v, want ErrExponentTooLarge", label, c, err)
			}
		}
	}
}

// Un error no se queda en el caché: lo válido de la misma cadena sigue valiendo
// y lo inválido sigue fallando en la segunda llamada.
func TestExpOverflowErrorDoesNotPoisonTheCache(t *testing.T) {
	svc, err := ucum.NewFromReader(strings.NewReader(dagOfficialXML(t, ovExtra())))
	if err != nil {
		t.Fatal(err)
	}
	for round := 0; round < 2; round++ {
		if _, err := svc.Canonical(1, "[ovs_0]"); !errors.Is(err, ucum.ErrExponentTooLarge) {
			t.Errorf("round %d: [ovs_0] error = %v, want ErrExponentTooLarge", round, err)
		}
		if p, err := svc.Canonical(1, "[ovs_2]"); err != nil || p.Code != "m"+ovTwo62 {
			t.Errorf("round %d: [ovs_2] = %v, %v, want m%s", round, p, err, ovTwo62)
		}
		if _, err := svc.Canonical(1, "[ovs_1]"); !errors.Is(err, ucum.ErrExponentTooLarge) {
			t.Errorf("round %d: [ovs_1] error = %v, want ErrExponentTooLarge", round, err)
		}
		if p, err := svc.Canonical(1, "[ovs_5]"); err != nil || p.Code != "m576460752303423488" {
			t.Errorf("round %d: [ovs_5] = %v, %v, want m2^59 = 576460752303423488", round, p, err)
		}
	}
}

// 4.2: Multiply y Divide de dos operandos válidos cuyo exponente suma de más.
func TestExpOverflowMultiplyDivide(t *testing.T) {
	cs, ci := ovServices(t)
	type op struct {
		name   string
		mul    bool
		a, b   string
		failed bool
		// want es el código canónico del resultado: -2^63+1, 1, 2^62+2^61 y
		// 2^62-2^61 = 2^61, por aritmética.
		want string
	}
	ops := []op{
		{"multiply 2^62 by 2^62", true, "[ovs_2]", "[ovs_2]", true, ""},
		{"multiply by the same 63-level product through d", true, "[ovd_2]", "[ovs_2]", true, ""},
		{"multiply MinInt by MinInt", true, "[ovn_1]", "[ovn_1]", true, ""},
		{"multiply MinInt by m", true, "[ovn_1]", "m", false, "m-9223372036854775807"},
		{"multiply 2^62 by its inverse", true, "[ovs_2]", "/[ovs_2]", false, "1"},
		{"multiply 2^62 by 2^61", true, "[ovs_2]", "[ovs_3]", false, "m6917529027641081856"},
		{"divide 2^62 by its inverse", false, "[ovs_2]", "/[ovs_2]", true, ""},
		{"divide 1 by MinInt", false, "1", "[ovn_1]", true, ""},
		{"divide m by MinInt", false, "m", "[ovn_1]", true, ""},
		{"divide 2^62 by itself", false, "[ovs_2]", "[ovs_2]", false, "1"},
		{"divide 2^62 by 2^61", false, "[ovs_2]", "[ovs_3]", false, "m2305843009213693952"},
	}
	for label, svc := range map[string]ucum.Service{"cs": cs, "ci": ci} {
		for _, o := range ops {
			t.Run(label+"/"+o.name, func(t *testing.T) {
				a, b := o.a, o.b
				if label == "ci" {
					a, b = strings.ToUpper(a), strings.ToUpper(b)
				}
				f := svc.Divide
				if o.mul {
					f = svc.Multiply
				}
				p, err := f(ucum.Pair{Value: 1, Code: a}, ucum.Pair{Value: 1, Code: b})
				if o.failed {
					if !errors.Is(err, ucum.ErrExponentTooLarge) {
						t.Errorf("(%q, %q) = %v, %v, want ErrExponentTooLarge", a, b, p, err)
					}
					return
				}
				if err != nil {
					t.Errorf("(%q, %q) error = %v, want none", a, b, err)
					return
				}
				// El código del resultado es elección del servicio; su forma
				// canónica no.
				if got, cerr := svc.Canonical(p.Value, p.Code); cerr != nil || got.Value != 1 || got.Code != o.want {
					t.Errorf("(%q, %q) = %v, canonical (%v, %v), want 1 %s", a, b, p, got, cerr, o.want)
				}
			})
		}
	}
}

// 4.5: el XML oficial no produce este error con ningún exponente de uso real,
// y el caché frío da lo mismo que el caliente. Cada unidad (con los prefijos de
// las métricas) va con los exponentes -4..4; ±1000 solo a las de valor 1, para
// que el valor no sea el que cuesta (§6 del plan, 019).
func TestExpOverflowOfficialUnitsNeverOverflow(t *testing.T) {
	m, err := essence.Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	var codes []string
	for _, b := range m.BaseUnits {
		codes = append(codes, b.Code)
	}
	for _, u := range m.DefinedUnits {
		codes = append(codes, u.Code)
		if !u.IsMetric {
			continue
		}
		for _, p := range m.Prefixes {
			codes = append(codes, p.Code+u.Code)
		}
	}
	cold, err := ucum.NewExact()
	if err != nil {
		t.Fatal(err)
	}
	type res struct {
		code string
		val  float64
		err  string
	}
	run := func(svc ucum.ExactService, code string) res {
		p, err := svc.Canonical(1, code)
		r := res{code: p.Code, val: p.Value}
		if err != nil {
			r.err = err.Error()
			if errors.Is(err, ucum.ErrExponentTooLarge) {
				t.Errorf("%q: unexpected ErrExponentTooLarge", code)
			}
		}
		return r
	}
	same := func(a, b res) bool {
		return a.code == b.code && a.err == b.err &&
			(a.val == b.val || (math.IsNaN(a.val) && math.IsNaN(b.val)))
	}
	var all []string
	for _, c := range codes {
		for e := -4; e <= 4; e++ {
			if e == 0 {
				all = append(all, c)
			} else {
				all = append(all, fmt.Sprintf("%s%d", c, e))
			}
		}
		if p, err := cold.Canonical(1, c); err == nil && p.Value == 1 {
			all = append(all, c+"1000", c+"-1000")
		}
	}
	first := make([]res, len(all))
	for i, c := range all {
		first[i] = run(cold, c)
	}
	warm, err := ucum.NewExact()
	if err != nil {
		t.Fatal(err)
	}
	for i := len(all) - 1; i >= 0; i-- { // otro orden, otro servicio
		if r := run(warm, all[i]); !same(r, first[i]) {
			t.Errorf("%q: first service %v, second service %v", all[i], first[i], r)
		}
	}
	for i, c := range all { // segunda pasada sobre el caché ya lleno
		if r := run(cold, c); !same(r, first[i]) {
			t.Errorf("%q: cold %v, warm %v", c, first[i], r)
		}
	}
	if len(all) < 10000 {
		t.Fatalf("swept only %d codes", len(all))
	}
}

// Un exponente 0 sobre una unidad de exponente 2^62 da la unidad (§18 ■4: u^0
// es la multiplicación vacía): mulExponent con b = 0 no divide por cero.
func TestExpOverflowZeroExponentIsUnity(t *testing.T) {
	cs, _ := ovServices(t)
	if got, err := cs.Canonical(1, "[ovs_2]0"); err != nil || got != (ucum.Pair{Value: 1, Code: "1"}) {
		t.Errorf("Canonical(1, [ovs_2]0) = (%v, %v), want 1 1", got, err)
	}
}

// El mensaje del error no crece con la profundidad: una cadena de 300 niveles
// desborda a 63 de la hoja, y los 237 de arriba no agregan «expand unit» cada
// uno (antes, unos 5400 bytes).
func TestExpOverflowMessageStaysShort(t *testing.T) {
	var b strings.Builder
	const n = 300
	for i := 0; i <= n; i++ {
		unit := "m"
		if i < n {
			unit = fmt.Sprintf("[ovl_%d]2", i+1)
		}
		fmt.Fprintf(&b, ` <unit Code="[ovl_%[1]d]" CODE="[OVL_%[1]d]" isMetric="no" class="dimless">
  <name>ov</name><printSymbol>ov</printSymbol><property>test</property>
  <value Unit="%[2]s" UNIT="%[3]s" value="1">ov</value>
 </unit>
`, i, unit, strings.ToUpper(unit))
	}
	svc, err := ucum.NewFromReader(strings.NewReader(dagOfficialXML(t, b.String())))
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Canonical(1, "[ovl_0]")
	if !errors.Is(err, ucum.ErrExponentTooLarge) {
		t.Fatalf("Canonical(1, [ovl_0]) error = %v, want ErrExponentTooLarge", err)
	}
	if len(err.Error()) > 300 {
		t.Errorf("error message is %d bytes, want at most 300: %.200s…", len(err.Error()), err.Error())
	}
	// [ovl_k] vale m^(2^(300-k)): [ovl_238] es m^(2^62) y elevarlo al cuadrado en
	// la definición de [ovl_237] es el primer desborde; el mensaje la nombra.
	if !strings.Contains(err.Error(), `"[ovl_237]"`) {
		t.Errorf("error message %q does not name [ovl_237], the unit whose definition overflows", err)
	}
}

// El error de Multiply o Divide nombra la operación: el exponente que no cabe es
// el del resultado, no el de un operando.
func TestExpOverflowMultiplyDivideNameTheOperation(t *testing.T) {
	cs, _ := ovServices(t)
	one := func(code string) ucum.Pair { return ucum.Pair{Value: 1, Code: code} }
	if _, err := cs.Multiply(one("[ovs_2]"), one("[ovs_2]")); err == nil || !strings.Contains(err.Error(), "[ovs_2] times [ovs_2]") {
		t.Errorf("Multiply([ovs_2], [ovs_2]) error = %v, want it to name the product", err)
	}
	if _, err := cs.Divide(one("[ovs_2]"), one("/[ovs_2]")); err == nil || !strings.Contains(err.Error(), "[ovs_2] divided by /[ovs_2]") {
		t.Errorf("Divide([ovs_2], /[ovs_2]) error = %v, want it to name the quotient", err)
	}
}
