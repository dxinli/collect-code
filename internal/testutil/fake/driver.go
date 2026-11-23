package fake

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dxinli/collect-code/internal/interfaces"
)

// ReadCall is one recorded Driver.Read request.
type ReadCall struct {
	Device   string
	Points   []string
	Deadline time.Time
}

// ReadHandler answers one Read request. Tests may block it to exercise
// deadline and cancellation paths.
type ReadHandler func(context.Context, ReadCall) ([]interfaces.Sample, interfaces.Outcome)

// Driver is a scriptable interfaces.Driver for tests. It records every Read,
// exposes call starts without blocking, and defaults to a pollable link.
type Driver struct {
	mu             sync.Mutex
	name           string
	capabilities   interfaces.Capabilities
	status         interfaces.LinkStatus
	calls          []ReadCall
	handlers       map[string]ReadHandler
	defaultHandler ReadHandler
	started        chan ReadCall
}

// NewDriver returns a pollable fake driver with an OK default handler.
func NewDriver(name string) *Driver {
	return &Driver{
		name:         name,
		capabilities: interfaces.Capabilities{Pollable: true},
		handlers:     make(map[string]ReadHandler),
		started:      make(chan ReadCall, 64),
	}
}

// Name returns the driver name.
func (d *Driver) Name() string { return d.name }

// Open is a no-op.
func (d *Driver) Open(context.Context) error { return nil }

// Close is a no-op.
func (d *Driver) Close() error { return nil }

// Capabilities returns the configured capabilities.
func (d *Driver) Capabilities() interfaces.Capabilities {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.capabilities
}

// SetCapabilities replaces the reported capabilities.
func (d *Driver) SetCapabilities(c interfaces.Capabilities) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.capabilities = c
}

// Read records the request and invokes the per-device or default handler.
func (d *Driver) Read(ctx context.Context, req interfaces.ReadRequest) ([]interfaces.Sample, interfaces.Outcome) {
	call := ReadCall{
		Device:   req.Device,
		Points:   append([]string(nil), req.Points...),
		Deadline: req.Deadline,
	}
	d.mu.Lock()
	d.calls = append(d.calls, call)
	handler := d.handlers[req.Device]
	if handler == nil {
		handler = d.defaultHandler
	}
	d.mu.Unlock()

	select {
	case d.started <- call:
	default:
	}
	if handler == nil {
		return nil, interfaces.OutcomeOK
	}
	return handler(ctx, call)
}

// Write reports unsupported.
func (d *Driver) Write(context.Context, interfaces.WriteRequest) (interfaces.CommandHandle, error) {
	return interfaces.CommandHandle{}, fmt.Errorf("fake driver: %w", interfaces.ErrUnsupported)
}

// Events is nil for this pull-only fake.
func (d *Driver) Events() <-chan interfaces.DriverEvent { return nil }

// Status returns the configured link status.
func (d *Driver) Status() interfaces.LinkStatus {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.status
}

// SetStatus replaces the reported link status.
func (d *Driver) SetStatus(st interfaces.LinkStatus) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.status = st
}

// SetReadHandler scripts one device. A nil handler removes the override.
func (d *Driver) SetReadHandler(device string, handler ReadHandler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if handler == nil {
		delete(d.handlers, device)
		return
	}
	d.handlers[device] = handler
}

// SetDefaultReadHandler scripts devices without a per-device handler.
func (d *Driver) SetDefaultReadHandler(handler ReadHandler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.defaultHandler = handler
}

// Calls returns a copy of every recorded read request.
func (d *Driver) Calls() []ReadCall {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]ReadCall, len(d.calls))
	for i, call := range d.calls {
		out[i] = call
		out[i].Points = append([]string(nil), call.Points...)
	}
	return out
}

// Started reports read starts without blocking. A slow test may lose events
// when the buffer fills; use Calls for the complete record.
func (d *Driver) Started() <-chan ReadCall { return d.started }
