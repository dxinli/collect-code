package engine_test

import (
	"context"
	"testing"
	"time"

	"github.com/dxinli/collect-code/internal/core/engine"
	"github.com/dxinli/collect-code/internal/testutil/fakeclock"
)

var start = time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

func TestNewRejectsNilClock(t *testing.T) {
	if _, err := engine.New(nil, nil); err == nil {
		t.Fatal("New(nil) returned no error")
	}
}

func TestEngineUsesInjectedClock(t *testing.T) {
	clock := fakeclock.New(start)
	defer clock.Close()
	eng, err := engine.New(clock, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := eng.StartedAt(); !got.Equal(start) {
		t.Fatalf("StartedAt = %s, want %s", got, start)
	}
}

func TestEngineUptimeUsesInjectedClock(t *testing.T) {
	clock := fakeclock.New(start)
	defer clock.Close()
	eng, err := engine.New(clock, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	clock.Advance(90 * time.Second)
	if got := eng.Uptime(); got != 90*time.Second {
		t.Fatalf("Uptime = %s, want 90s", got)
	}
}

func TestEngineRunStopsOnCancel(t *testing.T) {
	clock := fakeclock.New(start)
	defer clock.Close()
	eng, err := engine.New(clock, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- eng.Run(ctx) }()

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after cancellation")
	}
}
