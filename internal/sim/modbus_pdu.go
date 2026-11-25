package sim

import (
	"encoding/binary"
	"fmt"
)

const (
	modbusFuncReadHolding   = 0x03
	modbusFuncReadInput     = 0x04
	modbusFuncWriteSingle   = 0x06
	modbusFuncWriteMultiple = 0x10

	modbusExceptionIllegalFunction    = 0x01
	modbusExceptionIllegalDataAddress = 0x02
	modbusExceptionIllegalDataValue   = 0x03

	modbusMaxReadRegisters  = 125
	modbusMaxWriteRegisters = 123
)

// modbusRequest is one parsed request PDU.
type modbusRequest struct {
	function uint8
	address  uint16
	quantity uint16
	value    uint16
	values   []uint16
}

// modbusPDUError is a request error that maps to a Modbus exception code.
type modbusPDUError struct {
	code    uint8
	message string
}

func (e *modbusPDUError) Error() string { return e.message }

func newModbusPDUError(code uint8, format string, args ...any) error {
	return &modbusPDUError{code: code, message: fmt.Sprintf(format, args...)}
}

// parseModbusRequest validates the PDU shape and the protocol-level quantity
// bounds. Address-space validation belongs to the register file.
func parseModbusRequest(pdu []byte) (modbusRequest, error) {
	if len(pdu) == 0 {
		return modbusRequest{}, newModbusPDUError(modbusExceptionIllegalDataValue,
			"modbus: empty PDU")
	}
	function := pdu[0]
	switch function {
	case modbusFuncReadHolding, modbusFuncReadInput:
		if len(pdu) != 5 {
			return modbusRequest{function: function}, newModbusPDUError(
				modbusExceptionIllegalDataValue, "modbus: function 0x%02X PDU length %d", function, len(pdu))
		}
		req := modbusRequest{
			function: function,
			address:  binary.BigEndian.Uint16(pdu[1:3]),
			quantity: binary.BigEndian.Uint16(pdu[3:5]),
		}
		if req.quantity < 1 || req.quantity > modbusMaxReadRegisters {
			return req, newModbusPDUError(modbusExceptionIllegalDataValue,
				"modbus: read quantity %d outside 1..%d", req.quantity, modbusMaxReadRegisters)
		}
		return req, nil

	case modbusFuncWriteSingle:
		if len(pdu) != 5 {
			return modbusRequest{function: function}, newModbusPDUError(
				modbusExceptionIllegalDataValue, "modbus: function 0x%02X PDU length %d", function, len(pdu))
		}
		return modbusRequest{
			function: function,
			address:  binary.BigEndian.Uint16(pdu[1:3]),
			quantity: 1,
			value:    binary.BigEndian.Uint16(pdu[3:5]),
		}, nil

	case modbusFuncWriteMultiple:
		if len(pdu) < 6 {
			return modbusRequest{function: function}, newModbusPDUError(
				modbusExceptionIllegalDataValue, "modbus: function 0x%02X PDU length %d", function, len(pdu))
		}
		req := modbusRequest{
			function: function,
			address:  binary.BigEndian.Uint16(pdu[1:3]),
			quantity: binary.BigEndian.Uint16(pdu[3:5]),
		}
		byteCount := int(pdu[5])
		if int(req.quantity) < 1 || int(req.quantity) > modbusMaxWriteRegisters {
			return req, newModbusPDUError(modbusExceptionIllegalDataValue,
				"modbus: write quantity %d outside 1..%d", req.quantity, modbusMaxWriteRegisters)
		}
		if byteCount != int(req.quantity)*2 {
			return req, newModbusPDUError(modbusExceptionIllegalDataValue,
				"modbus: write byte count %d does not match quantity %d", byteCount, req.quantity)
		}
		if len(pdu) != 6+byteCount {
			return req, newModbusPDUError(modbusExceptionIllegalDataValue,
				"modbus: write payload length %d does not match byte count %d", len(pdu)-6, byteCount)
		}
		req.values = make([]uint16, req.quantity)
		for i := range req.values {
			req.values[i] = binary.BigEndian.Uint16(pdu[6+i*2:])
		}
		return req, nil

	default:
		return modbusRequest{function: function}, newModbusPDUError(
			modbusExceptionIllegalFunction, "modbus: unsupported function 0x%02X", function)
	}
}

func modbusReadResponse(function uint8, values []uint16) []byte {
	pdu := make([]byte, 2+len(values)*2)
	pdu[0] = function
	pdu[1] = byte(len(values) * 2)
	for i, value := range values {
		binary.BigEndian.PutUint16(pdu[2+i*2:], value)
	}
	return pdu
}

func modbusWriteSingleResponse(address, value uint16) []byte {
	pdu := make([]byte, 5)
	pdu[0] = modbusFuncWriteSingle
	binary.BigEndian.PutUint16(pdu[1:3], address)
	binary.BigEndian.PutUint16(pdu[3:5], value)
	return pdu
}

func modbusWriteMultipleResponse(address, quantity uint16) []byte {
	pdu := make([]byte, 5)
	pdu[0] = modbusFuncWriteMultiple
	binary.BigEndian.PutUint16(pdu[1:3], address)
	binary.BigEndian.PutUint16(pdu[3:5], quantity)
	return pdu
}

func modbusExceptionResponse(function, code uint8) []byte {
	return []byte{function | 0x80, code}
}
