package framing_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/dxinli/collect-code/internal/drivers/modbus/framing"
	"github.com/dxinli/collect-code/internal/drivers/modbus/pdu"
)

// Covers: REQ-SCOPE-01
func TestMBAPEncodeGolden(t *testing.T) {
	tests := []struct {
		name  string
		frame framing.Frame
		want  []byte
	}{
		{
			name:  "read holding request",
			frame: framing.Frame{TransactionID: 1, UnitID: 1, PDU: []byte{0x03, 0x00, 0x00, 0x00, 0x02}},
			want:  []byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x06, 0x01, 0x03, 0x00, 0x00, 0x00, 0x02},
		},
		{
			name:  "read holding response",
			frame: framing.Frame{TransactionID: 1, UnitID: 1, PDU: []byte{0x03, 0x04, 0x00, 0x32, 0x00, 0x64}},
			want:  []byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x07, 0x01, 0x03, 0x04, 0x00, 0x32, 0x00, 0x64},
		},
		{
			name:  "exception",
			frame: framing.Frame{TransactionID: 1, UnitID: 1, PDU: []byte{0x83, 0x02}},
			want:  []byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x03, 0x01, 0x83, 0x02},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := framing.NewMBAP().Encode(tt.frame)
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("Encode = % X, want % X", got, tt.want)
			}
		})
	}
}

// Covers: REQ-SCOPE-01
func TestMBAPDecodeRequestGolden(t *testing.T) {
	raw := []byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x06, 0x01, 0x03, 0x00, 0x00, 0x00, 0x02}
	frame, err := framing.NewMBAP().DecodeRequest(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("DecodeRequest: %v", err)
	}
	if frame.TransactionID != 1 || frame.UnitID != 1 || !bytes.Equal(frame.PDU, []byte{0x03, 0x00, 0x00, 0x00, 0x02}) {
		t.Fatalf("frame = %#v", frame)
	}
}

// Covers: REQ-SCOPE-01
func TestMBAPDecodeResponseGolden(t *testing.T) {
	raw := []byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x07, 0x01, 0x03, 0x04, 0x00, 0x32, 0x00, 0x64}
	req := &pdu.ReadRegistersRequest{Func: pdu.FuncReadHoldingRegisters, Quantity: 2}
	frame, err := framing.NewMBAP().DecodeResponse(bytes.NewReader(raw), req, 1)
	if err != nil {
		t.Fatalf("DecodeResponse: %v", err)
	}
	if frame.TransactionID != 1 || frame.UnitID != 1 {
		t.Fatalf("frame = %#v", frame)
	}
	if _, err := pdu.DecodeResponse(frame.PDU); err != nil {
		t.Fatalf("PDU DecodeResponse: %v", err)
	}
}

// Covers: REQ-SCOPE-01
func TestMBAPRejectsBadProtocolID(t *testing.T) {
	raw := []byte{0x00, 0x01, 0x00, 0x01, 0x00, 0x06, 0x01, 0x03, 0x00, 0x00, 0x00, 0x02}
	_, err := framing.NewMBAP().DecodeRequest(bytes.NewReader(raw))
	if !errors.Is(err, framing.ErrMalformedFrame) || !errors.Is(err, pdu.ErrProtocol) {
		t.Fatalf("err = %v, want malformed protocol error", err)
	}
	if errors.Is(err, framing.ErrUnrecoverable) {
		t.Fatalf("err = %v, want recoverable", err)
	}
}

// Covers: REQ-SCOPE-01
func TestMBAPRejectsBadLength(t *testing.T) {
	for _, length := range []uint16{0, 1, 255} {
		raw := []byte{0x00, 0x01, 0x00, 0x00, byte(length >> 8), byte(length), 0x01}
		_, err := framing.NewMBAP().DecodeRequest(bytes.NewReader(raw))
		if !errors.Is(err, framing.ErrUnrecoverable) {
			t.Fatalf("length %d err = %v, want ErrUnrecoverable", length, err)
		}
	}
}

// Covers: REQ-SCOPE-01
func TestMBAPRejectsTransactionMismatch(t *testing.T) {
	raw := []byte{0x00, 0x02, 0x00, 0x00, 0x00, 0x06, 0x01, 0x03, 0x00, 0x00, 0x00, 0x02}
	req := &pdu.ReadRegistersRequest{Func: pdu.FuncReadHoldingRegisters, Quantity: 2}
	_, err := framing.NewMBAP().DecodeResponse(bytes.NewReader(raw), req, 1)
	if !errors.Is(err, framing.ErrTransactionMismatch) || !errors.Is(err, pdu.ErrProtocol) {
		t.Fatalf("err = %v, want transaction mismatch", err)
	}
	if errors.Is(err, framing.ErrUnrecoverable) {
		t.Fatalf("err = %v, want recoverable", err)
	}
}

// Covers: REQ-SCOPE-01
func TestMBAPRecoverableErrorKeepsStreamAligned(t *testing.T) {
	bad := []byte{0x00, 0x01, 0x00, 0x01, 0x00, 0x06, 0x01, 0x03, 0x00, 0x00, 0x00, 0x02}
	good := []byte{0x00, 0x02, 0x00, 0x00, 0x00, 0x06, 0x01, 0x03, 0x00, 0x00, 0x00, 0x02}
	reader := bytes.NewReader(append(bad, good...))
	framer := framing.NewMBAP()
	if _, err := framer.DecodeRequest(reader); !errors.Is(err, framing.ErrMalformedFrame) {
		t.Fatalf("first err = %v, want malformed", err)
	}
	frame, err := framer.DecodeRequest(reader)
	if err != nil {
		t.Fatalf("second DecodeRequest: %v", err)
	}
	if frame.TransactionID != 2 {
		t.Fatalf("second transaction = %d, want 2", frame.TransactionID)
	}
}

// Covers: REQ-SCOPE-01
func TestMBAPInvalidLengthIsUnrecoverable(t *testing.T) {
	raw := []byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x01}
	_, err := framing.NewMBAP().DecodeRequest(bytes.NewReader(raw))
	if !errors.Is(err, framing.ErrUnrecoverable) {
		t.Fatalf("err = %v, want unrecoverable", err)
	}
}

// Covers: REQ-SCOPE-01
func TestMBAPTruncatedHeaderAndBody(t *testing.T) {
	framer := framing.NewMBAP()
	if _, err := framer.DecodeRequest(bytes.NewReader(nil)); !errors.Is(err, io.EOF) {
		t.Fatalf("empty err = %v, want io.EOF", err)
	}
	if _, err := framer.DecodeRequest(bytes.NewReader([]byte{0x00, 0x01, 0x00})); !errors.Is(err, framing.ErrUnrecoverable) {
		t.Fatalf("short header err = %v, want unrecoverable", err)
	}
	header := []byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x06, 0x01, 0x03}
	_, err := framer.DecodeRequest(bytes.NewReader(header))
	if !errors.Is(err, framing.ErrUnrecoverable) || !errors.Is(err, framing.ErrShortFrame) {
		t.Fatalf("short body err = %v, want unrecoverable short frame", err)
	}
}

// Covers: REQ-SCOPE-01
func TestMBAPEncodeRejectsBadPDULength(t *testing.T) {
	framer := framing.NewMBAP()
	if _, err := framer.Encode(framing.Frame{}); !errors.Is(err, framing.ErrMalformedFrame) {
		t.Fatalf("empty PDU err = %v, want malformed", err)
	}
	tooLong := make([]byte, pdu.MaxPDUSize+1)
	if _, err := framer.Encode(framing.Frame{PDU: tooLong}); !errors.Is(err, framing.ErrMalformedFrame) {
		t.Fatalf("long PDU err = %v, want malformed", err)
	}
}
