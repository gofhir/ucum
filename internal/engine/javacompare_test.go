package engine

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/gofhir/ucum/v4/internal/decimal"
)

// Plan 002: comparación contra Ucum-java 1.0.10. La salida de Java está
// versionada (make java-dump la regenera), así que este test corre sin Java,
// en cada go test y en CI. Cada diferencia tiene que caer en exactamente una
// clase de javaclasses_test.go, que explica los dos lados con evidencia.

const (
	javaResultFile = javaDir + "/ucum-java-1.0.10.tsv"
	javaManifest   = javaDir + "/manifest.json"
	javaDumper     = "../../tools/javadump/JavaDump.java"

	// Lo que el Decimal de Java puede apartarse del valor exacto en una
	// unidad lineal. Las multiplicaciones de Decimal son exactas (solo
	// recortan ceros finales: Decimal, líneas 535-620); al dividir se redondea
	// a una precisión que parte de la semilla de 31 dígitos del Converter
	// (líneas 62 y 130; Decimal, 722-760), y una expresión encadena unas
	// pocas. El plan lo detalla en su §5.
	javaRelTol = 1e-22

	// La tolerancia cuando nuestro lado es float64, que redondea una vez, más
	// el error de Java.
	floatRelTol = 1e-14
)

type javaManifestFile struct {
	Reference      string `json:"reference"`
	Commit         string `json:"commit"`
	JarSHA         string `json:"jar_sha256"`
	CommonTestSHA  string `json:"common_units_test_sha256"`
	DumperSHA      string `json:"dumper_sha256"`
	CasesSHA       string `json:"cases_sha256"`
	CommonUnitsSHA string `json:"common_units_sha256"`
	ResultSHA      string `json:"result_sha256"`
}

// javaOut es una línea del resultado de Java: Status es ok, invalid o exc; A
// y B dependen de él (valor y código, mensaje, o clase y mensaje). P es la
// precisión que el Decimal de Java declara para el valor, o 0 si no hay valor.
type javaOut struct {
	Status, A, B string
	P            int
}

// ourOut es nuestra respuesta al mismo caso.
type ourOut struct {
	Status string // ok, invalid o err
	Exact  *big.Rat
	Float  float64
	Code   string
	Bool   bool
	Msg    string
	Err    error // la causa, si Status es err (para errors.Is)
}

// javaTally cuenta, por operación, los casos que coinciden (y cuántos de ellos
// solo dentro del redondeo de Java) y los que difieren.
type javaTally struct{ agree, rounded, diff int }

// javaDiff es un caso en que las dos respuestas difieren.
type javaDiff struct {
	Case javaCase
	Java javaOut
	Ours ourOut
	What string // status, code, value o bool
}

func sha256File(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func unescapeJava(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 't':
			b.WriteByte('\t')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// loadJavaRun lee casos y resultado, comprueba el manifiesto y que cada caso
// tenga exactamente un resultado, en el mismo orden.
func loadJavaRun(t *testing.T) ([]javaCase, []javaOut) {
	t.Helper()
	raw, err := os.ReadFile(javaManifest)
	if err != nil {
		t.Fatalf("%v: run make java-dump", err)
	}
	var m javaManifestFile
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, f := range []struct{ path, want string }{
		{javaCasesFile, m.CasesSHA},
		{javaCommonFile, m.CommonUnitsSHA},
		{javaResultFile, m.ResultSHA},
		{javaDumper, m.DumperSHA},
	} {
		if got := sha256File(t, f.path); got != f.want {
			t.Fatalf("%s: SHA-256 %s, manifest says %s: run make java-dump", f.path, got, f.want)
		}
	}

	data, err := os.ReadFile(javaCasesFile)
	if err != nil {
		t.Fatal(err)
	}
	cases, err := decodeJavaCases(data)
	if err != nil {
		t.Fatal(err)
	}
	// Ids únicos también en el archivo versionado, no solo en lo que el
	// generador produce: TestJavaCasesFresh no ve la parte de FHIR sin packages.
	if err := checkJavaCases(cases); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(javaResultFile)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) != len(cases) {
		t.Fatalf("%d results for %d cases", len(lines), len(cases))
	}
	outs := make([]javaOut, len(lines))
	for i, line := range lines {
		f := strings.Split(line, "\t")
		if len(f) != 5 {
			t.Fatalf("result line %d: %d fields", i+1, len(f))
		}
		if f[0] != cases[i].ID {
			t.Fatalf("result line %d is %s, case is %s", i+1, f[0], cases[i].ID)
		}
		switch f[1] {
		case "ok", "invalid", "exc":
		default:
			t.Fatalf("result line %d: status %q", i+1, f[1])
		}
		outs[i] = javaOut{Status: f[1], A: unescapeJava(f[2]), B: unescapeJava(f[3])}
		if f[4] != "" {
			if _, err := fmt.Sscan(f[4], &outs[i].P); err != nil || outs[i].P <= 0 {
				t.Fatalf("result line %d: precision %q", i+1, f[4])
			}
		}
	}
	return cases, outs
}

func ratOf(s string) (*big.Rat, bool) {
	return new(big.Rat).SetString(s)
}

// ourAnswer corre el caso en nuestro motor; ci es el servicio que resuelve
// la forma CODE de los átomos, para los casos xml-ci.
func ourAnswer(s, ci *Service, c javaCase) ourOut {
	a := c.Args
	verdict := func(err error) ourOut {
		if err != nil {
			return ourOut{Status: "invalid", Msg: err.Error()}
		}
		return ourOut{Status: "ok"}
	}
	fail := func(err error) ourOut { return ourOut{Status: "err", Msg: err.Error(), Err: err} }
	switch c.Op {
	case "validate":
		if c.Source == "xml-ci" {
			return verdict(ci.Validate(a[0]))
		}
		return verdict(s.Validate(a[0]))
	case "inproperty":
		return verdict(s.ValidateInProperty(a[0], a[1]))
	case "comparable":
		ok, err := s.IsComparable(a[0], a[1])
		if err != nil {
			return fail(err)
		}
		return ourOut{Status: "ok", Bool: ok}
	case "canonical":
		return ourCanonical(s, a[0], a[1])
	case "convert":
		v, ok := ratOf(a[0])
		if !ok {
			return fail(fmt.Errorf("value %q is not a decimal", a[0]))
		}
		r, err := s.ConvertRat(v, a[1], a[2])
		if errors.Is(err, ErrNotRational) {
			f, _ := v.Float64()
			out, err := s.Convert(f, a[1], a[2])
			if err != nil {
				return fail(err)
			}
			return ourOut{Status: "ok", Float: out}
		}
		if err != nil {
			return fail(err)
		}
		return ourOut{Status: "ok", Exact: r}
	case "multiply", "divide":
		v1, ok1 := ratOf(a[0])
		v2, ok2 := ratOf(a[2])
		if !ok1 || !ok2 {
			return fail(fmt.Errorf("value %q or %q is not a decimal", a[0], a[2]))
		}
		f1, _ := v1.Float64()
		f2, _ := v2.Float64()
		op := s.Multiply
		if c.Op == "divide" {
			op = s.Divide
		}
		p, err := op(Pair{f1, a[1]}, Pair{f2, a[3]})
		if err != nil {
			return fail(err)
		}
		// Java da el resultado en unidades canónicas; nosotros, en las de los
		// operandos. Se compara en canónicas, y nuestro lado ya pasó por
		// float64.
		r := decimal.RatFromFloat(p.Value)
		if r == nil {
			return ourOut{Status: "ok", Float: p.Value, Code: p.Code}
		}
		out := ourCanonical(s, r.RatString(), p.Code)
		if out.Exact != nil {
			out.Float, _ = out.Exact.Float64()
			out.Exact = nil
		}
		return out
	}
	panic("unknown op " + c.Op)
}

func ourCanonical(s *Service, value, code string) ourOut {
	v, ok := ratOf(value)
	if !ok {
		return ourOut{Status: "err", Msg: fmt.Sprintf("value %q is not a decimal", value)}
	}
	p, err := s.CanonicalRat(v, code)
	if errors.Is(err, ErrNotRational) {
		f, _ := v.Float64()
		fp, err := s.Canonical(f, code)
		if err != nil {
			return ourOut{Status: "err", Msg: err.Error(), Err: err}
		}
		return ourOut{Status: "ok", Float: fp.Value, Code: fp.Code}
	}
	if err != nil {
		return ourOut{Status: "err", Msg: err.Error(), Err: err}
	}
	return ourOut{Status: "ok", Exact: p.Value, Code: p.Code}
}

// canonicalUnits lee un código canónico ("g.m-3", "C-1.g.m2.s-2", "" para
// adimensional) como átomo → exponente, sin usar nuestro parser: Java ordena
// los átomos alfabéticamente (Converter.java:119-123) y nosotros no.
func canonicalUnits(code string) (map[string]int, bool) {
	out := map[string]int{}
	if code == "" || code == "1" {
		return out, true
	}
	depth, start := 0, 0
	var parts []string
	for i := 0; i < len(code); i++ {
		switch code[i] {
		case '[':
			depth++
		case ']':
			depth--
		case '.':
			if depth == 0 {
				parts = append(parts, code[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, code[start:])
	for _, p := range parts {
		end := len(p)
		for end > 0 && p[end-1] >= '0' && p[end-1] <= '9' {
			end--
		}
		if end > 0 && (p[end-1] == '-' || p[end-1] == '+') && end < len(p) {
			end--
		}
		atom, exp := p[:end], 1
		if end < len(p) {
			if _, err := fmt.Sscan(p[end:], &exp); err != nil {
				return nil, false
			}
		}
		if atom == "" {
			return nil, false
		}
		out[atom] += exp
	}
	return out, true
}

func sameCanonicalUnits(a, b string) bool {
	ua, ok1 := canonicalUnits(a)
	ub, ok2 := canonicalUnits(b)
	if !ok1 || !ok2 || len(ua) != len(ub) {
		return false
	}
	for k, v := range ua {
		if ub[k] != v {
			return false
		}
	}
	return true
}

// relErr es |x - y| / |y|. Si y es cero, es 0 solo si x también lo es: un
// error relativo no puede aceptar que perdamos el valor entero.
func relErr(x, y *big.Rat) float64 {
	if y.Sign() == 0 {
		if x.Sign() == 0 {
			return 0
		}
		return math.Inf(1)
	}
	d := new(big.Rat).Sub(x, y)
	d.Abs(d)
	d.Quo(d, new(big.Rat).Abs(y))
	f, _ := d.Float64()
	return f
}

// numbersAgree aplica el contrato numérico: exacto contra el decimal de Java
// dentro de javaRelTol; float64 dentro de floatRelTol.
func numbersAgree(java string, ours ourOut) bool {
	j, ok := ratOf(java)
	if !ok {
		return false
	}
	if ours.Exact != nil {
		return j.Cmp(ours.Exact) == 0 || relErr(j, ours.Exact) <= javaRelTol
	}
	o := decimal.RatFromFloat(ours.Float)
	return o != nil && relErr(j, o) <= floatRelTol
}

// compareJava devuelve la diferencia del caso, o nil si las respuestas
// coinciden. Un error en los dos lados coincide: los mensajes no se comparan.
func compareJava(c javaCase, j javaOut, o ourOut) *javaDiff {
	d := &javaDiff{Case: c, Java: j, Ours: o}
	jOK := j.Status == "ok"
	oOK := o.Status == "ok"
	if jOK != oOK {
		d.What = "status"
		return d
	}
	if !jOK {
		return nil
	}
	switch c.Op {
	case "validate", "inproperty":
		return nil
	case "comparable":
		if (j.A == "true") != o.Bool {
			d.What = "bool"
			return d
		}
		return nil
	case "canonical", "multiply", "divide":
		if !sameCanonicalUnits(j.B, o.Code) {
			d.What = "code"
			return d
		}
	}
	if !numbersAgree(j.A, o) {
		d.What = "value"
		return d
	}
	return nil
}

func (o ourOut) String() string {
	switch {
	case o.Status != "ok":
		return o.Status + ": " + o.Msg
	case o.Exact != nil:
		return o.Exact.FloatString(30) + " " + o.Code
	case o.Code != "" || o.Float != 0:
		return fmt.Sprintf("%v %s", o.Float, o.Code)
	}
	return fmt.Sprintf("ok %v", o.Bool)
}

func (j javaOut) String() string {
	return strings.TrimSpace(j.Status + " " + j.A + " " + j.B)
}

func TestCompareJava(t *testing.T) {
	cases, outs := loadJavaRun(t)
	s := newTestService(t)
	ci, err := New(nil, true)
	if err != nil {
		t.Fatal(err)
	}

	x := newJavaContext(t, s, cases, outs)
	oracle := loadJavaOracle(t, x)
	byOp := map[string]*javaTally{}
	var maxRounded float64
	classHits := map[string][]javaDiff{}
	var unclassified, multi []string
	var oracleFails []string
	for i, c := range cases {
		o := ourAnswer(s, ci, c)
		if msg := oracle.check(c, o); msg != "" {
			oracleFails = append(oracleFails, c.ID+": "+msg)
		}
		tl := byOp[c.Op]
		if tl == nil {
			tl = &javaTally{}
			byOp[c.Op] = tl
		}
		d := compareJava(c, outs[i], o)
		if d == nil {
			tl.agree++
			if outs[i].Status == "ok" && (c.Op == "canonical" || c.Op == "convert") && o.Exact != nil {
				if j, ok := ratOf(outs[i].A); ok && j.Cmp(o.Exact) != 0 {
					tl.rounded++
					maxRounded = math.Max(maxRounded, relErr(j, o.Exact))
				}
			}
			continue
		}
		tl.diff++
		var names []string
		for _, cl := range javaClasses {
			if cl.match(x, *d) {
				names = append(names, cl.Name)
			}
		}
		line := fmt.Sprintf("%s %s %v: java %v, ours %v (%s)", c.ID, c.Op, c.Args, outs[i], o, d.What)
		switch len(names) {
		case 0:
			unclassified = append(unclassified, line)
		case 1:
			classHits[names[0]] = append(classHits[names[0]], *d)
		default:
			multi = append(multi, line+" in "+strings.Join(names, ", "))
		}
	}

	t.Logf("independent oracle: %d special-unit results checked against the specification; %d UcumCommonUnitsTest expectations set aside as contrary to UCUM §24",
		oracle.specialChecked, oracle.commonAgainstSpec)
	report := javaReport(cases, byOp, classHits, maxRounded, oracle)
	if p := os.Getenv("UCUM_JAVA_REPORT"); p != "" {
		if err := os.WriteFile(p, report, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("max relative error inside the Java contract: %.3g", maxRounded)

	for _, l := range first(unclassified, 40) {
		t.Errorf("unclassified: %s", l)
	}
	if len(unclassified) > 0 {
		t.Errorf("%d unclassified differences", len(unclassified))
	}
	for _, l := range first(multi, 20) {
		t.Errorf("more than one class: %s", l)
	}
	for _, cl := range javaClasses {
		if len(classHits[cl.Name]) == 0 {
			t.Errorf("class %s explains no case", cl.Name)
		}
		if cl.Kind == "bug-ours" && cl.Issue == "" {
			t.Errorf("class %s is bug-ours with no issue", cl.Name)
		}
	}
	// Cada expresión de javaSyntaxBugs tiene que seguir siendo una diferencia:
	// si dejamos de rechazarla, la clase no debe esconderlo.
	syntax := map[string]bool{}
	for _, d := range classHits["java-accepts-invalid-syntax"] {
		syntax[d.Case.Args[0]] = true
	}
	for u := range javaSyntaxBugs {
		if !syntax[u] {
			t.Errorf("javaSyntaxBugs: %q is no longer a difference", u)
		}
	}
	for _, l := range first(oracleFails, 40) {
		t.Errorf("independent oracle: %s", l)
	}
}

func first(xs []string, n int) []string {
	if len(xs) > n {
		return xs[:n]
	}
	return xs
}

// javaReport resume la comparación: conteos por operación y por clase, con
// ejemplos.
func javaReport(cases []javaCase, byOp map[string]*javaTally, hits map[string][]javaDiff, maxRounded float64, or *javaOracle) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "# Comparison against Ucum-java 1.0.10\n\n")
	fmt.Fprintf(&b, "Generated by `make compare-java` (TestCompareJava, plan 002) over %d cases.\n\n", len(cases))
	fmt.Fprintf(&b, "| Operation | Agree | of which within Java's decimal rounding | Differ |\n| --- | ---: | ---: | ---: |\n")
	ops := make([]string, 0, len(byOp))
	for op := range byOp {
		ops = append(ops, op)
	}
	sort.Strings(ops)
	for _, op := range ops {
		tl := byOp[op]
		fmt.Fprintf(&b, "| %s | %d | %d | %d |\n", op, tl.agree, tl.rounded, tl.diff)
	}
	fmt.Fprintf(&b, "\nLargest relative error accepted as Java's decimal rounding: %.3g (contract: %.0g).\n", maxRounded, javaRelTol)
	fmt.Fprintf(&b, "\nIndependent oracle: %d results on special units checked against the functions of the specification; "+
		"%d expectations of UcumCommonUnitsTest.java set aside because they drop arbitrary units, contrary to UCUM §24.\n",
		or.specialChecked, or.commonAgainstSpec)
	fmt.Fprintf(&b, "\n## Differences by class\n")
	for _, cl := range javaClasses {
		ds := hits[cl.Name]
		fmt.Fprintf(&b, "\n### %s — %s, %d cases\n\n%s\n\n", cl.Name, cl.Kind, len(ds), cl.Reason)
		if cl.Spec != "" {
			fmt.Fprintf(&b, "Specification: %s.\n", cl.Spec)
		}
		if cl.JavaRef != "" {
			fmt.Fprintf(&b, "Ucum-java: %s.\n", cl.JavaRef)
		}
		if cl.Issue != "" {
			fmt.Fprintf(&b, "Issue: %s.\n", cl.Issue)
		}
		fmt.Fprintf(&b, "\n")
		for _, d := range ds[:min(len(ds), 5)] {
			fmt.Fprintf(&b, "- `%s %s` — Java: `%v`; ours: `%v`\n", d.Case.Op, strings.Join(d.Case.Args, " "), d.Java, d.Ours)
		}
	}
	return b.Bytes()
}
