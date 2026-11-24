// Covers: REQ-QUALITY-02, REQ-QUALITY-04
package state_test

import (
	"testing"
	"time"

	"github.com/dxinli/collect-code/internal/interfaces"
)

func TestOfflineEmitsOnceAndMarksStale(t *testing.T) {
	h := newHarness(t, defaultConfig())
	seedGood(h)
	h.rec.reset()

	h.advance(181 * time.Second)
	h.tr.Sweep()

	calls := h.rec.all()
	if len(calls) != 2 {
		t.Fatalf("offline emitted %d calls, want 2: %+v", len(calls), calls)
	}
	if calls[0].kind != "online" || calls[0].change.Online {
		t.Fatalf("first call = %+v, want offline transition", calls[0])
	}
	if !calls[0].change.At.Equal(h.now) {
		t.Fatalf("offline at = %v, want %v", calls[0].change.At, h.now)
	}
	if calls[1].kind != "properties" || calls[1].mode != interfaces.ReportDelta || calls[1].online {
		t.Fatalf("second call = %+v, want delta with online=false", calls[1])
	}
	if len(calls[1].props) != 2 {
		t.Fatalf("stale delta has %d props, want 2", len(calls[1].props))
	}
	for _, prop := range calls[1].props {
		if prop.Quality != interfaces.QualityStale {
			t.Fatalf("prop %q quality = %q, want stale", prop.Key, prop.Quality)
		}
		if !prop.TS.Equal(baseTime) {
			t.Fatalf("prop %q ts moved to %v", prop.Key, prop.TS)
		}
	}

	h.rec.reset()
	h.tr.Sweep()
	if calls := h.rec.all(); len(calls) != 0 {
		t.Fatalf("repeated sweep emitted %d calls, want 0", len(calls))
	}
	if h.snapshot().Online {
		t.Fatal("device should stay offline")
	}
	h.wantLastSuccess(baseTime)
}

func TestOfflineKeepsBadQuality(t *testing.T) {
	h := newHarness(t, defaultConfig())
	seedGood(h)
	h.advance(time.Second)
	h.read(interfaces.OutcomePartial,
		badSample("a", 1.0, h.now, "crc"),
		sample("b", 2.0, h.now),
	)
	h.rec.reset()

	h.advance(181 * time.Second)
	h.tr.Sweep()

	h.wantQuality("a", interfaces.QualityBad)
	h.wantQuality("b", interfaces.QualityStale)

	calls := h.rec.all()
	if len(calls) != 2 {
		t.Fatalf("offline emitted %d calls, want 2: %+v", len(calls), calls)
	}
	for _, prop := range calls[1].props {
		if prop.Key == "a" {
			t.Fatalf("bad point was included in the stale delta: %+v", prop)
		}
	}
	if len(calls[1].props) != 1 || calls[1].props[0].Key != "b" {
		t.Fatalf("stale delta = %+v, want only b", calls[1].props)
	}
}

func TestRecoveryOrderAndSnapshot(t *testing.T) {
	h := newHarness(t, defaultConfig())
	seedGood(h)
	h.advance(181 * time.Second)
	h.tr.Sweep()
	h.rec.reset()

	h.advance(10 * time.Second)
	at := h.now
	h.read(interfaces.OutcomeOK, sample("a", 3.0, at), sample("b", 4.0, at))

	calls := h.rec.all()
	if len(calls) != 2 {
		t.Fatalf("recovery emitted %d calls, want 2: %+v", len(calls), calls)
	}
	if calls[0].kind != "online" || !calls[0].change.Online {
		t.Fatalf("first call = %+v, want online transition", calls[0])
	}
	if calls[1].kind != "properties" || calls[1].mode != interfaces.ReportSnapshot || !calls[1].online {
		t.Fatalf("second call = %+v, want online snapshot", calls[1])
	}
	if len(calls[1].props) != 2 {
		t.Fatalf("snapshot has %d props, want 2", len(calls[1].props))
	}
	if !h.snapshot().Online {
		t.Fatal("device should be online after recovery")
	}
	h.wantLastSuccess(at)
	h.wantQuality("a", interfaces.QualityGood)
}

func TestOfflineSuppressesNonFreshPropertyOutput(t *testing.T) {
	h := newHarness(t, defaultConfig())
	seedGood(h)
	h.advance(181 * time.Second)
	h.tr.Sweep()
	h.rec.reset()

	h.read(interfaces.OutcomeOK,
		sample("a", 1.0, baseTime),
		sample("b", 2.0, baseTime),
	)

	if calls := h.rec.all(); len(calls) != 0 {
		t.Fatalf("offline non-fresh read emitted %d calls, want 0: %+v", len(calls), calls)
	}
	h.wantQuality("a", interfaces.QualityStale)
	h.wantLastSuccess(baseTime)
}

func TestRecoveryRequiresTheOfflineIntervalToClear(t *testing.T) {
	h := newHarness(t, singlePointConfig(time.Hour, 60*time.Second, 3))
	seedGood(h)
	h.advance(181 * time.Second)
	h.tr.Sweep()
	if h.snapshot().Online {
		t.Fatal("device should be offline")
	}
	h.rec.reset()

	// Fresh with respect to the stale threshold, but its sample time is still
	// more than N periods old: the device stays offline.
	h.read(interfaces.OutcomeOK, sample("a", 1.0, h.now.Add(-181*time.Second)))

	if calls := h.rec.all(); len(calls) != 0 {
		t.Fatalf("old-but-fresh sample emitted %d calls, want 0: %+v", len(calls), calls)
	}
	if h.snapshot().Online {
		t.Fatal("device recovered from an old sample")
	}
}
