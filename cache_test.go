package ucum

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
)

// The parse cache is fed by whatever codes a caller passes, and an annotation is
// free text: "mg/dL{lot17}" is valid UCUM and there are unboundedly many of them.
// An unbounded cache therefore grows without limit on input a service cannot
// refuse — 100,000 distinct valid codes cost 28 MB before this was bounded.

// TestCacheIsBounded checks that a flood of distinct codes does not grow the
// heap without limit.
func TestCacheIsBounded(t *testing.T) {
	if testing.Short() {
		t.Skip("allocates a hundred thousand codes")
	}
	svc := newTestService(t)

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	const codes = 100_000
	for i := 0; i < codes; i++ {
		if err := svc.Validate(fmt.Sprintf("mg/dL{lot%d}", i)); err != nil {
			t.Fatalf("Validate: %v", err)
		}
	}

	runtime.GC()
	runtime.ReadMemStats(&after)

	// Two generations of MaxCacheEntries entries each, at a few hundred bytes
	// per parsed term. 8 MB leaves generous headroom over that while still
	// catching a cache that kept all 100,000.
	const budget = 8 << 20
	if grew := heapGrowth(before, after); grew > budget {
		t.Errorf("heap grew by %d bytes after %d distinct codes, want at most %d: the cache is unbounded",
			grew, codes, budget)
	}

	// The service still works, and the eviction did not corrupt anything.
	if err := svc.Validate("mg/dL"); err != nil {
		t.Errorf("Validate after eviction: %v", err)
	}
}

// TestCacheKeepsTheWorkingSet checks the other half of the bargain: evicting
// under pressure must not throw away a code that is still in use, or a service
// under a flood would reparse its own hot set on every call.
func TestCacheKeepsTheWorkingSet(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatal(err)
	}
	impl, ok := svc.(*service)
	if !ok {
		t.Fatalf("New returned %T, want *service", svc)
	}

	const hot = "mg/dL"
	if err := svc.Validate(hot); err != nil {
		t.Fatal(err)
	}

	// Push through several generations, touching the hot code each time round.
	for i := 0; i < MaxCacheEntries*3; i++ {
		if err := svc.Validate(fmt.Sprintf("g/L{n%d}", i)); err != nil {
			t.Fatal(err)
		}
		if i%100 == 0 {
			if err := svc.Validate(hot); err != nil {
				t.Fatal(err)
			}
		}
	}

	if _, cached := impl.cache.load(hot); !cached {
		t.Errorf("the hot code %q was evicted despite being used throughout", hot)
	}
}

// TestCacheIsConcurrentlySafe exercises the rotation from several goroutines,
// where a torn generation swap would show up as a lost or duplicated entry.
// Run under -race, where this test earns its keep.
func TestCacheIsConcurrentlySafe(t *testing.T) {
	svc := newTestService(t)

	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < MaxCacheEntries; i++ {
				code := fmt.Sprintf("kg{g%d_%d}", g, i)
				if err := svc.Validate(code); err != nil {
					t.Errorf("Validate(%q): %v", code, err)
					return
				}
				if _, err := svc.Convert(1, "mg/dL", "g/L"); err != nil {
					t.Errorf("Convert: %v", err)
					return
				}
			}
		}(g)
	}
	wg.Wait()
}

// heapGrowth returns how much the live heap grew between two readings, or 0 if
// it shrank.
func heapGrowth(before, after runtime.MemStats) uint64 {
	if after.HeapAlloc < before.HeapAlloc {
		return 0
	}
	return after.HeapAlloc - before.HeapAlloc
}
