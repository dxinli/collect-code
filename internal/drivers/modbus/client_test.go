package modbus_test

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dxinli/collect-code/internal/drivers/modbus"
	"github.com/dxinli/collect-code/internal/drivers/modbus/framing"
	"github.com/dxinli/collect-code/internal/drivers/modbus/pdu"
	"github.com/dxinli/collect-code/internal/interfaces"
	"github.com/dxinli/collect-code/internal/testutil/fakeclock"
)

var testStart = time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)

// scriptedConn is a deterministic Conn for Client tests. It never opens a
// socket and records every deadline and write.
type scriptedConn struct {
	mu        sync.Mutex
	readBuf   []byte
	writes    [][]byte
	deadlines []time.Time
	readErr   error
	writeErr  error
	writeN    int

	blockRead  bool
	blockWrite bool

	unblock     chan struct{}
	unblockOnce sync.Once
	readStart   chan struct{}
	readOnce    sync.Once
	writeStart  chan struct{}
	writeOnce   sync.Once

	closed        atomic.Bool
	now           func() time.Time
	onSetDeadline func()
}

func newScriptedConn(now func() time.Time, response []byte) *scriptedConn {
	return &scriptedConn{
		readBuf:    response,
		writeN:     -1,
		unblock:    make(chan struct{}),
		readStart:  make(chan struct{}),
		writeStart: make(chan struct{}),
		now:        now,
	}
}

func (c *scriptedConn) Read(p []byte) (int, error) {
	if c.blockRead {
		c.readOnce.Do(func() { close(c.readStart) })
		<-c.unblock
		if c.closed.Load() {
			return 0, net.ErrClosed
		}
		return 0, os.ErrDeadlineExceeded
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.readErr != nil {
		return 0, c.readErr
	}
	if len(c.readBuf) == 0 {
		return 0, io.EOF
	}
	n := copy(p, c.readBuf)
	c.readBuf = c.readBuf[n:]
	return n, nil
}

func (c *scriptedConn) Write(p []byte) (int, error) {
	if c.blockWrite {
		c.writeOnce.Do(func() { close(c.writeStart) })
		<-c.unblock
		if c.closed.Load() {
			return 0, net.ErrClosed
		}
		return 0, os.ErrDeadlineExceeded
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.writeErr != nil {
		return 0, c.writeErr
	}
	if c.writeN >= 0 && c.writeN < len(p) {
		return c.writeN, nil
	}
	c.writes = append(c.writes, append([]byte(nil), p...))
	return len(p), nil
}

func (c *scriptedConn) Close() error {
	c.closed.Store(true)
	c.unblockOnce.Do(func() { close(c.unblock) })
	return nil
}

func (c *scriptedConn) SetDeadline(t time.Time) error {
	c.mu.Lock()
	c.deadlines = append(c.deadlines, t)
	hook := c.onSetDeadline
	c.mu.Unlock()
	if hook != nil {
		hook()
	}
	if !t.IsZero() && !t.After(c.now()) {
		c.unblockOnce.Do(func() { close(c.unblock) })
	}
	return nil
}

func (c *scriptedConn) written() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([][]byte, len(c.writes))
	for i, raw := range c.writes {
		out[i] = append([]byte(nil), raw...)
	}
	return out
}

func (c *scriptedConn) deadlineCalls() []time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]time.Time(nil), c.deadlines...)
}

func waitFor(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for channel")
	}
}

func mbapResponse(t *testing.T, txn uint16, unitID uint8, resp pdu.Response) []byte {
	t.Helper()
	raw, err := pdu.EncodeResponse(resp)
	if err != nil {
		t.Fatalf("EncodeResponse: %v", err)
	}
	adu, err := framing.NewMBAP().Encode(framing.Frame{TransactionID: txn, UnitID: unitID, PDU: raw})
	if err != nil {
		t.Fatalf("MBAP Encode: %v", err)
	}
	return adu
}

func rtuResponse(t *testing.T, unitID uint8, resp pdu.Response) []byte {
	t.Helper()
	raw, err := pdu.EncodeResponse(resp)
	if err != nil {
		t.Fatalf("EncodeResponse: %v", err)
	}
	adu, err := framing.NewRTU().Encode(framing.Frame{UnitID: unitID, PDU: raw})
	if err != nil {
		t.Fatalf("RTU Encode: %v", err)
	}
	return adu
}

func readResponse(t *testing.T, fn pdu.FuncCode, values ...uint16) pdu.Response {
	t.Helper()
	return &pdu.ReadRegistersResponse{Func: fn, Registers: values}
}

// Covers: REQ-TIME-01, REQ-SCHED-02
func TestClientSetsAbsoluteDeadline(t *testing.T) {
	clock := fakeclock.New(testStart)
	defer clock.Close()
	conn := newScriptedConn(clock.Now, mbapResponse(t, 1, 1, readResponse(t, pdu.FuncReadHoldingRegisters, 1)))
	client, err := modbus.NewClient(conn, framing.NewMBAP(), clock)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	deadline := testStart.Add(time.Second)
	if _, err := client.ReadHoldingRegisters(context.Background(), 1, 0, 1, deadline); err != nil {
		t.Fatalf("ReadHoldingRegisters: %v", err)
	}
	calls := conn.deadlineCalls()
	if len(calls) == 0 || !calls[0].Equal(deadline) {
		t.Fatalf("first SetDeadline = %v, want %v (calls %v)", calls[0], deadline, calls)
	}
	if len(calls) < 2 || !calls[len(calls)-1].IsZero() {
		t.Fatalf("final SetDeadline = %v, want zero", calls[len(calls)-1])
	}
}

// Covers: REQ-TIME-01, REQ-SCHED-02
func TestClientZeroDeadline(t *testing.T) {
	clock := fakeclock.New(testStart)
	defer clock.Close()
	conn := newScriptedConn(clock.Now, mbapResponse(t, 1, 1, readResponse(t, pdu.FuncReadHoldingRegisters, 1)))
	client, err := modbus.NewClient(conn, framing.NewMBAP(), clock)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := client.ReadHoldingRegisters(context.Background(), 1, 0, 1, time.Time{}); err != nil {
		t.Fatalf("ReadHoldingRegisters: %v", err)
	}
	calls := conn.deadlineCalls()
	if len(calls) == 0 || !calls[0].IsZero() {
		t.Fatalf("first SetDeadline = %v, want zero", calls[0])
	}
}

// Covers: REQ-SCHED-02
func TestClientPastDeadline(t *testing.T) {
	clock := fakeclock.New(testStart)
	defer clock.Close()
	conn := newScriptedConn(clock.Now, nil)
	client, err := modbus.NewClient(conn, framing.NewMBAP(), clock)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = client.ReadHoldingRegisters(context.Background(), 1, 0, 1, testStart.Add(-time.Second))
	if !errors.Is(err, interfaces.ErrDeviceNoResponse) {
		t.Fatalf("err = %v, want ErrDeviceNoResponse", err)
	}
	if len(conn.written()) != 0 {
		t.Fatal("expired deadline wrote a frame")
	}
}

// Covers: REQ-TIME-01, REQ-SCHED-02
func TestClientTimeoutIsDeviceNoResponse(t *testing.T) {
	clock := fakeclock.New(testStart)
	defer clock.Close()
	conn := newScriptedConn(clock.Now, nil)
	conn.readErr = os.ErrDeadlineExceeded
	client, err := modbus.NewClient(conn, framing.NewMBAP(), clock)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = client.ReadHoldingRegisters(context.Background(), 1, 0, 1, testStart.Add(time.Second))
	if !errors.Is(err, interfaces.ErrDeviceNoResponse) {
		t.Fatalf("err = %v, want ErrDeviceNoResponse", err)
	}
	if !modbus.IsUnrecoverable(err) {
		t.Fatalf("err = %v, want unrecoverable", err)
	}
}

// Covers: REQ-TIME-01
func TestClientLinkFailureIsLinkDown(t *testing.T) {
	clock := fakeclock.New(testStart)
	defer clock.Close()
	conn := newScriptedConn(clock.Now, nil)
	client, err := modbus.NewClient(conn, framing.NewMBAP(), clock)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = client.ReadHoldingRegisters(context.Background(), 1, 0, 1, testStart.Add(time.Second))
	if !errors.Is(err, interfaces.ErrLinkDown) {
		t.Fatalf("err = %v, want ErrLinkDown", err)
	}
}

// Covers: REQ-TIME-01
func TestClientShortWriteIsLinkDown(t *testing.T) {
	clock := fakeclock.New(testStart)
	defer clock.Close()
	conn := newScriptedConn(clock.Now, nil)
	conn.writeN = 1
	client, err := modbus.NewClient(conn, framing.NewMBAP(), clock)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = client.ReadHoldingRegisters(context.Background(), 1, 0, 1, testStart.Add(time.Second))
	if !errors.Is(err, interfaces.ErrLinkDown) {
		t.Fatalf("err = %v, want ErrLinkDown", err)
	}
}

// Covers: REQ-TIME-01
func TestClientWriteTimeoutIsLinkDown(t *testing.T) {
	clock := fakeclock.New(testStart)
	defer clock.Close()
	conn := newScriptedConn(clock.Now, nil)
	conn.writeErr = os.ErrDeadlineExceeded
	client, err := modbus.NewClient(conn, framing.NewMBAP(), clock)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = client.ReadHoldingRegisters(context.Background(), 1, 0, 1, testStart.Add(time.Second))
	if !errors.Is(err, interfaces.ErrLinkDown) {
		t.Fatalf("err = %v, want ErrLinkDown", err)
	}
}

// Covers: REQ-TIME-01
func TestClientCloseInterruptsInFlightDo(t *testing.T) {
	clock := fakeclock.New(testStart)
	defer clock.Close()
	conn := newScriptedConn(clock.Now, nil)
	conn.blockRead = true
	client, err := modbus.NewClient(conn, framing.NewMBAP(), clock)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := client.ReadHoldingRegisters(context.Background(), 1, 0, 1, testStart.Add(time.Minute))
		done <- err
	}()
	waitFor(t, conn.readStart)
	if err := client.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, interfaces.ErrClosed) {
			t.Fatalf("err = %v, want ErrClosed", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Do did not return after Close")
	}
}

// Covers: REQ-TIME-01
func TestClientBlockedWriteUnblocksOnClose(t *testing.T) {
	clock := fakeclock.New(testStart)
	defer clock.Close()
	conn := newScriptedConn(clock.Now, nil)
	conn.blockWrite = true
	client, err := modbus.NewClient(conn, framing.NewMBAP(), clock)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := client.ReadHoldingRegisters(context.Background(), 1, 0, 1, testStart.Add(time.Minute))
		done <- err
	}()
	waitFor(t, conn.writeStart)
	if err := client.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, interfaces.ErrClosed) {
			t.Fatalf("err = %v, want ErrClosed", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Do did not return after Close")
	}
}

// Covers: REQ-TIME-01
func TestClientCloseIsIdempotent(t *testing.T) {
	clock := fakeclock.New(testStart)
	defer clock.Close()
	conn := newScriptedConn(clock.Now, nil)
	client, err := modbus.NewClient(conn, framing.NewMBAP(), clock)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	_, err = client.ReadHoldingRegisters(context.Background(), 1, 0, 1, time.Time{})
	if !errors.Is(err, interfaces.ErrClosed) {
		t.Fatalf("err = %v, want ErrClosed", err)
	}
}

// Covers: REQ-TIME-01
func TestClientCanceledBeforeDo(t *testing.T) {
	clock := fakeclock.New(testStart)
	defer clock.Close()
	conn := newScriptedConn(clock.Now, nil)
	client, err := modbus.NewClient(conn, framing.NewMBAP(), clock)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.ReadHoldingRegisters(ctx, 1, 0, 1, time.Time{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if len(conn.written()) != 0 {
		t.Fatal("cancelled Do wrote a frame")
	}
}

// Covers: REQ-TIME-01
func TestClientCanceledBeforeIO(t *testing.T) {
	clock := fakeclock.New(testStart)
	defer clock.Close()
	conn := newScriptedConn(clock.Now, nil)
	ctx, cancel := context.WithCancel(context.Background())
	var once sync.Once
	conn.onSetDeadline = func() {
		once.Do(cancel)
	}
	client, err := modbus.NewClient(conn, framing.NewMBAP(), clock)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = client.ReadHoldingRegisters(ctx, 1, 0, 1, time.Time{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if len(conn.written()) != 0 {
		t.Fatal("cancelled Do wrote a frame")
	}
}

// Covers: REQ-TIME-01
func TestClientCanceledDuringRead(t *testing.T) {
	clock := fakeclock.New(testStart)
	defer clock.Close()
	conn := newScriptedConn(clock.Now, nil)
	conn.blockRead = true
	client, err := modbus.NewClient(conn, framing.NewMBAP(), clock)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := client.ReadHoldingRegisters(ctx, 1, 0, 1, testStart.Add(time.Minute))
		done <- err
	}()
	waitFor(t, conn.readStart)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Do did not return after cancel")
	}
}

// Covers: REQ-TIME-01
func TestClientCancelUnblocksBlockedWrite(t *testing.T) {
	clock := fakeclock.New(testStart)
	defer clock.Close()
	conn := newScriptedConn(clock.Now, nil)
	conn.blockWrite = true
	client, err := modbus.NewClient(conn, framing.NewMBAP(), clock)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := client.ReadHoldingRegisters(ctx, 1, 0, 1, testStart.Add(time.Minute))
		done <- err
	}()
	waitFor(t, conn.writeStart)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Do did not return after cancel")
	}
}

// Covers: REQ-TIME-01
func TestClientCancelWinsOverTimeout(t *testing.T) {
	clock := fakeclock.New(testStart)
	defer clock.Close()
	conn := newScriptedConn(clock.Now, nil)
	conn.blockRead = true
	client, err := modbus.NewClient(conn, framing.NewMBAP(), clock)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := client.ReadHoldingRegisters(ctx, 1, 0, 1, testStart.Add(time.Minute))
		done <- err
	}()
	waitFor(t, conn.readStart)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
		if errors.Is(err, interfaces.ErrDeviceNoResponse) {
			t.Fatalf("err = %v, must not be classified as device no-response", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Do did not return after cancel")
	}
}

// Covers: REQ-TIME-01, REQ-SCHED-02
func TestClientCanceledAndPastDeadline(t *testing.T) {
	clock := fakeclock.New(testStart)
	defer clock.Close()
	conn := newScriptedConn(clock.Now, nil)
	client, err := modbus.NewClient(conn, framing.NewMBAP(), clock)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.ReadHoldingRegisters(ctx, 1, 0, 1, testStart.Add(-time.Second))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if errors.Is(err, interfaces.ErrDeviceNoResponse) {
		t.Fatalf("err = %v, must not be classified as device no-response", err)
	}
}

// Covers: REQ-INVOKE-05
func TestClientExceptionClassification(t *testing.T) {
	tests := []struct {
		code  pdu.ExceptionCode
		class error
	}{
		{pdu.ExceptionIllegalFunction, interfaces.ErrDeviceRejected},
		{pdu.ExceptionIllegalDataAddress, interfaces.ErrDeviceRejected},
		{pdu.ExceptionIllegalDataValue, interfaces.ErrDeviceRejected},
		{pdu.ExceptionServerDeviceFailure, interfaces.ErrDeviceInternal},
		{pdu.ExceptionAcknowledge, interfaces.ErrDeviceInternal},
		{pdu.ExceptionServerDeviceBusy, interfaces.ErrDeviceInternal},
		{pdu.ExceptionMemoryParity, interfaces.ErrDeviceInternal},
		{pdu.ExceptionGatewayPathUnavailable, interfaces.ErrDeviceInternal},
		{pdu.ExceptionGatewayTargetFailed, interfaces.ErrDeviceInternal},
		{0x00, interfaces.ErrDeviceInternal},
		{0x7F, interfaces.ErrDeviceInternal},
	}
	for _, tt := range tests {
		t.Run(tt.code.String(), func(t *testing.T) {
			clock := fakeclock.New(testStart)
			defer clock.Close()
			response := rtuResponse(t, 1, &pdu.ExceptionResponse{
				Func: pdu.FuncReadHoldingRegisters, Code: tt.code,
			})
			conn := newScriptedConn(clock.Now, response)
			client, err := modbus.NewClient(conn, framing.NewRTU(), clock)
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			_, err = client.ReadHoldingRegisters(context.Background(), 1, 0, 1, testStart.Add(time.Second))
			if !errors.Is(err, tt.class) {
				t.Fatalf("err = %v, want class %v", err, tt.class)
			}
			if modbus.IsProtocolError(err) {
				t.Fatalf("exception must not be a protocol error: %v", err)
			}
			exception, ok := modbus.ExceptionFrom(err)
			if !ok || exception.Code != tt.code {
				t.Fatalf("ExceptionFrom = %#v, %v; want code 0x%02X", exception, ok, uint8(tt.code))
			}
		})
	}
}

// Covers: REQ-INVOKE-05
func TestClientProtocolErrorMatchesProtocolAndNoResponse(t *testing.T) {
	t.Run("rtu bad crc", func(t *testing.T) {
		clock := fakeclock.New(testStart)
		defer clock.Close()
		raw := rtuResponse(t, 1, readResponse(t, pdu.FuncReadHoldingRegisters, 1))
		raw[len(raw)-1] ^= 0xFF
		conn := newScriptedConn(clock.Now, raw)
		client, err := modbus.NewClient(conn, framing.NewRTU(), clock)
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		_, err = client.ReadHoldingRegisters(context.Background(), 1, 0, 1, testStart.Add(time.Second))
		if !modbus.IsProtocolError(err) || !errors.Is(err, interfaces.ErrDeviceNoResponse) {
			t.Fatalf("err = %v, want protocol and ErrDeviceNoResponse", err)
		}
		if !modbus.IsUnrecoverable(err) {
			t.Fatalf("err = %v, want unrecoverable", err)
		}
		if _, ok := modbus.ExceptionFrom(err); ok {
			t.Fatal("protocol error reported as exception")
		}
	})
	t.Run("mbap bad protocol id", func(t *testing.T) {
		clock := fakeclock.New(testStart)
		defer clock.Close()
		raw := mbapResponse(t, 1, 1, readResponse(t, pdu.FuncReadHoldingRegisters, 1))
		raw[3] = 0x01
		conn := newScriptedConn(clock.Now, raw)
		client, err := modbus.NewClient(conn, framing.NewMBAP(), clock)
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		_, err = client.ReadHoldingRegisters(context.Background(), 1, 0, 1, testStart.Add(time.Second))
		if !modbus.IsProtocolError(err) || !errors.Is(err, interfaces.ErrDeviceNoResponse) {
			t.Fatalf("err = %v, want protocol and ErrDeviceNoResponse", err)
		}
		if modbus.IsUnrecoverable(err) {
			t.Fatalf("err = %v, want recoverable", err)
		}
	})
}

// Covers: REQ-INVOKE-05
func TestClientRTUValidationErrorIsUnrecoverable(t *testing.T) {
	clock := fakeclock.New(testStart)
	defer clock.Close()
	response := rtuResponse(t, 1, &pdu.WriteSingleRegisterResponse{Address: 2, Value: 99})
	conn := newScriptedConn(clock.Now, response)
	client, err := modbus.NewClient(conn, framing.NewRTU(), clock)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	err = client.WriteSingleRegister(context.Background(), 1, 2, 50, testStart.Add(time.Second))
	if !modbus.IsProtocolError(err) || !errors.Is(err, interfaces.ErrDeviceNoResponse) || !modbus.IsUnrecoverable(err) {
		t.Fatalf("err = %v, want protocol, no-response and unrecoverable", err)
	}
}

func TestClientPassesUnitZero(t *testing.T) {
	clock := fakeclock.New(testStart)
	defer clock.Close()
	conn := newScriptedConn(clock.Now, mbapResponse(t, 1, 0, readResponse(t, pdu.FuncReadHoldingRegisters, 7)))
	client, err := modbus.NewClient(conn, framing.NewMBAP(), clock)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	values, err := client.ReadHoldingRegisters(context.Background(), 0, 0, 1, testStart.Add(time.Second))
	if err != nil {
		t.Fatalf("ReadHoldingRegisters: %v", err)
	}
	if len(values) != 1 || values[0] != 7 {
		t.Fatalf("values = %v, want [7]", values)
	}
	writes := conn.written()
	if len(writes) != 1 || writes[0][6] != 0 {
		t.Fatalf("unit id in frame = %v, want 0", writes)
	}
}

// Covers: REQ-INVOKE-05
func TestClientRejectsUnitMismatch(t *testing.T) {
	clock := fakeclock.New(testStart)
	defer clock.Close()
	conn := newScriptedConn(clock.Now, mbapResponse(t, 1, 2, readResponse(t, pdu.FuncReadHoldingRegisters, 1)))
	client, err := modbus.NewClient(conn, framing.NewMBAP(), clock)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = client.ReadHoldingRegisters(context.Background(), 1, 0, 1, testStart.Add(time.Second))
	if !errors.Is(err, modbus.ErrUnitMismatch) || !errors.Is(err, interfaces.ErrDeviceNoResponse) {
		t.Fatalf("err = %v, want ErrUnitMismatch and ErrDeviceNoResponse", err)
	}
}

func TestClientTransactionIDIncrements(t *testing.T) {
	clock := fakeclock.New(testStart)
	defer clock.Close()
	first := mbapResponse(t, 1, 1, readResponse(t, pdu.FuncReadHoldingRegisters, 1))
	second := mbapResponse(t, 2, 1, readResponse(t, pdu.FuncReadHoldingRegisters, 2))
	conn := newScriptedConn(clock.Now, append(first, second...))
	client, err := modbus.NewClient(conn, framing.NewMBAP(), clock)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	for want := uint16(1); want <= 2; want++ {
		if _, err := client.ReadHoldingRegisters(context.Background(), 1, 0, 1, testStart.Add(time.Second)); err != nil {
			t.Fatalf("ReadHoldingRegisters: %v", err)
		}
		writes := conn.written()
		got := uint16(writes[len(writes)-1][0])<<8 | uint16(writes[len(writes)-1][1])
		if got != want {
			t.Fatalf("transaction id = %d, want %d", got, want)
		}
	}
}

func TestNewClientRejectsMissingDependencies(t *testing.T) {
	clock := fakeclock.New(testStart)
	defer clock.Close()
	conn := newScriptedConn(clock.Now, nil)
	if _, err := modbus.NewClient(nil, framing.NewMBAP(), clock); err == nil {
		t.Fatal("NewClient accepted nil conn")
	}
	if _, err := modbus.NewClient(conn, nil, clock); err == nil {
		t.Fatal("NewClient accepted nil framer")
	}
	if _, err := modbus.NewClient(conn, framing.NewMBAP(), nil); err == nil {
		t.Fatal("NewClient accepted nil clock")
	}
}

type typedNilConn struct{}

func (*typedNilConn) Read([]byte) (int, error)    { return 0, io.EOF }
func (*typedNilConn) Write([]byte) (int, error)   { return 0, io.EOF }
func (*typedNilConn) Close() error                { return nil }
func (*typedNilConn) SetDeadline(time.Time) error { return nil }

func TestNewClientRejectsTypedNil(t *testing.T) {
	clock := fakeclock.New(testStart)
	defer clock.Close()
	var conn *typedNilConn
	if _, err := modbus.NewClient(conn, framing.NewMBAP(), clock); err == nil {
		t.Fatal("NewClient accepted typed-nil conn")
	}
}

func TestClientNilContextIsArgumentError(t *testing.T) {
	clock := fakeclock.New(testStart)
	defer clock.Close()
	conn := newScriptedConn(clock.Now, nil)
	client, err := modbus.NewClient(conn, framing.NewMBAP(), clock)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	var nilContext context.Context
	_, err = client.Do(nilContext, 1, &pdu.ReadRegistersRequest{Func: pdu.FuncReadHoldingRegisters, Quantity: 1}, time.Time{})
	if !errors.Is(err, pdu.ErrArgument) {
		t.Fatalf("err = %v, want ErrArgument", err)
	}
}
