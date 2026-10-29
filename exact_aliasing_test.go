package ucum_test

import (
	"math/big"
	"testing"

	"github.com/gofhir/ucum/v4"
)

// Plan 007 (#56), test 3: la API exacta no modifica el valor que recibe, y lo
// que devuelve es de quien llama. Los pares cubren unidades lineales,
// especiales racionales sin y con prefijo (solo con prefijo el factor Alpha
// es distinto de 1) y una escala sin forma racional.
func TestExactAPIDoesNotAlias(t *testing.T) {
	svc, err := ucum.NewExact()
	if err != nil {
		t.Fatal(err)
	}
	big30, _ := new(big.Rat).SetString("1000000000000000000000000000000")
	values := []*big.Rat{big.NewRat(37, 1), big.NewRat(-3, 2), new(big.Rat), big.NewRat(1, 3), big30}
	pairs := [][2]string{
		{"mL", "L"}, {"mg/dL", "g/L"}, {"L", "L"},
		{"Cel", "K"}, {"[degF]", "Cel"}, {"mCel", "K"}, {"kCel", "Cel"}, {"K", "mCel"},
		{"[pH]", "[pH]"},
	}
	seven := big.NewRat(7, 1)
	for _, p := range pairs {
		for _, v := range values {
			in, want := new(big.Rat).Set(v), new(big.Rat).Set(v)

			if r, err := svc.ConvertRat(in, p[0], p[1]); err == nil {
				first := new(big.Rat).Set(r)
				r.Add(r, seven)
				again, _ := svc.ConvertRat(new(big.Rat).Set(want), p[0], p[1])
				if again.Cmp(first) != 0 {
					t.Errorf("ConvertRat(%s, %s, %s): changing a result changed the next one", want.RatString(), p[0], p[1])
				}
			}
			if in.Cmp(want) != 0 {
				t.Errorf("ConvertRat(%s, %s, %s) changed its input to %s", want.RatString(), p[0], p[1], in.RatString())
			}

			if rp, err := svc.CanonicalRat(in, p[0]); err == nil {
				first := new(big.Rat).Set(rp.Value)
				rp.Value.Add(rp.Value, seven)
				again, _ := svc.CanonicalRat(new(big.Rat).Set(want), p[0])
				if again.Value.Cmp(first) != 0 {
					t.Errorf("CanonicalRat(%s, %s): changing a result changed the next one", want.RatString(), p[0])
				}
			}
			if in.Cmp(want) != 0 {
				t.Errorf("CanonicalRat(%s, %s) changed its input to %s", want.RatString(), p[0], in.RatString())
			}
		}
		if f, err := svc.ConversionFactor(p[0], p[1]); err == nil {
			first := new(big.Rat).Set(f)
			f.Add(f, seven)
			if again, _ := svc.ConversionFactor(p[0], p[1]); again.Cmp(first) != 0 {
				t.Errorf("ConversionFactor(%s, %s): changing a result changed the next one", p[0], p[1])
			}
		}
	}
}
