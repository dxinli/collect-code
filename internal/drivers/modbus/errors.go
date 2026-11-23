package modbus

import (
	"errors"
	"fmt"

	"github.com/dxinli/collect-code/internal/drivers/modbus/framing"
	"github.com/dxinli/collect-code/internal/drivers/modbus/pdu"
	"github.com/dxinli/collect-code/internal/interfaces"
)

// ExceptionError is a valid Modbus exception response. It unwraps to the
// exception itself and to the interface sentinel that decides 4001 vs 5044.
type ExceptionError struct {
	Exception *pdu.ExceptionResponse
	Class     error
}

func (e *ExceptionError) Error() string {
	if e == nil || e.Exception == nil {
		return "modbus: device exception"
	}
	return e.Exception.Error()
}

func (e *ExceptionError) Unwrap() []error {
	if e == nil {
		return nil
	}
	if e.Class == nil {
		return []error{e.Exception}
	}
	return []error{e.Exception, e.Class}
}

// ErrUnitMismatch means a response came from a different unit id.
var ErrUnitMismatch = fmt.Errorf("modbus: response unit id mismatch: %w", pdu.ErrProtocol)

// protocolFailure makes a protocol error satisfy both pdu.ErrProtocol and the
// interface sentinel that the service-reply table maps to 5044.
func protocolFailure(err error) error {
	return errors.Join(err, interfaces.ErrDeviceNoResponse)
}

// IsProtocolError reports whether err is a Modbus protocol/framing error.
func IsProtocolError(err error) bool {
	return errors.Is(err, pdu.ErrProtocol)
}

// IsUnrecoverable reports whether the connection must be closed and rebuilt
// before the next exchange.
func IsUnrecoverable(err error) bool {
	return errors.Is(err, framing.ErrUnrecoverable)
}

// ExceptionFrom extracts a Modbus exception response from err.
func ExceptionFrom(err error) (*pdu.ExceptionResponse, bool) {
	var wrapped *ExceptionError
	if errors.As(err, &wrapped) && wrapped.Exception != nil {
		return wrapped.Exception, true
	}
	var raw *pdu.ExceptionResponse
	if errors.As(err, &raw) {
		return raw, true
	}
	return nil, false
}

func exceptionClass(code pdu.ExceptionCode) error {
	if code.Class() == pdu.ExceptionClassRejected {
		return interfaces.ErrDeviceRejected
	}
	return interfaces.ErrDeviceInternal
}
