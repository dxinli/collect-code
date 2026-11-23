package scheduler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dxinli/collect-code/internal/core/scheduler"
	"github.com/dxinli/collect-code/internal/interfaces"
	"github.com/dxinli/collect-code/internal/testutil/fake"
	"github.com/dxinli/collect-code/internal/testutil/fakeclock"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

type recorder struct {
	ticks chan scheduler.TickInfo
	reads chan scheduler.ReadResult
}

func newRecorder() *recorder {
	return &recorder{
		ticks: make(chan scheduler.TickInfo, 4096),
		reads: make(chan scheduler.ReadResult, 4096),
	}
}

func (r *recorder) ObserveRead(result scheduler.ReadResult) { r.reads <- result }
func (r *recorder) ObserveTick(info scheduler.TickInfo)     { r.ticks <- info }

func waitTick(t *testing.T, r *recorder) scheduler.TickInfo {
	t.Helper()
	select {
	case info := <-r.ticks:
		return info
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for tick")
		return scheduler.TickInfo{}
	}
}

func waitRead(t *testing.T, r *recorder) scheduler.ReadResult {
	t.Helper()
	select {
	case result := <-r.reads:
		return result
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for read")
		return scheduler.ReadResult{}
	}
}

func startRun(t *testing.T, s *scheduler.Scheduler) (context.CancelFunc, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	return cancel, done
}

func stopRun(t *testing.T, cancel context.CancelFunc, done <-chan error) {
	t.Helper()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}

func waitStarted(t *testing.T, s *scheduler.Scheduler) {
	t.Helper()
	select {
	case <-s.Started():
	case <-time.After(2 * time.Second):
		t.Fatal("Started() was not closed")
	}
}

func testDevice(id string, period time.Duration, points ...string) scheduler.Device {
	return scheduler.Device{ID: id, Period: period, Points: points}
}

func testLink(id string, driver interfaces.Driver, deadline time.Duration, devices ...scheduler.Device) scheduler.Link {
	return scheduler.Link{ID: id, Driver: driver, Devices: devices, ReadDeadline: deadline}
}

func TestNewRejectsInvalidPlan(t *testing.T) {
	start := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	validDevice := testDevice("d1", time.Minute, "p")
	validLink := func(d interfaces.Driver) scheduler.Link {
		return testLink("l1", d, 300*time.Millisecond, validDevice)
	}
	validDriver := func() *fake.Driver { return fake.NewDriver("modbus") }

	tests := []struct {
		name   string
		clock  interfaces.Clock
		config scheduler.Config
	}{
		{"nil clock", nil, scheduler.Config{Links: []scheduler.Link{validLink(validDriver())}}},
		{"typed nil clock", (*fakeclock.Clock)(nil), scheduler.Config{Links: []scheduler.Link{validLink(validDriver())}}},
		{"nil driver", fakeclock.New(start), scheduler.Config{Links: []scheduler.Link{testLink("l1", nil, time.Second, validDevice)}}},
		{"typed nil driver", fakeclock.New(start), scheduler.Config{Links: []scheduler.Link{testLink("l1", (*fake.Driver)(nil), time.Second, validDevice)}}},
		{"empty link id", fakeclock.New(start), scheduler.Config{Links: []scheduler.Link{testLink("", validDriver(), time.Second, validDevice)}}},
		{"duplicate link id", fakeclock.New(start), scheduler.Config{Links: []scheduler.Link{
			validLink(validDriver()),
			validLink(validDriver()),
		}}},
		{"empty device id", fakeclock.New(start), scheduler.Config{Links: []scheduler.Link{testLink("l1", validDriver(), time.Second, testDevice("", time.Minute, "p"))}}},
		{"duplicate device id", fakeclock.New(start), scheduler.Config{Links: []scheduler.Link{testLink("l1", validDriver(), time.Second,
			testDevice("d1", time.Minute, "p"), testDevice("d1", time.Minute, "q"))}}},
		{"empty device points", fakeclock.New(start), scheduler.Config{Links: []scheduler.Link{testLink("l1", validDriver(), time.Second, testDevice("d1", time.Minute))}}},
		{"duplicate point", fakeclock.New(start), scheduler.Config{Links: []scheduler.Link{testLink("l1", validDriver(), time.Second, testDevice("d1", time.Minute, "p", "p"))}}},
		{"empty point key", fakeclock.New(start), scheduler.Config{Links: []scheduler.Link{testLink("l1", validDriver(), time.Second, testDevice("d1", time.Minute, ""))}}},
		{"missing period", fakeclock.New(start), scheduler.Config{Links: []scheduler.Link{testLink("l1", validDriver(), time.Second, testDevice("d1", 0, "p"))}}},
		{"zero deadline", fakeclock.New(start), scheduler.Config{Links: []scheduler.Link{testLink("l1", validDriver(), 0, validDevice)}}},
		{"pollable without devices", fakeclock.New(start), scheduler.Config{Links: []scheduler.Link{testLink("l1", validDriver(), time.Second)}}},
		{"tick below one second", fakeclock.New(start), scheduler.Config{Links: []scheduler.Link{testLink("l1", validDriver(), time.Second,
			testDevice("d1", 500*time.Millisecond, "p"))}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := scheduler.New(tc.clock, nil, tc.config, nil); err == nil {
				t.Fatal("New accepted an invalid plan")
			}
		})
	}
}

func TestNewAllowsNilLogger(t *testing.T) {
	start := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	clock := fakeclock.New(start)
	driver := fake.NewDriver("modbus")
	cfg := scheduler.Config{Links: []scheduler.Link{testLink("l1", driver, time.Second, testDevice("d1", time.Minute, "p"))}}
	if _, err := scheduler.New(clock, nil, cfg, nil); err != nil {
		t.Fatalf("New with nil logger: %v", err)
	}
}

func TestStartedClosesAfterTimersArmed(t *testing.T) {
	start := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	clock := fakeclock.New(start)
	driver := fake.NewDriver("modbus")
	rec := newRecorder()
	s, err := scheduler.New(clock, discardLogger(), scheduler.Config{
		Links: []scheduler.Link{testLink("l1", driver, time.Second, testDevice("d1", time.Minute, "p"))},
	}, rec)
	if err != nil {
		t.Fatal(err)
	}
	cancel, done := startRun(t, s)
	waitStarted(t, s)
	if got := len(driver.Calls()); got != 0 {
		t.Fatalf("reads before first period = %d, want 0", got)
	}
	stopRun(t, cancel, done)
}

func TestRunPhaseLocked24Hours(t *testing.T) {
	start := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	clock := fakeclock.New(start)
	driver := fake.NewDriver("modbus")
	driver.SetDefaultReadHandler(func(context.Context, fake.ReadCall) ([]interfaces.Sample, interfaces.Outcome) {
		return []interfaces.Sample{{PointKey: "p", Quality: interfaces.QualityGood, At: clock.Now()}}, interfaces.OutcomeOK
	})
	rec := newRecorder()
	s, err := scheduler.New(clock, discardLogger(), scheduler.Config{
		Links: []scheduler.Link{testLink("l1", driver, 300*time.Millisecond, testDevice("d1", time.Minute, "p"))},
	}, rec)
	if err != nil {
		t.Fatal(err)
	}
	cancel, done := startRun(t, s)
	waitStarted(t, s)

	for i := uint64(1); i <= 1440; i++ {
		clock.Advance(time.Minute)
		info := waitTick(t, rec)
		want := start.Add(time.Duration(i) * time.Minute)
		if !info.Planned.Equal(want) {
			t.Fatalf("tick %d planned = %s, want %s", i, info.Planned, want)
		}
		if info.Index != i {
			t.Fatalf("tick %d index = %d, want %d", i, info.Index, i)
		}
		if info.Missed != 0 {
			t.Fatalf("tick %d missed = %d, want 0", i, info.Missed)
		}
		if info.ReadCount != 1 {
			t.Fatalf("tick %d read count = %d, want 1", i, info.ReadCount)
		}
	}

	stats := s.Stats()
	if len(stats) != 1 || stats[0].Ticks != 1440 || stats[0].Reads != 1440 {
		t.Fatalf("stats = %+v, want 1440 ticks and reads", stats)
	}
	if stats[0].MissedTicks != 0 || stats[0].ReadTimeouts != 0 {
		t.Fatalf("stats = %+v, want no missed ticks or timeouts", stats[0])
	}

	fired := clock.FiredOrder()
	if len(fired) != 1440 {
		t.Fatalf("fired events = %d, want 1440", len(fired))
	}
	for i, event := range fired {
		want := start.Add(time.Duration(i+1) * time.Minute)
		if event.Kind != fakeclock.KindTimer || !event.Deadline.Equal(want) {
			t.Fatalf("fired[%d] = %+v, want timer at %s", i, event, want)
		}
	}
	stopRun(t, cancel, done)
}

func TestRunDoesNotReadBeforeFirstPeriod(t *testing.T) {
	start := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	clock := fakeclock.New(start)
	driver := fake.NewDriver("modbus")
	rec := newRecorder()
	s, err := scheduler.New(clock, discardLogger(), scheduler.Config{
		Links: []scheduler.Link{testLink("l1", driver, time.Second, testDevice("d1", time.Minute, "p"))},
	}, rec)
	if err != nil {
		t.Fatal(err)
	}
	cancel, done := startRun(t, s)
	waitStarted(t, s)
	if got := len(driver.Calls()); got != 0 {
		t.Fatalf("calls before advance = %d, want 0", got)
	}
	clock.Advance(time.Minute)
	waitTick(t, rec)
	if got := len(driver.Calls()); got != 1 {
		t.Fatalf("calls after first period = %d, want 1", got)
	}
	stopRun(t, cancel, done)
}

func TestZeroDueTickCountsButDoesNotRead(t *testing.T) {
	t.Run("60/90 at 30s ticks", func(t *testing.T) {
		start := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
		clock := fakeclock.New(start)
		driver := fake.NewDriver("modbus")
		rec := newRecorder()
		s, err := scheduler.New(clock, discardLogger(), scheduler.Config{
			Links: []scheduler.Link{testLink("l1", driver, 300*time.Millisecond,
				testDevice("d60", time.Minute, "p"),
				testDevice("d90", 90*time.Second, "p"),
			)},
		}, rec)
		if err != nil {
			t.Fatal(err)
		}
		cancel, done := startRun(t, s)
		waitStarted(t, s)

		wantReads := []int{0, 1, 1, 1, 0, 2}
		for i, want := range wantReads {
			clock.Advance(30 * time.Second)
			info := waitTick(t, rec)
			if info.ReadCount != want {
				t.Fatalf("tick %d read count = %d, want %d", i+1, info.ReadCount, want)
			}
		}
		if got := s.Stats()[0].Ticks; got != 6 {
			t.Fatalf("Ticks = %d, want 6", got)
		}
		if got := s.Stats()[0].Reads; got != 5 {
			t.Fatalf("Reads = %d, want 5", got)
		}
		if got := len(driver.Calls()); got != 5 {
			t.Fatalf("calls = %d, want 5", got)
		}
		stopRun(t, cancel, done)
	})

	t.Run("7/11 at 1s ticks", func(t *testing.T) {
		start := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
		clock := fakeclock.New(start)
		driver := fake.NewDriver("modbus")
		rec := newRecorder()
		s, err := scheduler.New(clock, discardLogger(), scheduler.Config{
			Links: []scheduler.Link{testLink("l1", driver, 300*time.Millisecond,
				testDevice("d7", 7*time.Second, "p"),
				testDevice("d11", 11*time.Second, "p"),
			)},
		}, rec)
		if err != nil {
			t.Fatal(err)
		}
		cancel, done := startRun(t, s)
		waitStarted(t, s)

		for i := 1; i <= 6; i++ {
			clock.Advance(time.Second)
			info := waitTick(t, rec)
			if info.ReadCount != 0 {
				t.Fatalf("tick %d read count = %d, want 0", i, info.ReadCount)
			}
		}
		clock.Advance(time.Second)
		if info := waitTick(t, rec); info.ReadCount != 1 {
			t.Fatalf("tick 7 read count = %d, want 1", info.ReadCount)
		}
		for i := 8; i <= 10; i++ {
			clock.Advance(time.Second)
			if info := waitTick(t, rec); info.ReadCount != 0 {
				t.Fatalf("tick %d read count = %d, want 0", i, info.ReadCount)
			}
		}
		clock.Advance(time.Second)
		if info := waitTick(t, rec); info.ReadCount != 1 {
			t.Fatalf("tick 11 read count = %d, want 1", info.ReadCount)
		}
		stopRun(t, cancel, done)
	})
}

func TestReadDeadlineBoundary(t *testing.T) {
	start := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	tests := []struct {
		name        string
		advance     time.Duration
		wantTimeout bool
	}{
		{"D-1ns", 300*time.Millisecond - time.Nanosecond, false},
		{"D", 300 * time.Millisecond, false},
		{"D+1ns", 300*time.Millisecond + time.Nanosecond, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clock := fakeclock.New(start)
			driver := fake.NewDriver("modbus")
			started := make(chan struct{}, 1)
			release := make(chan struct{})
			driver.SetDefaultReadHandler(func(ctx context.Context, _ fake.ReadCall) ([]interfaces.Sample, interfaces.Outcome) {
				started <- struct{}{}
				select {
				case <-release:
					return []interfaces.Sample{{PointKey: "p", Quality: interfaces.QualityGood, At: clock.Now()}}, interfaces.OutcomeOK
				case <-ctx.Done():
					return nil, interfaces.OutcomeFailed
				}
			})
			rec := newRecorder()
			s, err := scheduler.New(clock, discardLogger(), scheduler.Config{
				Links: []scheduler.Link{testLink("l1", driver, 300*time.Millisecond, testDevice("d1", time.Minute, "p"))},
			}, rec)
			if err != nil {
				t.Fatal(err)
			}
			cancel, done := startRun(t, s)
			waitStarted(t, s)
			clock.Advance(time.Minute)
			select {
			case <-started:
			case <-time.After(2 * time.Second):
				t.Fatal("read did not start")
			}
			clock.Advance(tc.advance)
			if !tc.wantTimeout {
				close(release)
			}
			result := waitRead(t, rec)
			if result.TimedOut != tc.wantTimeout {
				t.Fatalf("TimedOut = %v, want %v", result.TimedOut, tc.wantTimeout)
			}
			wantFinished := start.Add(time.Minute + tc.advance)
			if !result.FinishedAt.Equal(wantFinished) {
				t.Fatalf("FinishedAt = %s, want %s", result.FinishedAt, wantFinished)
			}
			wantTimeouts := uint64(0)
			if tc.wantTimeout {
				wantTimeouts = 1
			}
			if got := s.Stats()[0].ReadTimeouts; got != wantTimeouts {
				t.Fatalf("ReadTimeouts = %d, want %d", got, wantTimeouts)
			}
			if tc.wantTimeout {
				if len(result.Samples) != 1 || result.Samples[0].Quality != interfaces.QualityBad {
					t.Fatalf("timeout samples = %+v, want one bad sample", result.Samples)
				}
				close(release)
			}
			stopRun(t, cancel, done)
		})
	}
}

func TestDriverLateFailureCountsAsTimeout(t *testing.T) {
	start := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	clock := fakeclock.New(start)
	driver := fake.NewDriver("modbus")
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	driver.SetDefaultReadHandler(func(context.Context, fake.ReadCall) ([]interfaces.Sample, interfaces.Outcome) {
		started <- struct{}{}
		<-release
		return []interfaces.Sample{{PointKey: "p", Quality: interfaces.QualityBad, Err: "late"}}, interfaces.OutcomeFailed
	})
	rec := newRecorder()
	s, err := scheduler.New(clock, discardLogger(), scheduler.Config{
		Links: []scheduler.Link{testLink("l1", driver, 300*time.Millisecond, testDevice("d1", time.Minute, "p"))},
	}, rec)
	if err != nil {
		t.Fatal(err)
	}
	cancel, done := startRun(t, s)
	waitStarted(t, s)
	clock.Advance(time.Minute)
	waitSignal(t, started)
	clock.Advance(300*time.Millisecond + time.Nanosecond)
	close(release)
	result := waitRead(t, rec)
	if !result.TimedOut || result.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("result = %+v, want timeout", result)
	}
	stats := s.Stats()[0]
	if stats.ReadTimeouts != 1 || stats.ReadFailures != 0 {
		t.Fatalf("stats = %+v, want one timeout and no plain failure", stats)
	}
	stopRun(t, cancel, done)
}

func TestReadDeadlineMarksDeviceBadAndContinues(t *testing.T) {
	start := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	clock := fakeclock.New(start)
	driver := fake.NewDriver("modbus")
	aStarted := make(chan struct{}, 1)
	aRelease := make(chan struct{})
	driver.SetReadHandler("a", func(context.Context, fake.ReadCall) ([]interfaces.Sample, interfaces.Outcome) {
		aStarted <- struct{}{}
		<-aRelease
		return nil, interfaces.OutcomeFailed
	})
	rec := newRecorder()
	s, err := scheduler.New(clock, discardLogger(), scheduler.Config{
		Links: []scheduler.Link{testLink("l1", driver, 300*time.Millisecond,
			testDevice("a", time.Minute, "p1", "p2"),
			testDevice("b", time.Minute, "q"),
		)},
	}, rec)
	if err != nil {
		t.Fatal(err)
	}
	cancel, done := startRun(t, s)
	waitStarted(t, s)
	clock.Advance(time.Minute)
	select {
	case <-aStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("device a did not start")
	}
	clock.Advance(300*time.Millisecond + time.Nanosecond)

	first := waitRead(t, rec)
	if first.Device != "a" || !first.TimedOut || first.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("first read = %+v, want timed-out a", first)
	}
	if len(first.Samples) != 2 || first.Samples[0].PointKey != "p1" || first.Samples[1].PointKey != "p2" {
		t.Fatalf("timeout samples = %+v", first.Samples)
	}
	for _, sample := range first.Samples {
		if sample.Quality != interfaces.QualityBad {
			t.Fatalf("timeout sample quality = %q, want bad", sample.Quality)
		}
	}
	second := waitRead(t, rec)
	if second.Device != "b" || second.TimedOut {
		t.Fatalf("second read = %+v, want b after a timeout", second)
	}
	close(aRelease)
	stopRun(t, cancel, done)
}

func TestPerDeviceDeadlineIsIndependent(t *testing.T) {
	start := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	clock := fakeclock.New(start)
	driver := fake.NewDriver("modbus")
	aStarted := make(chan struct{}, 1)
	bStarted := make(chan fake.ReadCall, 1)
	driver.SetReadHandler("a", func(ctx context.Context, _ fake.ReadCall) ([]interfaces.Sample, interfaces.Outcome) {
		aStarted <- struct{}{}
		<-ctx.Done()
		return nil, interfaces.OutcomeFailed
	})
	driver.SetReadHandler("b", func(_ context.Context, call fake.ReadCall) ([]interfaces.Sample, interfaces.Outcome) {
		bStarted <- call
		return []interfaces.Sample{{PointKey: "q", Quality: interfaces.QualityGood, At: clock.Now()}}, interfaces.OutcomeOK
	})
	rec := newRecorder()
	s, err := scheduler.New(clock, discardLogger(), scheduler.Config{
		Links: []scheduler.Link{testLink("l1", driver, 300*time.Millisecond,
			testDevice("a", time.Minute, "p"),
			testDevice("b", time.Minute, "q"),
		)},
	}, rec)
	if err != nil {
		t.Fatal(err)
	}
	cancel, done := startRun(t, s)
	waitStarted(t, s)
	clock.Advance(time.Minute)
	waitSignal(t, aStarted)
	clock.Advance(300*time.Millisecond + time.Nanosecond)
	select {
	case call := <-bStarted:
		want := start.Add(time.Minute + 300*time.Millisecond + time.Nanosecond + 300*time.Millisecond)
		if !call.Deadline.Equal(want) {
			t.Fatalf("b deadline = %s, want %s", call.Deadline, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("device b did not start")
	}
	waitRead(t, rec)
	waitRead(t, rec)
	stopRun(t, cancel, done)
}

func TestReadsAreSerialWithinLink(t *testing.T) {
	start := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	clock := fakeclock.New(start)
	driver := fake.NewDriver("modbus")
	var current, maximum atomic.Int32
	started := map[string]chan struct{}{
		"a": make(chan struct{}, 1),
		"b": make(chan struct{}, 1),
		"c": make(chan struct{}, 1),
	}
	release := map[string]chan struct{}{
		"a": make(chan struct{}),
		"b": make(chan struct{}),
		"c": make(chan struct{}),
	}
	handler := func(id string) fake.ReadHandler {
		return func(ctx context.Context, _ fake.ReadCall) ([]interfaces.Sample, interfaces.Outcome) {
			n := current.Add(1)
			for {
				m := maximum.Load()
				if n <= m || maximum.CompareAndSwap(m, n) {
					break
				}
			}
			defer current.Add(-1)
			started[id] <- struct{}{}
			select {
			case <-release[id]:
				return []interfaces.Sample{{PointKey: "p", Quality: interfaces.QualityGood, At: clock.Now()}}, interfaces.OutcomeOK
			case <-ctx.Done():
				return nil, interfaces.OutcomeFailed
			}
		}
	}
	for _, id := range []string{"a", "b", "c"} {
		driver.SetReadHandler(id, handler(id))
	}
	rec := newRecorder()
	s, err := scheduler.New(clock, discardLogger(), scheduler.Config{
		Links: []scheduler.Link{testLink("l1", driver, time.Second,
			testDevice("a", time.Minute, "p"),
			testDevice("b", time.Minute, "p"),
			testDevice("c", time.Minute, "p"),
		)},
	}, rec)
	if err != nil {
		t.Fatal(err)
	}
	cancel, done := startRun(t, s)
	waitStarted(t, s)
	clock.Advance(time.Minute)

	waitSignal(t, started["a"])
	assertNotStarted(t, started["b"])
	assertNotStarted(t, started["c"])
	close(release["a"])
	waitSignal(t, started["b"])
	assertNotStarted(t, started["c"])
	close(release["b"])
	waitSignal(t, started["c"])
	close(release["c"])
	waitTick(t, rec)

	if got := maximum.Load(); got != 1 {
		t.Fatalf("maximum concurrent reads = %d, want 1", got)
	}
	stopRun(t, cancel, done)
}

func waitSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for signal")
	}
}

func assertNotStarted(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
		t.Fatal("next device started before the previous read completed")
	default:
	}
}

func TestLinksRunIndependently(t *testing.T) {
	start := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	clock := fakeclock.New(start)
	a := fake.NewDriver("a")
	b := fake.NewDriver("b")
	aCanceled := make(chan struct{}, 1)
	a.SetDefaultReadHandler(func(ctx context.Context, _ fake.ReadCall) ([]interfaces.Sample, interfaces.Outcome) {
		<-ctx.Done()
		aCanceled <- struct{}{}
		return nil, interfaces.OutcomeFailed
	})
	rec := newRecorder()
	s, err := scheduler.New(clock, discardLogger(), scheduler.Config{
		Links: []scheduler.Link{
			testLink("a", a, 2*time.Hour, testDevice("da", time.Minute, "p")),
			testLink("b", b, time.Second, testDevice("db", time.Minute, "p")),
		},
	}, rec)
	if err != nil {
		t.Fatal(err)
	}
	cancel, done := startRun(t, s)
	waitStarted(t, s)
	clock.Advance(time.Minute)
	info := waitTick(t, rec)
	if info.Link != "b" {
		t.Fatalf("first tick link = %q, want b", info.Link)
	}
	select {
	case <-aCanceled:
		t.Fatal("link a was canceled before test cancellation")
	default:
	}
	cancel()
	select {
	case <-aCanceled:
	case <-time.After(2 * time.Second):
		t.Fatal("link a did not observe cancellation")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return")
	}
}

func TestObserverConcurrentLinks(t *testing.T) {
	start := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	clock := fakeclock.New(start)
	obs := &barrierObserver{
		ready:   make(chan string, 2),
		release: make(chan struct{}),
		ticks:   make(chan scheduler.TickInfo, 2),
	}
	s, err := scheduler.New(clock, discardLogger(), scheduler.Config{
		Links: []scheduler.Link{
			testLink("a", fake.NewDriver("a"), time.Second, testDevice("da", time.Minute, "p")),
			testLink("b", fake.NewDriver("b"), time.Second, testDevice("db", time.Minute, "p")),
		},
	}, obs)
	if err != nil {
		t.Fatal(err)
	}
	cancel, done := startRun(t, s)
	waitStarted(t, s)
	clock.Advance(time.Minute)
	for range 2 {
		select {
		case <-obs.ready:
		case <-time.After(2 * time.Second):
			t.Fatal("observer calls did not overlap")
		}
	}
	close(obs.release)
	for range 2 {
		select {
		case <-obs.ticks:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for observer ticks")
		}
	}
	stopRun(t, cancel, done)
}

type barrierObserver struct {
	ready   chan string
	release chan struct{}
	ticks   chan scheduler.TickInfo
	mu      sync.Mutex
	seen    []string
}

func (o *barrierObserver) ObserveRead(result scheduler.ReadResult) {
	o.mu.Lock()
	o.seen = append(o.seen, result.Link)
	o.mu.Unlock()
	o.ready <- result.Link
	<-o.release
}

func (o *barrierObserver) ObserveTick(info scheduler.TickInfo) { o.ticks <- info }

func TestPushLinkIsNotPolled(t *testing.T) {
	start := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	clock := fakeclock.New(start)
	driver := fake.NewDriver("lorawan")
	driver.SetCapabilities(interfaces.Capabilities{Pollable: false})
	rec := newRecorder()
	s, err := scheduler.New(clock, discardLogger(), scheduler.Config{
		Links: []scheduler.Link{{ID: "l1", Driver: driver}},
	}, rec)
	if err != nil {
		t.Fatal(err)
	}
	cancel, done := startRun(t, s)
	waitStarted(t, s)
	clock.Advance(3 * time.Minute)
	if got := len(driver.Calls()); got != 0 {
		t.Fatalf("push link calls = %d, want 0", got)
	}
	if got := s.Stats()[0].Ticks; got != 0 {
		t.Fatalf("push link ticks = %d, want 0", got)
	}
	stopRun(t, cancel, done)
}

func TestStatsReadFailures(t *testing.T) {
	start := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	clock := fakeclock.New(start)
	driver := fake.NewDriver("modbus")
	var calls atomic.Int32
	driver.SetDefaultReadHandler(func(context.Context, fake.ReadCall) ([]interfaces.Sample, interfaces.Outcome) {
		switch calls.Add(1) {
		case 1:
			return []interfaces.Sample{{PointKey: "p", Quality: interfaces.QualityBad, Err: "boom"}}, interfaces.OutcomeFailed
		case 2:
			return []interfaces.Sample{{PointKey: "p", Quality: interfaces.QualityGood}}, interfaces.OutcomeOK
		default:
			return []interfaces.Sample{{PointKey: "p", Quality: interfaces.QualityBad, Err: "partial"}}, interfaces.OutcomePartial
		}
	})
	rec := newRecorder()
	s, err := scheduler.New(clock, discardLogger(), scheduler.Config{
		Links: []scheduler.Link{testLink("l1", driver, time.Second, testDevice("d1", time.Minute, "p"))},
	}, rec)
	if err != nil {
		t.Fatal(err)
	}
	cancel, done := startRun(t, s)
	waitStarted(t, s)

	clock.Advance(time.Minute)
	waitTick(t, rec)
	stats := s.Stats()[0]
	if stats.ReadFailures != 1 || stats.LastError != "boom" {
		t.Fatalf("after failure: failures=%d last=%q", stats.ReadFailures, stats.LastError)
	}

	clock.Advance(time.Minute)
	waitTick(t, rec)
	stats = s.Stats()[0]
	if stats.ReadFailures != 1 || stats.LastError != "" {
		t.Fatalf("after success: failures=%d last=%q", stats.ReadFailures, stats.LastError)
	}

	clock.Advance(time.Minute)
	waitTick(t, rec)
	stats = s.Stats()[0]
	if stats.ReadFailures != 1 || stats.LastError != "partial" {
		t.Fatalf("after partial: failures=%d last=%q", stats.ReadFailures, stats.LastError)
	}
	stopRun(t, cancel, done)
}

func TestCancelStopsScheduler(t *testing.T) {
	start := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	clock := fakeclock.New(start)
	driver := fake.NewDriver("modbus")
	started := make(chan struct{}, 1)
	canceled := make(chan struct{}, 1)
	driver.SetDefaultReadHandler(func(ctx context.Context, _ fake.ReadCall) ([]interfaces.Sample, interfaces.Outcome) {
		started <- struct{}{}
		<-ctx.Done()
		canceled <- struct{}{}
		return nil, interfaces.OutcomeFailed
	})
	rec := newRecorder()
	s, err := scheduler.New(clock, discardLogger(), scheduler.Config{
		Links: []scheduler.Link{testLink("l1", driver, 2*time.Hour, testDevice("d1", time.Minute, "p"))},
	}, rec)
	if err != nil {
		t.Fatal(err)
	}
	cancel, done := startRun(t, s)
	waitStarted(t, s)
	clock.Advance(time.Minute)
	waitSignal(t, started)
	cancel()
	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("driver did not observe cancellation")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return")
	}
	if got := clock.NumActive(); got != 0 {
		t.Fatalf("active fake-clock events = %d, want 0", got)
	}
}

func TestRunTwiceReturnsError(t *testing.T) {
	start := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	clock := fakeclock.New(start)
	driver := fake.NewDriver("modbus")
	s, err := scheduler.New(clock, discardLogger(), scheduler.Config{
		Links: []scheduler.Link{testLink("l1", driver, time.Second, testDevice("d1", time.Minute, "p"))},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	cancel, done := startRun(t, s)
	waitStarted(t, s)
	if err := s.Run(t.Context()); err == nil {
		t.Fatal("second Run returned nil")
	}
	stopRun(t, cancel, done)
}

func TestRunWithNoLinksBlocksUntilCancel(t *testing.T) {
	clock := fakeclock.New(time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC))
	s, err := scheduler.New(clock, discardLogger(), scheduler.Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	cancel, done := startRun(t, s)
	waitStarted(t, s)
	select {
	case err := <-done:
		t.Fatalf("Run returned early: %v", err)
	default:
	}
	stopRun(t, cancel, done)
}

func TestConfigAnchorOverridesNow(t *testing.T) {
	start := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	clock := fakeclock.New(start)
	driver := fake.NewDriver("modbus")
	rec := newRecorder()
	anchor := start.Add(-10 * time.Second)
	s, err := scheduler.New(clock, discardLogger(), scheduler.Config{
		Anchor: anchor,
		Links:  []scheduler.Link{testLink("l1", driver, time.Second, testDevice("d1", time.Minute, "p"))},
	}, rec)
	if err != nil {
		t.Fatal(err)
	}
	cancel, done := startRun(t, s)
	waitStarted(t, s)
	clock.Advance(50 * time.Second)
	info := waitTick(t, rec)
	want := start.Add(50 * time.Second)
	if !info.Planned.Equal(want) || info.Index != 1 {
		t.Fatalf("tick = %+v, want planned %s index 1", info, want)
	}
	stopRun(t, cancel, done)
}

func TestMissedTickCountsAndLogs(t *testing.T) {
	start := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	clock := fakeclock.New(start)
	driver := fake.NewDriver("modbus")
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	release := make(chan struct{})
	started := make(chan struct{}, 4)
	driver.SetDefaultReadHandler(func(ctx context.Context, _ fake.ReadCall) ([]interfaces.Sample, interfaces.Outcome) {
		started <- struct{}{}
		select {
		case <-release:
			return []interfaces.Sample{{PointKey: "p", Quality: interfaces.QualityGood, At: clock.Now()}}, interfaces.OutcomeOK
		case <-ctx.Done():
			return nil, interfaces.OutcomeFailed
		}
	})
	rec := newRecorder()
	s, err := scheduler.New(clock, logger, scheduler.Config{
		Links: []scheduler.Link{testLink("l1", driver, 150*time.Second, testDevice("d1", time.Minute, "p"))},
	}, rec)
	if err != nil {
		t.Fatal(err)
	}
	cancel, done := startRun(t, s)
	waitStarted(t, s)

	clock.Advance(time.Minute)
	waitSignal(t, started)
	clock.Advance(150*time.Second + time.Nanosecond)
	first := waitTick(t, rec)
	if !first.Planned.Equal(start.Add(time.Minute)) || first.Missed != 0 {
		t.Fatalf("first tick = %+v", first)
	}
	waitSignal(t, started)
	close(release)
	second := waitTick(t, rec)
	if !second.Planned.Equal(start.Add(3*time.Minute)) || second.Missed != 1 {
		t.Fatalf("second tick = %+v, want planned +3m missed 1", second)
	}
	stats := s.Stats()[0]
	if stats.MissedTicks != 1 {
		t.Fatalf("MissedTicks = %d, want 1", stats.MissedTicks)
	}
	stopRun(t, cancel, done)

	var missed []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("log line %q: %v", line, err)
		}
		if entry["event"] == "missed_tick" {
			missed = append(missed, entry)
		}
	}
	if len(missed) != 1 {
		t.Fatalf("missed_tick logs = %d, want 1 (%s)", len(missed), buf.String())
	}
	if got := missed[0]["link"]; got != "l1" {
		t.Fatalf("log link = %v, want l1", got)
	}
	if got := missed[0]["skipped"]; got != float64(1) {
		t.Fatalf("log skipped = %v, want 1", got)
	}
	if got := missed[0]["planned"]; got != start.Add(2*time.Minute).UTC().Format(time.RFC3339Nano) {
		t.Fatalf("log planned = %v, want %s", got, start.Add(2*time.Minute).UTC().Format(time.RFC3339Nano))
	}
	if got := missed[0]["missed_total"]; got != float64(1) {
		t.Fatalf("log missed_total = %v, want 1", got)
	}
}
