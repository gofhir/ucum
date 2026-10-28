package engine

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"os"
	"strings"
	"testing"

	"github.com/gofhir/ucum/v4/internal/expr"
	"github.com/gofhir/ucum/v4/internal/model"
	"github.com/gofhir/ucum/v4/internal/ucumerr"
)

// Plan 002: las clases de diferencia con Ucum-java. Cada una es un predicado
// angosto que explica los dos lados: el valor de Java con el modelo de Java
// (citado de su código en db9d22ad) y el nuestro con un esperado independiente
// de los dos motores. TestCompareJava falla con una diferencia sin clase o con
// más de una, con una clase que no explica ningún caso y con un bug-ours sin
// issue.
//
// Las citas de Ucum-java son de src/main/java/org/fhir/ucum/ en el commit
// db9d22ade3f0f4a667b45817b85a9ac4534e4224 (tag 1.0.10).

type javaClass struct {
	Name    string
	Kind    string // bug-ours, bug-java o deliberate
	Reason  string
	Spec    string
	JavaRef string
	Issue   string
	match   func(x *javaContext, d javaDiff) bool
}

// El modelo de Java, escrito a mano desde su código a propósito: es la
// conducta de la referencia, no un dato de UCUM.
var (
	// Los HoldingHandler de special/Registry.java:57-70: unidades especiales
	// que Java reemplaza por una unidad fija, como si fueran lineales. El
	// factor que usa sale de lo que Java respondió para 1 de cada una.
	javaHolding = map[string]bool{
		"[p'diop]": true, "%[slope]": true, "[hp_X]": true, "[hp_C]": true, "[pH]": true,
		"Np": true, "B": true, "B[SPL]": true, "B[V]": true, "B[mV]": true, "B[uV]": true,
		"B[W]": true, "B[kW]": true, "bit_s": true,
	}
	// CelsiusHandler y FahrenheitHandler, en special/Registry.java:55-56, los
	// dos con offset: Converter.java:168-170 los rechaza.
	javaOffset = map[string]bool{"Cel": true, "[degF]": true}

	// Converter.java:164 y :170.
	javaMsgUnregistered = "Not handled yet (special unit)"
	javaMsgOffset       = "Not handled yet (special unit with offset from 0 at intersect)"
)

// javaSyntaxBugs son las expresiones que Java acepta y la gramática de UCUM
// no, con la regla que las rechaza.
var javaSyntaxBugs = map[string]string{
	"":       "UcumFunctionalTests.xml, case 1-103",
	"m//s":   "§7■3 and the BNF of §10 Exhibit 1: a solidus is unary only at the start of a term",
	"m)":     "BNF of §10 Exhibit 1: unbalanced parenthesis",
	"{a}{b}": "BNF of §10 Exhibit 1: an annotation follows a symbol or stands alone, never another annotation",
}

// essenceFile es ucum-essence.xml, el mismo archivo que el motor embebe.
const essenceFile = "../essence/ucum-essence.xml"

// essenceFunction es el elemento function de una unidad especial.
type essenceFunction struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
	Unit  string `xml:"Unit,attr"`
}

// essenceXML es lo que las clases leen de ucum-essence.xml.
type essenceXML struct {
	Prefixes []struct {
		Code  string `xml:"Code,attr"`
		Value struct {
			Value string `xml:"value,attr"`
		} `xml:"value"`
	} `xml:"prefix"`
	Bases []struct {
		Code     string `xml:"Code,attr"`
		Property string `xml:"property"`
	} `xml:"base-unit"`
	Units []struct {
		Code      string `xml:"Code,attr"`
		Special   string `xml:"isSpecial,attr"`
		Arbitrary string `xml:"isArbitrary,attr"`
		Metric    string `xml:"isMetric,attr"`
		Property  string `xml:"property"`
		Value     struct {
			Unit     string          `xml:"Unit,attr"`
			Value    string          `xml:"value,attr"`
			Function essenceFunction `xml:"function"`
		} `xml:"value"`
	} `xml:"unit"`
}

// readEssence lee ucum-essence.xml con encoding/xml y big.Rat, sin
// internal/essence ni internal/model.
func readEssence(t *testing.T) essenceXML {
	t.Helper()
	f, err := os.Open(essenceFile)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	dec := xml.NewDecoder(f)
	// El archivo declara encoding="ascii", que es un subconjunto de UTF-8.
	dec.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	var e essenceXML
	if err := dec.Decode(&e); err != nil {
		t.Fatal(err)
	}
	if len(e.Prefixes) == 0 || len(e.Bases) == 0 || len(e.Units) == 0 {
		t.Fatalf("%s: %d prefixes, %d base units, %d units", essenceFile, len(e.Prefixes), len(e.Bases), len(e.Units))
	}
	return e
}

// javaContext tiene lo que comparten los predicados: el servicio, los
// átomos del XML y lo que Java respondió para 1 de cada átomo.
type javaContext struct {
	s            *Service
	arbitrary    map[string]bool
	special      map[string]bool
	property     map[string]string          // código → property declarada en el XML
	definedAs    map[string]string          // código → la expresión de su definición
	atomChecked  map[string]bool            // memo de ourAtomFromXML
	base         map[string]bool            // unidades base del XML
	dims         map[string]map[string]int  // memo de xmlDims; nil si no se pudo
	prefix       map[string]*big.Rat        // código → valor del prefijo
	function     map[string]essenceFunction // unidad especial → su función
	definedValue map[string]*big.Rat        // código → el número de su definición
	javaAtom     map[string]javaOut
}

func newJavaContext(t *testing.T, s *Service, cases []javaCase, outs []javaOut) *javaContext {
	t.Helper()
	x := &javaContext{s: s, arbitrary: map[string]bool{}, special: map[string]bool{},
		property: map[string]string{}, definedAs: map[string]string{}, atomChecked: map[string]bool{},
		base: map[string]bool{}, dims: map[string]map[string]int{},
		prefix: map[string]*big.Rat{}, function: map[string]essenceFunction{},
		definedValue: map[string]*big.Rat{}, javaAtom: map[string]javaOut{}}
	// Lo demás sale del XML leído aparte (readEssence), no del modelo que el
	// motor cargó: un defecto del cargador no puede confirmar sus propios
	// valores.
	e := readEssence(t)
	for _, p := range e.Prefixes {
		v, ok := new(big.Rat).SetString(p.Value.Value)
		if !ok {
			t.Fatalf("prefix %s: value %q", p.Code, p.Value.Value)
		}
		x.prefix[p.Code] = v
	}
	for _, b := range e.Bases {
		x.base[b.Code] = true
		x.property[b.Code] = b.Property
	}
	for _, u := range e.Units {
		x.arbitrary[u.Code] = u.Arbitrary == "yes"
		x.property[u.Code] = u.Property
		x.special[u.Code] = u.Special == "yes"
		if u.Special == "yes" {
			x.function[u.Code] = u.Value.Function
			continue
		}
		v, ok := new(big.Rat).SetString(u.Value.Value)
		if !ok {
			t.Fatalf("unit %s: value %q", u.Code, u.Value.Value)
		}
		x.definedAs[u.Code] = u.Value.Unit
		x.definedValue[u.Code] = v
	}
	for i, c := range cases {
		if c.Source == "xml-atom" && c.Op == "canonical" && outs[i].Status == "ok" {
			x.javaAtom[c.Args[1]] = outs[i]
		}
	}
	return x
}

// prefixOf es el valor de un prefijo según readEssence; 1 sin prefijo.
func (x *javaContext) prefixOf(p *model.Prefix) (*big.Rat, bool) {
	if p == nil {
		return big.NewRat(1, 1), true
	}
	v, ok := x.prefix[p.Code]
	if !ok {
		return nil, false
	}
	return new(big.Rat).Set(v), true
}

// parse y defParse son los parsers del motor, con una comprobación que no
// depende de ellos: los átomos que resuelven (prefijo y unidad) tienen que
// estar escritos, en orden, en el texto. Así un parser que lea nV como uV no
// puede confirmar lo que calcula con ellos.
func (x *javaContext) parse(code string) (*expr.Term, error) {
	return checkSpelling(code, x.s.parser.Parse)
}

func (x *javaContext) defParse(code string) (*expr.Term, error) {
	return checkSpelling(code, x.s.defParser.Parse)
}

func checkSpelling(code string, p func(string) (*expr.Term, error)) (*expr.Term, error) {
	t, err := p(code)
	if err != nil {
		return nil, err
	}
	var sp []string
	var walk func(t *expr.Term)
	walk = func(t *expr.Term) {
		for ; t != nil; t = t.Term {
			switch c := t.Comp.(type) {
			case *expr.Symbol:
				pc := ""
				if c.Prefix != nil {
					pc = c.Prefix.Code
				}
				sp = append(sp, pc+c.Unit.Code)
			case *expr.Term:
				walk(c)
			}
		}
	}
	walk(t)
	rest := code
	for _, a := range sp {
		i := strings.Index(rest, a)
		if i < 0 {
			return nil, fmt.Errorf("parser resolved %q, which is not written in %q", a, code)
		}
		rest = rest[i+len(a):]
	}
	return t, nil
}

// canonicalScaleOne: un código canónico vale exactamente 1 en las unidades
// base según el XML (273.15 mK tiene la dimensión de K, pero no es canónico).
func (x *javaContext) canonicalScaleOne(code string) bool {
	if code == "" || code == "1" {
		return true
	}
	f, ok := x.xmlFactor(code, x.parse)
	return ok && f.Cmp(big.NewRat(1, 1)) == 0
}

// claimTol es lo que Java da por bueno de su valor: dos unidades en la última
// de las P cifras significativas que su Decimal declara.
func claimTol(j javaOut) *big.Rat {
	r := pow10(leadingExp(j.A) - j.P + 1)
	return r.Mul(r, big.NewRat(2, 1))
}

// within dice si |a − b| ≤ tol + javaRelTol·|b|.
func within(a, b, tol *big.Rat) bool {
	t := new(big.Rat).SetFloat64(javaRelTol)
	t.Mul(t, new(big.Rat).Abs(b))
	t.Add(t, tol)
	d := new(big.Rat).Sub(a, b)
	return d.Abs(d).Cmp(t) <= 0
}

// javaAtomValue es lo que Java respondió para 1 de un átomo.
func (x *javaContext) javaAtomValue(code string) (*big.Rat, bool) {
	j, ok := x.javaAtom[code]
	if !ok {
		return nil, false
	}
	return ratOf(j.A)
}

// javaFactor evalúa una expresión con la aritmética de Java sobre los valores
// que Java dio para cada átomo, de izquierda a derecha como su Converter
// (Converter.java:62-110). Los prefijos son potencias de diez exactas.
func (x *javaContext) javaFactor(t *expr.Term) (*big.Rat, bool) {
	return x.factorWith(t, x.javaAtomValue)
}

// ourAtomValue es nuestro valor exacto de 1 de un átomo lineal.
func (x *javaContext) ourAtomValue(code string) (*big.Rat, bool) {
	o := ourCanonical(x.s, "1", code)
	return o.Exact, o.Status == "ok" && o.Exact != nil
}

// factorWith evalúa una expresión exactamente, con el valor de cada átomo
// que da atom.
func (x *javaContext) factorWith(t *expr.Term, atom func(string) (*big.Rat, bool)) (*big.Rat, bool) {
	out := big.NewRat(1, 1)
	div := false
	for ; t != nil; t = t.Term {
		var f *big.Rat
		switch c := t.Comp.(type) {
		case *expr.Symbol:
			v, ok := atom(c.Unit.Code)
			if !ok {
				return nil, false
			}
			pv, ok := x.prefixOf(c.Prefix)
			if !ok {
				return nil, false
			}
			f = new(big.Rat).Mul(v, pv)
			if f.Sign() == 0 {
				return nil, false
			}
			p := big.NewRat(1, 1)
			for range abs(c.Exponent) {
				p.Mul(p, f)
			}
			if c.Exponent < 0 {
				p.Inv(p)
			}
			f = p
		case *expr.Factor:
			f = big.NewRat(int64(c.Value), 1)
		case *expr.Term:
			var ok bool
			if f, ok = x.factorWith(c, atom); !ok {
				return nil, false
			}
		default:
			return nil, false
		}
		if div {
			if f.Sign() == 0 {
				return nil, false
			}
			out.Quo(out, f)
		} else {
			out.Mul(out, f)
		}
		div = t.Op == expr.OpDivision
	}
	return out, true
}

// unitFactor es javaFactor de un código. En canonical (fromDefinition), un
// átomo solo, sin prefijo ni exponente, se evalúa desde su definición en el
// XML, porque evaluarlo con su propio valor de Java no probaría nada; en las
// demás operaciones Java usa ese valor, ya redondeado.
func (x *javaContext) unitFactor(code string, fromDefinition bool) (*big.Rat, bool) {
	t, err := x.parse(code)
	if err != nil {
		return nil, false
	}
	if sym := expr.LoneSymbol(t); fromDefinition && sym != nil && sym.Prefix == nil && sym.Exponent == 1 {
		if def, ok := x.definedAs[sym.Unit.Code]; ok {
			dt, err := x.defParse(def)
			if err != nil {
				return nil, false
			}
			f, ok := x.javaFactor(dt)
			if !ok {
				return nil, false
			}
			return f.Mul(f, x.definedValue[sym.Unit.Code]), true
		}
	}
	return x.javaFactor(t)
}

// javaModel es el resultado del caso según la aritmética de Java sobre sus
// propios valores de cada átomo.
func (x *javaContext) javaModel(c javaCase) (*big.Rat, bool) {
	a := c.Args
	val := func(s string) *big.Rat { v, _ := ratOf(s); return v }
	switch c.Op {
	case "canonical":
		f, ok := x.unitFactor(a[1], true)
		if !ok || val(a[0]) == nil {
			return nil, false
		}
		return f.Mul(f, val(a[0])), true
	case "convert":
		src, ok1 := x.unitFactor(a[1], false)
		dst, ok2 := x.unitFactor(a[2], false)
		if !ok1 || !ok2 || dst.Sign() == 0 || val(a[0]) == nil {
			return nil, false
		}
		return src.Mul(src, val(a[0])).Quo(src, dst), true
	case "multiply", "divide":
		l, ok1 := x.unitFactor(a[1], false)
		r, ok2 := x.unitFactor(a[3], false)
		if !ok1 || !ok2 || val(a[0]) == nil || val(a[2]) == nil {
			return nil, false
		}
		l.Mul(l, val(a[0]))
		r.Mul(r, val(a[2]))
		if c.Op == "multiply" {
			return l.Mul(l, r), true
		}
		if r.Sign() == 0 {
			return nil, false
		}
		return l.Quo(l, r), true
	}
	return nil, false
}

// javaSelfConsistent: el valor de Java es su propia aritmética sobre sus
// valores de cada átomo, dentro de lo que Java declara para el resultado.
func (x *javaContext) javaSelfConsistent(d javaDiff) bool {
	j, ok := ratOf(d.Java.A)
	if !ok || d.Java.P <= 0 {
		return false
	}
	w, ok := x.javaModel(d.Case)
	return ok && within(j, w, claimTol(d.Java))
}

// ourAtomsAgree: en cada átomo lineal del caso, nuestro valor de 1 es
// exactamente su definición en el XML (ourAtomFromXML) y coincide con el de
// Java dentro de lo que Java declara para ese valor. Lo primero no se puede
// omitir: un entero del XML le vale a Java una cifra, y con esa tolerancia
// sola un factor nuestro al doble pasaría.
func (x *javaContext) ourAtomsAgree(d javaDiff) bool {
	for _, u := range unitArgs(d.Case) {
		for _, a := range x.atomsOf(u) {
			if x.special[a.code] {
				continue
			}
			j, ok := x.javaAtom[a.code]
			jv, ok2 := ratOf(j.A)
			o := ourCanonical(x.s, "1", a.code)
			if !ok || !ok2 || j.P <= 0 || o.Exact == nil || !within(jv, o.Exact, claimTol(j)) ||
				!x.ourAtomFromXML(a.code) {
				return false
			}
		}
	}
	return true
}

// ourAtomFromXML: nuestro valor de 1 de un átomo es el valor de su definición
// por su expresión evaluada con los átomos que nombra, cada uno comprobado
// igual (xmlAtomValue), y el de una unidad base es 1. Así, por inducción sobre
// las definiciones, cada valor nuestro es el del XML sin pasar por el código
// que canonicaliza. Se memoriza por átomo; las definiciones no tienen ciclos,
// y uno daría false.
func (x *javaContext) ourAtomFromXML(code string) bool {
	if ok, seen := x.atomChecked[code]; seen {
		return ok
	}
	x.atomChecked[code] = false
	ok := false
	if o, found := x.ourAtomValue(code); found {
		if def, defined := x.definedAs[code]; !defined {
			ok = x.base[code] && o.Cmp(big.NewRat(1, 1)) == 0
		} else if dt, err := x.defParse(def); err == nil {
			if f, fok := x.factorWith(dt, x.xmlAtomValue); fok {
				ok = f.Mul(f, x.definedValue[code]).Cmp(o) == 0
			}
		}
	}
	x.atomChecked[code] = ok
	return ok
}

// xmlAtomValue es nuestro valor de un átomo lineal, solo si ourAtomFromXML lo
// ata al XML.
func (x *javaContext) xmlAtomValue(code string) (*big.Rat, bool) {
	if x.special[code] || !x.ourAtomFromXML(code) {
		return nil, false
	}
	return x.ourAtomValue(code)
}

// xmlFactor es el valor de 1 de una expresión lineal, en las unidades base,
// con los valores de xmlAtomValue.
func (x *javaContext) xmlFactor(code string, p func(string) (*expr.Term, error)) (*big.Rat, bool) {
	t, err := p(code)
	if err != nil {
		return nil, false
	}
	return x.factorWith(t, x.xmlAtomValue)
}

// xmlDims da la dimensión de un átomo según el XML, sin pasar por el
// motor: una unidad base es la suya; una arbitraria, la suya (§24■3); una
// especial, la de su unidad de referencia; una definida, la de su definición.
func (x *javaContext) xmlDims(code string) (map[string]int, bool) {
	if d, seen := x.dims[code]; seen {
		return d, d != nil
	}
	x.dims[code] = nil
	var out map[string]int
	switch {
	case x.base[code]:
		out = map[string]int{code: 1}
	case x.special[code]:
		if _, unit, _, ok := x.specialReference(code); ok {
			out, _ = x.exprDims(unit, x.defParse)
		}
	default:
		def, ok := x.definedAs[code]
		if !ok {
			break
		}
		if x.arbitrary[code] {
			// Cada unidad arbitraria es la suya, aunque se defina con otra:
			// §24■3 no deja comparar dos arbitrarias, y el motor tampoco
			// ([IU] se define como 1 [iU] y no se convierte en ella).
			out = map[string]int{code: 1}
			break
		}
		out, _ = x.exprDims(def, x.defParse)
	}
	x.dims[code] = out
	return out, out != nil
}

// exprDims da la dimensión de una expresión según xmlDims. "" y "1" son
// adimensionales.
func (x *javaContext) exprDims(code string, p func(string) (*expr.Term, error)) (map[string]int, bool) {
	if code == "" || code == "1" {
		return map[string]int{}, true
	}
	t, err := p(code)
	if err != nil {
		return nil, false
	}
	return x.termDims(t)
}

func (x *javaContext) termDims(t *expr.Term) (map[string]int, bool) {
	out := map[string]int{}
	div := false
	for ; t != nil; t = t.Term {
		var d map[string]int
		k := 1
		switch c := t.Comp.(type) {
		case *expr.Symbol:
			ad, ok := x.xmlDims(c.Unit.Code)
			if !ok {
				return nil, false
			}
			d, k = ad, c.Exponent
		case *expr.Factor:
		case *expr.Term:
			var ok bool
			if d, ok = x.termDims(c); !ok {
				return nil, false
			}
		default:
			return nil, false
		}
		if div {
			k = -k
		}
		for u, e := range d {
			out[u] += e * k
			if out[u] == 0 {
				delete(out, u)
			}
		}
		div = t.Op == expr.OpDivision
	}
	return out, true
}

// codeDims da la dimensión de un código de entrada o de resultado.
func (x *javaContext) codeDims(code string) (map[string]int, bool) {
	return x.exprDims(code, x.parse)
}

// caseDims da la dimensión que el resultado de un caso tiene que tener.
func (x *javaContext) caseDims(c javaCase) (map[string]int, bool) {
	a := c.Args
	switch c.Op {
	case "canonical":
		return x.codeDims(a[1])
	case "multiply", "divide":
		l, ok1 := x.codeDims(a[1])
		r, ok2 := x.codeDims(a[3])
		if !ok1 || !ok2 {
			return nil, false
		}
		k := 1
		if c.Op == "divide" {
			k = -1
		}
		for u, e := range r {
			l[u] += k * e
			if l[u] == 0 {
				delete(l, u)
			}
		}
		return l, true
	}
	return nil, false
}

// operandCanonical es el valor canónico de v en u sin pasar por nuestro
// cálculo del caso: una unidad especial sola, por la fórmula de la
// especificación; una expresión lineal, por nuestros valores de cada átomo
// (que ourAtomsAgree ata a los de Java). Una expresión que mezcla una unidad
// especial con otras no se modela.
func (x *javaContext) operandCanonical(v, u string) (*big.Rat, float64, bool) {
	r, ok := ratOf(v)
	if !ok {
		return nil, 0, false
	}
	t, err := x.parse(u)
	if err != nil {
		return nil, 0, false
	}
	if code, alpha, ok := x.loneSpecial(u); ok {
		// §22■4: m = f⁻¹(α r) × u.
		if x.specialExceeds(v, u) {
			// Fuera de float64: lo explica ours-out-of-range, no otra clase.
			return nil, 0, false
		}
		fv, _ := new(big.Rat).Mul(r, alpha).Float64()
		w, ok := x.specialCanonical(code, fv)
		return nil, w, ok
	}
	if x.anyAtom(u, func(a string) bool { return x.special[a] }) {
		return nil, 0, false
	}
	f, ok := x.factorWith(t, x.xmlAtomValue)
	if !ok {
		return nil, 0, false
	}
	return f.Mul(f, r), 0, true
}

// loneSpecial dice si u es una unidad especial sola, con su prefijo como el
// factor de escala α de §22■4; una unidad especial no admite exponente (§22■1).
func (x *javaContext) loneSpecial(u string) (code string, alpha *big.Rat, ok bool) {
	t, err := x.parse(u)
	if err != nil {
		return "", nil, false
	}
	sym := expr.LoneSymbol(t)
	if sym == nil || !x.special[sym.Unit.Code] || sym.Exponent != 1 {
		return "", nil, false
	}
	pv, ok := x.prefixOf(sym.Prefix)
	return sym.Unit.Code, pv, ok
}

// toSpecial lleva m, en las unidades base, a la unidad especial u:
// r = f(m / u_ref) / α, según §22■4.
func (x *javaContext) toSpecial(m float64, u string) (float64, bool) {
	code, alpha, ok := x.loneSpecial(u)
	if !ok {
		return 0, false
	}
	ref, ok := x.specialRefCanonical(code)
	if !ok {
		return 0, false
	}
	fwd, ok := specForward[x.functionOf(code)]
	if !ok {
		return 0, false
	}
	a, _ := alpha.Float64()
	rf, _ := ref.Float64()
	r := fwd(m/rf) / a
	return r, !math.IsInf(r, 0) && !math.IsNaN(r)
}

// ourComposition: nuestro valor del caso es la composición de los valores de
// operandCanonical. Sin esto, una clase podría excusar un resultado nuestro
// equivocado con tal de que Java fuera coherente consigo mismo.
func (x *javaContext) ourComposition(d javaDiff) bool {
	if d.Ours.Status != "ok" || (d.Ours.Exact == nil && (math.IsNaN(d.Ours.Float) || math.IsInf(d.Ours.Float, 0))) {
		return false
	}
	if d.Case.Op == "canonical" && !x.canonicalScaleOne(d.Ours.Code) {
		return false
	}
	a := d.Case.Args
	var exact *big.Rat
	var float float64
	var ok bool
	switch d.Case.Op {
	case "canonical":
		exact, float, ok = x.operandCanonical(a[0], a[1])
	case "convert":
		var dst *big.Rat
		exact, float, ok = x.operandCanonical(a[0], a[1])
		if !ok {
			return false
		}
		if _, _, special := x.loneSpecial(a[2]); special {
			if exact != nil {
				float, _ = exact.Float64()
				exact = nil
			}
			float, ok = x.toSpecial(float, a[2])
			break
		}
		dst, _, ok = x.operandCanonical("1", a[2])
		if !ok || dst == nil || dst.Sign() == 0 {
			return false
		}
		if exact != nil {
			exact.Quo(exact, dst)
		} else {
			f, _ := dst.Float64()
			float /= f
		}
	case "multiply", "divide":
		l, lf, ok1 := x.operandCanonical(a[0], a[1])
		r, rf, ok2 := x.operandCanonical(a[2], a[3])
		if !ok1 || !ok2 {
			return false
		}
		if l != nil && r != nil {
			if d.Case.Op == "divide" {
				if r.Sign() == 0 {
					return false
				}
				exact = l.Quo(l, r)
			} else {
				exact = l.Mul(l, r)
			}
			// Nuestro Multiply y Divide son de float64.
			float, _ = exact.Float64()
			exact = nil
		} else {
			if l != nil {
				lf, _ = l.Float64()
			}
			if r != nil {
				rf, _ = r.Float64()
			}
			float = lf * rf
			if d.Case.Op == "divide" {
				float = lf / rf
			}
		}
		ok = true
	}
	if !ok {
		return false
	}
	if exact != nil && d.Ours.Exact != nil {
		return exact.Cmp(d.Ours.Exact) == 0
	}
	if exact != nil {
		float, _ = exact.Float64()
	}
	got := d.Ours.Float
	if d.Ours.Exact != nil {
		got, _ = d.Ours.Exact.Float64()
	}
	if math.IsInf(float, 0) {
		return false
	}
	return floatClose(got, float)
}

// javaArithmetic junta los dos lados de una diferencia numérica.
func (x *javaContext) javaArithmetic(d javaDiff) bool {
	return x.javaSelfConsistent(d) && x.ourAtomsAgree(d) && x.ourComposition(d)
}

// atomUse es un átomo de una expresión con su exponente.
type atomUse struct {
	code string
	exp  int
}

// atomsOf devuelve los átomos de una expresión; el parser solo sirve aquí
// para encontrarlos, no es lo que se compara.
func (x *javaContext) atomsOf(code string) []atomUse {
	t, err := x.parse(code)
	if err != nil {
		return nil
	}
	return collectAtoms(t)
}

func collectAtoms(t *expr.Term) []atomUse {
	var out []atomUse
	var walk func(t *expr.Term)
	walk = func(t *expr.Term) {
		for ; t != nil; t = t.Term {
			switch c := t.Comp.(type) {
			case *expr.Symbol:
				out = append(out, atomUse{c.Unit.Code, c.Exponent})
			case *expr.Term:
				walk(c)
			}
		}
	}
	walk(t)
	return out
}

// unitArgs devuelve las unidades de un caso.
func unitArgs(c javaCase) []string {
	switch c.Op {
	case "validate":
		return c.Args[:1]
	case "canonical", "inproperty":
		return c.Args[1:2]
	case "convert":
		return c.Args[1:3]
	case "multiply", "divide":
		return []string{c.Args[1], c.Args[3]}
	case "comparable":
		return c.Args[:2]
	}
	return nil
}

func (x *javaContext) involves(c javaCase, pred func(code string) bool) bool {
	if c.Op == "inproperty" {
		return pred(c.Args[0]) || x.anyAtom(c.Args[0], pred)
	}
	for _, u := range unitArgs(c) {
		if x.anyAtom(u, pred) {
			return true
		}
	}
	return false
}

func (x *javaContext) anyAtom(code string, pred func(string) bool) bool {
	for _, a := range x.atomsOf(code) {
		if pred(a.code) {
			return true
		}
	}
	return false
}

// javaMessage es el mensaje de error de Java, sea de una excepción o de
// validate/validateInProperty.
func javaMessage(j javaOut) string {
	if j.Status == "exc" {
		return j.B
	}
	return j.A
}

// pow10 es 10^e.
func pow10(e int) *big.Rat {
	r := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(abs(e))), nil))
	if e < 0 {
		r.Inv(r)
	}
	return r
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// leadingExp es el exponente de la primera cifra significativa de un decimal
// sin exponente: 0.000946 → -4, 249089 → 5.
func leadingExp(s string) int {
	s = strings.TrimPrefix(s, "-")
	intPart, frac, _ := strings.Cut(s, ".")
	if t := strings.TrimLeft(intPart, "0"); t != "" {
		return len(t) - 1
	}
	return -(len(frac) - len(strings.TrimLeft(frac, "0")) + 1)
}

// withoutArbitrary quita del código canónico las unidades arbitrarias.
func (x *javaContext) withoutArbitrary(code string) (map[string]int, bool) {
	u, ok := canonicalUnits(code)
	if !ok {
		return nil, false
	}
	found := false
	for k := range u {
		if x.arbitrary[k] {
			delete(u, k)
			found = true
		}
	}
	return u, found
}

func sameUnits(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// onlyArbitraryApart dice si dos códigos (los nuestros) solo difieren en
// unidades arbitrarias, que Java descarta.
func (x *javaContext) onlyArbitraryApart(a, b string) bool {
	ua, fa := x.withoutArbitrary(a)
	ub, fb := x.withoutArbitrary(b)
	return (fa || fb) && ua != nil && ub != nil && sameUnits(ua, ub)
}

func (x *javaContext) ourCode(code string) (string, bool) {
	o := ourCanonical(x.s, "1", code)
	return o.Code, o.Status == "ok"
}

var javaClasses = []javaClass{
	{
		Name: "java-decimal-precision",
		Kind: "deliberate",
		Reason: "Java's Decimal rounds each result to the precision it attributes to the literals of the definitions " +
			"and of the input, so it prints fewer digits than the value has (1 [qt_us] is 0.000946 m3, exactly " +
			"0.000946352946). The definitions are exact; this package computes them exactly.",
		JavaRef: "definitions/XmlDefinitionsParser.java:125-130 (a value with a decimal point gets 24 digits, an " +
			"integer such as the 231 of [gal_us] only the digits it has), Decimal.java:601-620 (multiply keeps the " +
			"lower precision) and :722-760 (divide rounds to it); the precision Java declares for each value is " +
			"in the result file",
		match: func(x *javaContext, d javaDiff) bool {
			return d.What == "value" && !x.involves(d.Case, func(c string) bool { return x.special[c] }) &&
				x.javaArithmetic(d)
		},
	},
	{
		Name: "java-special-as-linear",
		Kind: "bug-java",
		Reason: "Java replaces the function of these special units with a fixed linear factor, so 1 [pH] becomes " +
			"1 mol/l (0.1 mol/l by §45■3) and 1 Np becomes 1, in every operation. This package applies the function the specification " +
			"defines; Java's value is checked against that linear model, within the precision Java declares.",
		Spec:    "§21-§23; functions in §44■3-4, §45■3, §46■3, §48■3",
		JavaRef: "special/Registry.java:57-70 (HoldingHandler), Converter.java:158-175 (expandDefinedUnit)",
		match: func(x *javaContext, d javaDiff) bool {
			if d.What != "value" || !x.involves(d.Case, func(c string) bool { return javaHolding[c] }) {
				return false
			}
			return x.javaArithmetic(d)
		},
	},
	{
		Name: "java-decimal-shift",
		Kind: "bug-java",
		Reason: "Java's result is off by a power of ten from its own arithmetic: with its own values for the atoms " +
			"(1 [bu_us] = 0.03523907016688 m3, 1 [pk_us] = 0.00881 m3), 2.5 [bu_us] is 9.9997 [pk_us], and " +
			"convert returns 1.00.",
		JavaRef: "Decimal.java:627-767 (divide)",
		match: func(x *javaContext, d javaDiff) bool {
			// Solo si su aritmética sin correr la coma no lo explica.
			if d.What != "value" || x.involves(d.Case, func(c string) bool { return x.special[c] }) ||
				!x.ourAtomsAgree(d) || !x.ourComposition(d) || x.javaSelfConsistent(d) {
				return false
			}
			j, ok1 := ratOf(d.Java.A)
			w, ok2 := x.javaModel(d.Case)
			if !ok1 || !ok2 || d.Java.P <= 0 {
				return false
			}
			for k := -3; k <= 3; k++ {
				if k == 0 {
					continue
				}
				scale := pow10(k)
				tol := claimTol(d.Java)
				if within(new(big.Rat).Mul(j, scale), w, tol.Mul(tol, scale)) {
					return true
				}
			}
			return false
		},
	},
	{
		Name:    "java-offset-unsupported",
		Kind:    "bug-java",
		Reason:  "Java refuses Cel and [degF] in every operation, including canonicalization and conversion.",
		Spec:    "§30■4, §43■4",
		JavaRef: "Converter.java:168-170",
		match: func(x *javaContext, d javaDiff) bool {
			return d.What == "status" && d.Ours.Status == "ok" && javaMessage(d.Java) != "" &&
				strings.HasSuffix(javaMessage(d.Java), javaMsgOffset) &&
				x.involves(d.Case, func(c string) bool { return javaOffset[c] })
		},
	},
	{
		Name: "ours-out-of-range",
		Kind: "deliberate",
		Reason: "The value in the canonical unit is outside the range of float64 (10^1000 for 1000 B, about " +
			"6×10^-974 m-3 for 1000 [pH]), and this package's float64 API reports ErrOutOfRange; Java, which " +
			"treats the unit as linear, gives a finite and wrong value.",
		Spec: "§22■4, §46■3; #59",
		match: func(x *javaContext, d javaDiff) bool {
			if d.What != "status" || d.Case.Op != "canonical" || !errors.Is(d.Ours.Err, ucumerr.ErrOutOfRange) {
				return false
			}
			return x.specialExceeds(d.Case.Args[0], d.Case.Args[1])
		},
	},
	{
		Name: "java-special-unregistered",
		Kind: "bug-java",
		Reason: "Java has no handler for these special units ([degRe], [hp'_X] and the rest of the Hahnemann series, " +
			"B[10.nV]) and refuses them. [m/s2/Hz^(1/2)] has none either, but Java's lexer fails on it first " +
			"(java-lexer-bracket).",
		Spec:    "§21-§23, §43■5, §44■3",
		JavaRef: "Converter.java:162-164, special/Registry.java:55-70",
		match: func(x *javaContext, d javaDiff) bool {
			return d.What == "status" && d.Ours.Status == "ok" &&
				strings.HasSuffix(javaMessage(d.Java), javaMsgUnregistered) &&
				x.involves(d.Case, func(c string) bool {
					return x.special[c] && !javaHolding[c] && !javaOffset[c]
				})
		},
	},
	{
		Name: "java-lexer-bracket",
		Kind: "bug-java",
		Reason: "Java's lexer ends a square-bracket atom at the first solidus, so [m/s2/Hz^(1/2)], an atom of " +
			"ucum-essence.xml, is read as the unknown unit '[m'.",
		Spec:    "§5, §14",
		JavaRef: "Lexer.java:136-140 (isValidSymbolChar admits no solidus, even inside brackets)",
		match: func(x *javaContext, d javaDiff) bool {
			return d.What == "status" && d.Ours.Status == "ok" &&
				strings.Contains(javaMessage(d.Java), "The unit '[m' is unknown") &&
				x.involves(d.Case, func(c string) bool { return c == "[m/s2/Hz^(1/2)]" })
		},
	},
	{
		Name: "java-no-case-insensitive",
		Kind: "deliberate",
		Reason: "Java resolves only case-sensitive codes; the case-insensitive spelling of every atom, which " +
			"ucum-essence.xml declares (CODE), is unknown to it. This package offers both vocabularies.",
		Spec: "§3, §4",
		match: func(_ *javaContext, d javaDiff) bool {
			return d.Case.Source == "xml-ci" && d.What == "status" && d.Java.Status == "invalid" &&
				d.Ours.Status == "ok" && strings.Contains(d.Java.A, "is unknown")
		},
	},
	{
		Name: "java-arbitrary-dimensionless",
		Kind: "bug-java",
		Reason: "Java drops arbitrary units from canonical forms, which makes them dimensionless and commensurable " +
			"with each other: [hp_X] converts to [hp_C], and /[IU] canonicalizes to the empty unit. UCUM has not " +
			"allowed that since version 1.7.",
		Spec: "§24■3, §25■1",
		JavaRef: "definitions/XmlDefinitionsParser.java:110-113 (isArbitrary is not read; the definition, 1 of unit 1, " +
			"makes the unit dimensionless), special/Registry.java:59-60",
		match: func(x *javaContext, d javaDiff) bool {
			switch {
			case d.What == "code":
				ju, _ := canonicalUnits(d.Java.B)
				ou, found := x.withoutArbitrary(d.Ours.Code)
				if !found || ju == nil || !sameUnits(ju, ou) {
					return false
				}
				// Y la unidad arbitraria es la de la entrada, no otra.
				od, ok1 := x.codeDims(d.Ours.Code)
				wd, ok2 := x.caseDims(d.Case)
				if !ok1 || !ok2 || !sameUnits(od, wd) || !x.canonicalScaleOne(d.Ours.Code) {
					return false
				}
				if numbersAgree(d.Java.A, d.Ours) {
					return true
				}
				return x.javaArithmetic(d)
			case d.Case.Op == "comparable" && d.What == "bool" && d.Java.A == "true",
				d.Case.Op == "convert" && d.What == "status" && d.Java.Status == "ok":
				u := unitArgs(d.Case)
				a, ok1 := x.ourCode(u[0])
				b, ok2 := x.ourCode(u[1])
				// Si nuestras formas canónicas son iguales, el fallo es nuestro.
				return ok1 && ok2 && !sameCanonicalUnits(a, b) && x.onlyArbitraryApart(a, b)
			}
			return false
		},
	},
	{
		Name: "java-inproperty-base-only",
		Kind: "bug-java",
		Reason: "Java's validateInProperty accepts a unit only if its canonical form is a single base unit of " +
			"that property, so it refuses J as energy and l as volume. ucum-essence.xml declares the property " +
			"of each atom, and this package reads it.",
		JavaRef: "UcumEssenceService.java:169-189 (validateInProperty)",
		match: func(x *javaContext, d javaDiff) bool {
			return d.Case.Op == "inproperty" && d.What == "status" && d.Java.Status == "invalid" &&
				d.Ours.Status == "ok" && strings.HasPrefix(d.Java.A, "unit ") &&
				strings.EqualFold(x.property[d.Case.Args[0]], d.Case.Args[1])
		},
	},
	{
		Name:   "java-accepts-invalid-syntax",
		Kind:   "bug-java",
		Reason: "Java accepts expressions that the grammar of UCUM rejects (each one with its rule in javaSyntaxBugs).",
		Spec:   "§7, §10 Exhibit 1",
		match: func(_ *javaContext, d javaDiff) bool {
			_, listed := javaSyntaxBugs[d.Case.Args[0]]
			return d.Case.Op == "validate" && d.What == "status" && d.Java.Status == "ok" &&
				d.Ours.Status == "invalid" && listed
		},
	},
	{
		Name: "java-accepts-space-in-annotation",
		Kind: "bug-java",
		Reason: "Java accepts a space inside curly braces, as in {delivered dose}. UCUM allows only characters " +
			"33-126 there, and reads the range as exhaustive: case 1-115a of UcumFunctionalTests.xml rejects " +
			"Unicode, and Java itself stops at '~' (126). Java checks for printable ASCII, which starts at the space (32).",
		Spec:    "§6■1",
		JavaRef: "Lexer.java:142-148 (checkAnnotation) calls Utilities.isAsciiChar, Utilities.java:87-88: ch >= ' ' && ch <= '~'",
		match:   func(_ *javaContext, d javaDiff) bool { return javaAcceptsAnnotation(d, ' ') },
	},
	{
		Name: "java-accepts-nested-annotation",
		Kind: "bug-java",
		Reason: "Java accepts a curly brace inside an annotation, as in m{a{b}. The first } closes an annotation, " +
			"so a { before it opens a second brace inside the first, which UCUM forbids. Java reads up to the first } " +
			"and checks only for ASCII; it rejects m{a{b}} for the } left over, not for the nesting.",
		Spec:    "§6■5",
		JavaRef: "Lexer.java:142-148 (checkAnnotation reads to the first } and asks only Utilities.isAsciiChar)",
		match:   func(_ *javaContext, d javaDiff) bool { return javaAcceptsAnnotation(d, '{') },
	},
}

// javaAcceptsAnnotation informa si d es Java aceptando un código que
// rechazamos por la regla de anotación rule: ' ' es el espacio (§6■1) y '{'
// una llave dentro de otra (§6■5). Mira cada unidad del caso por separado:
// toma el caso si en alguna la primera regla rota, la que informa nuestro
// error, es rule. Un caso con una regla en cada unidad caería en las dos
// clases, y el arnés lo informa.
func javaAcceptsAnnotation(d javaDiff, rule byte) bool {
	if d.What != "status" || d.Java.Status != "ok" {
		return false
	}
	// Nuestro rechazo tiene que ser el de validación: otro error del mismo
	// caso no lo explica esta clase.
	var ve *ucumerr.ValidationError
	if d.Ours.Status != "invalid" && (d.Ours.Status != "err" || !errors.As(d.Ours.Err, &ve)) {
		return false
	}
	for _, u := range unitArgs(d.Case) {
		if annotationViolation(u) == rule {
			return true
		}
	}
	return false
}

// annotationViolation devuelve, leyendo de izquierda a derecha, el primer
// byte con que una anotación de code rompe una regla que Java no ve: ' ' para
// el espacio (§6■1) y '{' para una llave dentro de otra (§6■5). Devuelve 0 si
// no hay ninguno. Como el lexer, lee un átomo entre corchetes sin buscar
// llaves en él (§5).
func annotationViolation(code string) byte {
	inside, bracket := false, false
	for i := 0; i < len(code); i++ {
		switch code[i] {
		case '[':
			bracket = bracket || !inside
		case ']':
			bracket = false
		}
		if bracket {
			continue
		}
		switch code[i] {
		case '{':
			if inside {
				return '{'
			}
			inside = true
		case '}':
			inside = false
		case ' ':
			if inside {
				return ' '
			}
		}
	}
	return 0
}

// javaOracle tiene los esperados independientes de los dos motores que no
// cubre otro test: el canónico que dan los autores de Java en
// UcumCommonUnitsTest.java y las funciones de las unidades especiales según
// la especificación. La suite oficial ya la exige TestFunctional*.
type javaOracle struct {
	x           *javaContext
	commonCanon map[string]string
	// commonAgainstSpec cuenta las expectativas de UcumCommonUnitsTest que
	// contradicen UCUM §24 (descartan unidades arbitrarias).
	commonAgainstSpec int
	specialChecked    int
}

func loadJavaOracle(t *testing.T, x *javaContext) *javaOracle {
	t.Helper()
	or := &javaOracle{x: x, commonCanon: map[string]string{}}
	for _, row := range readRows(t, javaCommonFile) {
		if len(row) == 2 && row[1] != "" {
			or.commonCanon[row[0]] = row[1]
		}
	}
	return or
}

// specInverse es f⁻¹ de cada función, de la especificación: lleva el valor
// en la unidad especial a la unidad de la función. Cel, degF y degRe dan
// kelvin directamente.
var specInverse = map[string]func(x float64) float64{
	"Cel":         func(x float64) float64 { return x + 273.15 },               // §30■4
	"degF":        func(x float64) float64 { return 5.0 / 9.0 * (x + 459.67) }, // §43■4
	"degRe":       func(x float64) float64 { return 5.0 / 4.0 * (x + 218.52) }, // §43■5
	"pH":          func(x float64) float64 { return math.Pow(10, -x) },         // §45■3
	"ln":          math.Exp,                                                    // §46■3
	"lg":          func(x float64) float64 { return math.Pow(10, x) },          // §46■3
	"lgTimes2":    func(x float64) float64 { return math.Pow(10, x/2) },        // §46■3
	"ld":          math.Exp2,                                                   // §48■3
	"sqrt":        func(x float64) float64 { return x * x },                    // §47■4
	"hpX":         func(x float64) float64 { return math.Pow(10, -x) },         // §44■3
	"hpC":         func(x float64) float64 { return math.Pow(100, -x) },        // §44■3
	"hpM":         func(x float64) float64 { return math.Pow(1000, -x) },       // §44■3
	"hpQ":         func(x float64) float64 { return math.Pow(50000, -x) },      // §44■3
	"100tan":      func(x float64) float64 { return math.Atan(x / 100) },       // §44■4, in rad
	"tanTimes100": func(x float64) float64 { return math.Atan(x / 100) },       // §44■4, in rad
}

// specForward es f de cada función, de la especificación: lleva el valor en
// la unidad de referencia a la unidad especial.
var specForward = map[string]func(x float64) float64{
	"Cel":         func(x float64) float64 { return x - 273.15 },
	"degF":        func(x float64) float64 { return x*9/5 - 459.67 },
	"degRe":       func(x float64) float64 { return x*4/5 - 218.52 },
	"pH":          func(x float64) float64 { return -math.Log10(x) },
	"ln":          math.Log,
	"lg":          math.Log10,
	"lgTimes2":    func(x float64) float64 { return 2 * math.Log10(x) },
	"ld":          math.Log2,
	"sqrt":        math.Sqrt,
	"hpX":         func(x float64) float64 { return -math.Log10(x) },
	"hpC":         func(x float64) float64 { return -math.Log(x) / math.Log(100) },
	"hpM":         func(x float64) float64 { return -math.Log(x) / math.Log(1000) },
	"hpQ":         func(x float64) float64 { return -math.Log(x) / math.Log(50000) },
	"100tan":      func(x float64) float64 { return 100 * math.Tan(x) },
	"tanTimes100": func(x float64) float64 { return 100 * math.Tan(x) },
}

// specInverseLog10 es log10 de f⁻¹ para las funciones exponenciales, para
// saber si f⁻¹(x) × referencia desborda float64 de verdad cuando f⁻¹(x) solo
// desborda en el paso intermedio.
var specInverseLog10 = map[string]func(x float64) float64{
	"pH":       func(x float64) float64 { return -x },
	"ln":       func(x float64) float64 { return x / math.Ln10 },
	"lg":       func(x float64) float64 { return x },
	"lgTimes2": func(x float64) float64 { return x / 2 },
	"ld":       func(x float64) float64 { return x * math.Log10(2) },
	"hpX":      func(x float64) float64 { return -x },
	"hpC":      func(x float64) float64 { return -2 * x },
	"hpM":      func(x float64) float64 { return -3 * x },
	"hpQ":      func(x float64) float64 { return -x * math.Log10(50000) },
}

var specKelvin = map[string]bool{"Cel": true, "degF": true, "degRe": true}

// specialReference es la unidad y el valor en que la especificación da f⁻¹
// de una unidad especial: la del XML, salvo Cel, degF y degRe, que dan kelvin,
// y la dioptría prismática y el porcentaje de pendiente, que dan radianes como
// dice §44■4, aunque el XML escriba deg para %[slope] (ucum-org/ucum#261).
func (x *javaContext) specialReference(code string) (inv func(float64) float64, unit string, val *big.Rat, ok bool) {
	fn, found := x.function[code]
	if !found {
		return nil, "", nil, false
	}
	inv, ok = specInverse[fn.Name]
	if !ok {
		return nil, "", nil, false
	}
	switch {
	case specKelvin[fn.Name]:
		return inv, "K", big.NewRat(1, 1), true
	case fn.Name == "100tan" || fn.Name == "tanTimes100":
		return inv, "rad", big.NewRat(1, 1), true
	}
	val, ok = new(big.Rat).SetString(fn.Value)
	return inv, fn.Unit, val, ok
}

// functionOf es el nombre de la función de una unidad especial en el XML.
func (x *javaContext) functionOf(code string) string {
	return x.function[code].Name
}

// specialRefCanonical es el valor, en las unidades base, de la unidad de
// referencia de una unidad especial (valor × unidad de la función, o K, o
// rad), con los valores de xmlAtomValue.
func (x *javaContext) specialRefCanonical(code string) (*big.Rat, bool) {
	_, unit, val, ok := x.specialReference(code)
	if !ok {
		return nil, false
	}
	f, ok := x.xmlFactor(unit, x.defParse)
	if !ok {
		return nil, false
	}
	return f.Mul(f, val), true
}

// specialExpected es el valor de v (ya multiplicado por α) en una unidad
// especial, en la unidad lineal to, según la especificación: f⁻¹(v) × valor ×
// unidad de referencia. Si to no mide lo mismo según xmlDims, commensurable
// es false; ok es false si algo no se puede evaluar, y entonces no hay
// esperado (el oráculo lo reporta, no lo acepta).
func (x *javaContext) specialExpected(code string, v float64, to string) (want float64, commensurable, ok bool) {
	rd, ok1 := x.xmlDims(code)
	td, ok2 := x.codeDims(to)
	if !ok1 || !ok2 {
		return 0, false, false
	}
	if !sameUnits(rd, td) {
		return 0, false, true
	}
	m, ok := x.specialCanonical(code, v)
	f, ok2 := x.xmlFactor(to, x.parse)
	if !ok || !ok2 || f.Sign() == 0 {
		return 0, false, false
	}
	ff, _ := f.Float64()
	return m / ff, true, true
}

// specialCanonical es f⁻¹(v) × la unidad de referencia, en las unidades base.
// Es +Inf o -Inf solo si el valor verdadero no cabe en float64; si float64 no
// lo puede decir (f⁻¹(v) desborda y la referencia es menor que 1, o el
// resultado cae por debajo del menor normal), ok es false.
func (x *javaContext) specialCanonical(code string, v float64) (float64, bool) {
	inv, _, _, ok := x.specialReference(code)
	ref, ok2 := x.specialRefCanonical(code)
	if !ok || !ok2 {
		return 0, false
	}
	rf, _ := ref.Float64()
	r := inv(v)
	if math.IsNaN(r) {
		return 0, false
	}
	if math.IsInf(r, 0) || math.Abs(r) < 0x1p-1022 {
		// f⁻¹(v) desborda o se pierde; f⁻¹(v) × referencia, en log10 (error
		// relativo del orden de 1e-13 cerca de los extremos).
		e, ok := x.specialLog10(code, v)
		if !ok {
			if r == 0 {
				return 0, true // f⁻¹ que da 0 exacto sin ser potencia: arctan(0), 0²
			}
			return 0, false
		}
		sign := math.Copysign(1, rf)
		switch {
		case e > log10Max:
			return math.Copysign(math.Inf(1), sign), true
		case e <= log10Min:
			return math.Copysign(0, sign), true
		case e < math.Log10(0x1p-1022):
			return 0, false // subnormal: float64 no lo dice con precisión
		}
		return math.Copysign(math.Pow(10, e), sign), true
	}
	w := r * rf
	if w != 0 && math.Abs(w) < 0x1p-1022 || w == 0 && r != 0 {
		return 0, false
	}
	return w, true
}

var (
	log10Max = math.Log10(math.MaxFloat64)
	// Log10 de 2^-1075, la mitad del menor subnormal: un valor de magnitud
	// menor o igual redondea a cero.
	log10Min = -1075 * math.Log10(2)
)

// specialLog10 es log10 |f⁻¹(v) × referencia| para las funciones
// exponenciales.
func (x *javaContext) specialLog10(code string, v float64) (float64, bool) {
	lg, ok := specInverseLog10[x.functionOf(code)]
	ref, ok2 := x.specialRefCanonical(code)
	if !ok || !ok2 || ref.Sign() == 0 {
		return 0, false
	}
	rf, _ := ref.Float64()
	return lg(v) + math.Log10(math.Abs(rf)), true
}

// specialExceeds: v en la unidad especial u vale, en las unidades base, más
// que el mayor float64 o tan poco que redondea a cero (#59).
func (x *javaContext) specialExceeds(v, u string) bool {
	code, alpha, ok := x.loneSpecial(u)
	r, ok2 := ratOf(v)
	if !ok || !ok2 {
		return false
	}
	fv, _ := new(big.Rat).Mul(r, alpha).Float64()
	e, ok := x.specialLog10(code, fv)
	return ok && (e > log10Max || e <= log10Min)
}

// outOfRange dice si el resultado de un caso con una unidad especial de
// origen no cabe en float64 según specialLog10: canonical, o convert a una
// unidad lineal (dividido por su factor).
func (or *javaOracle) outOfRange(c javaCase, code string, fv float64) bool {
	e, ok := or.x.specialLog10(code, fv)
	if !ok {
		return false
	}
	if c.Op == "convert" {
		if _, _, toSpecial := or.x.loneSpecial(c.Args[2]); toSpecial {
			return false
		}
		f, ok := or.x.xmlFactor(c.Args[2], or.x.parse)
		if !ok || f.Sign() <= 0 {
			return false
		}
		e -= ratLog10(f)
	}
	return e > log10Max || e <= log10Min
}

// floatClose compara un resultado float64 con un esperado, en error
// relativo y sin piso absoluto: un esperado de 0 solo acepta 0.
func floatClose(got, want float64) bool {
	if math.IsNaN(got) || math.IsNaN(want) || math.IsInf(want, 0) {
		return false
	}
	if want == 0 {
		return got == 0
	}
	return math.Abs(got-want) <= 1e-13*math.Abs(want)
}

// checkCommensurable revisa todo comparable y convert, coincida o no con
// Java: se comparan o se convierten si y solo si tienen la misma dimensión
// según xmlDims (con cada unidad arbitraria la suya, §24■3). Mirar solo las
// diferencias dejaría pasar un defecto nuestro que copie uno de Java.
func (or *javaOracle) checkCommensurable(c javaCase, o ourOut) string {
	if c.Op != "comparable" && c.Op != "convert" {
		return ""
	}
	u := unitArgs(c)
	a, ok1 := or.x.codeDims(u[0])
	b, ok2 := or.x.codeDims(u[1])
	if !ok1 || !ok2 {
		return "" // código inválido: lo decide validate
	}
	want := sameUnits(a, b)
	if c.Op == "comparable" {
		if o.Status == "ok" && o.Bool != want {
			return fmt.Sprintf("%s and %s: same dimension by the XML is %v, ours says %v", u[0], u[1], want, o.Bool)
		}
		return ""
	}
	if o.Status == "ok" && !want {
		return fmt.Sprintf("%s and %s have different dimensions by the XML, ours converts", u[0], u[1])
	}
	return ""
}

// checkToSpecial revisa la conversión de una unidad lineal a una especial:
// m = v × la unidad de origen según el XML, y r = f(m / u_ref) / α (§22■4).
func (or *javaOracle) checkToSpecial(c javaCase, o ourOut, toCode string) string {
	if or.x.anyAtom(c.Args[1], func(a string) bool { return or.x.special[a] }) {
		return "" // compuesta con otra unidad especial: no se modela (F4)
	}
	sd, ok1 := or.x.codeDims(c.Args[1])
	td, ok2 := or.x.xmlDims(toCode)
	if !ok1 || !ok2 {
		return "no specification value for " + c.Args[1] + " in " + c.Args[2]
	}
	or.specialChecked++
	if !sameUnits(sd, td) {
		if o.Status == "ok" {
			return fmt.Sprintf("%s and %s are not commensurable, ours converts", c.Args[1], c.Args[2])
		}
		return ""
	}
	v, _ := ratOf(c.Args[0])
	f, ok := or.x.xmlFactor(c.Args[1], or.x.parse)
	if !ok {
		return "no specification value for " + c.Args[1]
	}
	m, _ := f.Mul(f, v).Float64()
	want, ok := or.x.toSpecial(m, c.Args[2])
	if !ok {
		return "no specification value for " + c.Args[2]
	}
	if o.Status != "ok" {
		return fmt.Sprintf("specification gives %v %s, ours fails: %s", want, c.Args[2], o.Msg)
	}
	got := o.Float
	if o.Exact != nil {
		got, _ = o.Exact.Float64()
	}
	if !floatClose(got, want) {
		return fmt.Sprintf("specification gives %v %s, ours is %v", want, c.Args[2], got)
	}
	return ""
}

// check devuelve en qué se aparta nuestro resultado de un esperado
// independiente, o "" si coincide.
func (or *javaOracle) check(c javaCase, o ourOut) string {
	if msg := or.checkCommensurable(c, o); msg != "" {
		return msg
	}
	if c.Source == "java-common" && c.Op == "canonical" {
		if want, ok := or.commonCanon[c.Args[1]]; ok {
			if o.Status != "ok" {
				return "UcumCommonUnitsTest expects " + want + ", ours fails: " + o.Msg
			}
			if !sameCanonicalUnits(want, o.Code) {
				wu, _ := canonicalUnits(want)
				ou, found := or.x.withoutArbitrary(o.Code)
				if found && sameUnits(wu, ou) {
					or.commonAgainstSpec++
					return ""
				}
				return "UcumCommonUnitsTest expects " + want + ", ours is " + o.Code
			}
		}
	}
	nonFinite := o.Status == "ok" && o.Exact == nil && (math.IsNaN(o.Float) || math.IsInf(o.Float, 0))
	var code string
	var alpha *big.Rat
	special := false
	if c.Op == "canonical" || c.Op == "convert" {
		code, alpha, special = or.x.loneSpecial(c.Args[1])
	}
	if c.Op == "convert" && !special {
		if toCode, _, ok := or.x.loneSpecial(c.Args[2]); ok {
			return or.checkToSpecial(c, o, toCode)
		}
	}
	if !special {
		if nonFinite {
			return fmt.Sprintf("ours is %v", o.Float)
		}
		return ""
	}
	v, _ := ratOf(c.Args[0])
	fv, _ := new(big.Rat).Mul(v, alpha).Float64()
	var want float64
	to := ""
	switch c.Op {
	case "canonical":
		w, ok := or.x.specialCanonical(code, fv)
		rd, ok2 := or.x.xmlDims(code)
		if !ok || !ok2 {
			return "no specification value for " + c.Args[0] + " " + c.Args[1]
		}
		want = w
		if o.Status == "ok" {
			od, ok := or.x.codeDims(o.Code)
			if !ok || !sameUnits(od, rd) || !or.x.canonicalScaleOne(o.Code) {
				return fmt.Sprintf("specification gives the dimension %v in base units, ours is %q", rd, o.Code)
			}
		}
	default:
		to = c.Args[2]
		var comm, ok bool
		if _, _, toSpecial := or.x.loneSpecial(to); toSpecial {
			// De una escala especial a otra: m = f⁻¹ de la primera, y f de la
			// segunda (§22■4).
			rd, ok1 := or.x.xmlDims(code)
			td, ok2 := or.x.xmlDims(func() string { c, _, _ := or.x.loneSpecial(to); return c }())
			m, ok3 := or.x.specialCanonical(code, fv)
			ok, comm = ok1 && ok2 && ok3, ok1 && ok2 && sameUnits(rd, td)
			if ok && comm {
				want, ok = or.x.toSpecial(m, to)
			}
		} else {
			want, comm, ok = or.x.specialExpected(code, fv, to)
		}
		if !ok {
			return "no specification value for " + c.Args[0] + " " + code + " in " + to
		}
		if !comm {
			or.specialChecked++
			if o.Status == "ok" {
				return fmt.Sprintf("%s and %s are not commensurable, ours converts", code, to)
			}
			return ""
		}
	}
	or.specialChecked++
	if or.outOfRange(c, code, fv) {
		// El valor verdadero no cabe en float64: la respuesta es
		// ErrOutOfRange (#59).
		if errors.Is(o.Err, ucumerr.ErrOutOfRange) {
			return ""
		}
		return fmt.Sprintf("specification is outside float64, ours is %s %v %s", o.Status, o.Float, o.Msg)
	}
	if o.Status != "ok" {
		return fmt.Sprintf("specification gives %v, ours fails: %s", want, o.Msg)
	}
	got := o.Float
	if o.Exact != nil {
		got, _ = o.Exact.Float64()
	}
	if math.IsInf(want, 0) {
		return fmt.Sprintf("specification overflows float64 but outOfRange missed it, ours is %v", got)
	}
	if !floatClose(got, want) {
		return fmt.Sprintf("specification gives %v %s, ours is %v", want, to, got)
	}
	return ""
}
