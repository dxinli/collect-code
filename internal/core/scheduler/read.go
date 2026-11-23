package scheduler

import (
	"context"
	"time"

	"github.com/dxinli/collect-code/internal/interfaces"
)

type readOutcome struct {
	samples []interfaces.Sample
	outcome interfaces.Outcome
	at      time.Time
}

// readDevice performs one device read with a hard deadline. The deadline is
// strictly greater: the device may finish at D, and only D+1ns or later is a
// timeout. The scheduler never waits for a driver goroutine it abandoned.
func (s *Scheduler) readDevice(ctx context.Context, l *linkPlan, dev Device, planned time.Time) (ReadResult, bool) {
	started := s.clock.Now()
	deadline := started.Add(l.readDeadline)
	readCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	timer := s.clock.NewTimer(l.readDeadline + deadlineSlack)
	defer func() { timer.Stop() }()

	done := make(chan readOutcome, 1)
	go func() {
		samples, outcome := l.driver.Read(readCtx, interfaces.ReadRequest{
			Device:   dev.ID,
			Points:   append([]string(nil), dev.Points...),
			Deadline: deadline,
		})
		done <- readOutcome{samples: samples, outcome: outcome, at: s.clock.Now()}
	}()

	select {
	case <-ctx.Done():
		return ReadResult{}, false
	case r := <-done:
		return s.resultOrTimeout(l, dev, planned, started, deadline, r), true
	case <-timer.C():
		select {
		case r := <-done:
			return s.resultOrTimeout(l, dev, planned, started, deadline, r), true
		default:
			return s.timeoutResult(l, dev, planned, started, s.clock.Now()), true
		}
	}
}

func (s *Scheduler) resultOrTimeout(l *linkPlan, dev Device, planned, started, deadline time.Time, r readOutcome) ReadResult {
	if r.at.After(deadline) {
		return s.timeoutResult(l, dev, planned, started, r.at)
	}
	return ReadResult{
		Link:       l.id,
		Device:     dev.ID,
		Planned:    planned,
		Points:     append([]string(nil), dev.Points...),
		Samples:    append([]interfaces.Sample(nil), r.samples...),
		Outcome:    r.outcome,
		StartedAt:  started,
		FinishedAt: r.at,
	}
}

func (s *Scheduler) timeoutResult(l *linkPlan, dev Device, planned, started, finished time.Time) ReadResult {
	samples := make([]interfaces.Sample, 0, len(dev.Points))
	for _, point := range dev.Points {
		samples = append(samples, interfaces.Sample{
			PointKey: point,
			Quality:  interfaces.QualityBad,
			At:       finished,
			Err:      "read deadline exceeded",
		})
	}
	return ReadResult{
		Link:       l.id,
		Device:     dev.ID,
		Planned:    planned,
		Points:     append([]string(nil), dev.Points...),
		Samples:    samples,
		Outcome:    interfaces.OutcomeFailed,
		TimedOut:   true,
		StartedAt:  started,
		FinishedAt: finished,
	}
}
