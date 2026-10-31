package engine

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"os"
	"strings"
	"testing"

	"github.com/gofhir/ucum/v4/internal/ucumerr"
)

// Resultados fuera de float64 (#59, plan docs/plans/003-float-range.md).
//
// Los esperados salen de las fórmulas de la especificación y de los límites
// de IEEE 754 (el mayor float64, ≈ 1.797·10^308; el menor subnormal, 2^-1074;
// y su mitad, 2^-1075, que redondea a cero), no de correr el motor. Los que
// dependen de un factor del XML distinto de 1 se calculan además con
// rangeOracle, que lo lee aparte.

// rangeCall es una llamada pública con su resultado esperado.
type rangeCall struct {
	name string
	spec string // sección que fija el esperado
	call func(s *Service) (float64, error)
	kind rangeKind
	want float64 // si kind es rangeFits
	tol  float64 // error relativo aceptado; 0 exige el valor exacto
}

func canonicalCall(v float64, code string) func(s *Service) (float64, error) {
	return func(s *Service) (float64, error) {
		p, err := s.Canonical(v, code)
		return p.Value, err
	}
}

func convertCall(v float64, from, to string) func(s *Service) (float64, error) {
	return func(s *Service) (float64, error) { return s.Convert(v, from, to) }
}

func multiplyCall(a, b Pair) func(s *Service) (float64, error) {
	return func(s *Service) (float64, error) {
		p, err := s.Multiply(a, b)
		return p.Value, err
	}
}

func divideCall(a, b Pair) func(s *Service) (float64, error) {
	return func(s *Service) (float64, error) {
		p, err := s.Divide(a, b)
		return p.Value, err
	}
}

// checkRangeCall verifica una llamada contra su esperado.
func checkRangeCall(t *testing.T, s *Service, c rangeCall) {
	t.Helper()
	got, err := c.call(s)
	switch c.kind {
	case rangeOut:
		if !errors.Is(err, ucumerr.ErrOutOfRange) {
			t.Errorf("%s (%s) = %v, %v; want ErrOutOfRange", c.name, c.spec, got, err)
		}
	case rangeOutside:
		if !errors.Is(err, ucumerr.ErrOutOfDomain) {
			t.Errorf("%s (%s) = %v, %v; want ErrOutOfDomain", c.name, c.spec, got, err)
		}
	case rangeFits:
		w := rangeWant{value: c.want, relTol: c.tol}
		if c.tol != 0 {
			w.absTol = subnormalULP
		}
		if err != nil || !w.accepts(got) {
			t.Errorf("%s (%s) = %v, %v; want %v", c.name, c.spec, got, err, c.want)
		}
	default:
		t.Fatalf("%s: kind %d", c.name, c.kind)
	}
}

const tol13 = 1e-13

// TestRangeHandWritten son los casos del plan 003, §2, tests A.
func TestRangeHandWritten(t *testing.T) {
	s := newTestService(t)
	ln10 := math.Log(10)
	cases := []rangeCall{
		// Factor de referencia 1: f⁻¹ = 10^x (B), e^x (Np), 2^x (bit_s).
		{"308 B", "§46■3", canonicalCall(308, "B"), rangeFits, 1e308, tol13},
		{"309 B", "§46■3", canonicalCall(309, "B"), rangeOut, 0, 0},
		{"-323 B", "§46■3, subnormal", canonicalCall(-323, "B"), rangeFits, 1e-323, tol13},
		{"-323.6 B", "§46■3, 10^-323.6 > 2^-1075", canonicalCall(-323.6, "B"), rangeFits, 5e-324, tol13},
		{"-324 B", "§46■3, 10^-324 < 2^-1075", canonicalCall(-324, "B"), rangeOut, 0, 0},
		{"709 Np", "§46■3", canonicalCall(709, "Np"), rangeFits, math.Exp(709), tol13},
		{"710 Np", "§46■3", canonicalCall(710, "Np"), rangeOut, 0, 0},
		// Base e: sin potencia exacta ni partida, sigue por fromLog10 (plan 010).
		{"-710 Np", "§46■3, subnormal", canonicalCall(-710, "Np"), rangeFits, math.Exp(-710), tol13},
		{"-710 Np to 10*-300", "§46■3: e^-710·10^300", convertCall(-710, "Np", "10*-300"), rangeFits, math.Exp(-710) * 1e300, tol13},
		{"1023 bit_s", "§47■4", canonicalCall(1023, "bit_s"), rangeFits, 0x1p1023, tol13},
		{"1024 bit_s", "§47■4", canonicalCall(1024, "bit_s"), rangeOut, 0, 0},
		{"3080 dB", "§22■4: α escala el argumento", canonicalCall(3080, "dB"), rangeFits, 1e308, tol13},
		{"3090 dB", "§22■4", canonicalCall(3090, "dB"), rangeOut, 0, 0},

		// Factor distinto de 1 (los límites también los verifica
		// TestRangeHandWrittenLimitsFromXML): V = 1000 g.m2.s-2.C-1, la
		// referencia de B[SPL] es 2·10^-5 Pa = 0.02 g.m-1.s-2, y la de [pH] es
		// mol/l = 6.02214076·10^26 m-3.
		{"610 B[V]", "§46■3, 2lg", canonicalCall(610, "B[V]"), rangeFits, 1e308, tol13},
		{"613 B[V]", "§46■3, 2lg", canonicalCall(613, "B[V]"), rangeOut, 0, 0},
		{"618 B[SPL]", "§46■3, intermedio 10^309", canonicalCall(618, "B[SPL]"), rangeFits, 2e307, tol13},
		{"620 B[SPL]", "§46■3", canonicalCall(620, "B[SPL]"), rangeOut, 0, 0},
		{"324 [pH]", "§45■3, intermedio 10^-324", canonicalCall(324, "[pH]"), rangeFits, 6.02214076e-298, tol13},
		{"-281 [pH]", "§45■3", canonicalCall(-281, "[pH]"), rangeFits, 6.02214076e307, tol13},
		{"-282 [pH]", "§45■3", canonicalCall(-282, "[pH]"), rangeOut, 0, 0},
		{"350 [pH]", "§45■3, 6.02·10^-324 redondea a 2^-1074", canonicalCall(350, "[pH]"), rangeFits, 5e-324, tol13},
		{"351 [pH]", "§45■3", canonicalCall(351, "[pH]"), rangeOut, 0, 0},

		// De una escala logarítmica a otra: el intermedio no se materializa.
		{"1000 B to dB", "§22■4", convertCall(1000, "B", "dB"), rangeFits, 10000, tol13},
		{"-320 B to B", "§22■4", convertCall(-320, "B", "B"), rangeFits, -320, tol13},
		{"1e-17 B to B", "§22■4", convertCall(1e-17, "B", "B"), rangeFits, 1e-17, tol13},
		{"-323.5 B to dB", "§22■4", convertCall(-323.5, "B", "dB"), rangeFits, -3235, tol13},
		{"1000 B to Np", "§46■3: ln(10^1000)", convertCall(1000, "B", "Np"), rangeFits, 1000 * ln10, tol13},
		{"1e308 B to dB", "§22■4: 10^309 dB", convertCall(1e308, "B", "dB"), rangeOut, 0, 0},

		// Dominio: f solo existe para valores positivos (log) o no
		// negativos (sqrt).
		{"0 1 to B", "§46■3", convertCall(0, "1", "B"), rangeOutside, 0, 0},
		{"-1 1 to B", "§46■3", convertCall(-1, "1", "B"), rangeOutside, 0, 0},
		{"-1 1 to Np", "§46■3", convertCall(-1, "1", "Np"), rangeOutside, 0, 0},
		{"0 mol/l to [pH]", "§45■3", convertCall(0, "mol/l", "[pH]"), rangeOutside, 0, 0},
		{"-1 to sqrt", "§48", convertCall(-1, "m2/s3", "[m/s2/Hz^(1/2)]"), rangeOutside, 0, 0},
		{"1e-300 1 to B", "§46■3", convertCall(1e-300, "1", "B"), rangeFits, -300, tol13},
		{"1e300 Ym/m to B", "§46■3: lg(10^324)", convertCall(1e300, "Ym/m", "B"), rangeFits, 324, tol13},
		{"1e-300 Ym13/m13 to B", "§46■3: lg(10^12), factor 10^312", convertCall(1e-300, "Ym13/m13", "B"), rangeFits, 12, tol13},

		// Lineal a logarítmica con un factor de prefijo: exacto mientras el
		// valor sea normal (§46■3: lg 1 = 0).
		{"1000 W to B[kW]", "§46■3", convertCall(1000, "W", "B[kW]"), rangeFits, 0, 0},
		{"1000 mW to B[W]", "§46■3", convertCall(1000, "mW", "B[W]"), rangeFits, 0, 0},
		{"1000 uV to B[mV]", "§46■3, 2lg", convertCall(1000, "uV", "B[mV]"), rangeFits, 0, 0},

		{"1000 mV to B[V]", "§46■3, 2lg", convertCall(1000, "mV", "B[V]"), rangeFits, 0, 0},
		{"2e-5 Pa to dB[SPL]", "§46■3, la referencia de B[SPL]", convertCall(2e-5, "Pa", "dB[SPL]"), rangeFits, 0, 0},
		{"20 uPa to B[SPL]", "§46■3", convertCall(20, "uPa", "B[SPL]"), rangeFits, 0, 0},
		{"1e-3 V to B[mV]", "§46■3", convertCall(1e-3, "V", "B[mV]"), rangeFits, 0, 0},
		{"60 dB[mV] to B[V]", "§46■3: 6 B[mV] es 1 V", convertCall(60, "dB[mV]", "B[V]"), rangeFits, 0, 0},
		{"3 dB to B", "§22■4: α = 1/10", convertCall(3, "dB", "B"), rangeFits, 0.3, 0},
		{"30 dB to Np", "§46■3: ln(10^3)", convertCall(30, "dB", "Np"), rangeFits, 3 * ln10, tol13},

		// Lineal a logarítmica, redondeo único (§46■3): lg 10^3 y -lg 10^-3.
		{"1000 W to dB[W]", "§46■3", convertCall(1000, "W", "dB[W]"), rangeFits, 30, 0},
		{"0.001 mol/l to [pH]", "§46■3, f = -lg", convertCall(0.001, "mol/l", "[pH]"), rangeFits, 3, 0},
		// lgTimes2 en las dos: v B[uV] = 10^(v/2) uV, y en dB[mV] es
		// 10·(v + 2 lg 10^-3) = 10v - 60, redondeado una vez.
		{"5.780447444613257 B[uV] to dB[mV]", "§46■3, lgTimes2", convertCall(5.780447444613257, "B[uV]", "dB[mV]"), rangeFits, -2.19552555386743, 0},
		// α_d = 10^-24: 0 B[kW] es 1000 W, lg 1000 = 3 B[W], 3·10^24 yB[W].
		{"0 B[kW] to yB[W]", "§46■3, §22■4", convertCall(0, "B[kW]", "yB[W]"), rangeFits, 3e24, 0},
		{"0 B[W] to zB[kW]", "§46■3", convertCall(0, "B[W]", "zB[kW]"), rangeFits, -3e21, 0},
		// 10 nV en B[uV] es 2 lg(10^-8 / 10^-6) = -4, y en aB[uV], -4·10^18.
		{"0 B[10.nV] to aB[uV]", "§46■3, lgTimes2", convertCall(0, "B[10.nV]", "aB[uV]"), rangeFits, -4e18, 0},
		// [hp'_M] es -log1000 (§44): de B la razón de pendientes es -1/3.
		{"82.3 B to [hp'_M]", "§44, hpM", convertCall(82.3, "B", "[hp'_M]"), rangeFits, -27.433333333333334, 0},
		{"0.0876 kB to [hp'_M]", "§44, hpM", convertCall(0.0876, "kB", "[hp'_M]"), rangeFits, -29.2, 0},
		{"60 YB to [hp'_M]", "§44, hpM", convertCall(60, "YB", "[hp'_M]"), rangeFits, -2e25, 0},

		// Entre la misma referencia, r = v·α_s/α_d: 10^-324 redondea a cero.
		{"1e-300 yB to B", "§46■3, 10^-324", convertCall(1e-300, "yB", "B"), rangeOut, 0, 0},
		{"1e-280 yB to YB", "§46■3, 10^-328", convertCall(1e-280, "yB", "YB"), rangeOut, 0, 0},
		{"1e-320 B to YB", "§46■3, 10^-344", convertCall(1e-320, "B", "YB"), rangeOut, 0, 0},
		{"5e-324 dB to B", "§46■3, α = 1/10", convertCall(5e-324, "dB", "B"), rangeOut, 0, 0},
		{"-5e-324 dB to B", "§46■3", convertCall(-5e-324, "dB", "B"), rangeOut, 0, 0},
		{"5e-324 Np to B", "§46■3: 5e-324·2/ln 10", convertCall(5e-324, "Np", "B"), rangeOut, 0, 0},
		{"1e-320 B[W] to YB[W]", "§46■3", convertCall(1e-320, "B[W]", "YB[W]"), rangeOut, 0, 0},
		// Con referencias distintas el término de referencia domina, y el
		// redondeo es legítimo: lg(1 W / 1 kW) = -3, en B[kW] -3 y en YB 10^-24.
		{"1e-320 B[W] to YB[kW]", "§46■3", convertCall(1e-320, "B[W]", "YB[kW]"), rangeFits, -3e-24, 0},

		// 10^-500 / 10^-312: el valor va por log10 y el factor 10^312 no cabe en
		// float64.
		{"-500 B to ym13/m13", "§46■3, §25■1", convertCall(-500, "B", "ym13/m13"), rangeFits, 1e-188, tol13},

		// Un negativo que no es normal en la escala de referencia va por log10,
		// y ahí también carece de logaritmo o de raíz.
		{"-1e-300 fW to B[W]", "§46■3: lg(-10^-315)", convertCall(-1e-300, "fW", "B[W]"), rangeOutside, 0, 0},
		{"-1e300 YW to B[W]", "§46■3: lg(-10^324)", convertCall(-1e300, "YW", "B[W]"), rangeOutside, 0, 0},
		{"-1e-300 um2/s3 to sqrt", "§48: √(-10^-312)", convertCall(-1e-300, "um2/s3", "[m/s2/Hz^(1/2)]"), rangeOutside, 0, 0},

		// Un intermedio subnormal en una escala logarítmica: en amd64,
		// math.Log10 lo leía como normal (golang/go#56600) y daba -3079.5.
		{"1e-310 W to dB[W]", "§46■3", convertCall(1e-310, "W", "dB[W]"), rangeFits, -3100, tol13},
		{"5e-324 W to B[W]", "§46■3: lg 2^-1074", convertCall(5e-324, "W", "B[W]"), rangeFits, -1074 * math.Log10(2), tol13},
		{"1e-310 mol/l to [pH]", "§45■3", convertCall(1e-310, "mol/l", "[pH]"), rangeFits, 310, tol13},

		// Raíz cuadrada (§48: f(x) = √x).
		{"1e-300 um2/s3 to sqrt", "§48: √(10^-312)", convertCall(1e-300, "um2/s3", "[m/s2/Hz^(1/2)]"), rangeFits, 1e-156, tol13},
		{"4 m2/s3 to sqrt", "§48", convertCall(4, "m2/s3", "[m/s2/Hz^(1/2)]"), rangeFits, 2, 0},
		{"1e-300 to sqrt, 10^-330", "§48", convertCall(1e-300, "ym10/m8/Ys4/YHz", "[m/s2/Hz^(1/2)]"), rangeOut, 0, 0},
		{"1e300 to sqrt, 10^330", "§48", convertCall(1e300, "Ym10/m8/ys4/yHz", "[m/s2/Hz^(1/2)]"), rangeOut, 0, 0},

		// α·v desborda aunque v sea finito: 1e306 kB es 10^(10^309).
		{"1e306 kB x -1e306 kB", "§22■4", multiplyCall(Pair{1e306, "kB"}, Pair{-1e306, "kB"}), rangeOut, 0, 0},
		{"1e306 kB / 1e306 kB", "§22■4", divideCall(Pair{1e306, "kB"}, Pair{1e306, "kB"}), rangeOut, 0, 0},
		{"-1e306 kB / -1e306 kB", "§22■4", divideCall(Pair{-1e306, "kB"}, Pair{-1e306, "kB"}), rangeOut, 0, 0},

		// Funciones inyectivas que no son potencias: 0 solo en 0.
		{"5e-324 [p'diop]", "§44■4: arctan(x/100) > 0", canonicalCall(5e-324, "[p'diop]"), rangeOut, 0, 0},
		{"1e-200 sqrt", "§48: 10^-400", canonicalCall(1e-200, "[m/s2/Hz^(1/2)]"), rangeOut, 0, 0},
		{"1e-160 sqrt", "§48: 10^-320", canonicalCall(1e-160, "[m/s2/Hz^(1/2)]"), rangeFits, 1e-320, tol13},

		// Lineales, por la vía racional.
		{"1e300 Ym", "10^324 m", canonicalCall(1e300, "Ym"), rangeOut, 0, 0},
		{"1e-300 ym", "10^-324 m", canonicalCall(1e-300, "ym"), rangeOut, 0, 0},
		{"1e300 Ym to fm", "10^339 fm", convertCall(1e300, "Ym", "fm"), rangeOut, 0, 0},
		{"1e300 m x 1e300 m", "10^600 m2", multiplyCall(Pair{1e300, "m"}, Pair{1e300, "m"}), rangeOut, 0, 0},
		{"1e-300 m / 1e300 m", "10^-600", divideCall(Pair{1e-300, "m"}, Pair{1e300, "m"}), rangeOut, 0, 0},
		{"1e-310 m", "subnormal exacto", canonicalCall(1e-310, "m"), rangeFits, 1e-310, 0},

		// Cel y [degF], §21-22: m = (v + origen) × referencia.
		{"1e308 [degF]", "§43■4", canonicalCall(1e308, "[degF]"), rangeFits, degFKelvin(1e308), 0},
		{"-1e308 K to Cel", "§30■4", convertCall(-1e308, "K", "Cel"), rangeFits, -1e308, 0},
		{"1.7e308 MCel", "§30■4, 1.7·10^314 K", canonicalCall(1.7e308, "MCel"), rangeOut, 0, 0},
		{"max K to [degF]", "§43■4, factor 9/5", convertCall(math.MaxFloat64, "K", "[degF]"), rangeOut, 0, 0},

		// Multiply y Divide con unidades logarítmicas.
		{"200 B x 200 B", "10^400", multiplyCall(Pair{200, "B"}, Pair{200, "B"}), rangeOut, 0, 0},
		{"-200 B x -200 B", "10^-400", multiplyCall(Pair{-200, "B"}, Pair{-200, "B"}), rangeOut, 0, 0},
		{"200 B x -200 B", "10^0", multiplyCall(Pair{200, "B"}, Pair{-200, "B"}), rangeFits, 1, tol13},
		{"200 B / 200 B", "10^0", divideCall(Pair{200, "B"}, Pair{200, "B"}), rangeFits, 1, tol13},
		// 2lg: 10^155 cada uno; x1·x2 = 10^310 no cabe, pero por el factor
		// (2·10^-5 Pa = 0.02 g.m-1.s-2, al cuadrado) el resultado es 4·10^306.
		{"310 B[SPL] x 310 B[SPL]", "§46■3", multiplyCall(Pair{310, "B[SPL]"}, Pair{310, "B[SPL]"}), rangeFits, 4e306, tol13},
		{"1000 B x 1", "10^1000", multiplyCall(Pair{1000, "B"}, Pair{1, "1"}), rangeOut, 0, 0},
		// El divisor vale 10^-1000, no cero: el cociente no cabe.
		{"1 / -1000 B", "10^1000", divideCall(Pair{1, "1"}, Pair{-1000, "B"}), rangeOut, 0, 0},
		// x1·x2 subnormal con factor > 1: 10^-300 × 10^-15 V (V = 1000).
		{"-300 B x -30 B[V]", "§46■3", multiplyCall(Pair{-300, "B"}, Pair{-30, "B[V]"}), rangeFits, 1e-312, tol13},
		{"300 B x -330 B", "10^-30, con 10^-330 fuera de rango", multiplyCall(Pair{300, "B"}, Pair{-330, "B"}), rangeFits, 1e-30, tol13},
	}
	for _, c := range cases {
		checkRangeCall(t, s, c)
	}
}

// degFKelvin es (v + 459.67) × 5/9 (§43■4), exacto y redondeado una vez.
func degFKelvin(v float64) float64 {
	r := new(big.Rat).Add(new(big.Rat).SetFloat64(v), big.NewRat(45967, 100))
	f, _ := r.Mul(r, big.NewRat(5, 9)).Float64()
	return f
}

// TestRangeHandWrittenLimitsFromXML: los esperados de los casos con factor
// distinto de 1 coinciden con los que rangeOracle calcula desde el XML, para
// que un número escrito a mano no fije por sí solo un límite.
func TestRangeHandWrittenLimitsFromXML(t *testing.T) {
	o := newRangeOracle(t)
	for _, c := range []struct {
		v    float64
		code string
		kind rangeKind
		want float64
	}{
		{610, "B[V]", rangeFits, 1e308}, {613, "B[V]", rangeOut, 0},
		{618, "B[SPL]", rangeFits, 2e307}, {620, "B[SPL]", rangeOut, 0},
		{324, "[pH]", rangeFits, 6.02214076e-298},
		{-281, "[pH]", rangeFits, 6.02214076e307}, {-282, "[pH]", rangeOut, 0},
		{350, "[pH]", rangeFits, 5e-324}, {351, "[pH]", rangeOut, 0},
	} {
		w, ok := o.canonical(new(big.Rat).SetFloat64(c.v), c.code)
		if !ok || w.kind != c.kind || c.kind == rangeFits && !w.accepts(c.want) {
			t.Errorf("oracle %v %s = %+v, %v; hand-written %v %v", c.v, c.code, w, ok, c.kind, c.want)
		}
	}
}

// TestRangeErrors: el sentinel sobrevive al tipo de error de siempre, y el
// mensaje nombra el código.
func TestRangeErrors(t *testing.T) {
	s := newTestService(t)
	_, err := s.Canonical(1000, "B")
	var ve *ucumerr.ValidationError
	if !errors.As(err, &ve) || !errors.Is(err, ucumerr.ErrOutOfRange) || !strings.Contains(err.Error(), "B") {
		t.Errorf("Canonical(1000 B) error = %v, want a ValidationError wrapping ErrOutOfRange", err)
	}
	_, err = s.Convert(0, "1", "B")
	var ce *ucumerr.ConversionError
	if !errors.As(err, &ce) || !errors.Is(err, ucumerr.ErrOutOfDomain) || !strings.Contains(err.Error(), `"B"`) {
		t.Errorf("Convert(0, 1, B) error = %v, want a ConversionError wrapping ErrOutOfDomain", err)
	}
	_, err = s.Convert(1e300, "Ym", "fm")
	if !errors.As(err, &ce) || !errors.Is(err, ucumerr.ErrOutOfRange) {
		t.Errorf("Convert(1e300 Ym, fm) error = %v, want a ConversionError wrapping ErrOutOfRange", err)
	}
	// Un divisor cero sigue siendo división por cero aunque el dividendo vaya
	// por log10.
	if _, err := s.Divide(Pair{-400, "B"}, Pair{0, "m"}); !errors.Is(err, ucumerr.ErrDivisionByZero) {
		t.Errorf("Divide(-400 B, 0 m) error = %v, want ErrDivisionByZero", err)
	}
	_, err = s.Multiply(Pair{200, "B"}, Pair{200, "B"})
	if !errors.As(err, &ve) || !errors.Is(err, ucumerr.ErrOutOfRange) {
		t.Errorf("Multiply(200 B, 200 B) error = %v, want a ValidationError wrapping ErrOutOfRange", err)
	}
}

// TestRangeNonFinitePassesThrough: una entrada no finita no se verifica y da
// lo que da hoy (frontera con #55): 10^+Inf = +Inf, 10^-Inf = 0.
func TestRangeNonFinitePassesThrough(t *testing.T) {
	s := newTestService(t)
	for _, c := range []struct {
		v    float64
		code string
		want func(float64) bool
	}{
		{math.Inf(1), "B", func(f float64) bool { return math.IsInf(f, 1) }},
		{math.Inf(-1), "B", func(f float64) bool { return f == 0 }},
		{math.Inf(1), "[pH]", func(f float64) bool { return f == 0 }},
		{math.NaN(), "m", math.IsNaN},
		// Una escala logarítmica de base entera, con y sin prefijo: NaN no
		// entra a la potencia exacta (#75).
		{math.NaN(), "B", math.IsNaN},
		{math.NaN(), "dB", math.IsNaN},
		{math.NaN(), "[pH]", math.IsNaN},
		{math.Inf(1), "Ym", func(f float64) bool { return math.IsInf(f, 1) }},
	} {
		p, err := s.Canonical(c.v, c.code)
		if err != nil || !c.want(p.Value) {
			t.Errorf("Canonical(%v %s) = %v, %v; want it passed through with no error", c.v, c.code, p.Value, err)
		}
	}
	p, err := s.Multiply(Pair{math.Inf(1), "m"}, Pair{1, "m"})
	if err != nil || !math.IsInf(p.Value, 1) {
		t.Errorf("Multiply(+Inf m, 1 m) = %v, %v; want +Inf with no error", p, err)
	}
}

// rangeGrid son los log10 de prueba alrededor de cada límite.
var rangeGrid = []float64{-10, -1, -0.01, 0.01, 1, 10}

// TestRangeSweep es el barrido del plan 003, §2, tests B: cada unidad
// especial del XML, sola y con cada prefijo si es métrica, y cada átomo
// lineal con Y e y, en una grilla de valores alrededor de sus límites.
func TestRangeSweep(t *testing.T) {
	s := newTestService(t)
	o := newRangeOracle(t)
	e := readEssence(t)
	metric := map[string]bool{}
	for _, u := range e.Units {
		metric[u.Code] = u.Metric == "yes"
	}
	var specials, linear []string
	for _, u := range e.Units {
		switch {
		case u.Special == "yes":
			specials = append(specials, u.Code)
		case u.Arbitrary != "yes":
			linear = append(linear, u.Code)
		}
	}
	for _, b := range e.Bases {
		linear = append(linear, b.Code)
	}
	withPrefixes := func(codes []string, prefixes []string) []string {
		var out []string
		for _, c := range codes {
			out = append(out, c)
			if !metric[c] && !o.x.base[c] {
				continue
			}
			for _, p := range prefixes {
				out = append(out, p+c)
			}
		}
		return out
	}
	all := make([]string, 0, len(e.Prefixes))
	for _, p := range e.Prefixes {
		all = append(all, p.Code)
	}
	specialCodes := withPrefixes(specials, all)
	linearCodes := withPrefixes(linear, []string{"Y", "y"})

	var checked, border int
	check := func(name string, got float64, err error, w rangeWant) {
		t.Helper()
		checked++
		switch w.kind {
		case rangeAtBorder:
			border++
		case rangeOut:
			if !errors.Is(err, ucumerr.ErrOutOfRange) {
				t.Errorf("%s = %v, %v; want ErrOutOfRange", name, got, err)
			}
		case rangeOutside:
			if !errors.Is(err, ucumerr.ErrOutOfDomain) {
				t.Errorf("%s = %v, %v; want ErrOutOfDomain", name, got, err)
			}
		case rangeFits:
			if err != nil || !w.accepts(got) {
				t.Errorf("%s = %v, %v; want %v (rel %g, abs %g)", name, got, err, w.value, w.relTol, w.absTol)
			}
		}
	}
	canonical := func(v float64, code string) {
		t.Helper()
		w, ok := o.canonical(new(big.Rat).SetFloat64(v), code)
		if !ok {
			t.Errorf("oracle cannot evaluate %v %s", v, code)
			return
		}
		p, err := s.Canonical(v, code)
		check("Canonical("+fmtG(v)+" "+code+")", p.Value, err, w)
	}
	fixed := []float64{0, 1, -1, 1e-300, -1e-300, 1e300, -1e300}

	for _, code := range linearCodes {
		f, ok := o.factor(code, o.x.parse)
		if !ok {
			t.Errorf("oracle cannot evaluate %s", code)
			continue
		}
		lf := ratLog10(new(big.Rat).Abs(f))
		vs := fixed
		for _, d := range rangeGrid {
			for _, l := range []float64{rangeLog10Max + d, rangeLog10Zero + d} {
				if v := math.Pow(10, l-lf); v != 0 && !math.IsInf(v, 0) {
					vs = append(vs, v, -v)
				}
			}
		}
		for _, v := range vs {
			canonical(v, code)
		}
	}

	for _, code := range specialCodes {
		sp, ok := o.special(code)
		if !ok {
			t.Errorf("oracle cannot read special %s", code)
			continue
		}
		vs := append([]float64{}, fixed...)
		vs = append(vs, 5e-324, -5e-324, math.MaxFloat64, -math.MaxFloat64)
		if hasLog10Form(sp.fn.Name) {
			// v tal que el log10 del valor canónico caiga en la grilla.
			ref, _ := o.ref(sp)
			lr := ratLog10(ref)
			alpha, _ := sp.alpha.Float64()
			for _, d := range rangeGrid {
				for _, l := range []float64{rangeLog10Max + d, rangeLog10Zero + d} {
					if v, ok := inverseOfLog10(sp.fn.Name, l-lr); ok {
						vs = append(vs, v/alpha)
						if sp.fn.Name == "sqrt" {
							vs = append(vs, -v/alpha)
						}
					}
				}
			}
		}
		for _, v := range vs {
			canonical(v, code)
		}
		if !hasLog10Form(sp.fn.Name) {
			continue
		}
		// Ida y vuelta y entre unidades de la misma función (R1).
		for _, v := range vs {
			if math.Abs(v) == math.MaxFloat64 {
				continue // la ida y vuelta de MaxFloat64 cae justo en el límite
			}
			w, ok := o.toSpecial(new(big.Rat).SetFloat64(v), code, code)
			if ok {
				got, err := s.Convert(v, code, code)
				check("Convert("+fmtG(v)+" "+code+" to itself)", got, err, w)
			}
		}
		// Desde la unidad lineal de su referencia, con 0 y negativos (dominio).
		for _, v := range fixed {
			w, ok := o.toSpecial(new(big.Rat).SetFloat64(v), sp.fn.Unit, code)
			if !ok {
				t.Errorf("oracle cannot evaluate %v %s to %s", v, sp.fn.Unit, code)
				continue
			}
			got, err := s.Convert(v, sp.fn.Unit, code)
			check("Convert("+fmtG(v)+" "+sp.fn.Unit+" to "+code+")", got, err, w)
		}
	}
	// Entre escalas logarítmicas distintas de la misma dimensión.
	for _, pair := range [][2]string{{"B", "Np"}, {"Np", "B"}, {"B", "dB"}, {"dB", "cNp"}, {"B[V]", "B[mV]"}, {"B[uV]", "B[10.nV]"}, {"B[W]", "B[kW]"}, {"kB[kW]", "dB[W]"}} {
		for _, v := range []float64{-1e300, -340, -320, -1, -1e-17, 0, 1e-17, 1, 320, 1000, 1e300} {
			w, ok := o.toSpecial(new(big.Rat).SetFloat64(v), pair[0], pair[1])
			if !ok {
				t.Errorf("oracle cannot evaluate %v %s to %s", v, pair[0], pair[1])
				continue
			}
			got, err := s.Convert(v, pair[0], pair[1])
			check("Convert("+fmtG(v)+" "+pair[0]+" to "+pair[1]+")", got, err, w)
		}
	}
	t.Logf("%d special codes, %d linear codes, %d checks, %d at a border", len(specialCodes), len(linearCodes), checked, border)
	if border*100 > checked {
		t.Errorf("%d of %d points at a border: the grid does not test the rule", border, checked)
	}
}

// inverseOfLog10 es el argumento a de f⁻¹ tal que log10 |f⁻¹(a)| = l, para
// las funciones con forma log10.
func inverseOfLog10(fn string, l float64) (float64, bool) {
	if fn == "sqrt" {
		return math.Pow(10, l/2), true
	}
	g, ok := specInverseLog10[fn]
	if !ok {
		return 0, false
	}
	// specInverseLog10 es lineal en a: g(a) = g(1)·a.
	return l / g(1), true
}

func fmtG(v float64) string {
	return strings.TrimSpace(big.NewFloat(v).Text('g', 17))
}

// TestRangeCanonicalMatchesConvert (plan 003, tests C): Canonical(v, u) da
// error si y solo si Convert(v, u, su unidad canónica) lo da, con el mismo
// sentinel.
func TestRangeCanonicalMatchesConvert(t *testing.T) {
	s := newTestService(t)
	for _, code := range []string{"B", "dB", "Np", "B[SPL]", "B[V]", "[pH]", "bit_s", "Ym", "ym", "[m/s2/Hz^(1/2)]", "[p'diop]", "Cel", "MCel"} {
		for _, v := range []float64{-1e300, -1000, -330, -1, 0, 1, 330, 620, 1000, 1e300} {
			p, cerr := s.Canonical(v, code)
			can := p.Code
			if cerr != nil {
				q, err := s.Canonical(1, code)
				if err != nil {
					t.Fatalf("Canonical(1 %s): %v", code, err)
				}
				can = q.Code
			}
			if can == "" {
				can = "1"
			}
			_, verr := s.Convert(v, code, can)
			for _, sentinel := range []error{ucumerr.ErrOutOfRange, ucumerr.ErrOutOfDomain} {
				if errors.Is(cerr, sentinel) != errors.Is(verr, sentinel) {
					t.Errorf("%v %s: Canonical error %v, Convert to %s error %v", v, code, cerr, can, verr)
				}
			}
		}
	}
}

// TestRangeOracleIndependent (K12): el oráculo no llama al motor, salvo a sus
// parsers. Se verifica sobre el texto del archivo.
func TestRangeOracleIndependent(t *testing.T) {
	src, err := os.ReadFile("range_oracle_test.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"internal/special", "internal/decimal", "ourCanonical", "ourAtom",
		"xmlAtomValue", "xmlFactor", "specialCanonical", "specialLog10", ".Canonical(", ".Convert(",
		"canonicalize", "s.model", "toCanonical", "fromCanonical"} {
		if strings.Contains(string(src), banned) {
			t.Errorf("range_oracle_test.go uses %q: the oracle must not run the engine", banned)
		}
	}
}

// rangeFuzzSeeds mezcla unidades especiales de cada función con lineales de
// prefijo extremo.
var rangeFuzzSeeds = []string{"B", "dB", "Np", "B[SPL]", "B[V]", "[pH]", "bit_s", "[hp'_Q]",
	"[p'diop]", "%[slope]", "[m/s2/Hz^(1/2)]", "Cel", "[degF]", "Ym", "ym", "m", "1", "mol/l"}

// FuzzFiniteResults (plan 003, tests E): con entradas finitas, ninguna
// operación devuelve un valor no finito sin error, y todo error de valor es
// uno de los dos sentinels.
func FuzzFiniteResults(f *testing.F) {
	for _, a := range rangeFuzzSeeds {
		for _, b := range rangeFuzzSeeds {
			f.Add(a, b, 1000.0, -330.0)
		}
	}
	svc, err := New(nil, false)
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, code1, code2 string, v1, v2 float64) {
		if math.IsInf(v1, 0) || math.IsNaN(v1) || math.IsInf(v2, 0) || math.IsNaN(v2) {
			return
		}
		check := func(op string, got float64, err error) {
			t.Helper()
			if err == nil && (math.IsInf(got, 0) || math.IsNaN(got)) {
				t.Fatalf("%s = %v with no error", op, got)
			}
		}
		p, err := svc.Canonical(v1, code1)
		check(fmt.Sprintf("Canonical(%v, %q)", v1, code1), p.Value, err)
		r, err := svc.Convert(v1, code1, code2)
		check(fmt.Sprintf("Convert(%v, %q, %q)", v1, code1, code2), r, err)
		p, err = svc.Multiply(Pair{v1, code1}, Pair{v2, code2})
		check(fmt.Sprintf("Multiply(%v %q, %v %q)", v1, code1, v2, code2), p.Value, err)
		p, err = svc.Divide(Pair{v1, code1}, Pair{v2, code2})
		check(fmt.Sprintf("Divide(%v %q, %v %q)", v1, code1, v2, code2), p.Value, err)
	})
}
