package storetest

import (
	"sync"
	"testing"

	"github.com/dxinli/collect-code/internal/interfaces"
)

type fakeHarness struct {
	store     interfaces.Store
	onCleanup func(*testing.T)
}

func (f *fakeHarness) Store() interfaces.Store { return f.store }
func (f *fakeHarness) Reopen(*testing.T)       {}
func (f *fakeHarness) Cleanup(t *testing.T) {
	if f.onCleanup != nil {
		f.onCleanup(t)
	}
}

func TestRunCleansUpEachSubtest(t *testing.T) {
	var mu sync.Mutex
	var created, cleaned, ran int

	cases := []subtest{
		{"first", func(*testing.T, Harness) { mu.Lock(); ran++; mu.Unlock() }},
		{"second", func(*testing.T, Harness) { mu.Lock(); ran++; mu.Unlock() }},
		{"third", func(*testing.T, Harness) { mu.Lock(); ran++; mu.Unlock() }},
	}
	factory := func(t *testing.T) Harness {
		mu.Lock()
		created++
		mu.Unlock()
		return &fakeHarness{onCleanup: func(*testing.T) {
			mu.Lock()
			cleaned++
			mu.Unlock()
		}}
	}

	run(t, factory, cases)

	mu.Lock()
	defer mu.Unlock()
	if created != len(cases) {
		t.Fatalf("factory calls = %d, want %d", created, len(cases))
	}
	if ran != len(cases) {
		t.Fatalf("subtests run = %d, want %d", ran, len(cases))
	}
	if cleaned != len(cases) {
		t.Fatalf("cleanup calls = %d, want %d (one per subtest)", cleaned, len(cases))
	}
}
