package modbus

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dxinli/collect-code/internal/drivers/modbus/framing"
	"github.com/dxinli/collect-code/internal/drivers/modbus/pdu"
	"github.com/dxinli/collect-code/internal/interfaces"
)

// Client serializes request/response exchanges over one Conn. Callers may
// invoke it concurrently; one exchange is in flight at a time.
type Client struct {
	conn    Conn
	framer  framing.Framer
	clock   interfaces.Clock
	mu      sync.Mutex
	nextTxn uint16

	closed    atomic.Bool
	closeOnce sync.Once
	closeErr  error
}

// ReadHoldingRegisters reads function 03 registers. A zero deadline means no
// per-call deadline.
func (c *Client) ReadHoldingRegisters(ctx context.Context, unitID uint8,
	address, quantity uint16, deadline time.Time) ([]uint16, error) {
	resp, err := c.Do(ctx, unitID, &pdu.ReadRegistersRequest{
		Func:     pdu.FuncReadHoldingRegisters,
		Address:  address,
		Quantity: quantity,
	}, deadline)
	if err != nil {
		return nil, err
	}
	read, ok := resp.(*pdu.ReadRegistersResponse)
	if !ok {
		return nil, fmt.Errorf("%w: read response type %T", pdu.ErrMalformed, resp)
	}
	return read.Registers, nil
}

// ReadInputRegisters reads function 04 registers.
func (c *Client) ReadInputRegisters(ctx context.Context, unitID uint8,
	address, quantity uint16, deadline time.Time) ([]uint16, error) {
	resp, err := c.Do(ctx, unitID, &pdu.ReadRegistersRequest{
		Func:     pdu.FuncReadInputRegisters,
		Address:  address,
		Quantity: quantity,
	}, deadline)
	if err != nil {
		return nil, err
	}
	read, ok := resp.(*pdu.ReadRegistersResponse)
	if !ok {
		return nil, fmt.Errorf("%w: read response type %T", pdu.ErrMalformed, resp)
	}
	return read.Registers, nil
}

// WriteSingleRegister writes function 06.
func (c *Client) WriteSingleRegister(ctx context.Context, unitID uint8,
	address, value uint16, deadline time.Time) error {
	_, err := c.Do(ctx, unitID, &pdu.WriteSingleRegisterRequest{
		Address: address,
		Value:   value,
	}, deadline)
	return err
}

// WriteMultipleRegisters writes function 16.
func (c *Client) WriteMultipleRegisters(ctx context.Context, unitID uint8,
	address uint16, values []uint16, deadline time.Time) error {
	_, err := c.Do(ctx, unitID, &pdu.WriteMultipleRegistersRequest{
		Address: address,
		Values:  values,
	}, deadline)
	return err
}

// Do performs one exchange. deadline is absolute; the zero value means no
// per-call deadline. An expired deadline returns ErrDeviceNoResponse without
// sending a frame.
func (c *Client) Do(ctx context.Context, unitID uint8, req pdu.Request, deadline time.Time) (pdu.Response, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%w: nil context", pdu.ErrArgument)
	}
	if req == nil {
		return nil, fmt.Errorf("%w: nil request", pdu.ErrArgument)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed.Load() {
		return nil, interfaces.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return nil, canceled(err)
	}
	if !deadline.IsZero() && !deadline.After(c.clock.Now()) {
		return nil, fmt.Errorf("modbus: deadline expired: %w", interfaces.ErrDeviceNoResponse)
	}
	if err := c.conn.SetDeadline(deadline); err != nil {
		if c.closed.Load() {
			return nil, interfaces.ErrClosed
		}
		return nil, fmt.Errorf("modbus: set deadline: %w", errors.Join(interfaces.ErrLinkDown, err))
	}

	done := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		defer close(done)
		_ = c.conn.SetDeadline(c.clock.Now())
	})
	defer func() {
		if !stop() {
			<-done
		}
		_ = c.conn.SetDeadline(time.Time{})
	}()

	if err := ctx.Err(); err != nil {
		return nil, canceled(err)
	}
	raw, err := pdu.EncodeRequest(req)
	if err != nil {
		return nil, err
	}
	txn := c.nextTxn
	c.nextTxn++
	if c.nextTxn == 0 {
		c.nextTxn = 1
	}
	encoded, err := c.framer.Encode(framing.Frame{TransactionID: txn, UnitID: unitID, PDU: raw})
	if err != nil {
		return nil, err
	}
	if err := writeFull(c.conn, encoded); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, canceled(ctxErr)
		}
		if c.closed.Load() {
			return nil, interfaces.ErrClosed
		}
		return nil, fmt.Errorf("modbus: link write failed: %w", errors.Join(interfaces.ErrLinkDown, err))
	}
	if err := ctx.Err(); err != nil {
		return nil, canceled(err)
	}

	frame, err := c.framer.DecodeResponse(c.conn, req, txn)
	if err != nil {
		return nil, c.decodeError(ctx, err)
	}
	if frame.UnitID != unitID {
		return nil, c.pduError(ErrUnitMismatch)
	}
	resp, err := pdu.DecodeResponse(frame.PDU)
	if err != nil {
		return nil, c.pduError(err)
	}
	if exception, ok := resp.(*pdu.ExceptionResponse); ok {
		return nil, &ExceptionError{Exception: exception, Class: exceptionClass(exception.Code)}
	}
	if err := pdu.ValidateResponse(req, resp); err != nil {
		return nil, c.pduError(err)
	}
	return resp, nil
}

// Close marks the client closed and closes the underlying Conn without
// waiting for an in-flight exchange. It is idempotent.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		c.closed.Store(true)
		c.closeErr = c.conn.Close()
	})
	return c.closeErr
}

func (c *Client) decodeError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return canceled(ctxErr)
	}
	if c.closed.Load() {
		return interfaces.ErrClosed
	}
	if errors.Is(err, io.EOF) {
		return fmt.Errorf("modbus: link closed: %w", interfaces.ErrLinkDown)
	}
	if isTimeout(err) {
		return fmt.Errorf("modbus: device did not respond: %w",
			errors.Join(interfaces.ErrDeviceNoResponse, framing.ErrUnrecoverable, err))
	}
	if errors.Is(err, pdu.ErrProtocol) {
		result := protocolFailure(err)
		if !c.framer.PDUErrorsAreRecoverable() {
			result = errors.Join(result, framing.ErrUnrecoverable)
		}
		return result
	}
	return fmt.Errorf("modbus: link failure: %w", errors.Join(interfaces.ErrLinkDown, err))
}

func (c *Client) pduError(err error) error {
	result := protocolFailure(err)
	if !c.framer.PDUErrorsAreRecoverable() {
		result = errors.Join(result, framing.ErrUnrecoverable)
	}
	return result
}

func canceled(err error) error {
	return fmt.Errorf("modbus: exchange canceled: %w", err)
}

func isTimeout(err error) bool {
	if errors.Is(err, os.ErrDeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func writeFull(w io.Writer, p []byte) error {
	n, err := w.Write(p)
	if err != nil {
		return err
	}
	if n != len(p) {
		return io.ErrShortWrite
	}
	return nil
}
