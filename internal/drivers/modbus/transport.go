package modbus

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"reflect"
	"time"

	"github.com/dxinli/collect-code/internal/drivers/modbus/framing"
	"github.com/dxinli/collect-code/internal/drivers/modbus/pdu"
	"github.com/dxinli/collect-code/internal/interfaces"
)

// Conn is one opened Modbus byte stream. net.Conn and net.Pipe satisfy it; a
// serial opener returns the same shape after configuring termios.
type Conn interface {
	io.ReadWriteCloser
	SetDeadline(time.Time) error
}

// DialTCP opens a TCP connection. The context only propagates cancellation;
// connect timeouts and retries belong to the caller's link policy.
func DialTCP(ctx context.Context, addr string) (Conn, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%w: nil context", pdu.ErrArgument)
	}
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("modbus: dial %s: %w", addr, errors.Join(interfaces.ErrLinkDown, err))
	}
	return conn, nil
}

// NewTCPClient dials addr and returns a Modbus TCP client.
func NewTCPClient(ctx context.Context, addr string, clock interfaces.Clock) (*Client, error) {
	return newDialClient(ctx, addr, framing.NewMBAP(), clock)
}

// NewRTUOverTCPClient dials addr and returns an RTU-over-TCP client.
func NewRTUOverTCPClient(ctx context.Context, addr string, clock interfaces.Clock) (*Client, error) {
	return newDialClient(ctx, addr, framing.NewRTU(), clock)
}

func newDialClient(ctx context.Context, addr string, fr framing.Framer, clock interfaces.Clock) (*Client, error) {
	conn, err := DialTCP(ctx, addr)
	if err != nil {
		return nil, err
	}
	client, err := NewClient(conn, fr, clock)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return client, nil
}

// NewRTUClient wraps an already-opened serial or pipe stream.
func NewRTUClient(conn Conn, clock interfaces.Clock) (*Client, error) {
	return NewClient(conn, framing.NewRTU(), clock)
}

// NewClient builds a client over an injected transport and framing.
func NewClient(conn Conn, fr framing.Framer, clock interfaces.Clock) (*Client, error) {
	if isNil(conn) {
		return nil, fmt.Errorf("%w: nil conn", pdu.ErrArgument)
	}
	if isNil(fr) {
		return nil, fmt.Errorf("%w: nil framer", pdu.ErrArgument)
	}
	if isNil(clock) {
		return nil, fmt.Errorf("%w: nil clock", pdu.ErrArgument)
	}
	return &Client{conn: conn, framer: fr, clock: clock, nextTxn: 1}, nil
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}
