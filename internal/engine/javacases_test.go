package engine

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gofhir/ucum/v4/internal/fhirexamples"
)

// Plan 002: los casos que se mandan a Ucum-java. El corpus sale de fuentes que
// no dependen de este motor (ucum-essence.xml, la suite oficial, ucum-common,
// UcumCommonUnitsTest.java y los ejemplos de FHIR), en un orden fijo, para que
// la elección de casos no pueda esconder un error nuestro.
//
// Lo regenera make java-dump (UCUM_JAVA_CASES=<ruta>), y TestJavaCasesFresh
// comprueba que el archivo versionado sigue siendo lo que el generador produce.

const (
	javaDir         = "testdata/java"
	javaCasesFile   = javaDir + "/cases.tsv"
	javaCommonFile  = javaDir + "/common-units.tsv"
	javaCaseBudget  = 25000
	javaPairsPerGrp = 6
	fhirSource      = "fhir"
)

// Los literales con que se canonicaliza cada código de ucum-common. El
// Decimal de Java toma su precisión del texto (Decimal.java:135-145), así que
// el literal es parte del caso y no se formatea desde un número.
var javaLiterals = []string{"1", "0.07", "2.5", "1000", "0.000123"}

// javaCase is one call to the reference. Args depend on Op:
//
//	validate    unit
//	canonical   value unit
//	convert     value from to
//	multiply    v1 u1 v2 u2
//	divide      v1 u1 v2 u2
//	comparable  u1 u2
//	inproperty  unit property
type javaCase struct {
	ID, Op, Source string
	Args           []string
}

var javaArity = map[string]int{
	"validate": 1, "canonical": 2, "convert": 3, "multiply": 4,
	"divide": 4, "comparable": 2, "inproperty": 2,
}

type caseBuilder struct {
	cases []javaCase
	next  map[string]int
}

func (b *caseBuilder) add(source, op string, args ...string) {
	if b.next == nil {
		b.next = map[string]int{}
	}
	b.cases = append(b.cases, javaCase{
		ID: fmt.Sprintf("%s:%05d", source, b.next[source]), Op: op, Source: source, Args: args,
	})
	b.next[source]++
}

// buildJavaCases arma el corpus. Sin packages (pkgDir vacío) omite la parte
// de FHIR, que va al final y con su propia numeración, de modo que el resto
// es idéntico con o sin ella.
func buildJavaCases(t *testing.T, s *Service, pkgDir string) []javaCase {
	t.Helper()
	var b caseBuilder
	addXMLCases(&b, s)
	addSpecCases(&b)
	addSuiteCases(t, &b)
	addCommonCases(t, &b)
	addJavaCommonCases(t, &b)
	if pkgDir != "" {
		addFHIRCases(t, &b, pkgDir)
	}
	return b.cases
}

type xmlAtom struct {
	code, codeCI, property  string
	metric, special, arbitr bool
	funcUnit                string
}

func xmlAtoms(s *Service) []xmlAtom {
	out := make([]xmlAtom, 0, len(s.model.BaseUnits)+len(s.model.DefinedUnits))
	for _, u := range s.model.BaseUnits {
		out = append(out, xmlAtom{code: u.Code, codeCI: u.CodeCI, property: u.Property, metric: true})
	}
	for _, u := range s.model.DefinedUnits {
		a := xmlAtom{code: u.Code, codeCI: u.CodeCI, property: u.Property,
			metric: u.IsMetric, special: u.IsSpecial, arbitr: u.IsArbitrary}
		if u.Value != nil && u.Value.Function != nil {
			a.funcUnit = u.Value.Function.Unit
		}
		out = append(out, a)
	}
	return out
}

// addXMLCases es la obligación de cobertura desde ucum-essence.xml: cada
// átomo, su forma CODE, cada prefijo, cada unidad especial y arbitraria, y
// pares elegidos por el atributo property, no por este motor.
func addXMLCases(b *caseBuilder, s *Service) {
	atoms := xmlAtoms(s)
	for _, a := range atoms {
		b.add("xml-atom", "validate", a.code)
		b.add("xml-atom", "canonical", "1", a.code)
	}
	for _, a := range atoms {
		if a.codeCI != "" && a.codeCI != a.code {
			b.add("xml-ci", "validate", a.codeCI)
		}
	}

	// Prefijos sobre la primera unidad base, sobre la primera unidad definida
	// métrica y sobre la primera no métrica (que UCUM §11 no deja prefijar).
	bases := make([]string, 0, 3)
	bases = append(bases, atoms[0].code)
	var metricDef, nonMetric string
	for _, a := range atoms[len(s.model.BaseUnits):] {
		if metricDef == "" && a.metric && !a.special && !a.arbitr {
			metricDef = a.code
		}
		if nonMetric == "" && !a.metric && !a.special && !a.arbitr {
			nonMetric = a.code
		}
	}
	bases = append(bases, metricDef, nonMetric)
	for _, p := range s.model.Prefixes {
		for _, u := range bases {
			b.add("xml-prefix", "validate", p.Code+u)
			b.add("xml-prefix", "canonical", "1", p.Code+u)
		}
		// El exponente se aplica también al prefijo (UCUM §14).
		b.add("xml-prefix", "canonical", "1", p.Code+atoms[0].code+"2")
		b.add("xml-prefix", "canonical", "1", p.Code+atoms[0].code+"-1")
	}

	for _, a := range atoms {
		if a.special {
			for _, v := range []string{"1", "2.5", "-1", "0"} {
				b.add("xml-special", "canonical", v, a.code)
			}
			if a.funcUnit != "" {
				b.add("xml-special", "convert", "1", a.code, a.funcUnit)
			}
		}
		if a.arbitr {
			b.add("xml-arbitrary", "canonical", "2.5", a.code+"/mL")
		}
	}

	// Grupos por property, en el orden del XML.
	var order []string
	groups := map[string][]string{}
	for _, a := range atoms {
		if a.property == "" {
			continue
		}
		if _, ok := groups[a.property]; !ok {
			order = append(order, a.property)
		}
		groups[a.property] = append(groups[a.property], a.code)
	}
	for i, prop := range order {
		g := groups[prop]
		if len(g) > javaPairsPerGrp {
			g = g[:javaPairsPerGrp]
		}
		for _, x := range g {
			for _, y := range g {
				if x != y {
					b.add("xml-pairs", "convert", "2.5", x, y)
					b.add("xml-pairs", "comparable", x, y)
				}
			}
		}
		h := groups[order[(i+1)%len(order)]][0]
		b.add("xml-pairs", "convert", "2.5", g[0], h)
		b.add("xml-pairs", "comparable", g[0], h)
		b.add("xml-pairs", "multiply", "2.5", g[0], "4", h)
		b.add("xml-pairs", "divide", "2.5", g[0], "4", h)
		b.add("xml-pairs", "multiply", "2.5", g[0], "4", g[len(g)-1])
	}
	for i, prop := range order {
		other := order[(i+1)%len(order)]
		for _, code := range groups[prop] {
			b.add("xml-inproperty", "inproperty", code, prop)
			b.add("xml-inproperty", "inproperty", code, other)
		}
	}
}

// addSpecCases son las formas sintácticas de UCUM §5-§12 que el XML no
// enumera: anotaciones, exponentes, números, paréntesis, división por cero.
func addSpecCases(b *caseBuilder) {
	for _, u := range []string{
		"", " ", "m{a}", "{a}", "{a}/m", "10*3{cells}/uL", "m{é}", "{a}{b}", "m{", "{",
		"m2", "m-2", "m+2", "(m/s)2", "/s", "1/s", "10*-3", "10^3", "10*3.m", "4.m",
		"m.s-1", "kg.m/s2", "m//s", "m..s", "m.", ".m", "(m", "m)", "()", "[", "m]",
		"10*3.(Cel)", "Cel/min", "Cel2", "[pH]2", "/[pH]", "2m", "m2.5", "mm{a}2", "M",
	} {
		b.add("spec", "validate", u)
	}
	for _, c := range [][]string{
		{"1", "m{a}"}, {"1", "{a}/m"}, {"1", "(m/s)2"}, {"1", "/s"}, {"1", "10*-3"},
		{"1", "10^3"}, {"1", "4.m"}, {"0", "m"}, {"-2.5", "km"}, {"1e30", "km"},
		{"1e-30", "mm"}, {"1.0", "m"}, {"1.00000", "m"}, {"0.1", "/min"}, {"3", "/h"},
		{"1", "[in_i]"}, {"7", "[in_i]/s"}, {"1", "Cel/min"}, {"1", "mm{a}2"},
	} {
		b.add("spec", "canonical", c...)
	}
	for _, c := range [][]string{
		{"1", "Cel/min", "K/min"}, {"1", "%", "1"}, {"2", "m", "s"}, {"1", "m{a}", "m"},
		{"100", "[degF]", "Cel"}, {"37", "Cel", "K"}, {"1", "mg/dL", "g/L"},
	} {
		b.add("spec", "convert", c...)
	}
	for _, c := range [][]string{
		{"1", "{a}", "2", "m"}, {"2", "m", "3", "m"}, {"2", "cm", "3", "mm"}, {"2", "Cel", "3", "m"},
	} {
		b.add("spec", "multiply", c...)
	}
	for _, c := range [][]string{
		{"1", "m", "0", "s"}, {"0", "m", "2", "s"}, {"6", "m2", "3", "m"}, {"1", "m", "1", "m"},
	} {
		b.add("spec", "divide", c...)
	}
}

// addSuiteCases toma UcumFunctionalTests.xml tal cual. Un <case> dentro de
// un comentario XML no cuenta: encoding/xml lo ignora, como el lector de Java.
func addSuiteCases(t *testing.T, b *caseBuilder) {
	t.Helper()
	data, err := os.ReadFile("testdata/UcumFunctionalTests.xml")
	if err != nil {
		t.Fatal(err)
	}
	var suite ucumTests
	if err := xml.Unmarshal(data, &suite); err != nil {
		t.Fatal(err)
	}
	for _, c := range suite.Validation.Cases {
		b.add("suite", "validate", c.Unit)
	}
	for _, c := range suite.Conversion.Cases {
		b.add("suite", "convert", c.Value, c.SrcUnit, c.DstUnit)
	}
	for _, c := range suite.Multiplication.Cases {
		b.add("suite", "multiply", c.V1, c.U1, c.V2, c.U2)
	}
	for _, c := range suite.Division.Cases {
		b.add("suite", "divide", c.V1, c.U1, c.V2, c.U2)
	}
}

// addCommonCases: los códigos distintos del ValueSet ucum-common, en el
// orden del archivo.
func addCommonCases(t *testing.T, b *caseBuilder) {
	t.Helper()
	for _, code := range readCodes(t, "../../fhir/ucum-common.tsv", 0) {
		b.add("ucum-common", "validate", code)
		for _, v := range javaLiterals {
			b.add("ucum-common", "canonical", v, code)
		}
	}
}

// addJavaCommonCases: las unidades de UcumCommonUnitsTest.java, desde la
// copia extraída en common-units.tsv.
func addJavaCommonCases(t *testing.T, b *caseBuilder) {
	t.Helper()
	for _, code := range readCodes(t, javaCommonFile, 0) {
		b.add("java-common", "validate", code)
		b.add("java-common", "canonical", "1", code)
	}
}

// readRows devuelve las filas de un TSV con comentarios '#'.
func readRows(t *testing.T, path string) [][]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out [][]string
	for line := range strings.SplitSeq(strings.TrimRight(string(data), "\n"), "\n") {
		if !strings.HasPrefix(line, "#") {
			out = append(out, strings.Split(line, "\t"))
		}
	}
	return out
}

// readCodes devuelve los valores distintos de la columna col de un TSV con
// comentarios '#', en orden de aparición.
func readCodes(t *testing.T, path string, col int) []string {
	t.Helper()
	seen := map[string]bool{}
	var out []string
	for _, f := range readRows(t, path) {
		if col >= len(f) {
			t.Fatalf("%s: row %q has no column %d", path, f, col)
		}
		if !seen[f[col]] {
			seen[f[col]] = true
			out = append(out, f[col])
		}
	}
	return out
}

// addFHIRCases: las cantidades UCUM distintas de los tres packages, con el
// valor tal como está en el JSON.
func addFHIRCases(t *testing.T, b *caseBuilder, dir string) {
	t.Helper()
	seen := map[fhirexamples.Quantity]bool{}
	var all []fhirexamples.Quantity
	codes := map[string]bool{}
	for _, pkg := range fhirexamples.Packages {
		qs, undecodable, err := fhirexamples.Read(filepath.Join(dir, pkg.File))
		if err != nil {
			t.Fatal(err)
		}
		if undecodable > 0 {
			t.Fatalf("%s: %d JSON files do not decode", pkg.File, undecodable)
		}
		for _, q := range qs {
			if !seen[q] {
				seen[q] = true
				all = append(all, q)
			}
		}
	}
	for _, q := range all {
		if !codes[q.Code] {
			codes[q.Code] = true
			b.add(fhirSource, "validate", q.Code)
		}
		b.add(fhirSource, "canonical", q.Value, q.Code)
	}
}

// commonUnitLine es una entrada activa de UcumCommonUnitsTest.java:
// new CommonUnit("unit", dim) o new CommonUnit("unit", dim, "canonical").
var commonUnitLine = regexp.MustCompile(`^\s*new CommonUnit\("([^"]*)",\s*(?:"[^"]*"|null)(?:,\s*"([^"]*)")?\),?\s*$`)

// extractJavaCommonUnits lee UcumCommonUnitsTest.java y devuelve, en orden,
// cada unidad con el canónico que esperan sus autores (vacío si no lo dan).
// Una línea "new CommonUnit" con otra forma es un error, para no perder
// unidades en silencio; las comentadas con // no cuentan.
func extractJavaCommonUnits(src []byte) ([][2]string, error) {
	var out [][2]string
	sc := bufio.NewScanner(bytes.NewReader(src))
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if !strings.Contains(line, "new CommonUnit(") || strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		m := commonUnitLine.FindStringSubmatch(line)
		if m == nil {
			return nil, fmt.Errorf("line %d: unexpected form %q", n, line)
		}
		out = append(out, [2]string{m[1], m[2]})
	}
	return out, sc.Err()
}

func encodeJavaCases(cases []javaCase) []byte {
	var buf bytes.Buffer
	for _, c := range cases {
		buf.WriteString(strings.Join(append([]string{c.ID, c.Op, c.Source}, c.Args...), "\t"))
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

func decodeJavaCases(data []byte) ([]javaCase, error) {
	var out []javaCase
	for i, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		f := strings.Split(line, "\t")
		if len(f) < 3 {
			return nil, fmt.Errorf("line %d: %d fields", i+1, len(f))
		}
		c := javaCase{ID: f[0], Op: f[1], Source: f[2], Args: f[3:]}
		if n, ok := javaArity[c.Op]; !ok || n != len(c.Args) {
			return nil, fmt.Errorf("line %d: op %q with %d args", i+1, c.Op, len(c.Args))
		}
		out = append(out, c)
	}
	return out, nil
}

// checkJavaCases rechaza lo que el formato no puede llevar y lo que pasa del
// presupuesto.
func checkJavaCases(cases []javaCase) error {
	if len(cases) > javaCaseBudget {
		return fmt.Errorf("%d cases, budget is %d", len(cases), javaCaseBudget)
	}
	ids := map[string]bool{}
	for _, c := range cases {
		if ids[c.ID] {
			return fmt.Errorf("duplicate id %s", c.ID)
		}
		ids[c.ID] = true
		for _, a := range c.Args {
			if strings.ContainsAny(a, "\t\n\r") {
				return fmt.Errorf("%s: argument %q has a tab or a line break", c.ID, a)
			}
		}
	}
	return nil
}

// TestWriteJavaCases regenera los casos (y common-units.tsv, si se le da el
// fuente de Java). Solo corre con UCUM_JAVA_CASES; lo llama make java-dump.
func TestWriteJavaCases(t *testing.T) {
	out := os.Getenv("UCUM_JAVA_CASES")
	if out == "" {
		t.Skip("UCUM_JAVA_CASES not set; run make java-dump")
	}
	pkgDir := os.Getenv("UCUM_PACKAGES")
	if pkgDir == "" {
		t.Fatal("UCUM_PACKAGES not set: the FHIR part of the corpus would be missing")
	}
	if src := os.Getenv("UCUM_JAVA_COMMON_SRC"); src != "" {
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		units, err := extractJavaCommonUnits(data)
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		buf.WriteString(javaCommonHeader)
		for _, u := range units {
			fmt.Fprintf(&buf, "%s\t%s\n", u[0], u[1])
		}
		if err := os.WriteFile(javaCommonFile, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("%d units from UcumCommonUnitsTest.java", len(units))
	}
	cases := buildJavaCases(t, newTestService(t), pkgDir)
	if err := checkJavaCases(cases); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, encodeJavaCases(cases), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d cases", len(cases))
}

const javaCommonHeader = `# Units and expected canonical units from UcumCommonUnitsTest.java of
# FHIR/Ucum-java at commit db9d22ade3f0f4a667b45817b85a9ac4534e4224 (tag 1.0.10),
# in file order; the second column is empty where the test gives none.
# Extracted by TestWriteJavaCases (make java-dump).
#
# Copyright (c) 2006+, Health Intersections Pty Ltd. All rights reserved.
# BSD 3-Clause License; see NOTICE.
`

// TestJavaCasesFresh comprueba que cases.tsv es lo que el generador produce
// hoy: sin packages, la parte que no depende de ellos; con UCUM_PACKAGES, el
// archivo entero.
func TestJavaCasesFresh(t *testing.T) {
	data, err := os.ReadFile(javaCasesFile)
	if err != nil {
		t.Fatal(err)
	}
	pkgDir := os.Getenv("UCUM_PACKAGES")
	want := buildJavaCases(t, newTestService(t), pkgDir)
	if err := checkJavaCases(want); err != nil {
		t.Fatal(err)
	}
	got, err := decodeJavaCases(data)
	if err != nil {
		t.Fatal(err)
	}
	if pkgDir == "" {
		kept := got[:0]
		for _, c := range got {
			if c.Source != fhirSource {
				kept = append(kept, c)
			}
		}
		got = kept
	}
	if g, w := encodeJavaCases(got), encodeJavaCases(want); !bytes.Equal(g, w) {
		t.Fatalf("%s is stale (%d cases, generator gives %d): run make java-dump",
			javaCasesFile, len(got), len(want))
	}
}
