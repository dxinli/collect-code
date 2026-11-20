package fakeclock_test

import (
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/dxinli/collect-code/internal/testutil/fakeclock"
)

var start = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func newTicker(t *testing.T, c *fakeclock.Clock, d time.Duration) *fakeclock.Ticker {
	t.Helper()
	tk, ok := c.NewTicker(d).(*fakeclock.Ticker)
	if !ok {
		t.Fatalf("NewTicker returned %T, want *fakeclock.Ticker", c.NewTicker(d))
	}
	return tk
}

func newTimer(t *testing.T, c *fakeclock.Clock, d time.Duration) *fakeclock.Timer {
	t.Helper()
	tm, ok := c.NewTimer(d).(*fakeclock.Timer)
	if !ok {
		t.Fatalf("NewTimer returned %T, want *fakeclock.Timer", c.NewTimer(d))
	}
	return tm
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met before timeout")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestFakeClockNowAndAdvance(t *testing.T) {
	c := fakeclock.New(start)
	if got := c.Now(); !got.Equal(start) {
		t.Fatalf("Now = %s, want %s", got, start)
	}
	c.Advance(5 * time.Second)
	if got, want := c.Now(), start.Add(5*time.Second); !got.Equal(want) {
		t.Fatalf("Now = %s, want %s", got, want)
	}
}

func TestFakeClockAdvanceNegativePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Advance(-1ns) did not panic")
		}
	}()
	fakeclock.New(start).Advance(-time.Nanosecond)
}

func TestFakeClockTickerPeriodAndDrop(t *testing.T) {
	c := fakeclock.New(start)
	tk := newTicker(t, c, 60*time.Second)

	c.Advance(60 * time.Second)
	select {
	case got := <-tk.C():
		if want := start.Add(60 * time.Second); !got.Equal(want) {
			t.Fatalf("first tick = %s, want %s", got, want)
		}
	default:
		t.Fatal("no tick after one period")
	}

	// Three more periods without reading: the buffer keeps the first tick and
	// the other two are dropped.
	c.Advance(180 * time.Second)
	select {
	case got := <-tk.C():
		if want := start.Add(120 * time.Second); !got.Equal(want) {
			t.Fatalf("buffered tick = %s, want %s", got, want)
		}
	default:
		t.Fatal("tick was not buffered")
	}
	if got := tk.DroppedTicks(); got != 2 {
		t.Fatalf("DroppedTicks = %d, want 2", got)
	}
}

func TestFakeClockTickerStop(t *testing.T) {
	c := fakeclock.New(start)
	tk := newTicker(t, c, 60*time.Second)
	c.Advance(60 * time.Second)
	<-tk.C()

	tk.Stop()
	tk.Stop() // idempotent
	c.Advance(600 * time.Second)
	select {
	case v := <-tk.C():
		t.Fatalf("tick after Stop: %s", v)
	default:
	}
	if got := tk.DroppedTicks(); got != 0 {
		t.Fatalf("DroppedTicks = %d, want 0", got)
	}
}

func TestFakeClockStopDoesNotCloseChannel(t *testing.T) {
	c := fakeclock.New(start)

	tk := newTicker(t, c, 60*time.Second)
	tk.Stop()
	select {
	case _, ok := <-tk.C():
		if !ok {
			t.Fatal("Stop closed the ticker channel")
		}
		t.Fatal("ticker delivered after Stop")
	default:
	}

	tm := c.NewTimer(60 * time.Second)
	tm.Stop()
	select {
	case _, ok := <-tm.C():
		if !ok {
			t.Fatal("Stop closed the timer channel")
		}
		t.Fatal("timer delivered after Stop")
	default:
	}
}

func TestFakeClockTickerNonPositivePanics(t *testing.T) {
	for _, d := range []time.Duration{0, -time.Second} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("NewTicker(%s) did not panic", d)
				}
			}()
			fakeclock.New(start).NewTicker(d)
		}()
	}
}

func TestFakeClockTimerFiresOnce(t *testing.T) {
	c := fakeclock.New(start)
	tm := newTimer(t, c, 5*time.Second)

	c.Advance(4999 * time.Millisecond)
	select {
	case v := <-tm.C():
		t.Fatalf("timer fired early: %s", v)
	default:
	}
	if tm.Fired() {
		t.Fatal("Fired before the deadline")
	}

	c.Advance(time.Millisecond)
	select {
	case got := <-tm.C():
		if want := start.Add(5 * time.Second); !got.Equal(want) {
			t.Fatalf("fired at %s, want %s", got, want)
		}
	default:
		t.Fatal("timer did not fire")
	}
	if !tm.Fired() {
		t.Fatal("Fired = false after firing")
	}

	c.Advance(time.Hour)
	select {
	case v := <-tm.C():
		t.Fatalf("timer fired twice: %s", v)
	default:
	}
}

func TestFakeClockTimerNonPositiveFiresImmediately(t *testing.T) {
	for _, d := range []time.Duration{0, -time.Second} {
		c := fakeclock.New(start)
		tm, ok := c.NewTimer(d).(*fakeclock.Timer)
		if !ok {
			t.Fatalf("NewTimer(%s) returned %T", d, c.NewTimer(d))
		}
		select {
		case got := <-tm.C():
			if !got.Equal(start) {
				t.Fatalf("NewTimer(%s) fired at %s, want %s", d, got, start)
			}
		default:
			t.Fatalf("NewTimer(%s) did not fire immediately", d)
		}
		if !tm.Fired() {
			t.Fatalf("NewTimer(%s) Fired = false", d)
		}
	}
}

func TestFakeClockTimerStop(t *testing.T) {
	c := fakeclock.New(start)
	tm := newTimer(t, c, 5*time.Second)
	tm.Stop()
	tm.Stop() // idempotent
	c.Advance(time.Hour)
	select {
	case v := <-tm.C():
		t.Fatalf("stopped timer fired: %s", v)
	default:
	}
	if tm.Fired() {
		t.Fatal("stopped timer reports Fired")
	}
}

func TestFakeClockAdvanceOrdering(t *testing.T) {
	c := fakeclock.New(start)
	tm := newTimer(t, c, 5*time.Second)
	tk := newTicker(t, c, 5*time.Second)
	_ = tm

	c.Advance(5 * time.Second)
	order := c.FiredOrder()
	if len(order) < 2 {
		t.Fatalf("FiredOrder = %v, want at least 2 entries", order)
	}
	if order[0].Kind != fakeclock.KindTimer || order[1].Kind != fakeclock.KindTicker {
		t.Fatalf("firing order = %v, want timer then ticker", order[:2])
	}
	if order[0].Seq > order[1].Seq {
		t.Fatalf("same-deadline events did not fire in registration order: %v", order[:2])
	}
	if !order[0].Deadline.Equal(start.Add(5 * time.Second)) {
		t.Fatalf("first deadline = %s, want %s", order[0].Deadline, start.Add(5*time.Second))
	}
	_ = tk
}

func TestFakeClockSleepUnblocksOnAdvance(t *testing.T) {
	c := fakeclock.New(start)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.Sleep(10 * time.Second)
	}()

	waitFor(t, func() bool { return c.NumSleepers() == 1 })
	c.Advance(9 * time.Second)

	blocked := make(chan struct{})
	go func() { wg.Wait(); close(blocked) }()
	select {
	case <-blocked:
		t.Fatal("Sleep returned before the deadline")
	case <-time.After(10 * time.Millisecond):
	}

	c.Advance(time.Second)
	select {
	case <-blocked:
	case <-time.After(2 * time.Second):
		t.Fatal("Sleep did not return at the deadline")
	}
	if got, want := c.Now(), start.Add(10*time.Second); !got.Equal(want) {
		t.Fatalf("Now = %s, want %s", got, want)
	}
}

func TestFakeClockSleepNonPositiveReturns(t *testing.T) {
	c := fakeclock.New(start)
	c.Sleep(0)
	c.Sleep(-time.Second)
	if got := c.NumSleepers(); got != 0 {
		t.Fatalf("NumSleepers = %d, want 0", got)
	}
}

func TestFakeClockCloseReleasesSleepers(t *testing.T) {
	c := fakeclock.New(start)
	const n = 5
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			c.Sleep(time.Hour)
		}()
	}
	waitFor(t, func() bool { return c.NumSleepers() == n })

	c.Close()
	c.Close() // idempotent

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not release sleepers")
	}
	if got := c.NumSleepers(); got != 0 {
		t.Fatalf("NumSleepers = %d, want 0", got)
	}
	before := c.Now()
	c.Advance(time.Hour)
	if after := c.Now(); !after.Equal(before) {
		t.Fatalf("Advance after Close moved time from %s to %s", before, after)
	}
}

func TestFakeClockNoGoroutineLeakAfterClose(t *testing.T) {
	baseline := runtime.NumGoroutine()
	c := fakeclock.New(start)
	const n = 100
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			c.Sleep(time.Hour)
		}()
	}
	waitFor(t, func() bool { return c.NumSleepers() == n })

	c.Close()
	wg.Wait()
	waitFor(t, func() bool { return runtime.NumGoroutine() <= baseline+2 })
}
