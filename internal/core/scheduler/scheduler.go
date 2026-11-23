package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"time"

	"github.com/dxinli/collect-code/internal/interfaces"
	"github.com/dxinli/collect-code/internal/logging"
)

const (
	minTickPeriod  = time.Second
	deadlineSlack  = time.Nanosecond
	timeLayout     = time.RFC3339Nano
	reasonDeadline = "deadline_exceeded"
)

// Device is one addressable device on a link.
type Device struct {
	ID     string
	Period time.Duration // effective poll interval
	Points []string      // readable keys, used to synthesize bad samples on timeout
}

// Link is one southbound link and the devices reached through it.
type Link struct {
	ID           string
	Driver       interfaces.Driver
	Devices      []Device
	ReadDeadline time.Duration
}

// Config is the scheduler plan. Anchor is optional; the zero value means the
// clock time at New, and a reload can pass the previous anchor to keep phase.
type Config struct {
	Links  []Link
	Anchor time.Time
}

// ReadResult is one device read, including a synthesized timeout result.
type ReadResult struct {
	Link, Device          string
	Planned               time.Time
	Points                []string
	Samples               []interfaces.Sample
	Outcome               interfaces.Outcome
	TimedOut              bool
	StartedAt, FinishedAt time.Time
}

// TickInfo is one executed link tick.
type TickInfo struct {
	Link      string
	Planned   time.Time
	Index     uint64 // (Planned - anchor) / tick period, starting at 1
	Missed    uint64 // ticks skipped immediately before this one
	ReadCount int
	At        time.Time
}

// Observer receives read and tick observations. It must not block and must be
// safe for concurrent calls because links run independently. It may be nil.
type Observer interface {
	ObserveRead(ReadResult)
	ObserveTick(TickInfo)
}

// LinkStats is a snapshot of one link's scheduler counters.
type LinkStats struct {
	ID           string
	Pollable     bool
	TickPeriod   time.Duration
	Devices      int
	Ticks        uint64
	MissedTicks  uint64
	Reads        uint64
	ReadTimeouts uint64
	ReadFailures uint64
	LastTickAt   time.Time
	LastError    string
}

type linkPlan struct {
	id           string
	driver       interfaces.Driver
	devices      []Device
	readDeadline time.Duration
	tickPeriod   time.Duration
	pollable     bool
	statsIndex   int
}

// Scheduler runs phase-locked ticks for every pollable link.
type Scheduler struct {
	clock  interfaces.Clock
	log    *slog.Logger
	obs    Observer
	anchor time.Time
	links  []*linkPlan
	stats  []LinkStats

	started chan struct{}

	runMu sync.Mutex
	ran   bool

	mu sync.Mutex
}

// New validates cfg and captures the phase anchor. A nil logger falls back to
// slog.Default.
func New(clock interfaces.Clock, log *slog.Logger, cfg Config, obs Observer) (*Scheduler, error) {
	if isNil(clock) {
		return nil, errors.New("scheduler: clock is required")
	}
	if log == nil {
		log = slog.Default()
	}
	s := &Scheduler{clock: clock, log: log, obs: obs, started: make(chan struct{})}
	if cfg.Anchor.IsZero() {
		s.anchor = clock.Now()
	} else {
		s.anchor = cfg.Anchor
	}
	seenLinks := make(map[string]bool, len(cfg.Links))
	for i, link := range cfg.Links {
		if link.ID == "" {
			return nil, fmt.Errorf("scheduler: link %d has empty id", i)
		}
		if seenLinks[link.ID] {
			return nil, fmt.Errorf("scheduler: duplicate link id %q", link.ID)
		}
		seenLinks[link.ID] = true
		if isNil(link.Driver) {
			return nil, fmt.Errorf("scheduler: link %q driver is required", link.ID)
		}
		lp := &linkPlan{
			id:           link.ID,
			driver:       link.Driver,
			devices:      copyDevices(link.Devices),
			readDeadline: link.ReadDeadline,
			pollable:     link.Driver.Capabilities().Pollable,
			statsIndex:   len(s.links),
		}
		if lp.pollable {
			if len(lp.devices) == 0 {
				return nil, fmt.Errorf("scheduler: pollable link %q has no devices", link.ID)
			}
			if lp.readDeadline <= 0 {
				return nil, fmt.Errorf("scheduler: link %q read deadline must be positive", link.ID)
			}
			if err := validateDevices(lp.devices); err != nil {
				return nil, fmt.Errorf("scheduler: link %q: %w", link.ID, err)
			}
			lp.tickPeriod = tickPeriod(lp.devices)
			if lp.tickPeriod < minTickPeriod {
				return nil, fmt.Errorf("scheduler: link %q tick period %s is below %s", link.ID, lp.tickPeriod, minTickPeriod)
			}
		}
		s.links = append(s.links, lp)
		s.stats = append(s.stats, LinkStats{
			ID:         lp.id,
			Pollable:   lp.pollable,
			TickPeriod: lp.tickPeriod,
			Devices:    len(lp.devices),
		})
	}
	return s, nil
}

func validateDevices(devices []Device) error {
	seen := make(map[string]bool, len(devices))
	for _, d := range devices {
		if d.ID == "" {
			return errors.New("device id is required")
		}
		if seen[d.ID] {
			return fmt.Errorf("duplicate device id %q", d.ID)
		}
		seen[d.ID] = true
		if d.Period <= 0 {
			return fmt.Errorf("device %q period must be positive", d.ID)
		}
		if len(d.Points) == 0 {
			return fmt.Errorf("device %q has no points", d.ID)
		}
		points := make(map[string]bool, len(d.Points))
		for _, p := range d.Points {
			if p == "" {
				return fmt.Errorf("device %q has an empty point key", d.ID)
			}
			if points[p] {
				return fmt.Errorf("device %q has duplicate point %q", d.ID, p)
			}
			points[p] = true
		}
	}
	return nil
}

// Started is closed after every pollable link has armed its first timer, or
// immediately on Run when there is no pollable link.
func (s *Scheduler) Started() <-chan struct{} { return s.started }

// StartedAt returns the phase anchor captured at New.
func (s *Scheduler) StartedAt() time.Time { return s.anchor }

// Stats returns a copy of the per-link counters in config order.
func (s *Scheduler) Stats() []LinkStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]LinkStats(nil), s.stats...)
}

// Run starts every pollable link and blocks until ctx is cancelled. A nil ctx
// is treated as context.Background(), mirroring engine.Run. Run may be called
// only once.
func (s *Scheduler) Run(ctx context.Context) error {
	ctx = normalizeContext(ctx)
	s.runMu.Lock()
	if s.ran {
		s.runMu.Unlock()
		return errors.New("scheduler: already run")
	}
	s.ran = true
	s.runMu.Unlock()

	pollable := make([]*linkPlan, 0, len(s.links))
	for _, l := range s.links {
		if l.pollable {
			pollable = append(pollable, l)
		}
	}
	if len(pollable) == 0 {
		close(s.started)
		<-ctx.Done()
		return nil
	}

	var startWG, runWG sync.WaitGroup
	startWG.Add(len(pollable))
	runWG.Add(len(pollable))
	for _, l := range pollable {
		go func(l *linkPlan) {
			defer runWG.Done()
			s.runLink(ctx, l, &startWG)
		}(l)
	}
	go func() {
		startWG.Wait()
		close(s.started)
	}()
	runWG.Wait()
	return nil
}

func normalizeContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func (s *Scheduler) runLink(ctx context.Context, l *linkPlan, startWG *sync.WaitGroup) {
	next := plannedAt(s.anchor, l.tickPeriod, 1)
	timer := s.clock.NewTimer(next.Sub(s.clock.Now()))
	startWG.Done()
	defer func() { timer.Stop() }()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C():
		}

		now := s.clock.Now()
		runAt, missed := advance(now, next, l.tickPeriod)
		if missed > 0 {
			s.recordMissed(l, missed, next, now)
		}
		next = runAt.Add(l.tickPeriod)
		timer.Stop()
		timer = s.clock.NewTimer(next.Sub(s.clock.Now()))

		readCount, status := s.runDueDevices(ctx, l, runAt)
		if ctx.Err() != nil {
			return
		}
		at := s.clock.Now()
		s.recordTick(l, status, at)
		if s.obs != nil {
			s.obs.ObserveTick(TickInfo{
				Link:      l.id,
				Planned:   runAt,
				Index:     uint64(runAt.Sub(s.anchor) / l.tickPeriod),
				Missed:    missed,
				ReadCount: readCount,
				At:        at,
			})
		}
	}
}

type tickStatus struct {
	any    bool
	failed bool
	reason string
}

func (t *tickStatus) observe(r ReadResult) {
	t.any = true
	if r.TimedOut {
		t.failed = true
		t.setReason("read deadline exceeded")
		return
	}
	if r.Outcome == interfaces.OutcomeOK {
		return
	}
	t.failed = true
	t.setReason(readReason(r.Outcome, r.Samples))
}

func (t *tickStatus) setReason(reason string) {
	if t.reason != "" {
		return
	}
	if reason == "" {
		reason = "read failed"
	}
	t.reason = reason
}

func (s *Scheduler) runDueDevices(ctx context.Context, l *linkPlan, planned time.Time) (int, tickStatus) {
	var status tickStatus
	due := dueDeviceIndexes(l.devices, planned.Sub(s.anchor))
	count := 0
	for _, i := range due {
		dev := l.devices[i]
		result, ok := s.readDevice(ctx, l, dev, planned)
		if !ok {
			return count, status
		}
		count++
		status.observe(result)
		s.recordRead(l, result)
		if result.TimedOut {
			s.log.Warn("read deadline exceeded",
				"event", "read_deadline",
				logging.KeyLink, l.id,
				logging.KeyDevice, dev.ID,
				logging.KeyReason, reasonDeadline,
				"deadline", formatTime(result.StartedAt.Add(l.readDeadline)),
			)
		}
		if s.obs != nil {
			s.obs.ObserveRead(result)
		}
		if ctx.Err() != nil {
			return count, status
		}
	}
	return count, status
}

func (s *Scheduler) recordRead(l *linkPlan, r ReadResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := &s.stats[l.statsIndex]
	st.Reads++
	switch {
	case r.TimedOut:
		st.ReadTimeouts++
	case r.Outcome == interfaces.OutcomeFailed:
		st.ReadFailures++
	}
}

func (s *Scheduler) recordTick(l *linkPlan, status tickStatus, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := &s.stats[l.statsIndex]
	st.Ticks++
	st.LastTickAt = at
	if status.any {
		if status.failed {
			st.LastError = status.reason
		} else {
			st.LastError = ""
		}
	}
}

func (s *Scheduler) recordMissed(l *linkPlan, missed uint64, planned, now time.Time) {
	s.mu.Lock()
	st := &s.stats[l.statsIndex]
	st.MissedTicks += missed
	total := st.MissedTicks
	s.mu.Unlock()

	s.log.Info("missed tick",
		"event", "missed_tick",
		logging.KeyLink, l.id,
		"skipped", missed,
		"planned", formatTime(planned),
		"now", formatTime(now),
		"missed_total", total,
	)
}

func readReason(outcome interfaces.Outcome, samples []interfaces.Sample) string {
	for _, sample := range samples {
		if sample.Err != "" {
			return sample.Err
		}
	}
	switch outcome {
	case interfaces.OutcomePartial:
		return "outcome partial"
	case interfaces.OutcomeFailed:
		return "outcome failed"
	case interfaces.OutcomeUnsupported:
		return "outcome unsupported"
	default:
		return ""
	}
}

func formatTime(t time.Time) string {
	return t.UTC().Format(timeLayout)
}

func copyDevices(in []Device) []Device {
	out := make([]Device, len(in))
	for i, d := range in {
		out[i] = d
		out[i].Points = append([]string(nil), d.Points...)
	}
	return out
}

func isNil(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}
