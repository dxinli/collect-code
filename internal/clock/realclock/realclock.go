// Package realclock adapts the standard library's time functions to
// interfaces.Clock. It is the only runtime package allowed to call time.*
// directly; kernel and driver code receive a Clock instead.
package realclock

import (
	"time"

	"github.com/dxinli/collect-code/internal/interfaces"
)

// New returns the production clock.
func New() interfaces.Clock { return clock{} }

type clock struct{}

func (clock) Now() time.Time { return time.Now() }

func (clock) NewTicker(d time.Duration) interfaces.Ticker {
	return &ticker{t: time.NewTicker(d)}
}

func (clock) NewTimer(d time.Duration) interfaces.Timer {
	return &timer{t: time.NewTimer(d)}
}

func (clock) Sleep(d time.Duration) { time.Sleep(d) }

type ticker struct{ t *time.Ticker }

func (t *ticker) C() <-chan time.Time { return t.t.C }
func (t *ticker) Stop()               { t.t.Stop() }

type timer struct{ t *time.Timer }

func (t *timer) C() <-chan time.Time { return t.t.C }
func (t *timer) Stop()               { t.t.Stop() }
