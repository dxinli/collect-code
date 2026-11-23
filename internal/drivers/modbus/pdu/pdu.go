package pdu

import "fmt"

// FuncCode is a Modbus function code.
type FuncCode uint8

const (
	FuncReadHoldingRegisters   FuncCode = 0x03
	FuncReadInputRegisters     FuncCode = 0x04
	FuncWriteSingleRegister    FuncCode = 0x06
	FuncWriteMultipleRegisters FuncCode = 0x10
)

const (
	// MaxReadRegisters is the protocol limit for functions 03 and 04.
	MaxReadRegisters = 125
	// MaxWriteRegisters is the protocol limit for function 16.
	MaxWriteRegisters = 123
	// MaxPDUSize is the largest application PDU.
	MaxPDUSize = 253
	// ExceptionFlag marks an exception response.
	ExceptionFlag = 0x80
)

// ExceptionCode is a Modbus exception code.
type ExceptionCode uint8

const (
	ExceptionIllegalFunction        ExceptionCode = 0x01
	ExceptionIllegalDataAddress     ExceptionCode = 0x02
	ExceptionIllegalDataValue       ExceptionCode = 0x03
	ExceptionServerDeviceFailure    ExceptionCode = 0x04
	ExceptionAcknowledge            ExceptionCode = 0x05
	ExceptionServerDeviceBusy       ExceptionCode = 0x06
	ExceptionMemoryParity           ExceptionCode = 0x08
	ExceptionGatewayPathUnavailable ExceptionCode = 0x0A
	ExceptionGatewayTargetFailed    ExceptionCode = 0x0B
)

// ExceptionClass groups exception codes by the service-reply class the kernel
// maps them to: rejected writes become 4001, internal failures become 5044.
type ExceptionClass uint8

const (
	// ExceptionClassRejected means the device rejected the request arguments.
	ExceptionClassRejected ExceptionClass = iota
	// ExceptionClassInternal means the device reported an internal failure.
	ExceptionClassInternal
)

func (c ExceptionCode) String() string {
	switch c {
	case ExceptionIllegalFunction:
		return "illegal function"
	case ExceptionIllegalDataAddress:
		return "illegal data address"
	case ExceptionIllegalDataValue:
		return "illegal data value"
	case ExceptionServerDeviceFailure:
		return "server device failure"
	case ExceptionAcknowledge:
		return "acknowledge"
	case ExceptionServerDeviceBusy:
		return "server device busy"
	case ExceptionMemoryParity:
		return "memory parity error"
	case ExceptionGatewayPathUnavailable:
		return "gateway path unavailable"
	case ExceptionGatewayTargetFailed:
		return "gateway target failed to respond"
	default:
		return fmt.Sprintf("unknown exception 0x%02X", uint8(c))
	}
}

// Class classifies an exception code. Codes not listed by the specification
// are treated as internal failures so they cannot silently become 4001.
func (c ExceptionCode) Class() ExceptionClass {
	switch c {
	case ExceptionIllegalFunction, ExceptionIllegalDataAddress, ExceptionIllegalDataValue:
		return ExceptionClassRejected
	default:
		return ExceptionClassInternal
	}
}

// Request is one sealed Modbus request PDU.
type Request interface {
	Function() FuncCode
	encode() ([]byte, error)
}

// Response is one sealed Modbus response PDU.
type Response interface {
	Function() FuncCode
	encode() ([]byte, error)
}

// ReadRegistersRequest is a function 03 or 04 request.
type ReadRegistersRequest struct {
	Func     FuncCode
	Address  uint16
	Quantity uint16
}

func (r *ReadRegistersRequest) Function() FuncCode { return r.Func }

// ReadRegistersResponse is a function 03 or 04 response.
type ReadRegistersResponse struct {
	Func      FuncCode
	Registers []uint16
}

func (r *ReadRegistersResponse) Function() FuncCode { return r.Func }

// WriteSingleRegisterRequest is a function 06 request.
type WriteSingleRegisterRequest struct {
	Address uint16
	Value   uint16
}

func (*WriteSingleRegisterRequest) Function() FuncCode { return FuncWriteSingleRegister }

// WriteSingleRegisterResponse is a function 06 response (the request echo).
type WriteSingleRegisterResponse struct {
	Address uint16
	Value   uint16
}

func (*WriteSingleRegisterResponse) Function() FuncCode { return FuncWriteSingleRegister }

// WriteMultipleRegistersRequest is a function 16 request.
type WriteMultipleRegistersRequest struct {
	Address uint16
	Values  []uint16
}

func (*WriteMultipleRegistersRequest) Function() FuncCode { return FuncWriteMultipleRegisters }

// WriteMultipleRegistersResponse is a function 16 response.
type WriteMultipleRegistersResponse struct {
	Address  uint16
	Quantity uint16
}

func (*WriteMultipleRegistersResponse) Function() FuncCode { return FuncWriteMultipleRegisters }

// ExceptionResponse is a valid Modbus exception response.
type ExceptionResponse struct {
	Func FuncCode // base function code without the exception flag
	Code ExceptionCode
}

func (e *ExceptionResponse) Function() FuncCode { return e.Func }

func (e *ExceptionResponse) Error() string {
	return fmt.Sprintf("modbus: exception 0x%02X (%s) for function 0x%02X",
		uint8(e.Code), e.Code, uint8(e.Func))
}
