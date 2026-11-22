package bbolt_test

import (
	"path/filepath"
	"testing"

	"github.com/dxinli/collect-code/internal/interfaces"
	storebbolt "github.com/dxinli/collect-code/internal/store/bbolt"
	"github.com/dxinli/collect-code/internal/store/storetest"
)

type bboltHarness struct {
	path  string
	store *storebbolt.Store
}

func (h *bboltHarness) Store() interfaces.Store { return h.store }

func (h *bboltHarness) Reopen(t *testing.T) {
	t.Helper()
	if err := h.store.Close(); err != nil {
		t.Fatalf("Close before Reopen: %v", err)
	}
	h.store = nil
	st, err := storebbolt.Open(h.path)
	if err != nil {
		t.Fatalf("reopen %s: %v", h.path, err)
	}
	h.store = st
}

func (h *bboltHarness) Cleanup(t *testing.T) {
	t.Helper()
	if h.store == nil {
		return
	}
	if err := h.store.Close(); err != nil {
		t.Fatalf("Cleanup Close: %v", err)
	}
	h.store = nil
}

func TestBboltStoreContract(t *testing.T) {
	storetest.Run(t, func(t *testing.T) storetest.Harness {
		path := filepath.Join(t.TempDir(), "state.db")
		st, err := storebbolt.Open(path)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		return &bboltHarness{path: path, store: st}
	})
}
