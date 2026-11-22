package memory_test

import (
	"testing"

	"github.com/dxinli/collect-code/internal/interfaces"
	"github.com/dxinli/collect-code/internal/store/memory"
	"github.com/dxinli/collect-code/internal/store/storetest"
)

type harness struct {
	store *memory.Store
}

func (h *harness) Store() interfaces.Store { return h.store }

func (h *harness) Reopen(t *testing.T) {
	t.Helper()
	if err := h.store.Close(); err != nil {
		t.Fatalf("Close before Reopen: %v", err)
	}
	h.store.Reopen()
}

func (h *harness) Cleanup(t *testing.T) {
	t.Helper()
	if err := h.store.Close(); err != nil {
		t.Fatalf("Cleanup Close: %v", err)
	}
}

func TestMemoryStoreContract(t *testing.T) {
	storetest.Run(t, func(*testing.T) storetest.Harness {
		return &harness{store: memory.New()}
	})
}
