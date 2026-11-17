package essence

import (
	"bufio"
	"encoding/xml"
	"errors"
	"io"
	"math/big"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/gofhir/ucum/v4/internal/decimal"
)

// officialLiterals lee el XML embebido y devuelve el atributo value de cada
// <value> y de cada <function> que lo lleva.
func officialLiterals(t *testing.T) []string {
	t.Helper()
	f, err := embeddedDefinitions.Open("ucum-essence.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	dec := xml.NewDecoder(f)
	dec.CharsetReader = func(_ string, in io.Reader) (io.Reader, error) { return in, nil }
	var out []string
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok || (se.Name.Local != "value" && se.Name.Local != "function") {
			continue
		}
		for _, a := range se.Attr {
			if a.Name.Local == "value" {
				out = append(out, a.Value)
			}
		}
	}
}

// TestOfficialLiteralsPassTheGrammar: los 329 literales del XML oficial, que el
// plan 023 cuenta (seis formas, ninguno vacío, con signo ni espacios), se leen.
func TestOfficialLiteralsPassTheGrammar(t *testing.T) {
	lits := officialLiterals(t)
	if len(lits) != 329 {
		t.Fatalf("%d literals in the official XML, the plan counts 329", len(lits))
	}
	for _, s := range lits {
		if s == "" {
			t.Errorf("empty literal in the official XML")
			continue
		}
		if _, err := decimal.FromString(s); err != nil {
			t.Errorf("official literal %q: %v", s, err)
		}
	}
}

// unescape deshace los escapes de java-literals.txt: \\, \t y \uXXXX.
func unescape(t *testing.T, s string) string {
	t.Helper()
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 't':
			b.WriteByte('\t')
		case 'u':
			n, err := strconv.ParseUint(s[i+1:i+5], 16, 32)
			if err != nil {
				t.Fatal(err)
			}
			b.WriteRune(rune(n))
			i += 4
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// attrRef escribe s como referencias numéricas, como la sonda de Java, para que
// un tabulador no se normalice a espacio dentro del atributo.
func attrRef(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '.' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteString("&#" + strconv.Itoa(int(r)) + ";")
		}
	}
	return b.String()
}

// TestMatchesJavaProbe compara la carga con la salida versionada de
// tools/javavalues (make java-dump): mismo veredicto y, si carga, el mismo valor
// de 1 [mi_i]. La única diferencia de los literales que se le pasan a Java es
// "-" solo, que Java lee como 0 y aquí es un error (plan 023, §2). Los otros dos
// (dígitos no ASCII y exponente de más de cuatro cifras) no están en la lista
// porque Java no termina con ellos.
func TestMatchesJavaProbe(t *testing.T) {
	raw, err := os.ReadFile("ucum-essence.xml")
	if err != nil {
		t.Fatal(err)
	}
	const anchor = `<value Unit="[ft_i]" UNIT="[FT_I]" value="5280">`
	xmlText := string(raw)
	if strings.Count(xmlText, anchor) != 1 {
		t.Fatalf("anchor of [mi_i] found %d times", strings.Count(xmlText, anchor))
	}

	f, err := os.Open("testdata/java-values.tsv")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	// 1 [ft_i] son 0.3048 m, escrito como fracción.
	footInMeters := big.NewRat(381, 1250)
	n := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		cols := strings.Split(sc.Text(), "\t")
		if len(cols) != 3 {
			t.Fatalf("bad line %q", sc.Text())
		}
		lit := unescape(t, cols[0])
		javaAccepts := cols[1] == "accept"
		n++

		doc := strings.Replace(xmlText, anchor,
			`<value Unit="[ft_i]" UNIT="[FT_I]" value="`+attrRef(lit)+`">`, 1)
		m, err := Load(strings.NewReader(doc))
		wantAccept := javaAccepts && lit != "-"
		if (err == nil) != wantAccept {
			t.Errorf("literal %q: Java accepts = %v, here error = %v", lit, javaAccepts, err)
			continue
		}
		if err != nil {
			continue
		}
		got := new(big.Rat).Mul(m.Lookup("[mi_i]").Value.Value.Rat(), footInMeters)
		want, ok := new(big.Rat).SetString(cols[2])
		if !ok {
			t.Fatalf("Java value %q is not a number", cols[2])
		}
		if got.Cmp(want) != 0 {
			t.Errorf("literal %q: 1 [mi_i] = %s m, Java says %s", lit, got.FloatString(12), cols[2])
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if n < 30 {
		t.Fatalf("only %d lines in java-values.tsv", n)
	}
}
