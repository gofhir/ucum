package decimal

import (
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"testing"
)

// pow10 es 10^n como entero, para escribir las fracciones esperadas.
func pow10(n int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
}

func frac(num, den *big.Int) *big.Rat { return new(big.Rat).SetFrac(num, den) }

func small(num, den int64) *big.Rat { return big.NewRat(num, den) }

// TestFromStringGrammar fija la gramática del plan 023: cada literal válido con
// su valor escrito como fracción, y cada inválido con su mensaje.
func TestFromStringGrammar(t *testing.T) {
	valid := []struct {
		in   string
		want *big.Rat
	}{
		{"", small(1, 1)},
		{"9", small(9, 1)},
		{"9.9", small(99, 10)},
		{"9e-9", frac(big.NewInt(9), pow10(9))},
		{"9e9", frac(new(big.Int).Mul(big.NewInt(9), pow10(9)), big.NewInt(1))},
		{"9.9e-9", frac(big.NewInt(99), pow10(10))},
		{"9.9e9", small(9900000000, 1)},
		{"-5", small(-5, 1)},
		{".5", small(1, 2)},
		{"-.5", small(-1, 2)},
		{"-0", small(0, 1)},
		{"0008", small(8, 1)},
		{"00.10", small(1, 10)},
		{"1e-0", small(1, 1)},
		{"1E-3", small(1, 1000)},
		{"1E3", small(1000, 1)},
		{"1.5e-3", small(3, 2000)},
		{"-5e3", small(-5000, 1)},
		{"-5e-3", small(-1, 200)},
		{"-.5e2", small(-50, 1)},
		{"1e9999", frac(pow10(9999), big.NewInt(1))},
		{"1e-9999", frac(big.NewInt(1), pow10(9999))},
		{"1e0000000009999", frac(pow10(9999), big.NewInt(1))},
		{"1e-0009999", frac(big.NewInt(1), pow10(9999))},
	}
	for _, c := range valid {
		d, err := FromString(c.in)
		if err != nil {
			t.Errorf("FromString(%q) error: %v", c.in, err)
			continue
		}
		if d.Rat().Cmp(c.want) != 0 {
			t.Errorf("FromString(%q) = %s, want %s", c.in, d.Rat(), c.want)
		}
	}

	invalid := []string{
		"abc", "1p20", "0x10", "1_000", "1/3", "+5", "5.", "1e", "1e+3", "1E+3",
		" 7 ", "-", ".", "e5", "1e-", "--1", "1.2.3", "１", "1e٣", "NaN",
		"Inf", "1 ", "\t1", "1\n", "1p999999", "-.", ".e1", "1.e2", "0b1", "1e1.5",
	}
	for _, in := range invalid {
		_, err := FromString(in)
		want := "invalid decimal " + quote(in)
		if err == nil || err.Error() != want {
			t.Errorf("FromString(%q) error = %v, want %q", in, err, want)
		}
	}

	beyond := []string{"1e10000", "1e-10000", "1e2147483648", "1e-2147483648", "1e99999999999999999999", "1E10000"}
	for _, in := range beyond {
		_, err := FromString(in)
		want := "exponent of " + quote(in) + " is beyond ±9999"
		if err == nil || err.Error() != want {
			t.Errorf("FromString(%q) error = %v, want %q", in, err, want)
		}
	}
}

func quote(s string) string { return fmt.Sprintf("%q", s) }

// A literal beyond the bound must be refused without building the power.
func TestFromStringBoundIsCheap(t *testing.T) {
	allocs := testing.AllocsPerRun(10, func() { _, _ = FromString("1e1000000000") })
	if allocs > 20 {
		t.Errorf("a literal beyond the bound allocated %v times; it must be rejected before the power is built", allocs)
	}
}

// oracleGrammar está escrita aparte del código y no calcula potencias: solo
// decide aceptación.
var oracleGrammar = regexp.MustCompile(`^-?(\d+(\.\d+)?|\.\d+)([eE]-?\d+)?$`)

func oracleAccepts(s string) bool {
	if s == "" {
		return true
	}
	if !oracleGrammar.MatchString(s) {
		return false
	}
	i := strings.IndexAny(s, "eE")
	if i < 0 {
		return true
	}
	digits := strings.TrimLeft(strings.TrimPrefix(s[i+1:], "-"), "0")
	return len(digits) < 4 || (len(digits) == 4 && digits <= "9999")
}

func FuzzFromString(f *testing.F) {
	for _, s := range []string{
		"", "9", "9.9", "9e-9", "9e9", "-5", ".5", "-.5", "-0", "0008", "1E-3", "1e9999", "1e-9999",
		"1e10000", "1e-10000", "1e2147483648", "abc", "1p20", "0x10", "1_000", "1/3", "+5", "5.",
		"1e", "1e+3", " 7 ", "-", ".", "e5", "1e-", "--1", "1.2.3", "１", "1e٣", "NaN",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 64 {
			t.Skip()
		}
		_, err := FromString(s)
		if (err == nil) != oracleAccepts(s) {
			t.Errorf("FromString(%q) error = %v, oracle accepts = %v", s, err, oracleAccepts(s))
		}
	})
}
