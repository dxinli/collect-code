package state_test

import (
	"sync"
	"testing"
	"time"

	"github.com/dxinli/collect-code/internal/core/scheduler"
	"github.com/dxinli/collect-code/internal/core/state"
	"github.com/dxinli/collect-code/internal/interfaces"
	"github.com/dxinli/collect-code/internal/testutil/fakeclock"
)

var baseTime = time.Date(2026, 11, 24, 8, 0, 0, 0, time.UTC)

type sinkCall struct {
	kind   string
	device string
	mode   interfaces.ReportMode
	online bool
	props  []interfaces.Property
	change state.OnlineChange
}

type recorder struct {
	mu    sync.Mutex
	calls []sinkCall
}

func newRecorder() *recorder {
	return &recorder{}
}

func (r *recorder) PublishProperties(device string, mode interfaces.ReportMode, online bool, props []interfaces.Property) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, sinkCall{
		kind:   "properties",
		device: device,
		mode:   mode,
		online: online,
		props:  append([]interfaces.Property(nil), props...),
	})
}

func (r *recorder) PublishOnline(change state.OnlineChange) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, sinkCall{kind: "online", device: change.Device, change: change})
}

func (r *recorder) all() []sinkCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]sinkCall(nil), r.calls...)
}

func (r *recorder) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = nil
}

type harness struct {
	t     *testing.T
	clock *fakeclock.Clock
	tr    *state.Tracker
	rec   *recorder
}

func newHarness(t *testing.T, cfg state.Config) *harness {
	t.Helper()
	clock := fakeclock.New(baseTime)
	t.Cleanup(clock.Close)
	rec := newRecorder()
	tr, err := state.New(clock, cfg, rec)
	if err != nil {
		t.Fatalf("state.New: %v", err)
	}
	return &harness{t: t, clock: clock, tr: tr, rec: rec}
}

func (h *harness) advance(d time.Duration) {
	h.clock.Advance(d)
}

func (h *harness) read(outcome interfaces.Outcome, samples ...interfaces.Sample) {
	h.tr.ObserveRead(scheduler.ReadResult{
		Device:  "dev-1",
		Points:  []string{"a", "b"},
		Samples: samples,
		Outcome: outcome,
	})
}

func (h *harness) event(kind interfaces.EventKind, samples ...interfaces.Sample) {
	h.tr.ObserveEvent(interfaces.DriverEvent{
		Kind:   kind,
		Device: "dev-1",
		Sample: samples,
	})
}

func (h *harness) snapshot() state.DeviceSnapshot {
	h.t.Helper()
	snap, ok := h.tr.Snapshot("dev-1")
	if !ok {
		h.t.Fatal("device not found")
	}
	return snap
}

func (h *harness) quality(key string) interfaces.Quality {
	h.t.Helper()
	for _, prop := range h.snapshot().Properties {
		if prop.Key == key {
			return prop.Quality
		}
	}
	h.t.Fatalf("point %q not found in snapshot", key)
	return ""
}

func (h *harness) property(key string) interfaces.Property {
	h.t.Helper()
	for _, prop := range h.snapshot().Properties {
		if prop.Key == key {
			return prop
		}
	}
	h.t.Fatalf("point %q not found in snapshot", key)
	return interfaces.Property{}
}

func (h *harness) wantQuality(key string, want interfaces.Quality) {
	h.t.Helper()
	if got := h.quality(key); got != want {
		h.t.Fatalf("quality[%s] = %q, want %q", key, got, want)
	}
}

func (h *harness) wantLastSuccess(want time.Time) {
	h.t.Helper()
	if got := h.snapshot().LastSuccessAt; !got.Equal(want) {
		h.t.Fatalf("last success = %v, want %v", got, want)
	}
}

func sample(key string, value any, at time.Time) interfaces.Sample {
	return interfaces.Sample{PointKey: key, Value: value, Quality: interfaces.QualityGood, At: at}
}

func badSample(key string, value any, at time.Time, reason string) interfaces.Sample {
	return interfaces.Sample{PointKey: key, Value: value, Quality: interfaces.QualityBad, At: at, Err: reason}
}

func defaultConfig() state.Config {
	return state.Config{
		OfflineAfter: 3,
		SweepEvery:   time.Second,
		Devices: []state.Device{{
			ID:     "dev-1",
			Period: 60 * time.Second,
			Points: []state.Point{
				{Key: "a", Unit: "%", StaleAfter: 180 * time.Second},
				{Key: "b", Unit: "C", StaleAfter: 180 * time.Second},
			},
		}},
	}
}

func singlePointConfig(stale, period time.Duration, offlineAfter int) state.Config {
	return state.Config{
		OfflineAfter: offlineAfter,
		SweepEvery:   time.Second,
		Devices: []state.Device{{
			ID:     "dev-1",
			Period: period,
			Points: []state.Point{{Key: "a", Unit: "%", StaleAfter: stale}},
		}},
	}
}

func seedGood(h *harness) {
	h.t.Helper()
	h.read(interfaces.OutcomeOK,
		sample("a", 1.0, h.clock.Now()),
		sample("b", 2.0, h.clock.Now()),
	)
}
