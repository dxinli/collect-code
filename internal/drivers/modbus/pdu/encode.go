package pdu

import (
	"encoding/binary"
	"fmt"
)

// EncodeRequest validates and encodes one request PDU.
func EncodeRequest(req Request) ([]byte, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: nil request", ErrArgument)
	}
	raw, err := req.encode()
	if err != nil {
		return nil, err
	}
	return checkEncoded(raw)
}

// EncodeResponse validates and encodes one response PDU.
func EncodeResponse(resp Response) ([]byte, error) {
	if resp == nil {
		return nil, fmt.Errorf("%w: nil response", ErrArgument)
	}
	raw, err := resp.encode()
	if err != nil {
		return nil, err
	}
	return checkEncoded(raw)
}

func checkEncoded(raw []byte) ([]byte, error) {
	if len(raw) == 0 || len(raw) > MaxPDUSize {
		return nil, fmt.Errorf("%w: encoded length %d", ErrMalformed, len(raw))
	}
	return raw, nil
}

func (r *ReadRegistersRequest) encode() ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("%w: nil read request", ErrArgument)
	}
	if r.Func != FuncReadHoldingRegisters && r.Func != FuncReadInputRegisters {
		return nil, fmt.Errorf("%w: 0x%02X", ErrUnsupportedFunction, uint8(r.Func))
	}
	if err := validateReadRange(r.Address, r.Quantity); err != nil {
		return nil, err
	}
	raw := make([]byte, 5)
	raw[0] = byte(r.Func)
	binary.BigEndian.PutUint16(raw[1:3], r.Address)
	binary.BigEndian.PutUint16(raw[3:5], r.Quantity)
	return raw, nil
}

func (r *ReadRegistersResponse) encode() ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("%w: nil read response", ErrArgument)
	}
	if r.Func != FuncReadHoldingRegisters && r.Func != FuncReadInputRegisters {
		return nil, fmt.Errorf("%w: 0x%02X", ErrUnsupportedFunction, uint8(r.Func))
	}
	if len(r.Registers) < 1 || len(r.Registers) > MaxReadRegisters {
		return nil, fmt.Errorf("%w: %d registers", ErrInvalidQuantity, len(r.Registers))
	}
	raw := make([]byte, 2+2*len(r.Registers))
	raw[0] = byte(r.Func)
	raw[1] = byte(2 * len(r.Registers))
	for i, value := range r.Registers {
		binary.BigEndian.PutUint16(raw[2+2*i:], value)
	}
	return raw, nil
}

func (r *WriteSingleRegisterRequest) encode() ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("%w: nil write request", ErrArgument)
	}
	raw := make([]byte, 5)
	raw[0] = byte(FuncWriteSingleRegister)
	binary.BigEndian.PutUint16(raw[1:3], r.Address)
	binary.BigEndian.PutUint16(raw[3:5], r.Value)
	return raw, nil
}

func (r *WriteSingleRegisterResponse) encode() ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("%w: nil write response", ErrArgument)
	}
	raw := make([]byte, 5)
	raw[0] = byte(FuncWriteSingleRegister)
	binary.BigEndian.PutUint16(raw[1:3], r.Address)
	binary.BigEndian.PutUint16(raw[3:5], r.Value)
	return raw, nil
}

func (r *WriteMultipleRegistersRequest) encode() ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("%w: nil write request", ErrArgument)
	}
	if err := validateWriteRange(r.Address, len(r.Values)); err != nil {
		return nil, err
	}
	raw := make([]byte, 6+2*len(r.Values))
	raw[0] = byte(FuncWriteMultipleRegisters)
	binary.BigEndian.PutUint16(raw[1:3], r.Address)
	binary.BigEndian.PutUint16(raw[3:5], uint16(len(r.Values)))
	raw[5] = byte(2 * len(r.Values))
	for i, value := range r.Values {
		binary.BigEndian.PutUint16(raw[6+2*i:], value)
	}
	return raw, nil
}

func (r *WriteMultipleRegistersResponse) encode() ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("%w: nil write response", ErrArgument)
	}
	if err := validateWriteRange(r.Address, int(r.Quantity)); err != nil {
		return nil, err
	}
	raw := make([]byte, 5)
	raw[0] = byte(FuncWriteMultipleRegisters)
	binary.BigEndian.PutUint16(raw[1:3], r.Address)
	binary.BigEndian.PutUint16(raw[3:5], r.Quantity)
	return raw, nil
}

func (e *ExceptionResponse) encode() ([]byte, error) {
	if e == nil {
		return nil, fmt.Errorf("%w: nil exception response", ErrArgument)
	}
	if e.Func&ExceptionFlag != 0 {
		return nil, fmt.Errorf("%w: exception response carries the flag", ErrMalformed)
	}
	return []byte{byte(e.Func) | ExceptionFlag, byte(e.Code)}, nil
}

func validateReadRange(address, quantity uint16) error {
	if quantity < 1 || quantity > MaxReadRegisters {
		return fmt.Errorf("%w: %d", ErrInvalidQuantity, quantity)
	}
	return validateAddressRange(address, quantity)
}

func validateWriteRange(address uint16, quantity int) error {
	if quantity < 1 || quantity > MaxWriteRegisters {
		return fmt.Errorf("%w: %d", ErrInvalidQuantity, quantity)
	}
	return validateAddressRange(address, uint16(quantity))
}

func validateAddressRange(address, quantity uint16) error {
	end := uint32(address) + uint32(quantity) - 1
	if end > 0xFFFF {
		return fmt.Errorf("%w: address 0x%04X quantity %d", ErrInvalidAddress, address, quantity)
	}
	return nil
}
