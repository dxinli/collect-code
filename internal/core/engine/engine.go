// Package engine holds the kernel lifecycle component. It reads time only
// through the injected Clock, proving the time seam is wired; the scheduler
// is a peer component wired when drivers exist.
package engine

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/dxinli/collect-code/internal/interfaces"
)

// Engine is the kernel lifecycle component.
type Engine struct {
	clock     interfaces.Clock
	log       *slog.Logger
	startedAt time.Time
}

// New records the start time from clock. A nil logger falls back to
// slog.Default.
func New(clock interfaces.Clock, log *slog.Logger) (*Engine, error) {
	if clock == nil {
		return nil, fmt.Errorf("engine: clock is required")
	}
	if log == nil {
		log = slog.Default()
	}
	return &Engine{clock: clock, log: log, startedAt: clock.Now()}, nil
}

// StartedAt returns the time recorded at construction.
func (e *Engine) StartedAt() time.Time { return e.startedAt }

// Uptime is the elapsed time according to the injected clock.
func (e *Engine) Uptime() time.Duration { return e.clock.Now().Sub(e.startedAt) }

// Run blocks until ctx is done and logs the final uptime.
func (e *Engine) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	<-ctx.Done()
	e.log.Info("engine stopped", "component", "engine", "uptime", e.Uptime().String())
	return nil
}
