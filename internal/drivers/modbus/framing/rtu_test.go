package framing_test

import (
	"bytes"
	"errors"
	"testing"
	"testing/iotest"

	"github.com/dxinli/collect-code/internal/drivers/modbus/framing"
	"github.com/dxinli/collect-code/internal/drivers/modbus/pdu"
)

var (
	readHoldingRequest = &pdu.ReadRegistersRequest{Func: pdu.FuncReadHoldingRegisters, Quantity: 2}
	readInputRequest   = &pdu.ReadRegistersRequest{Func: pdu.FuncReadInputRegisters, Quantity: 1}
	writeSingleRequest = &pdu.WriteSingleRegisterRequest{Address: 2, Value: 50}
	writeMultiRequest  = &pdu.WriteMultipleRegistersRequest{Address: 2, Values: []uint16{50, 100}}
)

// Covers: REQ-SCOPE-01
func TestRTUEncodeGolden(t *testing.T) {
	tests := []struct {
		name  string
		frame framing.Frame
		want  []byte
	}{
		{"read holding", framing.Frame{UnitID: 1, PDU: []byte{0x03, 0x00, 0x00, 0x00, 0x02}}, []byte{0x01, 0x03, 0x00, 0x00, 0x00, 0x02, 0xC4, 0x0B}},
		{"read input", framing.Frame{UnitID: 1, PDU: []byte{0x04, 0x00, 0x00, 0x00, 0x01}}, []byte{0x01, 0x04, 0x00, 0x00, 0x00, 0x01, 0x31, 0xCA}},
		{"write single", framing.Frame{UnitID: 1, PDU: []byte{0x06, 0x00, 0x02, 0x00, 0x32}}, []byte{0x01, 0x06, 0x00, 0x02, 0x00, 0x32, 0xA9, 0xDF}},
		{"write multiple", framing.Frame{UnitID: 1, PDU: []byte{0x10, 0x00, 0x02, 0x00, 0x02, 0x04, 0x00, 0x32, 0x00, 0x64}}, []byte{0x01, 0x10, 0x00, 0x02, 0x00, 0x02, 0x04, 0x00, 0x32, 0x00, 0x64, 0xD2, 0x52}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := framing.NewRTU().Encode(tt.frame)
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
func TestRTUDecodeRequestGolden(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want []byte
	}{
		{"read holding", []byte{0x01, 0x03, 0x00, 0x00, 0x00, 0x02, 0xC4, 0x0B}, []byte{0x03, 0x00, 0x00, 0x00, 0x02}},
		{"read input", []byte{0x01, 0x04, 0x00, 0x00, 0x00, 0x01, 0x31, 0xCA}, []byte{0x04, 0x00, 0x00, 0x00, 0x01}},
		{"write single", []byte{0x01, 0x06, 0x00, 0x02, 0x00, 0x32, 0xA9, 0xDF}, []byte{0x06, 0x00, 0x02, 0x00, 0x32}},
		{"write multiple", []byte{0x01, 0x10, 0x00, 0x02, 0x00, 0x02, 0x04, 0x00, 0x32, 0x00, 0x64, 0xD2, 0x52}, []byte{0x10, 0x00, 0x02, 0x00, 0x02, 0x04, 0x00, 0x32, 0x00, 0x64}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame, err := framing.NewRTU().DecodeRequest(bytes.NewReader(tt.raw))
			if err != nil {
				t.Fatalf("DecodeRequest: %v", err)
			}
			if frame.UnitID != 1 || !bytes.Equal(frame.PDU, tt.want) {
				t.Fatalf("frame = %#v, want unit 1 PDU % X", frame, tt.want)
			}
		})
	}
}

// Covers: REQ-SCOPE-01
func TestRTUDecodeResponseGolden(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		req  pdu.Request
		want []byte
	}{
		{"read holding", []byte{0x01, 0x03, 0x04, 0x00, 0x32, 0x00, 0x64, 0x5A, 0x17}, readHoldingRequest, []byte{0x03, 0x04, 0x00, 0x32, 0x00, 0x64}},
		{"read input", []byte{0x01, 0x04, 0x02, 0xFF, 0xFF, 0xB8, 0x80}, readInputRequest, []byte{0x04, 0x02, 0xFF, 0xFF}},
		{"write single", []byte{0x01, 0x06, 0x00, 0x02, 0x00, 0x32, 0xA9, 0xDF}, writeSingleRequest, []byte{0x06, 0x00, 0x02, 0x00, 0x32}},
		{"write multiple", []byte{0x01, 0x10, 0x00, 0x02, 0x00, 0x02, 0xE0, 0x08}, writeMultiRequest, []byte{0x10, 0x00, 0x02, 0x00, 0x02}},
		{"exception", []byte{0x01, 0x83, 0x02, 0xC0, 0xF1}, readHoldingRequest, []byte{0x83, 0x02}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame, err := framing.NewRTU().DecodeResponse(bytes.NewReader(tt.raw), tt.req, 0)
			if err != nil {
				t.Fatalf("DecodeResponse: %v", err)
			}
			if frame.UnitID != 1 || !bytes.Equal(frame.PDU, tt.want) {
				t.Fatalf("frame = %#v, want unit 1 PDU % X", frame, tt.want)
			}
		})
	}
}

// Covers: REQ-SCOPE-01
func TestRTUDecodeFragmented(t *testing.T) {
	raw := []byte{0x01, 0x03, 0x00, 0x00, 0x00, 0x02, 0xC4, 0x0B}
	frame, err := framing.NewRTU().DecodeRequest(iotest.OneByteReader(bytes.NewReader(raw)))
	if err != nil {
		t.Fatalf("DecodeRequest: %v", err)
	}
	if !bytes.Equal(frame.PDU, []byte{0x03, 0x00, 0x00, 0x00, 0x02}) {
		t.Fatalf("PDU = % X", frame.PDU)
	}
}

// Covers: REQ-SCOPE-01
func TestRTUDecodeCoalesced(t *testing.T) {
	first := []byte{0x01, 0x03, 0x00, 0x00, 0x00, 0x02, 0xC4, 0x0B}
	second := []byte{0x01, 0x03, 0x00, 0x00, 0x00, 0x0A, 0xC5, 0xCD}
	reader := bytes.NewReader(append(first, second...))
	framer := framing.NewRTU()
	for _, want := range [][]byte{
		{0x03, 0x00, 0x00, 0x00, 0x02},
		{0x03, 0x00, 0x00, 0x00, 0x0A},
	} {
		frame, err := framer.DecodeRequest(reader)
		if err != nil {
			t.Fatalf("DecodeRequest: %v", err)
		}
		if !bytes.Equal(frame.PDU, want) {
			t.Fatalf("PDU = % X, want % X", frame.PDU, want)
		}
	}
}

// Covers: REQ-SCOPE-01
func TestRTURejectsBadCRC(t *testing.T) {
	raw := []byte{0x01, 0x03, 0x00, 0x00, 0x00, 0x02, 0xC4, 0x0C}
	_, err := framing.NewRTU().DecodeRequest(bytes.NewReader(raw))
	if !errors.Is(err, framing.ErrBadCRC) || !errors.Is(err, pdu.ErrProtocol) || !errors.Is(err, framing.ErrUnrecoverable) {
		t.Fatalf("err = %v, want bad CRC, protocol and unrecoverable", err)
	}
}

// Covers: REQ-SCOPE-01
func TestRTURejectsShortFrame(t *testing.T) {
	raw := []byte{0x01, 0x03, 0x00, 0x00, 0x00, 0x02, 0xC4}
	_, err := framing.NewRTU().DecodeRequest(bytes.NewReader(raw))
	if !errors.Is(err, framing.ErrShortFrame) || !errors.Is(err, framing.ErrUnrecoverable) {
		t.Fatalf("err = %v, want short frame and unrecoverable", err)
	}
}

// Covers: REQ-SCOPE-01
func TestRTURejectsFunctionMismatch(t *testing.T) {
	raw := []byte{0x01, 0x04, 0x02, 0xFF, 0xFF, 0xB8, 0x80}
	_, err := framing.NewRTU().DecodeResponse(bytes.NewReader(raw), readHoldingRequest, 0)
	if !errors.Is(err, framing.ErrMalformedFrame) || !errors.Is(err, framing.ErrUnrecoverable) {
		t.Fatalf("err = %v, want malformed and unrecoverable", err)
	}
}

// Covers: REQ-SCOPE-01
func TestRTURejectsOversizedByteCount(t *testing.T) {
	raw := []byte{0x01, 0x10, 0x00, 0x02, 0x00, 0x01, 0xFF}
	_, err := framing.NewRTU().DecodeRequest(bytes.NewReader(raw))
	if !errors.Is(err, framing.ErrUnrecoverable) {
		t.Fatalf("err = %v, want unrecoverable", err)
	}
}

// Covers: REQ-SCOPE-01
func TestRTURejectsWrongResponseQuantity(t *testing.T) {
	raw := []byte{0x01, 0x03, 0x02, 0xFF, 0xFF, 0xB8, 0x80}
	_, err := framing.NewRTU().DecodeResponse(bytes.NewReader(raw), readHoldingRequest, 0)
	if !errors.Is(err, framing.ErrUnrecoverable) {
		t.Fatalf("err = %v, want unrecoverable", err)
	}
}

// Covers: REQ-SCOPE-01
func TestRTUEncodeIgnoresTransactionID(t *testing.T) {
	framer := framing.NewRTU()
	first, err := framer.Encode(framing.Frame{TransactionID: 1, UnitID: 1, PDU: []byte{0x03, 0x00, 0x00, 0x00, 0x02}})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	second, err := framer.Encode(framing.Frame{TransactionID: 9, UnitID: 1, PDU: []byte{0x03, 0x00, 0x00, 0x00, 0x02}})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("transaction id changed RTU bytes: % X vs % X", first, second)
	}
}

// Covers: REQ-SCOPE-01
func TestRTUEncodeRejectsBadPDULength(t *testing.T) {
	framer := framing.NewRTU()
	if _, err := framer.Encode(framing.Frame{}); !errors.Is(err, framing.ErrMalformedFrame) {
		t.Fatalf("empty PDU err = %v, want malformed", err)
	}
	tooLong := make([]byte, pdu.MaxPDUSize+1)
	if _, err := framer.Encode(framing.Frame{PDU: tooLong}); !errors.Is(err, framing.ErrMalformedFrame) {
		t.Fatalf("long PDU err = %v, want malformed", err)
	}
}
