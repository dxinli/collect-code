package interfaces

import (
	"context"
	"time"
)

// Driver is one southbound protocol implementation.
//
// Read reports observations and an overall Outcome; per-point failures are
// carried in Sample.Err. Write returns a handle that correlates later
// asynchronous events through DriverEvent.Ref. Link and device failures are
// reported through the sentinel errors in this package so the kernel can map
// them to service-reply codes without importing a concrete driver.
type Driver interface {
	Name() string
	Open(ctx context.Context) error
	Close() error
	Capabilities() Capabilities
	Read(ctx context.Context, req ReadRequest) ([]Sample, Outcome)
	Write(ctx context.Context, req WriteRequest) (CommandHandle, error)
	Events() <-chan DriverEvent // nil for pull-only drivers
	Status() LinkStatus
}

// ReadRequest asks for one device's observations.
type ReadRequest struct {
	Device   string
	Points   []string  // empty means all readable points for the device
	Deadline time.Time // zero means no per-call deadline; derived from Clock
}

// PointWrite is one point/value pair in a write request.
type PointWrite struct {
	PointKey string
	Value    any
}

// WriteRequest asks for one service invocation's writes.
type WriteRequest struct {
	Device    string
	ServiceID string // service that owns this write; required by LoRa queue merging (15a)
	Confirmed bool   // service.confirmed; Modbus ignores it, LoRa maps to queueItem.confirmed
	Writes    []PointWrite
	Deadline  time.Time // zero means no per-call deadline; derived from Clock
}

// CommandHandle is the driver's correlation id for an asynchronous command.
type CommandHandle struct {
	Ref string // opaque driver-assigned correlation id, echoed in DriverEvent.Ref
}

// DriverEvent is a push event from a driver.
type DriverEvent struct {
	Kind    EventKind
	Device  string
	Ref     string
	State   CommandState
	Sample  []Sample
	Payload []byte
	FPort   int // LoRa uplink port; 0 for non-LoRa events
	At      time.Time
}
