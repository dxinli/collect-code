package fake

import (
	"sync"
	"time"

	"github.com/dxinli/collect-code/internal/interfaces"
)

// UplinkAdapter records reports and returns deterministic messages.
type UplinkAdapter struct {
	mu      sync.Mutex
	reports []interfaces.PropertyReport
	events  []interfaces.EventReport
	decoded []interfaces.Sample
	err     error
}

// NewUplinkAdapter returns an empty fake.
func NewUplinkAdapter() *UplinkAdapter { return &UplinkAdapter{} }

// PropertyReport records report and returns a message keyed by its MsgID.
func (a *UplinkAdapter) PropertyReport(report interfaces.PropertyReport) (interfaces.Message, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.err != nil {
		return interfaces.Message{}, a.err
	}
	a.reports = append(a.reports, report)
	return interfaces.Message{Topic: "uplink/property", Payload: []byte(report.MsgID)}, nil
}

// EventReport records report and returns a message keyed by its MsgID.
func (a *UplinkAdapter) EventReport(report interfaces.EventReport) (interfaces.Message, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.err != nil {
		return interfaces.Message{}, a.err
	}
	a.events = append(a.events, report)
	return interfaces.Message{Topic: "uplink/event", Payload: []byte(report.MsgID)}, nil
}

// DecodePayload returns the configured samples stamped with at.
func (a *UplinkAdapter) DecodePayload(_ string, _ []byte, _ int, at time.Time) ([]interfaces.Sample, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.err != nil {
		return nil, a.err
	}
	out := make([]interfaces.Sample, len(a.decoded))
	copy(out, a.decoded)
	for i := range out {
		out[i].At = at
	}
	return out, nil
}

// Reports returns a copy of the recorded property reports.
func (a *UplinkAdapter) Reports() []interfaces.PropertyReport {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]interfaces.PropertyReport(nil), a.reports...)
}

// Events returns a copy of the recorded event reports.
func (a *UplinkAdapter) Events() []interfaces.EventReport {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]interfaces.EventReport(nil), a.events...)
}

// SetDecoded configures what DecodePayload returns.
func (a *UplinkAdapter) SetDecoded(samples []interfaces.Sample) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.decoded = append([]interfaces.Sample(nil), samples...)
}

// SetError makes every method fail with err; nil clears it.
func (a *UplinkAdapter) SetError(err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.err = err
}
