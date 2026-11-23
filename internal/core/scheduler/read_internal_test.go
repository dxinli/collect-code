package scheduler

import (
	"testing"
	"time"

	"github.com/dxinli/collect-code/internal/interfaces"
)

func TestResultOrTimeoutStrictDeadline(t *testing.T) {
	started := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	deadline := started.Add(300 * time.Millisecond)
	link := &linkPlan{id: "l1"}
	dev := Device{ID: "d1", Points: []string{"a", "b"}}
	s := &Scheduler{}

	atDeadline := s.resultOrTimeout(link, dev, started, started, deadline, readOutcome{
		samples: []interfaces.Sample{{PointKey: "a", Quality: interfaces.QualityGood}},
		outcome: interfaces.OutcomeOK,
		at:      deadline,
	})
	if atDeadline.TimedOut {
		t.Fatal("read finishing exactly at the deadline must not time out")
	}

	late := s.resultOrTimeout(link, dev, started, started, deadline, readOutcome{
		samples: []interfaces.Sample{{PointKey: "a", Quality: interfaces.QualityGood}},
		outcome: interfaces.OutcomeOK,
		at:      deadline.Add(time.Nanosecond),
	})
	if !late.TimedOut {
		t.Fatal("read finishing after the deadline must time out")
	}
	if late.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("late outcome = %v, want failed", late.Outcome)
	}
	if len(late.Samples) != 2 {
		t.Fatalf("late samples = %d, want 2", len(late.Samples))
	}
	for i, sample := range late.Samples {
		if sample.PointKey != dev.Points[i] || sample.Quality != interfaces.QualityBad {
			t.Fatalf("sample %d = %+v, want bad %q", i, sample, dev.Points[i])
		}
	}
}

func TestTimeoutResultUsesDevicePointOrder(t *testing.T) {
	started := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	link := &linkPlan{id: "l1"}
	dev := Device{ID: "d1", Points: []string{"z", "a", "m"}}
	s := &Scheduler{}
	got := s.timeoutResult(link, dev, started, started, started.Add(time.Second))
	if len(got.Samples) != len(dev.Points) {
		t.Fatalf("samples = %d, want %d", len(got.Samples), len(dev.Points))
	}
	for i, sample := range got.Samples {
		if sample.PointKey != dev.Points[i] {
			t.Fatalf("sample %d point = %q, want %q", i, sample.PointKey, dev.Points[i])
		}
	}
}

func TestRecordReadCounts(t *testing.T) {
	s := &Scheduler{stats: []LinkStats{{ID: "l1"}}}
	l := &linkPlan{statsIndex: 0}

	s.recordRead(l, ReadResult{Outcome: interfaces.OutcomeOK})
	s.recordRead(l, ReadResult{Outcome: interfaces.OutcomePartial})
	s.recordRead(l, ReadResult{Outcome: interfaces.OutcomeFailed})
	s.recordRead(l, ReadResult{Outcome: interfaces.OutcomeFailed, TimedOut: true})

	st := s.stats[0]
	if st.Reads != 4 {
		t.Fatalf("Reads = %d, want 4", st.Reads)
	}
	if st.ReadFailures != 1 {
		t.Fatalf("ReadFailures = %d, want 1", st.ReadFailures)
	}
	if st.ReadTimeouts != 1 {
		t.Fatalf("ReadTimeouts = %d, want 1", st.ReadTimeouts)
	}
}

func TestRecordTickLastError(t *testing.T) {
	s := &Scheduler{stats: []LinkStats{{ID: "l1"}}}
	l := &linkPlan{statsIndex: 0}
	at := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)

	s.recordTick(l, tickStatus{any: true, failed: true, reason: "boom"}, at)
	if got := s.stats[0].LastError; got != "boom" {
		t.Fatalf("LastError = %q, want boom", got)
	}
	if got := s.stats[0].Ticks; got != 1 {
		t.Fatalf("Ticks = %d, want 1", got)
	}

	s.recordTick(l, tickStatus{any: true}, at.Add(time.Minute))
	if got := s.stats[0].LastError; got != "" {
		t.Fatalf("LastError after all-OK tick = %q, want empty", got)
	}

	s.stats[0].LastError = "keep"
	s.recordTick(l, tickStatus{}, at.Add(2*time.Minute))
	if got := s.stats[0].LastError; got != "keep" {
		t.Fatalf("LastError on zero-read tick = %q, want keep", got)
	}
}

func TestTickStatusUsesFirstSampleError(t *testing.T) {
	var status tickStatus
	status.observe(ReadResult{
		Outcome: interfaces.OutcomePartial,
		Samples: []interfaces.Sample{
			{PointKey: "first", Err: "crc"},
			{PointKey: "second", Err: "short frame"},
		},
	})
	if status.reason != "crc" {
		t.Fatalf("reason = %q, want crc", status.reason)
	}
	if !status.failed {
		t.Fatal("partial read must mark the tick failed for LastError")
	}
}

func TestReadReasonFallbacks(t *testing.T) {
	if got := readReason(interfaces.OutcomePartial, nil); got != "outcome partial" {
		t.Fatalf("partial reason = %q", got)
	}
	if got := readReason(interfaces.OutcomeFailed, nil); got != "outcome failed" {
		t.Fatalf("failed reason = %q", got)
	}
}
