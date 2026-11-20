// Package fakeclock provides a deterministic interfaces.Clock for tests.
//
// The fake never starts goroutines: Sleep blocks the caller on a channel that
// Advance or Close releases. All timers, tickers and sleepers fire in
// (deadline, registration order) order, and Advance records each firing in
// FiredOrder so tests can assert ordering without relying on goroutine
// scheduling.
//
// # Documented deviations from Go 1.27
//
// The standard library delivers ticker and timer values on unbuffered
// channels (cap 0). This fake uses capacity 1 so a single Advance can deliver
// a tick without a concurrent receiver, and drops ticks when that buffer is
// full. Timer and ticker channels are never closed by Stop, and a stopped
// timer may still hold a value it already delivered.
//
// # Step, do not jump
//
// Advance holds the clock lock while firing events, so a component that
// registers its next timer from inside a tick waits until Advance returns. A
// test must therefore advance one period at a time and wait for the component
// to become idle before the next step; a single Advance(24*time.Hour) will not
// reproduce 1440 scheduler ticks.
package fakeclock

import (
	"sync"
	"time"

	"github.com/dxinli/collect-code/internal/interfaces"
)

// Kind identifies the source of a fired event.
type Kind string

const (
	KindSleep  Kind = "sleep"
	KindTimer  Kind = "timer"
	KindTicker Kind = "ticker"
)

// Event is one entry in the firing trace.
type Event struct {
	Kind     Kind
	Seq      uint64
	Deadline time.Time
}

type entry struct {
	seq      uint64
	kind     Kind
	deadline time.Time
	period   time.Duration
	active   bool
	fire     func(time.Time)
}

// Clock is a deterministic interfaces.Clock. Call Close when the test ends.
type Clock struct {
	mu     sync.Mutex
	now    time.Time
	seq    uint64
	closed bool
	active map[uint64]*entry
	fired  []Event
}

// New returns a clock frozen at start.
func New(start time.Time) *Clock {
	return &Clock{now: start, active: map[uint64]*entry{}}
}

// Now returns the current fake time.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves time forward, firing every event due up to the new time.
// It panics on a negative duration, mirroring the rule that time never moves
// backwards.
func (c *Clock) Advance(d time.Duration) {
	if d < 0 {
		panic("fakeclock: cannot advance backwards")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	target := c.now.Add(d)
	for {
		next := c.earliest(target)
		if next == nil {
			break
		}
		c.now = next.deadline
		c.fired = append(c.fired, Event{Kind: next.kind, Seq: next.seq, Deadline: next.deadline})
		next.fire(next.deadline)
		if next.kind == KindTicker && next.active {
			next.deadline = next.deadline.Add(next.period)
			continue
		}
		next.active = false
		delete(c.active, next.seq)
	}
	c.now = target
}

// Sleep blocks until the fake clock reaches now+d, or until Close.
func (c *Clock) Sleep(d time.Duration) {
	if d <= 0 {
		return
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	done := make(chan struct{})
	c.register(KindSleep, d, func(time.Time) { close(done) })
	c.mu.Unlock()
	<-done
}

// NewTimer returns a timer that fires once after d.
func (c *Clock) NewTimer(d time.Duration) interfaces.Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	tm := &Timer{clock: c, ch: make(chan time.Time, 1)}
	if c.closed {
		return tm
	}
	if d <= 0 {
		c.seq++
		tm.seq = c.seq
		tm.fired = true
		tm.ch <- c.now
		c.fired = append(c.fired, Event{Kind: KindTimer, Seq: tm.seq, Deadline: c.now})
		return tm
	}
	e := c.register(KindTimer, d, func(at time.Time) {
		tm.ch <- at
		tm.fired = true
	})
	tm.seq = e.seq
	return tm
}

// NewTicker returns a ticker firing every d. A non-positive d panics, as
// time.NewTicker does.
func (c *Clock) NewTicker(d time.Duration) interfaces.Ticker {
	if d <= 0 {
		panic("fakeclock: non-positive interval for NewTicker")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	tk := &Ticker{clock: c, ch: make(chan time.Time, 1)}
	if c.closed {
		return tk
	}
	e := c.register(KindTicker, d, func(at time.Time) {
		select {
		case tk.ch <- at:
		default:
			tk.dropped++
		}
	})
	e.period = d
	tk.seq = e.seq
	return tk
}

// Close releases every sleeper, stops every ticker and timer, and makes
// later Advance calls no-ops. It is idempotent.
func (c *Clock) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	for seq, e := range c.active {
		if e.kind == KindSleep {
			e.fire(c.now)
		}
		delete(c.active, seq)
	}
}

// NumSleepers reports how many goroutines are currently blocked in Sleep.
func (c *Clock) NumSleepers() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, e := range c.active {
		if e.kind == KindSleep {
			n++
		}
	}
	return n
}

// NumActive reports active tickers, unfired timers and sleepers.
func (c *Clock) NumActive() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.active)
}

// FiredOrder returns a copy of the firing trace in actual firing order.
func (c *Clock) FiredOrder() []Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Event(nil), c.fired...)
}

func (c *Clock) register(kind Kind, d time.Duration, fire func(time.Time)) *entry {
	c.seq++
	e := &entry{seq: c.seq, kind: kind, deadline: c.now.Add(d), active: true, fire: fire}
	c.active[e.seq] = e
	return e
}

func (c *Clock) earliest(target time.Time) *entry {
	var best *entry
	for _, e := range c.active {
		if e.deadline.After(target) {
			continue
		}
		if best == nil || e.deadline.Before(best.deadline) ||
			(e.deadline.Equal(best.deadline) && e.seq < best.seq) {
			best = e
		}
	}
	return best
}

func (c *Clock) stop(seq uint64) {
	if seq == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.active[seq]; ok {
		e.active = false
		delete(c.active, seq)
	}
}

// Ticker is the fake ticker.
type Ticker struct {
	clock   *Clock
	seq     uint64
	ch      chan time.Time
	dropped uint64
}

// C returns the tick channel. It is never closed.
func (t *Ticker) C() <-chan time.Time { return t.ch }

// Stop stops the ticker. It is idempotent.
func (t *Ticker) Stop() { t.clock.stop(t.seq) }

// DroppedTicks reports ticks discarded because the receiver was not ready.
func (t *Ticker) DroppedTicks() uint64 {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	return t.dropped
}

// Timer is the fake timer.
type Timer struct {
	clock *Clock
	seq   uint64
	ch    chan time.Time
	fired bool
}

// C returns the timer channel. It is never closed.
func (t *Timer) C() <-chan time.Time { return t.ch }

// Stop stops the timer. It is idempotent and does not clear a delivered value.
func (t *Timer) Stop() { t.clock.stop(t.seq) }

// Fired reports whether the timer has fired.
func (t *Timer) Fired() bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	return t.fired
}
