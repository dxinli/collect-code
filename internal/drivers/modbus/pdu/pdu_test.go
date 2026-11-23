package pdu_test

import (
	"errors"
	"testing"

	"github.com/dxinli/collect-code/internal/drivers/modbus/pdu"
)

// Covers: REQ-SCOPE-01
func TestPDUQuantityBoundaries(t *testing.T) {
	for _, quantity := range []uint16{1, pdu.MaxReadRegisters} {
		req := &pdu.ReadRegistersRequest{Func: pdu.FuncReadHoldingRegisters, Quantity: quantity}
		if _, err := pdu.EncodeRequest(req); err != nil {
			t.Fatalf("read quantity %d: %v", quantity, err)
		}
	}
	for _, quantity := range []uint16{0, pdu.MaxReadRegisters + 1} {
		req := &pdu.ReadRegistersRequest{Func: pdu.FuncReadHoldingRegisters, Quantity: quantity}
		if _, err := pdu.EncodeRequest(req); !errors.Is(err, pdu.ErrInvalidQuantity) {
			t.Fatalf("read quantity %d: err = %v, want ErrInvalidQuantity", quantity, err)
		}
	}
	for _, quantity := range []int{1, pdu.MaxWriteRegisters} {
		req := &pdu.WriteMultipleRegistersRequest{Values: make([]uint16, quantity)}
		if _, err := pdu.EncodeRequest(req); err != nil {
			t.Fatalf("write quantity %d: %v", quantity, err)
		}
	}
	for _, quantity := range []int{0, pdu.MaxWriteRegisters + 1} {
		req := &pdu.WriteMultipleRegistersRequest{Values: make([]uint16, quantity)}
		if _, err := pdu.EncodeRequest(req); !errors.Is(err, pdu.ErrInvalidQuantity) {
			t.Fatalf("write quantity %d: err = %v, want ErrInvalidQuantity", quantity, err)
		}
	}
}

// Covers: REQ-SCOPE-01
func TestPDUDecodeQuantityBoundariesAreProtocolErrors(t *testing.T) {
	_, err := pdu.DecodeRequest([]byte{0x03, 0x00, 0x00, 0x00, 0x00})
	if !errors.Is(err, pdu.ErrProtocol) || !errors.Is(err, pdu.ErrInvalidQuantity) {
		t.Fatalf("zero quantity err = %v, want protocol and invalid-quantity", err)
	}
	_, err = pdu.DecodeRequest([]byte{0x03, 0x00, 0x00, 0x00, 0x7E})
	if !errors.Is(err, pdu.ErrProtocol) || !errors.Is(err, pdu.ErrInvalidQuantity) {
		t.Fatalf("126 quantity err = %v, want protocol and invalid-quantity", err)
	}
	_, err = pdu.DecodeRequest([]byte{0x10, 0x00, 0x00, 0x00, 0x00, 0x00})
	if !errors.Is(err, pdu.ErrProtocol) || !errors.Is(err, pdu.ErrInvalidQuantity) {
		t.Fatalf("zero write quantity err = %v, want protocol and invalid-quantity", err)
	}
}

// Covers: REQ-SCOPE-01
func TestPDUAddressOverflow(t *testing.T) {
	_, err := pdu.EncodeRequest(&pdu.ReadRegistersRequest{
		Func: pdu.FuncReadHoldingRegisters, Address: 0xFFFF, Quantity: 2,
	})
	if !errors.Is(err, pdu.ErrInvalidAddress) {
		t.Fatalf("read overflow err = %v, want ErrInvalidAddress", err)
	}
	_, err = pdu.EncodeRequest(&pdu.WriteMultipleRegistersRequest{
		Address: 0xFFFF, Values: []uint16{1, 2},
	})
	if !errors.Is(err, pdu.ErrInvalidAddress) {
		t.Fatalf("write overflow err = %v, want ErrInvalidAddress", err)
	}
	_, err = pdu.DecodeRequest([]byte{0x03, 0xFF, 0xFF, 0x00, 0x02})
	if !errors.Is(err, pdu.ErrProtocol) || !errors.Is(err, pdu.ErrInvalidAddress) {
		t.Fatalf("decode overflow err = %v, want protocol and invalid-address", err)
	}
}

// Covers: REQ-SCOPE-01
func TestPDURejectsUnknownFunction(t *testing.T) {
	for _, fn := range []byte{0x01, 0x05, 0x0F, 0x11} {
		raw := []byte{fn, 0x00, 0x00, 0x00, 0x01}
		if _, err := pdu.DecodeRequest(raw); !errors.Is(err, pdu.ErrUnsupportedFunction) {
			t.Fatalf("function 0x%02X err = %v, want ErrUnsupportedFunction", fn, err)
		}
		if _, err := pdu.DecodeResponse(raw); !errors.Is(err, pdu.ErrUnsupportedFunction) {
			t.Fatalf("response function 0x%02X err = %v, want ErrUnsupportedFunction", fn, err)
		}
	}
}

// Covers: REQ-SCOPE-01
func TestEncodeRequestRejectsWrongReadFunc(t *testing.T) {
	_, err := pdu.EncodeRequest(&pdu.ReadRegistersRequest{
		Func: pdu.FuncWriteSingleRegister, Quantity: 1,
	})
	if !errors.Is(err, pdu.ErrUnsupportedFunction) {
		t.Fatalf("err = %v, want ErrUnsupportedFunction", err)
	}
}

// Covers: REQ-SCOPE-01
func TestPDUMalformedRequests(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
	}{
		{"empty", nil},
		{"truncated read", []byte{0x03, 0x00, 0x00}},
		{"read trailing byte", []byte{0x03, 0x00, 0x00, 0x00, 0x01, 0x00}},
		{"write single truncated", []byte{0x06, 0x00, 0x00}},
		{"write multiple byte count mismatch", []byte{0x10, 0x00, 0x00, 0x00, 0x02, 0x02, 0x00, 0x01}},
		{"write multiple odd byte count", []byte{0x10, 0x00, 0x00, 0x00, 0x01, 0x03, 0x00, 0x01, 0x00}},
		{"write multiple truncated", []byte{0x10, 0x00, 0x00, 0x00, 0x02, 0x04, 0x00, 0x01}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := pdu.DecodeRequest(tt.raw); !errors.Is(err, pdu.ErrMalformed) {
				t.Fatalf("DecodeRequest err = %v, want ErrMalformed", err)
			}
		})
	}
}

// Covers: REQ-SCOPE-01
func TestPDUMalformedResponses(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
	}{
		{"empty", nil},
		{"read odd byte count", []byte{0x03, 0x03, 0x00, 0x01, 0x00}},
		{"read trailing byte", []byte{0x03, 0x02, 0x00, 0x01, 0x00}},
		{"exception trailing byte", []byte{0x83, 0x02, 0x00}},
		{"write single truncated", []byte{0x06, 0x00, 0x00}},
		{"write multiple zero quantity", []byte{0x10, 0x00, 0x00, 0x00, 0x00}},
		{"write multiple bad quantity", []byte{0x10, 0x00, 0x00, 0x00, 0x7C}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := pdu.DecodeResponse(tt.raw); !errors.Is(err, pdu.ErrMalformed) {
				t.Fatalf("DecodeResponse err = %v, want ErrMalformed", err)
			}
		})
	}
}

// Covers: REQ-SCOPE-01
func TestPDUEncodedLengthLimit(t *testing.T) {
	resp := &pdu.ReadRegistersResponse{
		Func:      pdu.FuncReadHoldingRegisters,
		Registers: make([]uint16, pdu.MaxReadRegisters),
	}
	raw, err := pdu.EncodeResponse(resp)
	if err != nil {
		t.Fatalf("EncodeResponse: %v", err)
	}
	if len(raw) > pdu.MaxPDUSize {
		t.Fatalf("encoded length %d > %d", len(raw), pdu.MaxPDUSize)
	}
	req := &pdu.WriteMultipleRegistersRequest{Values: make([]uint16, pdu.MaxWriteRegisters)}
	raw, err = pdu.EncodeRequest(req)
	if err != nil {
		t.Fatalf("EncodeRequest: %v", err)
	}
	if len(raw) > pdu.MaxPDUSize {
		t.Fatalf("encoded length %d > %d", len(raw), pdu.MaxPDUSize)
	}
}

// Covers: REQ-SCOPE-01, REQ-INVOKE-05
func TestExceptionClassification(t *testing.T) {
	rejected := []pdu.ExceptionCode{
		pdu.ExceptionIllegalFunction,
		pdu.ExceptionIllegalDataAddress,
		pdu.ExceptionIllegalDataValue,
	}
	for _, code := range rejected {
		if code.Class() != pdu.ExceptionClassRejected {
			t.Fatalf("code 0x%02X class = %v, want rejected", uint8(code), code.Class())
		}
	}
	internal := []pdu.ExceptionCode{
		pdu.ExceptionServerDeviceFailure,
		pdu.ExceptionAcknowledge,
		pdu.ExceptionServerDeviceBusy,
		pdu.ExceptionMemoryParity,
		pdu.ExceptionGatewayPathUnavailable,
		pdu.ExceptionGatewayTargetFailed,
		0x00,
		0x7F,
	}
	for _, code := range internal {
		if code.Class() != pdu.ExceptionClassInternal {
			t.Fatalf("code 0x%02X class = %v, want internal", uint8(code), code.Class())
		}
	}
}

// Covers: REQ-SCOPE-01
func TestValidateResponse(t *testing.T) {
	read := &pdu.ReadRegistersRequest{Func: pdu.FuncReadHoldingRegisters, Quantity: 2}
	if err := pdu.ValidateResponse(read, &pdu.ReadRegistersResponse{
		Func: pdu.FuncReadHoldingRegisters, Registers: []uint16{1, 2},
	}); err != nil {
		t.Fatalf("matching read response: %v", err)
	}
	if err := pdu.ValidateResponse(read, &pdu.ReadRegistersResponse{
		Func: pdu.FuncReadHoldingRegisters, Registers: []uint16{1},
	}); !errors.Is(err, pdu.ErrMalformed) {
		t.Fatalf("short read response err = %v, want ErrMalformed", err)
	}
	if err := pdu.ValidateResponse(read, &pdu.ExceptionResponse{
		Func: pdu.FuncReadHoldingRegisters, Code: pdu.ExceptionIllegalDataAddress,
	}); err != nil {
		t.Fatalf("matching exception: %v", err)
	}
	if err := pdu.ValidateResponse(read, &pdu.ExceptionResponse{
		Func: pdu.FuncReadInputRegisters, Code: pdu.ExceptionIllegalDataAddress,
	}); !errors.Is(err, pdu.ErrMalformed) {
		t.Fatalf("wrong exception function err = %v, want ErrMalformed", err)
	}
	single := &pdu.WriteSingleRegisterRequest{Address: 1, Value: 2}
	if err := pdu.ValidateResponse(single, &pdu.WriteSingleRegisterResponse{Address: 1, Value: 3}); !errors.Is(err, pdu.ErrMalformed) {
		t.Fatalf("write single echo err = %v, want ErrMalformed", err)
	}
	multiple := &pdu.WriteMultipleRegistersRequest{Address: 1, Values: []uint16{1, 2}}
	if err := pdu.ValidateResponse(multiple, &pdu.WriteMultipleRegistersResponse{Address: 1, Quantity: 2}); err != nil {
		t.Fatalf("matching write multiple: %v", err)
	}
}

// Covers: REQ-SCOPE-01
func TestExceptionResponseErrorText(t *testing.T) {
	err := (&pdu.ExceptionResponse{Func: pdu.FuncReadHoldingRegisters, Code: pdu.ExceptionIllegalDataAddress}).Error()
	if err == "" {
		t.Fatal("ExceptionResponse.Error() is empty")
	}
}
