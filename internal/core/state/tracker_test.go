// Covers: REQ-QUALITY-01
package state_test

import (
	"testing"
	"time"

	"github.com/dxinli/collect-code/internal/core/scheduler"
	"github.com/dxinli/collect-code/internal/core/state"
	"github.com/dxinli/collect-code/internal/interfaces"
	"github.com/dxinli/collect-code/internal/testutil/fakeclock"
)

func TestNewValidation(t *testing.T) {
	clock := fakeclock.New(baseTime)
	t.Cleanup(clock.Close)

	cases := []struct {
		name   string
		mutate func(*state.Config)
	}{
		{name: "offline after zero", mutate: func(c *state.Config) { c.OfflineAfter = 0 }},
		{name: "negative sweep", mutate: func(c *state.Config) { c.SweepEvery = -time.Second }},
		{name: "empty device id", mutate: func(c *state.Config) { c.Devices[0].ID = "" }},
		{name: "duplicate device id", mutate: func(c *state.Config) {
			c.Devices = append(c.Devices, c.Devices[0])
		}},
		{name: "zero period", mutate: func(c *state.Config) { c.Devices[0].Period = 0 }},
		{name: "no points", mutate: func(c *state.Config) { c.Devices[0].Points = nil }},
		{name: "empty point key", mutate: func(c *state.Config) { c.Devices[0].Points[0].Key = "" }},
		{name: "duplicate point key", mutate: func(c *state.Config) {
			c.Devices[0].Points[1].Key = c.Devices[0].Points[0].Key
		}},
		{name: "zero stale threshold", mutate: func(c *state.Config) { c.Devices[0].Points[0].StaleAfter = 0 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultConfig()
			tc.mutate(&cfg)
			if _, err := state.New(clock, cfg, newRecorder()); err == nil {
				t.Fatal("New accepted an invalid config")
			}
		})
	}

	t.Run("nil clock", func(t *testing.T) {
		if _, err := state.New(nil, defaultConfig(), newRecorder()); err == nil {
			t.Fatal("New accepted a nil clock")
		}
	})
	t.Run("typed nil clock", func(t *testing.T) {
		var typed *fakeclock.Clock
		if _, err := state.New(typed, defaultConfig(), newRecorder()); err == nil {
			t.Fatal("New accepted a typed nil clock")
		}
	})
	t.Run("nil sink", func(t *testing.T) {
		if _, err := state.New(clock, defaultConfig(), nil); err == nil {
			t.Fatal("New accepted a nil sink")
		}
	})
	t.Run("typed nil sink", func(t *testing.T) {
		var typed *recorder
		if _, err := state.New(clock, defaultConfig(), typed); err == nil {
			t.Fatal("New accepted a typed nil sink")
		}
	})
	t.Run("zero sweep uses the default", func(t *testing.T) {
		cfg := defaultConfig()
		cfg.SweepEvery = 0
		if _, err := state.New(clock, cfg, newRecorder()); err != nil {
			t.Fatalf("New with zero sweep: %v", err)
		}
	})
}

func TestSnapshotIsACopy(t *testing.T) {
	h := newHarness(t, defaultConfig())
	seedGood(h)

	snap := h.snapshot()
	if len(snap.Properties) != 2 {
		t.Fatalf("snapshot has %d props, want 2", len(snap.Properties))
	}
	snap.Properties[0].Value = "mutated"
	snap.Properties = append(snap.Properties, interfaces.Property{Key: "extra"})

	again := h.snapshot()
	if len(again.Properties) != 2 {
		t.Fatalf("snapshot after mutation has %d props, want 2", len(again.Properties))
	}
	if again.Properties[0].Value == "mutated" {
		t.Fatal("snapshot mutation leaked into tracker state")
	}
}

func TestUnknownDeviceIsIgnored(t *testing.T) {
	h := newHarness(t, defaultConfig())
	h.rec.reset()

	h.tr.ObserveRead(scheduler.ReadResult{Device: "nope", Outcome: interfaces.OutcomeOK})
	h.tr.ObserveEvent(interfaces.DriverEvent{Device: "nope", Kind: interfaces.EventUplink})
	if _, ok := h.tr.Snapshot("nope"); ok {
		t.Fatal("unknown device returned a snapshot")
	}

	if calls := h.rec.all(); len(calls) != 0 {
		t.Fatalf("unknown device emitted %d calls, want 0", len(calls))
	}
}

func TestObserveTickDoesNothing(t *testing.T) {
	h := newHarness(t, defaultConfig())
	seedGood(h)
	h.rec.reset()

	h.tr.ObserveTick(scheduler.TickInfo{Link: "l1", ReadCount: 1})

	if calls := h.rec.all(); len(calls) != 0 {
		t.Fatalf("tick emitted %d calls, want 0", len(calls))
	}
}

func TestDeltaSuppressedWhenUnchanged(t *testing.T) {
	h := newHarness(t, defaultConfig())
	seedGood(h)
	h.rec.reset()

	seedGood(h)

	if calls := h.rec.all(); len(calls) != 0 {
		t.Fatalf("unchanged observation emitted %d calls, want 0: %+v", len(calls), calls)
	}
}

func TestOnlyChangedPointsArePublished(t *testing.T) {
	h := newHarness(t, defaultConfig())
	seedGood(h)
	h.rec.reset()
	h.advance(time.Second)

	h.read(interfaces.OutcomeOK,
		sample("a", 5.0, h.clock.Now()),
		sample("b", 2.0, baseTime),
	)

	calls := h.rec.all()
	if len(calls) != 1 {
		t.Fatalf("changed observation emitted %d calls, want 1: %+v", len(calls), calls)
	}
	if calls[0].mode != interfaces.ReportDelta {
		t.Fatalf("mode = %q, want delta", calls[0].mode)
	}
	if len(calls[0].props) != 1 || calls[0].props[0].Key != "a" {
		t.Fatalf("delta = %+v, want only a", calls[0].props)
	}
}

func TestFirstBadValueIsKept(t *testing.T) {
	h := newHarness(t, defaultConfig())

	h.read(interfaces.OutcomePartial,
		badSample("a", 9.9, h.clock.Now(), "out of range"),
	)

	h.wantQuality("a", interfaces.QualityBad)
	if got := h.property("a").Value; got != 9.9 {
		t.Fatalf("bad value = %v, want 9.9", got)
	}
}

func TestSnapshotsFollowConfigOrder(t *testing.T) {
	cfg := defaultConfig()
	cfg.Devices = append(cfg.Devices, state.Device{
		ID:     "dev-2",
		Period: 60 * time.Second,
		Points: []state.Point{{Key: "x", StaleAfter: time.Minute}},
	})
	clock := fakeclock.New(baseTime)
	t.Cleanup(clock.Close)
	tr, err := state.New(clock, cfg, newRecorder())
	if err != nil {
		t.Fatalf("state.New: %v", err)
	}

	snaps := tr.Snapshots()
	if len(snaps) != 2 || snaps[0].Device != "dev-1" || snaps[1].Device != "dev-2" {
		t.Fatalf("snapshots = %+v, want dev-1 then dev-2", snaps)
	}
}
