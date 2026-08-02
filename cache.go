package ucum

import (
	"sync"
	"sync/atomic"
)

// MaxCacheEntries is how many parsed codes a service keeps per cache generation.
//
// A service caches the codes it is given so that repeating one is cheap, and
// what it is given is not under its control: an annotation is free text, so
// "mg/dL{lot17}" is a valid code and there are unboundedly many of them. An
// unbounded cache therefore grows without limit on input the service cannot
// refuse. The bound is generous — a deployment sees tens of distinct codes, not
// thousands — and eviction costs a reparse, not an error.
const MaxCacheEntries = 4096

// termCache is a bounded, concurrency-safe cache of parsed codes.
//
// It keeps two generations. Lookups read the young one first and fall back to
// the old; a hit in the old generation is promoted, so a code that stays in use
// stays cached however much unique traffic passes through. When the young
// generation fills, it becomes the old one and a fresh generation starts, which
// drops at most the entries that were not touched in a full cycle.
//
// Two generations rather than an LRU because the hot path is the lookup, and an
// LRU has to write on every read to maintain its order. Here a read is a
// sync.Map load, with no lock and no write at all in the common case.
type termCache struct {
	young atomic.Pointer[sync.Map]
	old   atomic.Pointer[sync.Map]

	// count is the number of stores into the current young generation. It only
	// ever over-counts — two goroutines can store the same key — which rotates
	// slightly early and is harmless.
	count atomic.Int64

	// rotating serializes generation swaps. It is never held during a lookup.
	rotating sync.Mutex
}

func newTermCache() *termCache {
	c := &termCache{}
	c.young.Store(&sync.Map{})
	c.old.Store(&sync.Map{})
	return c
}

// load returns the cached term for a code, promoting it out of the old
// generation if that is where it was found.
func (c *termCache) load(code string) (*term, bool) {
	if v, ok := c.young.Load().Load(code); ok {
		t, ok := v.(*term)
		return t, ok
	}
	v, ok := c.old.Load().Load(code)
	if !ok {
		return nil, false
	}
	t, ok := v.(*term)
	if !ok {
		return nil, false
	}
	// Still in use: carry it into the current generation so it survives the next
	// rotation.
	c.store(code, t)
	return t, true
}

// store caches a parsed term, rotating the generations if the young one is full.
func (c *termCache) store(code string, t *term) {
	if c.count.Add(1) > MaxCacheEntries {
		c.rotate()
	}
	c.young.Load().Store(code, t)
}

// rotate retires the young generation and starts a new one. A second goroutine
// arriving here while the first is rotating finds the count already reset and
// returns without rotating again.
func (c *termCache) rotate() {
	c.rotating.Lock()
	defer c.rotating.Unlock()

	if c.count.Load() <= MaxCacheEntries {
		return
	}
	c.old.Store(c.young.Load())
	c.young.Store(&sync.Map{})
	c.count.Store(0)
}
