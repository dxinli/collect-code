package modbus_test

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/dxinli/collect-code/internal/clock/realclock"
	"github.com/dxinli/collect-code/internal/drivers/modbus"
	"github.com/dxinli/collect-code/internal/drivers/modbus/framing"
	"github.com/dxinli/collect-code/internal/drivers/modbus/modbustest"
	"github.com/dxinli/collect-code/internal/drivers/modbus/pdu"
	"github.com/dxinli/collect-code/internal/testutil/fakeclock"
)

func newSlave() *modbustest.Slave {
	return modbustest.New(map[uint8]*modbustest.Unit{
		1: {
			Holding: map[uint16]uint16{
				0: 0x1234,
				1: 0x5678,
				2: 0,
				3: 0,
				4: 0,
			},
			Input: map[uint16]uint16{0: 0x00FF},
		},
	})
}

func runExchanges(t *testing.T, client *modbus.Client) {
	t.Helper()
	ctx := context.Background()
	zero := time.Time{}

	values, err := client.ReadHoldingRegisters(ctx, 1, 0, 2, zero)
	if err != nil {
		t.Fatalf("ReadHoldingRegisters: %v", err)
	}
	if len(values) != 2 || values[0] != 0x1234 || values[1] != 0x5678 {
		t.Fatalf("holding values = %v", values)
	}

	inputs, err := client.ReadInputRegisters(ctx, 1, 0, 1, zero)
	if err != nil {
		t.Fatalf("ReadInputRegisters: %v", err)
	}
	if len(inputs) != 1 || inputs[0] != 0x00FF {
		t.Fatalf("input values = %v", inputs)
	}

	if err := client.WriteSingleRegister(ctx, 1, 2, 50, zero); err != nil {
		t.Fatalf("WriteSingleRegister: %v", err)
	}
	if err := client.WriteMultipleRegisters(ctx, 1, 3, []uint16{7, 8}, zero); err != nil {
		t.Fatalf("WriteMultipleRegisters: %v", err)
	}
	values, err = client.ReadHoldingRegisters(ctx, 1, 2, 3, zero)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if len(values) != 3 || values[0] != 50 || values[1] != 7 || values[2] != 8 {
		t.Fatalf("read back values = %v", values)
	}
}

func runPipeTransport(t *testing.T, fr framing.Framer, rtu bool) {
	t.Helper()
	slave := newSlave()
	server, clientConn := net.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = slave.ServeConn(server, fr)
	}()
	clock := fakeclock.New(testStart)
	defer clock.Close()
	var (
		client *modbus.Client
		err    error
	)
	if rtu {
		client, err = modbus.NewRTUClient(clientConn, clock)
	} else {
		client, err = modbus.NewClient(clientConn, fr, clock)
	}
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	runExchanges(t, client)
	if err := client.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	<-done
	_ = server.Close()
}

func runTCPTransport(t *testing.T, rtu bool) {
	t.Helper()
	slave := newSlave()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() { _ = listener.Close() }()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		fr := framing.NewMBAP()
		if rtu {
			fr = framing.NewRTU()
		}
		_ = slave.ServeConn(conn, fr)
	}()
	clock := realclock.New()
	var client *modbus.Client
	if rtu {
		client, err = modbus.NewRTUOverTCPClient(context.Background(), listener.Addr().String(), clock)
	} else {
		client, err = modbus.NewTCPClient(context.Background(), listener.Addr().String(), clock)
	}
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	runExchanges(t, client)
	if err := client.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("slave did not exit after client close")
	}
}

// Covers: REQ-SCOPE-01
func TestThreeTransportsSameExchange(t *testing.T) {
	t.Run("mbap pipe", func(t *testing.T) { runPipeTransport(t, framing.NewMBAP(), false) })
	t.Run("rtu pipe", func(t *testing.T) { runPipeTransport(t, framing.NewRTU(), true) })
	t.Run("rtu over tcp", func(t *testing.T) { runTCPTransport(t, true) })
}

// Covers: REQ-SCOPE-01
func TestTCPLoopbackMBAP(t *testing.T) { runTCPTransport(t, false) }

// Covers: REQ-SCOPE-01
func TestTCPLoopbackRTUOverTCP(t *testing.T) { runTCPTransport(t, true) }

// Covers: REQ-SCOPE-01
func TestClientSerializesConcurrentCalls(t *testing.T) {
	const count = 16
	units := map[uint16]uint16{}
	for i := range count {
		units[uint16(i)] = uint16(i)
	}
	slave := modbustest.New(map[uint8]*modbustest.Unit{1: {Holding: units}})
	server, clientConn := net.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = slave.ServeConn(server, framing.NewMBAP())
	}()
	clock := fakeclock.New(testStart)
	defer clock.Close()
	client, err := modbus.NewClient(clientConn, framing.NewMBAP(), clock)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := range count {
		wg.Add(1)
		go func(address uint16) {
			defer wg.Done()
			values, err := client.ReadHoldingRegisters(context.Background(), 1, address, 1, time.Time{})
			if err != nil {
				errs <- fmt.Errorf("address %d: %w", address, err)
				return
			}
			if len(values) != 1 || values[0] != address {
				errs <- fmt.Errorf("address %d: values = %v", address, values)
			}
		}(uint16(i))
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	<-done
	_ = server.Close()
	if got := len(slave.Requests()); got != count {
		t.Fatalf("request count = %d, want %d", got, count)
	}
}

// Covers: REQ-SCOPE-01, REQ-TIME-01
func TestNoGoroutineLeakAfterClose(t *testing.T) {
	baseline := runtime.NumGoroutine()
	for range 10 {
		slave := newSlave()
		server, clientConn := net.Pipe()
		done := make(chan struct{})
		go func() {
			defer close(done)
			_ = slave.ServeConn(server, framing.NewMBAP())
		}()
		clock := fakeclock.New(testStart)
		client, err := modbus.NewClient(clientConn, framing.NewMBAP(), clock)
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		if _, err := client.ReadHoldingRegisters(context.Background(), 1, 0, 1, time.Time{}); err != nil {
			t.Fatalf("ReadHoldingRegisters: %v", err)
		}
		if err := client.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		clock.Close()
		<-done
		_ = server.Close()
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= baseline+2 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("goroutines = %d, baseline = %d", runtime.NumGoroutine(), baseline)
}

// Covers: REQ-SCOPE-01
func TestRTUErrorRequiresRebuild(t *testing.T) {
	bad := []byte{0x01, 0x11, 0x00, 0x00, 0x00, 0x01}
	crc := framing.CRC16(bad)
	bad = append(bad, byte(crc), byte(crc>>8))

	goodPDU := encodeRequest(t, &pdu.ReadRegistersRequest{Func: pdu.FuncReadHoldingRegisters, Quantity: 1})
	good, err := framing.NewRTU().Encode(framing.Frame{UnitID: 1, PDU: goodPDU})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	reader := bytes.NewReader(append(bad, good...))
	framer := framing.NewRTU()
	if _, err := framer.DecodeRequest(reader); !modbus.IsUnrecoverable(err) {
		t.Fatalf("first err = %v, want unrecoverable", err)
	}
	frame, err := framer.DecodeRequest(reader)
	if err == nil && bytes.Equal(frame.PDU, goodPDU) {
		t.Fatal("stale stream produced the next frame without a rebuild")
	}
}

func encodeRequest(t *testing.T, req pdu.Request) []byte {
	t.Helper()
	raw, err := pdu.EncodeRequest(req)
	if err != nil {
		t.Fatalf("EncodeRequest: %v", err)
	}
	return raw
}
