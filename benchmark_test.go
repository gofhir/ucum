package ucum

import "testing"

var benchSvc Service

func init() {
	var err error
	benchSvc, err = New()
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
		_, _ = New()
	}
}
