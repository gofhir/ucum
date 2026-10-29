package ucum_test

import (
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/gofhir/ucum/v4"
)

// Plan 006 (#61), test 2: la reproducción del issue, de punta a punta. Un
// código recortado de una entrada grande no la mantiene viva.
func TestCacheDoesNotRetainInput(t *testing.T) {
	const pad = 2 << 20
	svc, err := ucum.New()
	if err != nil {
		t.Fatal(err)
	}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for i := range 4 {
		big := strings.Repeat("0", pad) + "mg{r" + strconv.Itoa(i) + "}"
		if err := svc.Validate(big[pad:]); err != nil {
			t.Fatal(err)
		}
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(svc)
	// Sin la copia quedan los 4 strings, unos 8 MB.
	if kept := int64(after.HeapAlloc) - int64(before.HeapAlloc); kept > 1<<20 {
		t.Errorf("the cache keeps %d bytes after validating four 6-byte codes", kept)
	}
}

// Test 4: un código recortado da el mismo resultado que el mismo código
// escrito aparte.
func TestSlicedCodeSameResult(t *testing.T) {
	svc, err := ucum.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"mg/dL{x}", "kg.m/s2", "[in_i]", "10*3/uL", "m{a}.s-1"} {
		want, err := svc.Canonical(1, strings.Clone(code))
		if err != nil {
			t.Fatal(err)
		}
		big := strings.Repeat("0", 1<<10) + code + "000"
		got, err := svc.Canonical(1, big[1<<10:1<<10+len(code)])
		if err != nil {
			t.Fatal(err)
		}
		if got.Code != want.Code || got.Value != want.Value {
			t.Errorf("Canonical(1, %q) sliced = %+v, written apart = %+v", code, got, want)
		}
	}
}
