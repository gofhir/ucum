package engine

import (
	"math"
	"testing"

	"github.com/gofhir/ucum/v4/internal/ucumerr"
)

// Plan 014 §2.5: el oráculo de Java (specForward de 100·tan) sabe el dominio de
// una escala de tangente, para que un caso futuro del corpus lo codifique.
// UCUM §44 ■4 (tabla 18): f_PD⁻¹(x) = arctan(x/100) devuelve solo (-π/2, π/2).
// El corpus de hoy no trae un ángulo fuera del dominio hacia una escala de
// tangente, así que el oráculo se prueba aquí con casos propios.
func TestJavaOracleKnowsTheTanDomain(t *testing.T) {
	cases, outs := loadJavaRun(t)
	s := newTestService(t)
	ci, err := New(nil, true)
	if err != nil {
		t.Fatal(err)
	}
	or := loadJavaOracle(t, newJavaContext(t, s, cases, outs))

	conv := func(v, from, to string) javaCase {
		return javaCase{ID: "tan-" + v + from + to, Op: "convert", Source: "plan-014", Args: []string{v, from, to}}
	}
	domain := ourOut{Status: "err", Err: ucumerr.ErrOutOfDomain, Msg: "outside the domain"}
	literal := func(rad float64) ourOut { return ourOut{Status: "ok", Float: 100 * math.Tan(rad)} }

	// Fuera del dominio: el valor literal de tan se rechaza, y ErrOutOfDomain se acepta.
	for _, c := range []javaCase{
		conv("2", "rad", "[p'diop]"), conv("-2", "rad", "%[slope]"), conv("90", "deg", "[p'diop]"),
		conv("100", "gon", "%[slope]"), conv("5400", "'", "[p'diop]"), conv("0.002", "krad", "[p'diop]"),
	} {
		if msg := or.check(c, literal(2)); msg == "" {
			t.Errorf("%v: the oracle accepts the literal tan outside the domain", c.Args)
		}
		if msg := or.check(c, domain); msg != "" {
			t.Errorf("%v: the oracle rejects ErrOutOfDomain: %s", c.Args, msg)
		}
		// Y la librería misma, con el servicio real.
		if msg := or.check(c, ourAnswer(s, ci, c)); msg != "" {
			t.Errorf("%v: our answer disagrees with the oracle: %s", c.Args, msg)
		}
	}

	// Dentro del dominio: el valor de tan se acepta, y un error de dominio no.
	for _, tc := range []struct {
		c   javaCase
		rad float64
	}{
		{conv("1", "rad", "[p'diop]"), 1}, {conv("45", "deg", "%[slope]"), math.Pi / 4},
		{conv("1.6", "mrad", "[p'diop]"), 1.6e-3}, {conv("-1.5", "rad", "%[slope]"), -1.5},
	} {
		if msg := or.check(tc.c, literal(tc.rad)); msg != "" {
			t.Errorf("%v: the oracle rejects the value inside the domain: %s", tc.c.Args, msg)
		}
		if msg := or.check(tc.c, domain); msg == "" {
			t.Errorf("%v: the oracle accepts ErrOutOfDomain inside the domain", tc.c.Args)
		}
		if msg := or.check(tc.c, ourAnswer(s, ci, tc.c)); msg != "" {
			t.Errorf("%v: our answer disagrees with the oracle: %s", tc.c.Args, msg)
		}
	}
}
