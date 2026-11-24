// Covers: REQ-QUALITY-01, REQ-QUALITY-02, REQ-QUALITY-03
package state_test

import (
	"testing"
	"time"

	"github.com/dxinli/collect-code/internal/interfaces"
)

func TestTruthTableOKFresh(t *testing.T) {
	h := newHarness(t, defaultConfig())
	h.advance(10 * time.Second)
	at := h.now

	h.read(interfaces.OutcomeOK, sample("a", 1.5, at), sample("b", 2.5, at))

	h.wantQuality("a", interfaces.QualityGood)
	h.wantQuality("b", interfaces.QualityGood)
	h.wantLastSuccess(at)
}

func TestTruthTableOKMixedFreshAndExpired(t *testing.T) {
	h := newHarness(t, defaultConfig())
	h.advance(100 * time.Second)

	h.read(interfaces.OutcomeOK,
		sample("a", 1.5, h.now.Add(-181*time.Second)),
		sample("b", 2.5, h.now.Add(-10*time.Second)),
	)

	h.wantQuality("a", interfaces.QualityStale)
	h.wantQuality("b", interfaces.QualityGood)
	h.wantLastSuccess(h.now.Add(-10 * time.Second))
}

func TestTruthTableAllExpired(t *testing.T) {
	h := newHarness(t, defaultConfig())
	h.advance(100 * time.Second)
	got := h.now.Add(-181 * time.Second)

	h.read(interfaces.OutcomeOK, sample("a", 1.5, got), sample("b", 2.5, got))

	h.wantQuality("a", interfaces.QualityStale)
	h.wantQuality("b", interfaces.QualityStale)
	h.wantLastSuccess(baseTime)
	if gotTS := h.property("a").TS; !gotTS.Equal(got) {
		t.Fatalf("ts = %v, want the sample time %v", gotTS, got)
	}
}

func TestTruthTablePartial(t *testing.T) {
	h := newHarness(t, defaultConfig())
	h.advance(10 * time.Second)
	at := h.now

	h.read(interfaces.OutcomePartial,
		sample("a", 1.5, at),
		badSample("b", 2.5, at, "crc"),
	)

	h.wantQuality("a", interfaces.QualityGood)
	h.wantQuality("b", interfaces.QualityBad)
	h.wantLastSuccess(at)
}

func TestTruthTableFailed(t *testing.T) {
	h := newHarness(t, defaultConfig())
	seedGood(h)
	h.advance(10 * time.Second)

	h.read(interfaces.OutcomeFailed)

	h.wantQuality("a", interfaces.QualityBad)
	h.wantQuality("b", interfaces.QualityBad)
	h.wantLastSuccess(baseTime)
}

func TestTruthTableUnsupportedKeepsValues(t *testing.T) {
	h := newHarness(t, defaultConfig())
	seedGood(h)
	h.rec.reset()
	h.advance(10 * time.Second)

	h.read(interfaces.OutcomeUnsupported)

	h.wantQuality("a", interfaces.QualityGood)
	h.wantQuality("b", interfaces.QualityGood)
	h.wantLastSuccess(baseTime)
	if calls := h.rec.all(); len(calls) != 0 {
		t.Fatalf("unsupported read emitted %d calls, want 0", len(calls))
	}
}

func TestTruthTableEmptyOKKeepsValues(t *testing.T) {
	h := newHarness(t, defaultConfig())
	seedGood(h)
	h.rec.reset()
	h.advance(10 * time.Second)

	h.read(interfaces.OutcomeOK)

	h.wantQuality("a", interfaces.QualityGood)
	h.wantQuality("b", interfaces.QualityGood)
	if calls := h.rec.all(); len(calls) != 0 {
		t.Fatalf("empty OK read emitted %d calls, want 0", len(calls))
	}
}

func TestBadIsNotUpgraded(t *testing.T) {
	h := newHarness(t, defaultConfig())
	at := h.now

	h.read(interfaces.OutcomeOK,
		badSample("a", 9.9, at, "out of range"),
		sample("b", 2.0, at),
	)
	h.wantQuality("a", interfaces.QualityBad)
	h.wantQuality("b", interfaces.QualityGood)

	// A later legal observation may recover the point; the bad observation
	// itself was never turned into good.
	h.advance(time.Second)
	h.read(interfaces.OutcomeOK,
		sample("a", 1.0, h.now),
		sample("b", 2.0, h.now),
	)
	h.wantQuality("a", interfaces.QualityGood)
}

func TestBadSampleDoesNotAdvanceLastSuccess(t *testing.T) {
	h := newHarness(t, defaultConfig())
	h.advance(10 * time.Second)

	h.read(interfaces.OutcomePartial,
		badSample("a", 1.0, h.now, "decode"),
		badSample("b", 2.0, h.now, "decode"),
	)

	h.wantQuality("a", interfaces.QualityBad)
	h.wantQuality("b", interfaces.QualityBad)
	h.wantLastSuccess(baseTime)
}

func TestMissingPointIsBad(t *testing.T) {
	h := newHarness(t, defaultConfig())
	seedGood(h)
	bBefore := h.property("b")
	h.advance(10 * time.Second)

	h.read(interfaces.OutcomeOK, sample("a", 1.5, h.now))

	h.wantQuality("a", interfaces.QualityGood)
	h.wantQuality("b", interfaces.QualityBad)
	if bAfter := h.property("b"); bAfter.Value != bBefore.Value || !bAfter.TS.Equal(bBefore.TS) {
		t.Fatalf("missing point lost its last value: before=%+v after=%+v", bBefore, bAfter)
	}
}

func TestInvalidPayloadIsBad(t *testing.T) {
	h := newHarness(t, defaultConfig())
	seedGood(h)
	h.advance(10 * time.Second)

	h.read(interfaces.OutcomePartial,
		badSample("a", nil, h.now, "invalid payload"),
		badSample("b", nil, h.now, "invalid payload"),
	)

	h.wantQuality("a", interfaces.QualityBad)
	h.wantQuality("b", interfaces.QualityBad)
	h.wantLastSuccess(baseTime)
}

func TestUnknownPointIsIgnored(t *testing.T) {
	h := newHarness(t, defaultConfig())
	seedGood(h)
	h.advance(10 * time.Second)

	h.read(interfaces.OutcomeOK,
		sample("a", 1.5, h.now),
		sample("b", 2.5, h.now),
		sample("z", 9.9, h.now),
	)

	for _, prop := range h.snapshot().Properties {
		if prop.Key == "z" {
			t.Fatal("unknown point was stored")
		}
	}
	h.wantQuality("a", interfaces.QualityGood)
}

func TestEventUplinkIsAFreshObservation(t *testing.T) {
	h := newHarness(t, defaultConfig())
	h.advance(10 * time.Second)
	at := h.now

	h.event(interfaces.EventUplink, sample("a", 1.5, at), sample("b", 2.5, at))

	h.wantQuality("a", interfaces.QualityGood)
	h.wantQuality("b", interfaces.QualityGood)
	h.wantLastSuccess(at)
}

func TestEventUplinkSubsetKeepsOtherPoints(t *testing.T) {
	h := newHarness(t, defaultConfig())
	seedGood(h)
	h.advance(10 * time.Second)

	h.event(interfaces.EventUplink, sample("a", 1.5, h.now))

	h.wantQuality("a", interfaces.QualityGood)
	h.wantQuality("b", interfaces.QualityGood)
	h.wantLastSuccess(h.now)
}

func TestEventUplinkExpiredIsStale(t *testing.T) {
	h := newHarness(t, defaultConfig())
	h.advance(100 * time.Second)
	at := h.now.Add(-181 * time.Second)

	h.event(interfaces.EventUplink, sample("a", 1.5, at), sample("b", 2.5, at))

	h.wantQuality("a", interfaces.QualityStale)
	h.wantQuality("b", interfaces.QualityStale)
	h.wantLastSuccess(baseTime)
}

func TestEventAcknowledgementsDoNotTouchProperties(t *testing.T) {
	for _, kind := range []interfaces.EventKind{
		interfaces.EventTxAck,
		interfaces.EventAck,
		interfaces.EventDeviceStatus,
		interfaces.EventDriverError,
	} {
		t.Run(string(kind), func(t *testing.T) {
			h := newHarness(t, defaultConfig())
			seedGood(h)
			h.rec.reset()
			h.advance(10 * time.Second)

			h.event(kind, sample("a", 9.9, h.now))

			if calls := h.rec.all(); len(calls) != 0 {
				t.Fatalf("%s emitted %d calls, want 0", kind, len(calls))
			}
			h.wantQuality("a", interfaces.QualityGood)
			h.wantLastSuccess(baseTime)
		})
	}
}

func TestUnknownEventKindIsIgnored(t *testing.T) {
	h := newHarness(t, defaultConfig())
	seedGood(h)
	h.rec.reset()
	h.advance(10 * time.Second)

	h.event(interfaces.EventKind("weird"), sample("a", 9.9, h.now))

	if calls := h.rec.all(); len(calls) != 0 {
		t.Fatalf("unknown event emitted %d calls, want 0", len(calls))
	}
	h.wantQuality("a", interfaces.QualityGood)
}
