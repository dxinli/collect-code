package framing

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/dxinli/collect-code/internal/drivers/modbus/pdu"
)

type rtu struct{}

// NewRTU returns the serial / RTU-over-TCP framing.
func NewRTU() Framer { return rtu{} }

func (rtu) Name() string { return "rtu" }

func (rtu) PDUErrorsAreRecoverable() bool { return false }

func (rtu) Encode(f Frame) ([]byte, error) {
	if err := validateFrame(f); err != nil {
		return nil, err
	}
	raw := make([]byte, 1+len(f.PDU))
	raw[0] = f.UnitID
	copy(raw[1:], f.PDU)
	return appendCRC(raw), nil
}

func (rtu) DecodeRequest(r io.Reader) (Frame, error) {
	var head [2]byte
	if err := readFull(r, head[:]); err != nil {
		return Frame{}, err
	}
	unitID := head[0]
	fn := pdu.FuncCode(head[1])
	switch fn {
	case pdu.FuncReadHoldingRegisters, pdu.FuncReadInputRegisters, pdu.FuncWriteSingleRegister:
		rest := make([]byte, 6) // address/value, quantity/value, CRC
		if err := rtuReadRest(r, rest); err != nil {
			return Frame{}, err
		}
		raw := joinBytes(head[:], rest)
		if err := checkCRC(raw); err != nil {
			return Frame{}, err
		}
		return Frame{UnitID: unitID, PDU: raw[1:6]}, nil
	case pdu.FuncWriteMultipleRegisters:
		var header [5]byte // address, quantity, byte count
		if err := rtuReadRest(r, header[:]); err != nil {
			return Frame{}, err
		}
		address := binary.BigEndian.Uint16(header[0:2])
		quantity := binary.BigEndian.Uint16(header[2:4])
		byteCount := int(header[4])
		if quantity < 1 || quantity > pdu.MaxWriteRegisters {
			return Frame{}, unrecoverable(fmt.Errorf("%w: quantity %d", ErrMalformedFrame, quantity))
		}
		if byteCount < 2 || byteCount%2 != 0 || byteCount > 2*pdu.MaxWriteRegisters || byteCount != 2*int(quantity) {
			return Frame{}, unrecoverable(fmt.Errorf("%w: byte count %d for quantity %d", ErrMalformedFrame, byteCount, quantity))
		}
		if uint32(address)+uint32(quantity)-1 > 0xFFFF {
			return Frame{}, unrecoverable(fmt.Errorf("%w: address 0x%04X quantity %d", ErrMalformedFrame, address, quantity))
		}
		rest := make([]byte, byteCount+2)
		if err := rtuReadRest(r, rest); err != nil {
			return Frame{}, err
		}
		raw := joinBytes(head[:], joinBytes(header[:], rest))
		if err := checkCRC(raw); err != nil {
			return Frame{}, err
		}
		return Frame{UnitID: unitID, PDU: raw[1 : len(raw)-2]}, nil
	default:
		return Frame{}, unrecoverable(fmt.Errorf("%w: 0x%02X", pdu.ErrUnsupportedFunction, head[1]))
	}
}

func (rtu) DecodeResponse(r io.Reader, req pdu.Request, _ uint16) (Frame, error) {
	if req == nil {
		return Frame{}, fmt.Errorf("%w: nil request", pdu.ErrArgument)
	}
	var head [2]byte
	if err := readFull(r, head[:]); err != nil {
		return Frame{}, err
	}
	unitID := head[0]
	fn := pdu.FuncCode(head[1])
	reqFn := req.Function()
	if fn == reqFn|pdu.ExceptionFlag {
		rest := make([]byte, 3) // exception code, CRC
		if err := rtuReadRest(r, rest); err != nil {
			return Frame{}, err
		}
		raw := joinBytes(head[:], rest)
		if err := checkCRC(raw); err != nil {
			return Frame{}, err
		}
		return Frame{UnitID: unitID, PDU: raw[1:3]}, nil
	}
	if fn != reqFn {
		return Frame{}, unrecoverable(fmt.Errorf("%w: function 0x%02X want 0x%02X",
			ErrMalformedFrame, head[1], uint8(reqFn)))
	}
	switch fn {
	case pdu.FuncReadHoldingRegisters, pdu.FuncReadInputRegisters:
		return decodeRTUReadResponse(r, req, head[:], unitID)
	case pdu.FuncWriteSingleRegister, pdu.FuncWriteMultipleRegisters:
		rest := make([]byte, 6) // address, value/quantity, CRC
		if err := rtuReadRest(r, rest); err != nil {
			return Frame{}, err
		}
		raw := joinBytes(head[:], rest)
		if err := checkCRC(raw); err != nil {
			return Frame{}, err
		}
		return Frame{UnitID: unitID, PDU: raw[1:6]}, nil
	default:
		return Frame{}, unrecoverable(fmt.Errorf("%w: 0x%02X", pdu.ErrUnsupportedFunction, head[1]))
	}
}

func decodeRTUReadResponse(r io.Reader, req pdu.Request, head []byte, unitID uint8) (Frame, error) {
	request, ok := req.(*pdu.ReadRegistersRequest)
	if !ok {
		return Frame{}, fmt.Errorf("%w: read request type %T", pdu.ErrArgument, req)
	}
	var byteCountBuf [1]byte
	if err := rtuReadRest(r, byteCountBuf[:]); err != nil {
		return Frame{}, err
	}
	byteCount := int(byteCountBuf[0])
	if request.Quantity < 1 || request.Quantity > pdu.MaxReadRegisters {
		return Frame{}, unrecoverable(fmt.Errorf("%w: quantity %d", ErrMalformedFrame, request.Quantity))
	}
	if byteCount < 2 || byteCount > 2*pdu.MaxReadRegisters || byteCount%2 != 0 || byteCount != 2*int(request.Quantity) {
		return Frame{}, unrecoverable(fmt.Errorf("%w: byte count %d for quantity %d",
			ErrMalformedFrame, byteCount, request.Quantity))
	}
	rest := make([]byte, byteCount+2)
	if err := rtuReadRest(r, rest); err != nil {
		return Frame{}, err
	}
	raw := joinBytes(head, joinBytes(byteCountBuf[:], rest))
	if err := checkCRC(raw); err != nil {
		return Frame{}, err
	}
	return Frame{UnitID: unitID, PDU: raw[1 : len(raw)-2]}, nil
}

func rtuReadRest(r io.Reader, buf []byte) error {
	if err := readFull(r, buf); err != nil {
		if errors.Is(err, io.EOF) {
			return unrecoverable(errors.Join(ErrShortFrame, io.EOF))
		}
		return unrecoverable(err)
	}
	return nil
}

func joinBytes(parts ...[]byte) []byte {
	size := 0
	for _, part := range parts {
		size += len(part)
	}
	raw := make([]byte, 0, size)
	for _, part := range parts {
		raw = append(raw, part...)
	}
	return raw
}
