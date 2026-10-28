package engine

import (
	"errors"
	"testing"

	"github.com/gofhir/ucum/v4/internal/ucumerr"
)

// Plan 004, test 6: el control de la clase java-accepts-space-in-annotation.
// La corrida de Java está sellada en manifest.json, así que la clase se prueba
// directo, con diferencias construidas: tiene que tomar las del espacio dentro
// de llaves (UCUM §6■1) y ninguna otra.
func TestJavaClassSpaceInAnnotation(t *testing.T) {
	var class *javaClass
	for i := range javaClasses {
		if javaClasses[i].Name == "java-accepts-space-in-annotation" {
			class = &javaClasses[i]
		}
	}
	if class == nil {
		t.Fatal("no class java-accepts-space-in-annotation")
	}
	invalid := func(code string) ourOut {
		err := ucumerr.Validation(code, ucumerr.At(0, "invalid"))
		return ourOut{Status: "err", Msg: err.Error(), Err: err}
	}
	ok := javaOut{Status: "ok"}
	for _, tt := range []struct {
		name string
		d    javaDiff
		want bool
	}{
		{"validate", javaDiff{javaCase{"c1", "validate", "fhir", []string{"{delivered dose}"}},
			ok, ourOut{Status: "invalid"}, "status"}, true},
		{"canonical", javaDiff{javaCase{"c2", "canonical", "fhir", []string{"1", "{delivered dose}"}},
			ok, invalid("{delivered dose}"), "status"}, true},
		{"convert target", javaDiff{javaCase{"c3", "convert", "x", []string{"1", "m", "m{a b}"}},
			ok, invalid("m{a b}"), "status"}, true},
		{"no space", javaDiff{javaCase{"c4", "validate", "x", []string{"m{ab}"}},
			ok, ourOut{Status: "invalid"}, "status"}, false},
		{"space outside braces", javaDiff{javaCase{"c5", "validate", "x", []string{"m /s{a}"}},
			ok, ourOut{Status: "invalid"}, "status"}, false},
		// Cada control negativo cumple todo lo demás, para que lo rechace solo la
		// condición que nombra.
		{"java rejects too", javaDiff{javaCase{"c6", "canonical", "x", []string{"1", "m{a b}"}},
			javaOut{Status: "invalid"}, invalid("m{a b}"), "status"}, false},
		{"not a status difference", javaDiff{javaCase{"c7", "canonical", "x", []string{"1", "m{a b}"}},
			ok, invalid("m{a b}"), "value"}, false},
		{"space after the annotation", javaDiff{javaCase{"c9", "validate", "x", []string{"m{a}.s b"}},
			ok, ourOut{Status: "invalid"}, "status"}, false},
		{"another error", javaDiff{javaCase{"c8", "canonical", "x", []string{"1", "m{a b}"}},
			ok, ourOut{Status: "err", Err: errors.New("overflow")}, "status"}, false},
	} {
		if got := class.match(nil, tt.d); got != tt.want {
			t.Errorf("%s: match = %t, want %t", tt.name, got, tt.want)
		}
	}
}

// Plan 005, test 6: el control de java-accepts-nested-annotation, con el mismo
// esquema, y la frontera con la clase del espacio: cada código lo toma la
// clase de la primera regla que rompe, leyendo de izquierda a derecha.
func TestJavaClassNestedAnnotation(t *testing.T) {
	find := func(name string) *javaClass {
		for i := range javaClasses {
			if javaClasses[i].Name == name {
				return &javaClasses[i]
			}
		}
		t.Fatalf("no class %s", name)
		return nil
	}
	nested := find("java-accepts-nested-annotation")
	space := find("java-accepts-space-in-annotation")
	invalid := func(code string) ourOut {
		err := ucumerr.Validation(code, ucumerr.At(0, "invalid"))
		return ourOut{Status: "err", Msg: err.Error(), Err: err}
	}
	ok := javaOut{Status: "ok"}
	validate := func(code string) javaDiff {
		return javaDiff{javaCase{"v", "validate", "x", []string{code}}, ok, ourOut{Status: "invalid"}, "status"}
	}
	for _, tt := range []struct {
		name          string
		d             javaDiff
		nested, space bool
	}{
		{"validate", validate("m{a{b}"), true, false},
		{"canonical", javaDiff{javaCase{"c", "canonical", "x", []string{"1", "m{a{b}"}},
			ok, invalid("m{a{b}"), "status"}, true, false},
		// Cada control negativo cumple todo lo demás.
		{"java rejects too", javaDiff{javaCase{"c", "canonical", "x", []string{"1", "m{a{b}"}},
			javaOut{Status: "invalid"}, invalid("m{a{b}"), "status"}, false, false},
		{"not a status difference", javaDiff{javaCase{"c", "canonical", "x", []string{"1", "m{a{b}"}},
			ok, invalid("m{a{b}"), "value"}, false, false},
		{"another error", javaDiff{javaCase{"c", "canonical", "x", []string{"1", "m{a{b}"}},
			ok, ourOut{Status: "err", Err: errors.New("overflow")}, "status"}, false, false},
		{"brace after a closed annotation", validate("m{a}.{b}"), false, false},
		{"brace outside braces", validate("m{a}{"), false, false},
		// La frontera entre las dos clases.
		{"nesting before a space", validate("m{a{ b}"), true, false},
		{"space before a nesting", validate("m{a b{c}"), false, true},
		{"convert target", javaDiff{javaCase{"c", "convert", "x", []string{"1", "m", "m{a{b}"}},
			ok, invalid("m{a{b}"), "status"}, true, false},
		// Una llave dentro de corchetes es parte del átomo (§5), no una anotación.
		{"brace inside brackets", validate("[x{]{a b}"), false, true},
	} {
		if got := nested.match(nil, tt.d); got != tt.nested {
			t.Errorf("%s: nested class match = %t, want %t", tt.name, got, tt.nested)
		}
		if got := space.match(nil, tt.d); got != tt.space {
			t.Errorf("%s: space class match = %t, want %t", tt.name, got, tt.space)
		}
	}
}
