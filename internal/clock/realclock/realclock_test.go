package realclock_test

import (
	"testing"
	"time"

	"github.com/dxinli/collect-code/internal/clock/realclock"
	"github.com/dxinli/collect-code/internal/interfaces"
)

var _ interfaces.Clock = realclock.New()

func TestRealClockNowAndSleep(t *testing.T) {
	c := realclock.New()
	start := c.Now()
	c.Sleep(time.Millisecond)
	if after := c.Now(); after.Before(start) {
		t.Fatalf("Now moved backwards: %s -> %s", start, after)
	}
}

func TestRealClockTickerStop(t *testing.T) {
	c := realclock.New()
	tk := c.NewTicker(time.Millisecond)
	select {
	case <-tk.C():
	case <-time.After(time.Second):
		t.Fatal("ticker produced no value")
	}
	tk.Stop()
	// Stop is idempotent and does not close the channel.
	tk.Stop()
}

func TestRealClockTimerStop(t *testing.T) {
	c := realclock.New()
	tm := c.NewTimer(5 * time.Millisecond)
	tm.Stop()
	tm.Stop() // idempotent
}
