package ucum_test

import (
	"errors"
	"math/big"
	"strings"
	"testing"

	"github.com/gofhir/ucum/v4"
)

// Plan 005. UCUM v2.2 §6■5: «Curly braces must not be nested.» La primera }
// cierra la anotación, así que una { antes de ella abre una llave dentro de
// otra. Los esperados salen de esa regla y de la precedencia del plan (§1):
// gana el primer byte que rompe una regla, leyendo de izquierda a derecha.

// Tests 1 a 3: el error, su posición (la llave interior) y su mensaje.
func TestNestedBraces(t *testing.T) {
	svc, err := ucum.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		code   string
		offset int
	}{
		{"m{a{b}", 3},
		{"{{}", 1},
		{"m{a{b}}", 3}, // antes, el } que sobra en 6
		{"{a{}", 2},
		{"m{{a}}", 2},
		{"/{a{b}", 3},
		{"(m{a{b})", 4},
		// Test 2: la llave interior aparece antes que el final o que el byte
		// inválido, y gana.
		{"{{", 1},
		{"m{a{", 3},
		{"{a{b", 2},
		{"m{a{ b}", 3},
		{"m{a{é}", 3},
	} {
		err := svc.Validate(tt.code)
		var ve *ucum.ValidationError
		if !errors.As(err, &ve) {
			t.Errorf("Validate(%q) = %v, want a ValidationError", tt.code, err)
			continue
		}
		if ve.Code != tt.code || ve.Offset != tt.offset {
			t.Errorf("Validate(%q): Code %q, Offset %d; want %q, %d", tt.code, ve.Code, ve.Offset, tt.code, tt.offset)
		}
		if !strings.Contains(ve.Error(), "nested") {
			t.Errorf("Validate(%q) = %v, want the error to name the nesting", tt.code, err)
		}
	}
}

// Test 4: lo que no anida no cambia.
func TestNestedBracesUnchanged(t *testing.T) {
	svc, err := ucum.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"m{a}.s{b}", "m{a}.{b}", "{a}", "m{}", "{}"} {
		if err := svc.Validate(code); err != nil {
			t.Errorf("Validate(%q) = %v, want nil: no brace is inside another", code, err)
		}
	}
	// Dos anotaciones seguidas rompen otra regla (BNF de §10), en el mismo
	// lugar que antes.
	err = svc.Validate("m{a}{b}")
	var ve *ucum.ValidationError
	if !errors.As(err, &ve) || ve.Offset != 4 {
		t.Errorf("Validate(%q) = %v, want a ValidationError at 4", "m{a}{b}", err)
	}
}

// Test 5: las demás entradas rechazan el código.
func TestNestedBracesEverywhere(t *testing.T) {
	svc, err := ucum.NewExact()
	if err != nil {
		t.Fatal(err)
	}
	const bad = "m{a{b}"
	for _, pair := range [][2]string{{bad, "m"}, {"m", bad}, {bad, bad}} {
		_, err := svc.ConvertRat(big.NewRat(1, 1), pair[0], pair[1])
		var ce *ucum.ConversionError
		if !errors.As(err, &ce) {
			t.Errorf("ConvertRat(%q -> %q) = %v, want a ConversionError", pair[0], pair[1], err)
		}
	}
	one := ucum.Pair{Value: 1, Code: "m"}
	nested := ucum.Pair{Value: 1, Code: bad}
	if _, err := svc.Multiply(one, nested); err == nil {
		t.Errorf("Multiply(1 m, 1 %s) = nil error", bad)
	}
	if _, err := svc.Divide(nested, one); err == nil {
		t.Errorf("Divide(1 %s, 1 m) = nil error", bad)
	}
}

// Barrido generado: la llave interior en cada posición, detrás de átomos,
// prefijos, números, corchetes y operadores, sola y como destino de una
// conversión. El esperado es su índice (§6■5), calculado sin el lexer.
func TestNestedBracesSweep(t *testing.T) {
	svc, err := ucum.NewExact()
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, base := range []string{"", "m", "mg", "10*3", "[in_i]", "/", "m/", "m.", "(m", "kg.m/s2."} {
		for k := 0; k <= 6; k++ {
			code := base + "{" + strings.Repeat("a", k) + "{b}"
			want := len(base) + 1 + k
			err := svc.Validate(code)
			var ve *ucum.ValidationError
			if !errors.As(err, &ve) || ve.Offset != want || !strings.Contains(ve.Error(), "nested") {
				t.Errorf("Validate(%q) = %v, want nesting at %d", code, err, want)
			}
			if _, err := svc.ConvertRat(big.NewRat(1, 1), "m", code); err == nil {
				t.Errorf("ConvertRat(m -> %q) = nil error", code)
			}
			n++
		}
	}
	if n != 70 {
		t.Fatalf("%d codes, want 70", n)
	}
}
