package modbus_test

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/dxinli/collect-code/internal/drivers/modbus"
	"github.com/dxinli/collect-code/internal/drivers/modbus/framing"
	"github.com/dxinli/collect-code/internal/interfaces"
	"github.com/dxinli/collect-code/internal/testutil/fakeclock"
)

var _ modbus.Conn = net.Conn(nil)

// Covers: REQ-SCOPE-01
func TestDialTCPFailureIsLinkDown(t *testing.T) {
	conn, err := modbus.DialTCP(context.Background(), "not a valid address")
	if conn != nil {
		_ = conn.Close()
	}
	if !errors.Is(err, interfaces.ErrLinkDown) {
		t.Fatalf("err = %v, want ErrLinkDown", err)
	}
}

// Covers: REQ-SCOPE-01
func TestDialTCPNilContext(t *testing.T) {
	var ctx context.Context
	if _, err := modbus.DialTCP(ctx, "127.0.0.1:1"); err == nil {
		t.Fatal("DialTCP accepted a nil context")
	}
}

// Covers: REQ-SCOPE-01
func TestNewTCPClientLoopback(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() { _ = listener.Close() }()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		accepted <- conn
	}()
	clock := fakeclock.New(testStart)
	defer clock.Close()
	client, err := modbus.NewTCPClient(context.Background(), listener.Addr().String(), clock)
	if err != nil {
		t.Fatalf("NewTCPClient: %v", err)
	}
	defer func() { _ = client.Close() }()
	select {
	case conn := <-accepted:
		_ = conn.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("listener did not accept the client connection")
	}
}

// Covers: REQ-SCOPE-01
func TestNewRTUClientWrapsConn(t *testing.T) {
	clock := fakeclock.New(testStart)
	defer clock.Close()
	conn := newScriptedConn(clock.Now, nil)
	client, err := modbus.NewRTUClient(conn, clock)
	if err != nil {
		t.Fatalf("NewRTUClient: %v", err)
	}
	if client == nil {
		t.Fatal("NewRTUClient returned nil client")
	}
	if _, err := modbus.NewClient(conn, framing.NewRTU(), clock); err != nil {
		t.Fatalf("NewClient: %v", err)
	}
}
