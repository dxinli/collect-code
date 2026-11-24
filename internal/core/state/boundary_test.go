// Covers: REQ-QUALITY-05
package state_test

import (
	"testing"
	"time"

	"github.com/dxinli/collect-code/internal/interfaces"
)

func TestStaleBoundary(t *testing.T) {
	const stale = 180 * time.Second
	for _, tc := range []struct {
		name      string
		delta     time.Duration
		want      interfaces.Quality
		wantFresh bool
	}{
		{name: "threshold minus one nanosecond", delta: stale - time.Nanosecond, want: interfaces.QualityGood, wantFresh: true},
		{name: "exactly at threshold", delta: stale, want: interfaces.QualityGood, wantFresh: true},
		{name: "threshold plus one nanosecond", delta: stale + time.Nanosecond, want: interfaces.QualityStale, wantFresh: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, singlePointConfig(stale, 60*time.Second, 1000))
			h.advance(1000 * time.Second)
			at := h.clock.Now().Add(-tc.delta)

			h.read(interfaces.OutcomeOK, sample("a", 1.0, at))

			h.wantQuality("a", tc.want)
			if tc.wantFresh {
				h.wantLastSuccess(at)
			} else {
				h.wantLastSuccess(baseTime)
			}
		})
	}
}

func TestOfflineBoundary(t *testing.T) {
	const period = 60 * time.Second
	const offlineAfter = 3

	t.Run("one nanosecond before the boundary stays online", func(t *testing.T) {
		h := newHarness(t, singlePointConfig(time.Hour, period, offlineAfter))
		h.advance(offlineAfter*period - time.Nanosecond)

		h.tr.Sweep()

		if !h.snapshot().Online {
			t.Fatal("device went offline before N x period")
		}
		if calls := h.rec.all(); len(calls) != 0 {
			t.Fatalf("boundary sweep emitted %d calls, want 0", len(calls))
		}
	})

	t.Run("exactly at the boundary stays online", func(t *testing.T) {
		h := newHarness(t, singlePointConfig(time.Hour, period, offlineAfter))
		h.advance(offlineAfter * period)

		h.tr.Sweep()

		if !h.snapshot().Online {
			t.Fatal("device went offline exactly at N x period")
		}
		if calls := h.rec.all(); len(calls) != 0 {
			t.Fatalf("boundary sweep emitted %d calls, want 0", len(calls))
		}
	})

	t.Run("one nanosecond later goes offline", func(t *testing.T) {
		h := newHarness(t, singlePointConfig(time.Hour, period, offlineAfter))
		h.advance(offlineAfter*period + time.Nanosecond)

		h.tr.Sweep()

		if h.snapshot().Online {
			t.Fatal("device stayed online past N x period")
		}
		calls := h.rec.all()
		if len(calls) != 1 || calls[0].kind != "online" || calls[0].change.Online {
			t.Fatalf("offline calls = %+v, want one offline transition", calls)
		}
	})
}
