package engine

import (
	"strings"
	"sync"
	"testing"
	"unsafe"
)

// Plan 006 (#61). Una clave que es un trozo de un string más grande usa los
// mismos bytes, y una caché que la guarda mantiene vivo el string entero. Estos
// tests miran las claves mismas: ninguna puede apuntar dentro de la entrada
// de quien llama.

// aliases dice si los bytes de key están dentro de los de big.
func aliases(key, big string) bool {
	if key == "" {
		return false
	}
	k := uintptr(unsafe.Pointer(unsafe.StringData(key)))
	b := uintptr(unsafe.Pointer(unsafe.StringData(big)))
	return k >= b && k < b+uintptr(len(big))
}

// sliceOf devuelve un string grande y el trozo final que es code.
func sliceOf(pad byte, code string) (big, slice string) {
	big = strings.Repeat(string(pad), 64<<10) + code
	return big, big[64<<10:]
}

// keyIn devuelve la clave de m igual a want, si está.
func keyIn(t *testing.T, m *sync.Map, want string) (string, bool) {
	t.Helper()
	var found string
	ok := false
	m.Range(func(k, _ any) bool {
		s, isString := k.(string)
		if !isString {
			t.Fatalf("cache key of type %T", k)
		}
		if s == want {
			found, ok = s, true
			return false
		}
		return true
	})
	return found, ok
}

// Test 1: los tres caminos por los que una clave entra a una caché.
func TestCacheKeysDoNotAliasInput(t *testing.T) {
	t.Run("new code", func(t *testing.T) {
		svc := newTestService(t)
		big, code := sliceOf('0', "mg{new}")
		if err := svc.Validate(code); err != nil {
			t.Fatal(err)
		}
		key, ok := keyIn(t, svc.cache.young.Load(), "mg{new}")
		if !ok {
			t.Fatal("code not cached")
		}
		if aliases(key, big) {
			t.Error("the cache key shares the caller's string")
		}
	})

	t.Run("promotion", func(t *testing.T) {
		svc := newTestService(t)
		if err := svc.Validate(strings.Clone("mg{old}")); err != nil {
			t.Fatal(err)
		}
		// Una rotación pasa la entrada a la generación vieja sin guardar 4097
		// códigos.
		svc.cache.count.Store(MaxCacheEntries + 1)
		svc.cache.rotate()
		if _, ok := keyIn(t, svc.cache.old.Load(), "mg{old}"); !ok {
			t.Fatal("code not in the old generation after the rotation")
		}

		big, code := sliceOf('0', "mg{old}")
		if err := svc.Validate(code); err != nil {
			t.Fatal(err)
		}
		// La vieja guarda todavía la clave propia; la promovida está en la
		// joven.
		key, ok := keyIn(t, svc.cache.young.Load(), "mg{old}")
		if !ok {
			t.Fatal("code not promoted")
		}
		if aliases(key, big) {
			t.Error("the promoted key shares the caller's string")
		}
	})

	t.Run("property", func(t *testing.T) {
		svc := newTestService(t)
		big, property := sliceOf(' ', "velocity")
		if err := svc.ValidateInProperty("m/s", big); err != nil {
			t.Fatal(err)
		}
		key, ok := keyIn(t, &svc.propertyForms, property)
		if !ok {
			t.Fatal("property not memoized")
		}
		if aliases(key, big) {
			t.Error("the property key shares the caller's string")
		}
	})
}

// Test 3: un acierto en la caché no asigna; la copia va solo al guardar. El
// código es largo a propósito: el compilador deja en el stack la copia de un
// string corto que no escapa, y una copia en cada consulta no se vería.
func TestCacheHitDoesNotAllocate(t *testing.T) {
	svc := newTestService(t)
	code := "mg/dL{" + strings.Repeat("x", 64) + "}"
	if err := svc.Validate(code); err != nil {
		t.Fatal(err)
	}
	if n := testing.AllocsPerRun(100, func() { _ = svc.Validate(code) }); n != 0 {
		t.Errorf("a cache hit makes %v allocations, want 0", n)
	}
}
