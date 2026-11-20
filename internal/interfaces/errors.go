package interfaces

import "errors"

// Sentinel errors shared across the seams. Implementations wrap them with
// fmt.Errorf("...: %w", err) so callers can classify failures with errors.Is.
var (
	// Driver write errors; 09 maps them to the service-reply code table:
	// ErrLinkDown -> 5040, ErrDeviceRejected -> 4001,
	// ErrDeviceNoResponse/ErrDeviceInternal -> 5044, anything else -> 4001.
	// 5043 is produced by 09 after a read-back, never by Write.
	ErrLinkDown         = errors.New("interfaces: link down")
	ErrDeviceNoResponse = errors.New("interfaces: device did not respond")
	ErrDeviceRejected   = errors.New("interfaces: device rejected the write")
	ErrDeviceInternal   = errors.New("interfaces: device reported an internal fault")

	// Channel lifecycle errors.
	ErrNotConnected = errors.New("interfaces: not connected")
	ErrClosed       = errors.New("interfaces: closed")
	ErrUnsupported  = errors.New("interfaces: unsupported")

	// Store errors.
	ErrRecordNotFound  = errors.New("interfaces: store record not found")
	ErrAlreadyAccepted = errors.New("interfaces: store record already accepted")
	ErrAlreadyFinal    = errors.New("interfaces: store record already final")
)
