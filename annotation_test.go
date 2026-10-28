package ucum_test

import (
	"errors"
	"math/big"
	"testing"

	"github.com/gofhir/ucum/v4"
)

// Plan 004. UCUM v2.2 §6■1: «The full range of characters 33–126 can be used
// within a pair of curly braces». El espacio es el 32. Los esperados salen de
// esa regla: un byte fuera del rango es inválido, y el error apunta a ese
// byte.

// Test 1: los bordes del rango. Basta un servicio: el sensible y el
// insensible a mayúsculas comparten el lexer.
func TestAnnotationCharacterRange(t *testing.T) {
	svc, err := ucum.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		code  string
		valid bool
	}{
		{"m{\x1f}", false}, // 31
		{"m{\x20}", false}, // 32, el espacio
		{"m{!}", true},     // 33, el primero admitido
		{"m{~}", true},     // 126, el último admitido
		{"m{\x7f}", false}, // 127
	} {
		err := svc.Validate(tt.code)
		if tt.valid && err != nil {
			t.Errorf("Validate(%q) = %v, want nil: §6■1 admits it", tt.code, err)
		}
		if !tt.valid && err == nil {
			t.Errorf("Validate(%q) = nil, want an error: §6■1 admits only 33–126", tt.code)
		}
	}
}

// Test 2: el espacio en distintas posiciones da ValidationError, y Offset es
// el índice del espacio en el código.
func TestAnnotationSpacePosition(t *testing.T) {
	svc, err := ucum.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		code   string
		offset int
	}{
		{"m{ a}", 2},                // al inicio
		{"m{a b}", 3},               // al medio
		{"m{a }", 3},                // al final
		{"{ }", 1},                  // anotación sola
		{"/{a b}", 3},               // detrás de la barra
		{"(m{a b})", 4},             // en un término agrupado
		{"%{Negative Control}", 10}, // R4 ucum-common
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
	}
}

// Test 3: la conversión rechaza el código en el origen, en el destino y en
// los dos.
func TestAnnotationSpaceConversion(t *testing.T) {
	svc, err := ucum.NewExact()
	if err != nil {
		t.Fatal(err)
	}
	const bad = "m{a b}"
	for _, pair := range [][2]string{{bad, "m"}, {"m", bad}, {bad, bad}} {
		_, err := svc.ConvertRat(big.NewRat(1, 1), pair[0], pair[1])
		var ce *ucum.ConversionError
		if !errors.As(err, &ce) {
			t.Errorf("ConvertRat(%q -> %q) = %v, want a ConversionError", pair[0], pair[1], err)
		}
	}
}
