package framing

import (
	"errors"
	"fmt"
	"io"

	"github.com/dxinli/collect-code/internal/drivers/modbus/pdu"
)

// Framing errors. Every error also satisfies pdu.ErrProtocol; ErrUnrecoverable
// additionally means the stream boundary cannot be trusted and the connection
// must be closed and rebuilt.
var (
	ErrBadCRC              = fmt.Errorf("modbus: bad CRC: %w", pdu.ErrProtocol)
	ErrShortFrame          = fmt.Errorf("modbus: short frame: %w", pdu.ErrProtocol)
	ErrFrameTooLong        = fmt.Errorf("modbus: frame too long: %w", pdu.ErrProtocol)
	ErrMalformedFrame      = fmt.Errorf("modbus: malformed frame: %w", pdu.ErrProtocol)
	ErrTransactionMismatch = fmt.Errorf("modbus: transaction id mismatch: %w", pdu.ErrProtocol)
	ErrUnrecoverable       = errors.New("modbus: framing stream is unrecoverable")
)

func unrecoverable(err error) error {
	return errors.Join(ErrUnrecoverable, err)
}

func shortFrame(err error) error {
	return unrecoverable(errors.Join(ErrShortFrame, err))
}

// readFull reads exactly len(buf) bytes. A clean EOF before any byte is
// returned as io.EOF so the caller can classify it as a link failure; a
// partial read is an unrecoverable short frame.
func readFull(r io.Reader, buf []byte) error {
	n, err := io.ReadFull(r, buf)
	if err == nil {
		return nil
	}
	if errors.Is(err, io.EOF) {
		if n == 0 {
			return io.EOF
		}
		return shortFrame(err)
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return shortFrame(err)
	}
	if n > 0 {
		return shortFrame(err)
	}
	return err
}
