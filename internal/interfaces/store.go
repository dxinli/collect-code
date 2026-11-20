package interfaces

import (
	"context"
	"time"
)

// RecordPhase is the two-phase state of one idempotency record.
type RecordPhase string

const (
	PhaseAccepted RecordPhase = "accepted"
	PhaseFinal    RecordPhase = "final"
)

// AcceptedRecord is written before a command's side effect is attempted.
type AcceptedRecord struct {
	Device    string
	Seq       uint64
	ServiceID string
	At        time.Time
}

// FinalRecord completes a previously accepted record.
type FinalRecord struct {
	Device string
	Seq    uint64
	Code   int
	Msg    string
	Data   map[string]any
	At     time.Time
}

// Record is the stored idempotency entry.
type Record struct {
	Device     string
	Seq        uint64
	ServiceID  string
	Phase      RecordPhase
	AcceptedAt time.Time
	FinalAt    time.Time
	Code       int
	Msg        string
	Data       map[string]any
}

// DeviceState is the persisted per-device runtime state.
type DeviceState struct {
	Device        string
	Online        bool
	LastSuccessAt time.Time
	LastSeq       uint64
	UpdatedAt     time.Time
}

// Store is the kernel's persistence seam.
//
// Frozen semantics (verified by the 02b contract tests):
//
//   - BeginAccepted returns ErrAlreadyAccepted and does not overwrite when
//     (device, seq) already exists in any phase.
//   - Finish returns ErrRecordNotFound for an unknown record and
//     ErrAlreadyFinal when the record is already final.
//   - Lookup reports found=false with a nil error for an unknown record.
//   - LastSeq is a monotonic high-water mark of accepted seq values. It
//     starts at 0, never moves backwards, and survives DeleteExpired.
//   - LoadDevices returns an empty slice and nil error for an empty store.
//   - SaveDevices upserts by device and never deletes absent devices; core
//     reconciliation (17b) owns removal.
//   - LoadSnapshot reports found=false for an unknown device and returns a
//     copy otherwise; SaveSnapshot atomically replaces one device snapshot.
//   - DeleteExpired removes only final records whose FinalAt is before the
//     cutoff, returns the number removed, and never removes accepted records.
//   - Every method accepts a context and returns promptly when it is done.
type Store interface {
	BeginAccepted(ctx context.Context, rec AcceptedRecord) error
	Finish(ctx context.Context, rec FinalRecord) error
	Lookup(ctx context.Context, device string, seq uint64) (Record, bool, error)
	LastSeq(ctx context.Context, device string) (uint64, error)
	LoadDevices(ctx context.Context) ([]DeviceState, error)
	SaveDevices(ctx context.Context, states []DeviceState) error
	LoadSnapshot(ctx context.Context, device string) ([]Property, bool, error)
	SaveSnapshot(ctx context.Context, device string, props []Property) error
	DeleteExpired(ctx context.Context, before time.Time) (int, error)
	Close() error
}
