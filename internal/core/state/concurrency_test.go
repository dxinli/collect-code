// Covers: REQ-QUALITY-03
package state_test

import (
	"sync"
	"testing"
	"time"

	"github.com/dxinli/collect-code/internal/core/scheduler"
	"github.com/dxinli/collect-code/internal/interfaces"
)

func TestConcurrentObserveAndSweep(t *testing.T) {
	cfg := defaultConfig()
	cfg.OfflineAfter = 1
	cfg.Devices[0].Period = time.Second
	for i := range cfg.Devices[0].Points {
		cfg.Devices[0].Points[i].StaleAfter = time.Second
	}
	h := newHarness(t, cfg)

	const workers = 8
	const iterations = 100
	var wg sync.WaitGroup
	wg.Add(workers)
	for worker := range workers {
		go func(worker int) {
			defer wg.Done()
			for i := range iterations {
				value := float64(worker*iterations + i)
				h.tr.ObserveRead(scheduler.ReadResult{
					Device:  "dev-1",
					Outcome: interfaces.OutcomeOK,
					Samples: []interfaces.Sample{
						sample("a", value, h.clock.Now()),
						sample("b", value, h.clock.Now()),
					},
				})
				h.tr.ObserveEvent(interfaces.DriverEvent{
					Kind:   interfaces.EventUplink,
					Device: "dev-1",
					Sample: []interfaces.Sample{sample("a", value, h.clock.Now())},
				})
				h.tr.ObserveEvent(interfaces.DriverEvent{
					Kind:   interfaces.EventAck,
					Device: "dev-1",
				})
				h.tr.Sweep()
				_, _ = h.tr.Snapshot("dev-1")
			}
		}(worker)
	}
	wg.Wait()

	snap := h.snapshot()
	if len(snap.Properties) != 2 {
		t.Fatalf("snapshot has %d props, want 2", len(snap.Properties))
	}
}
