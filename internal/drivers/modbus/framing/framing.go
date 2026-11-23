package framing

import (
	"fmt"
	"io"

	"github.com/dxinli/collect-code/internal/drivers/modbus/pdu"
)

// Frame is one transport-independent Modbus ADU.
type Frame struct {
	TransactionID uint16 // MBAP only; RTU ignores it
	UnitID        uint8
	PDU           []byte
}

// Framer encodes and decodes one framing. The Client chooses a Framer; the PDU
// codec never sees the framing.
type Framer interface {
	Name() string
	// PDUErrorsAreRecoverable reports whether a PDU decode or echo validation
	// error leaves the stream aligned. MBAP returns true because a complete ADU
	// was consumed by its length field; RTU returns false.
	PDUErrorsAreRecoverable() bool
	Encode(f Frame) ([]byte, error)
	DecodeRequest(r io.Reader) (Frame, error)
	DecodeResponse(r io.Reader, req pdu.Request, expectTxn uint16) (Frame, error)
}

func validateFrame(f Frame) error {
	if len(f.PDU) < 1 || len(f.PDU) > pdu.MaxPDUSize {
		return fmt.Errorf("%w: PDU length %d", ErrMalformedFrame, len(f.PDU))
	}
	return nil
}
