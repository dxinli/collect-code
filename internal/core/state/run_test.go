// Covers: REQ-QUALITY-04
package state_test

import (
	"context"
	"testing"
	"time"
)

func TestRunSweepsAndStops(t *testing.T) {
	h := newHarness(t, singlePointConfig(time.Hour, 60*time.Second, 1))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- h.tr.Run(ctx) }()

	// Wait until Run has armed its ticker before moving the fake clock.
	waitFor(t, func() bool { return h.clock.NumActive() > 0 })
	h.advance(60*time.Second + time.Nanosecond)
	waitFor(t, func() bool {
		calls := h.rec.all()
		return len(calls) > 0 && !h.snapshot().Online
	})

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after cancellation")
	}

	if err := h.tr.Run(context.Background()); err == nil {
		t.Fatal("second Run was accepted")
	}
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition was not met before the deadline")
}
