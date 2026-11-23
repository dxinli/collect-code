package pdu

import (
	"encoding/binary"
	"fmt"
)

// DecodeRequest validates and decodes one request PDU.
func DecodeRequest(raw []byte) (Request, error) {
	if len(raw) < 1 || len(raw) > MaxPDUSize {
		return nil, fmt.Errorf("%w: length %d", ErrMalformed, len(raw))
	}
	fn := FuncCode(raw[0])
	if fn&ExceptionFlag != 0 {
		return nil, fmt.Errorf("%w: exception flag in request", ErrMalformed)
	}
	switch fn {
	case FuncReadHoldingRegisters, FuncReadInputRegisters:
		if len(raw) != 5 {
			return nil, fmt.Errorf("%w: read request length %d", ErrMalformed, len(raw))
		}
		address := binary.BigEndian.Uint16(raw[1:3])
		quantity := binary.BigEndian.Uint16(raw[3:5])
		if err := validateReadRange(address, quantity); err != nil {
			return nil, malformed(err)
		}
		return &ReadRegistersRequest{Func: fn, Address: address, Quantity: quantity}, nil
	case FuncWriteSingleRegister:
		if len(raw) != 5 {
			return nil, fmt.Errorf("%w: write single length %d", ErrMalformed, len(raw))
		}
		return &WriteSingleRegisterRequest{
			Address: binary.BigEndian.Uint16(raw[1:3]),
			Value:   binary.BigEndian.Uint16(raw[3:5]),
		}, nil
	case FuncWriteMultipleRegisters:
		return decodeWriteMultipleRequest(raw)
	default:
		return nil, fmt.Errorf("%w: 0x%02X", ErrUnsupportedFunction, raw[0])
	}
}

func decodeWriteMultipleRequest(raw []byte) (Request, error) {
	if len(raw) < 6 {
		return nil, fmt.Errorf("%w: write multiple length %d", ErrMalformed, len(raw))
	}
	address := binary.BigEndian.Uint16(raw[1:3])
	quantity := binary.BigEndian.Uint16(raw[3:5])
	byteCount := int(raw[5])
	if quantity < 1 || quantity > MaxWriteRegisters {
		return nil, malformed(fmt.Errorf("%w: %d", ErrInvalidQuantity, quantity))
	}
	if byteCount%2 != 0 || byteCount != 2*int(quantity) {
		return nil, fmt.Errorf("%w: byte count %d for quantity %d", ErrMalformed, byteCount, quantity)
	}
	if len(raw) != 6+byteCount {
		return nil, fmt.Errorf("%w: write multiple length %d byte count %d", ErrMalformed, len(raw), byteCount)
	}
	if err := validateAddressRange(address, quantity); err != nil {
		return nil, malformed(err)
	}
	values := make([]uint16, quantity)
	for i := range values {
		values[i] = binary.BigEndian.Uint16(raw[6+2*i:])
	}
	return &WriteMultipleRegistersRequest{Address: address, Values: values}, nil
}

// DecodeResponse validates and decodes one response PDU.
func DecodeResponse(raw []byte) (Response, error) {
	if len(raw) < 1 || len(raw) > MaxPDUSize {
		return nil, fmt.Errorf("%w: length %d", ErrMalformed, len(raw))
	}
	if raw[0]&ExceptionFlag != 0 {
		if len(raw) != 2 {
			return nil, fmt.Errorf("%w: exception length %d", ErrMalformed, len(raw))
		}
		return &ExceptionResponse{
			Func: FuncCode(raw[0] &^ ExceptionFlag),
			Code: ExceptionCode(raw[1]),
		}, nil
	}
	switch FuncCode(raw[0]) {
	case FuncReadHoldingRegisters, FuncReadInputRegisters:
		return decodeReadRegistersResponse(raw)
	case FuncWriteSingleRegister:
		if len(raw) != 5 {
			return nil, fmt.Errorf("%w: write single length %d", ErrMalformed, len(raw))
		}
		return &WriteSingleRegisterResponse{
			Address: binary.BigEndian.Uint16(raw[1:3]),
			Value:   binary.BigEndian.Uint16(raw[3:5]),
		}, nil
	case FuncWriteMultipleRegisters:
		if len(raw) != 5 {
			return nil, fmt.Errorf("%w: write multiple length %d", ErrMalformed, len(raw))
		}
		address := binary.BigEndian.Uint16(raw[1:3])
		quantity := binary.BigEndian.Uint16(raw[3:5])
		if err := validateWriteRange(address, int(quantity)); err != nil {
			return nil, malformed(err)
		}
		return &WriteMultipleRegistersResponse{Address: address, Quantity: quantity}, nil
	default:
		return nil, fmt.Errorf("%w: 0x%02X", ErrUnsupportedFunction, raw[0])
	}
}

func decodeReadRegistersResponse(raw []byte) (Response, error) {
	if len(raw) < 2 {
		return nil, fmt.Errorf("%w: read response length %d", ErrMalformed, len(raw))
	}
	byteCount := int(raw[1])
	if byteCount < 2 || byteCount > 2*MaxReadRegisters || byteCount%2 != 0 {
		return nil, fmt.Errorf("%w: byte count %d", ErrMalformed, byteCount)
	}
	if len(raw) != 2+byteCount {
		return nil, fmt.Errorf("%w: read response length %d byte count %d", ErrMalformed, len(raw), byteCount)
	}
	registers := make([]uint16, byteCount/2)
	for i := range registers {
		registers[i] = binary.BigEndian.Uint16(raw[2+2*i:])
	}
	return &ReadRegistersResponse{Func: FuncCode(raw[0]), Registers: registers}, nil
}

// ValidateResponse checks that a response matches its request. An exception
// response is a valid response: it must reference the same base function.
func ValidateResponse(req Request, resp Response) error {
	if req == nil || resp == nil {
		return fmt.Errorf("%w: nil request or response", ErrArgument)
	}
	if exception, ok := resp.(*ExceptionResponse); ok {
		if exception.Func != req.Function() {
			return fmt.Errorf("%w: exception function 0x%02X for request 0x%02X",
				ErrMalformed, uint8(exception.Func), uint8(req.Function()))
		}
		return nil
	}
	if resp.Function() != req.Function() {
		return fmt.Errorf("%w: response function 0x%02X for request 0x%02X",
			ErrMalformed, uint8(resp.Function()), uint8(req.Function()))
	}
	switch r := req.(type) {
	case *ReadRegistersRequest:
		response, ok := resp.(*ReadRegistersResponse)
		if !ok {
			return fmt.Errorf("%w: read response type %T", ErrMalformed, resp)
		}
		if len(response.Registers) != int(r.Quantity) {
			return fmt.Errorf("%w: got %d registers, want %d",
				ErrMalformed, len(response.Registers), r.Quantity)
		}
	case *WriteSingleRegisterRequest:
		response, ok := resp.(*WriteSingleRegisterResponse)
		if !ok {
			return fmt.Errorf("%w: write single response type %T", ErrMalformed, resp)
		}
		if response.Address != r.Address || response.Value != r.Value {
			return fmt.Errorf("%w: write single echo mismatch", ErrMalformed)
		}
	case *WriteMultipleRegistersRequest:
		response, ok := resp.(*WriteMultipleRegistersResponse)
		if !ok {
			return fmt.Errorf("%w: write multiple response type %T", ErrMalformed, resp)
		}
		if response.Address != r.Address || response.Quantity != uint16(len(r.Values)) {
			return fmt.Errorf("%w: write multiple echo mismatch", ErrMalformed)
		}
	default:
		return fmt.Errorf("%w: request type %T", ErrArgument, req)
	}
	return nil
}
