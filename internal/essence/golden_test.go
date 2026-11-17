package essence

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"testing"
)

const goldenPath = "testdata/definitions.golden.tsv"

// goldenLines vuelca el modelo cargado, una línea por definición, con cada
// valor como fracción exacta num/den.
func goldenLines(t *testing.T) []string {
	t.Helper()
	m, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	var out []string
	for _, p := range m.Prefixes {
		out = append(out, fmt.Sprintf("prefix\t%s\t%s", p.Code, p.Value.Rat().String()))
	}
	for _, u := range m.DefinedUnits {
		if u.Value == nil {
			out = append(out, fmt.Sprintf("unit\t%s\t-", u.Code))
			continue
		}
		line := fmt.Sprintf("unit\t%s\t%s\t%s", u.Code, u.Value.Value.Rat().String(), u.Value.Unit)
		if f := u.Value.Function; f != nil {
			line += fmt.Sprintf("\tfunction\t%s\t%s\t%s", f.Name, f.Value.Rat().String(), f.Unit)
		}
		out = append(out, line)
	}
	return out
}

// TestDefinitionsGolden compara el modelo del XML oficial con el dorado
// generado en main antes del plan 023: la lectura estricta no cambia ninguna
// definición oficial. UCUM_WRITE_GOLDEN=1 lo regenera.
func TestDefinitionsGolden(t *testing.T) {
	got := goldenLines(t)
	if os.Getenv("UCUM_WRITE_GOLDEN") == "1" {
		if err := os.WriteFile(goldenPath, []byte(strings.Join(got, "\n")+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	f, err := os.Open(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var want []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		want = append(want, sc.Text())
	}
	if len(got) != len(want) {
		t.Fatalf("%d definitions, golden has %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("definition %d:\n got %q\nwant %q", i, got[i], want[i])
		}
	}
}
