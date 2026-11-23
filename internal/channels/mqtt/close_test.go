package mqtt

import (
	"sync"
	"testing"
)

// TestCloseConcurrentRegisterCleanup pins the Close/registerCleanup
// handshake: a late registerCleanup must not add to cleanupWG while Close is
// waiting on it. Run with -race.
func TestCloseConcurrentRegisterCleanup(t *testing.T) {
	opts := l2Options("mqtt://127.0.0.1:1", "race-close")
	for range 100 {
		c, err := newChannel(opts, nil)
		if err != nil {
			t.Fatalf("newChannel: %v", err)
		}

		var wg sync.WaitGroup
		wg.Go(func() { _ = c.Close() })
		for range 8 {
			wg.Go(func() { c.registerCleanup(func() {}) })
		}
		wg.Wait()
	}
}
