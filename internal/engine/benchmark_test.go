package engine

import "testing"

var benchSvc *Service

func init() {
	var err error
	benchSvc, err = New(nil, false)
	if err != nil {
		panic(err)
	}
}

func BenchmarkValidateSimple(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = benchSvc.Validate("m")
	}
}

func BenchmarkValidateComplex(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = benchSvc.Validate("mg/dL")
	}
}

func BenchmarkValidateMixed(b *testing.B) {
	codes := []string{"m", "kg", "mg/dL", "10*3/uL", "mm[Hg]", "%", "[lb_av]", "mol/L"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = benchSvc.Validate(codes[i%len(codes)])
	}
}

func BenchmarkConvertSimple(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = benchSvc.Convert(1, "km", "m")
	}
}

func BenchmarkConvertSpecial(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = benchSvc.Convert(37, "Cel", "[degF]")
	}
}

func BenchmarkCanonical(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = benchSvc.Canonical(1, "kg.m/s2")
	}
}

func BenchmarkIsComparable(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = benchSvc.IsComparable("mg", "g")
	}
}

func BenchmarkNew(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = New(nil, false)
	}
}

// Escalas logarítmicas (plan 011): el costo de la vía exacta de un exponente
// entero, y que un exponente no entero no pague nada por ella.
func BenchmarkCanonicalLogScale(b *testing.B) {
	for _, tc := range []struct {
		name string
		v    float64
		code string
	}{
		{"7.4 [pH]", 7.4, "[pH]"},
		{"1 [pH]", 1, "[pH]"},
		{"3 B", 3, "B"},
		{"3 B[W]", 3, "B[W]"},
		{"60 B[SPL]", 60, "B[SPL]"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, _ = benchSvc.Canonical(tc.v, tc.code)
			}
		})
	}
}

func BenchmarkMultiplyLogScale(b *testing.B) {
	x, y := Pair{Value: 2, Code: "[pH]"}, Pair{Value: 3, Code: "L"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = benchSvc.Multiply(x, y)
	}
}
