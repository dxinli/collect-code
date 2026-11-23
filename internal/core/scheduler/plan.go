package scheduler

import (
	"time"
)

// gcdDuration returns the greatest common divisor of a and b in nanoseconds.
// Non-positive inputs have no useful GCD and return zero.
func gcdDuration(a, b time.Duration) time.Duration {
	if a <= 0 || b <= 0 {
		return 0
	}
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// tickPeriod returns the GCD of every device period. Callers validate that
// periods are positive and that the result is at least one second.
func tickPeriod(devices []Device) time.Duration {
	if len(devices) == 0 {
		return 0
	}
	p := devices[0].Period
	for _, d := range devices[1:] {
		p = gcdDuration(p, d.Period)
	}
	return p
}

// plannedAt returns the k-th phase-locked plan time, k >= 1.
func plannedAt(anchor time.Time, period time.Duration, k uint64) time.Time {
	return anchor.Add(time.Duration(k) * period)
}

// dueDeviceIndexes returns the indexes of devices due at elapsed, the time
// since the scheduler anchor. A zero-due tick is valid.
func dueDeviceIndexes(devices []Device, elapsed time.Duration) []int {
	out := make([]int, 0, len(devices))
	for i, d := range devices {
		if d.Period > 0 && elapsed%d.Period == 0 {
			out = append(out, i)
		}
	}
	return out
}

// advance returns the newest plan time not older than now and the number of
// plan times skipped immediately before it. A late tick is skipped when now
// is at least one full period past planned, matching REQ-SCHED-01's
// "a period or more late" wording.
func advance(now, planned time.Time, period time.Duration) (time.Time, uint64) {
	if period <= 0 || now.Before(planned.Add(period)) {
		return planned, 0
	}
	delta := now.Sub(planned)
	missed := uint64(delta / period)
	return planned.Add(time.Duration(missed) * period), missed
}
