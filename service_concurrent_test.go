package ucum

import (
	"sync"
	"testing"
)

func TestServiceConcurrentValidate(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	codes := []string{"m", "kg", "mg/dL", "10*3/uL", "mm[Hg]", "%", "[lb_av]", "mol/L", "m/s2"}
	for i := 0; i < 100; i++ {
		for _, code := range codes {
			wg.Add(1)
			go func(c string) {
				defer wg.Done()
				_ = svc.Validate(c)
			}(code)
		}
	}
	wg.Wait()
}

func TestServiceConcurrentConvert(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	conversions := []struct{ from, to string }{
		{"m", "cm"}, {"km", "m"}, {"kg", "g"}, {"L", "mL"}, {"mg", "g"},
	}
	for i := 0; i < 100; i++ {
		for _, conv := range conversions {
			wg.Add(1)
			go func(f, t string) {
				defer wg.Done()
				_, _ = svc.Convert(1, f, t)
			}(conv.from, conv.to)
		}
	}
	wg.Wait()
}

func TestServiceConcurrentMixed(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(4)
		go func() { defer wg.Done(); _ = svc.Validate("mg/dL") }()
		go func() { defer wg.Done(); _, _ = svc.Convert(1, "m", "cm") }()
		go func() { defer wg.Done(); _, _ = svc.IsComparable("mg", "g") }()
		go func() { defer wg.Done(); _, _ = svc.Analyse("kg.m/s2") }()
	}
	wg.Wait()
}

func TestServiceCacheHit(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatal(err)
	}
	// Parse twice - second should hit cache
	if err := svc.Validate("mg/dL"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Validate("mg/dL"); err != nil {
		t.Fatal(err)
	}
}
