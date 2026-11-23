package scheduler

import (
	"context"
	"testing"
	"time"
)

func TestGCDDuration(t *testing.T) {
	tests := []struct {
		name string
		a, b time.Duration
		want time.Duration
	}{
		{"60/90", 60 * time.Second, 90 * time.Second, 30 * time.Second},
		{"5/60", 5 * time.Second, 60 * time.Second, 5 * time.Second},
		{"7/11", 7 * time.Second, 11 * time.Second, time.Second},
		{"same", 15 * time.Second, 15 * time.Second, 15 * time.Second},
		{"zero", 0, time.Second, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := gcdDuration(tc.a, tc.b); got != tc.want {
				t.Fatalf("gcdDuration(%s, %s) = %s, want %s", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestTickPeriod(t *testing.T) {
	devices := []Device{
		{Period: 60 * time.Second},
		{Period: 90 * time.Second},
	}
	if got := tickPeriod(devices); got != 30*time.Second {
		t.Fatalf("tickPeriod = %s, want 30s", got)
	}
	if got := tickPeriod(nil); got != 0 {
		t.Fatalf("tickPeriod(nil) = %s, want 0", got)
	}
}

func TestDueDeviceIndexes(t *testing.T) {
	tests := []struct {
		name    string
		periods []time.Duration
		elapsed time.Duration
		want    []int
	}{
		{"60/90 at 30s", []time.Duration{60 * time.Second, 90 * time.Second}, 30 * time.Second, nil},
		{"60/90 at 60s", []time.Duration{60 * time.Second, 90 * time.Second}, 60 * time.Second, []int{0}},
		{"60/90 at 90s", []time.Duration{60 * time.Second, 90 * time.Second}, 90 * time.Second, []int{1}},
		{"60/90 at 120s", []time.Duration{60 * time.Second, 90 * time.Second}, 120 * time.Second, []int{0}},
		{"60/90 at 150s", []time.Duration{60 * time.Second, 90 * time.Second}, 150 * time.Second, nil},
		{"60/90 at 180s", []time.Duration{60 * time.Second, 90 * time.Second}, 180 * time.Second, []int{0, 1}},
		{"7/11 at 6s", []time.Duration{7 * time.Second, 11 * time.Second}, 6 * time.Second, nil},
		{"7/11 at 7s", []time.Duration{7 * time.Second, 11 * time.Second}, 7 * time.Second, []int{0}},
		{"7/11 at 11s", []time.Duration{7 * time.Second, 11 * time.Second}, 11 * time.Second, []int{1}},
		{"7/11 at 77s", []time.Duration{7 * time.Second, 11 * time.Second}, 77 * time.Second, []int{0, 1}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			devices := make([]Device, len(tc.periods))
			for i, p := range tc.periods {
				devices[i] = Device{ID: string(rune('a' + i)), Period: p}
			}
			got := dueDeviceIndexes(devices, tc.elapsed)
			if len(got) != len(tc.want) {
				t.Fatalf("dueDeviceIndexes(%s) = %v, want %v", tc.elapsed, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("dueDeviceIndexes(%s) = %v, want %v", tc.elapsed, got, tc.want)
				}
			}
		})
	}
}

func TestPlannedTimesDoNotAccumulate(t *testing.T) {
	anchor := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	const period = 60 * time.Second
	for k := uint64(1); k <= 1440; k++ {
		want := anchor.Add(time.Duration(k) * period)
		if got := plannedAt(anchor, period, k); !got.Equal(want) {
			t.Fatalf("plannedAt(%d) = %s, want %s", k, got, want)
		}
		if k > 1 {
			prev := plannedAt(anchor, period, k-1)
			if gap := plannedAt(anchor, period, k).Sub(prev); gap != period {
				t.Fatalf("gap at %d = %s, want %s", k, gap, period)
			}
		}
	}
}

func TestMissedTickBoundary(t *testing.T) {
	planned := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	const period = 60 * time.Second
	tests := []struct {
		name       string
		now        time.Time
		wantRunAt  time.Time
		wantMissed uint64
	}{
		{"P-1ns", planned.Add(period - time.Nanosecond), planned, 0},
		{"on time", planned, planned, 0},
		{"P", planned.Add(period), planned.Add(period), 1},
		{"P+1ns", planned.Add(period + time.Nanosecond), planned.Add(period), 1},
		{"2P+1ns", planned.Add(2*period + time.Nanosecond), planned.Add(2 * period), 2},
		{"2.5P", planned.Add(5 * period / 2), planned.Add(2 * period), 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runAt, missed := advance(tc.now, planned, period)
			if !runAt.Equal(tc.wantRunAt) || missed != tc.wantMissed {
				t.Fatalf("advance(%s) = (%s, %d), want (%s, %d)", tc.now, runAt, missed, tc.wantRunAt, tc.wantMissed)
			}
		})
	}
}

func TestAdvanceWhenOnTime(t *testing.T) {
	planned := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	runAt, missed := advance(planned, planned, time.Minute)
	if !runAt.Equal(planned) || missed != 0 {
		t.Fatalf("advance(on time) = (%s, %d), want (%s, 0)", runAt, missed, planned)
	}
}

func TestMissedTickSkipsBacklog(t *testing.T) {
	planned := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	const period = time.Minute
	runAt, missed := advance(planned.Add(5*period/2), planned, period)
	if missed != 2 {
		t.Fatalf("missed = %d, want 2", missed)
	}
	if want := planned.Add(2 * period); !runAt.Equal(want) {
		t.Fatalf("runAt = %s, want %s", runAt, want)
	}
}

func TestNormalizeContext(t *testing.T) {
	var nilContext context.Context
	if normalizeContext(nilContext) == nil {
		t.Fatal("normalizeContext(nil) = nil")
	}
	ctx := t.Context()
	if got := normalizeContext(ctx); got != ctx {
		t.Fatal("normalizeContext changed a non-nil context")
	}
}
