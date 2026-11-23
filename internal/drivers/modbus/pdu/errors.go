package pdu

import (
	"errors"
	"fmt"
)

// Sentinel errors shared by the PDU encoder and decoder. Protocol errors wrap
// ErrProtocol so callers can classify malformed wire data with errors.Is.
var (
	ErrProtocol            = errors.New("modbus: protocol error")
	ErrMalformed           = fmt.Errorf("modbus: malformed PDU: %w", ErrProtocol)
	ErrUnsupportedFunction = fmt.Errorf("modbus: unsupported function: %w", ErrProtocol)
	ErrArgument            = errors.New("modbus: invalid argument")
	ErrInvalidQuantity     = fmt.Errorf("modbus: invalid quantity: %w", ErrArgument)
	ErrInvalidAddress      = fmt.Errorf("modbus: invalid address range: %w", ErrArgument)
)

// malformed keeps both the protocol classification and the precise argument
// error when wire data violates a range rule.
func malformed(err error) error {
	if err == nil {
		return ErrMalformed
	}
	return errors.Join(ErrMalformed, err)
}
