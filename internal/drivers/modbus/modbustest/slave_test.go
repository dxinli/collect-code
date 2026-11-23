package modbustest

import (
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/dxinli/collect-code/internal/drivers/modbus/framing"
	"github.com/dxinli/collect-code/internal/drivers/modbus/pdu"
)

// Covers: REQ-SCOPE-01
func TestSlaveServesMultiUnit(t *testing.T) {
	slave := New(map[uint8]*Unit{
		1: {Holding: map[uint16]uint16{0: 0x1234, 1: 0x5678}},
		2: {Holding: map[uint16]uint16{0: 7}},
	})
	read := mustRequest(t, &pdu.ReadRegistersRequest{Func: pdu.FuncReadHoldingRegisters, Quantity: 2})
	response := mustHandle(t, slave, framing.Frame{TransactionID: 1, UnitID: 1, PDU: read})
	decoded := mustResponse(t, response)
	values := decoded.(*pdu.ReadRegistersResponse).Registers
	if len(values) != 2 || values[0] != 0x1234 || values[1] != 0x5678 {
		t.Fatalf("unit 1 values = %v", values)
	}

	single := mustRequest(t, &pdu.ReadRegistersRequest{Func: pdu.FuncReadHoldingRegisters, Quantity: 1})
	response = mustHandle(t, slave, framing.Frame{TransactionID: 2, UnitID: 2, PDU: single})
	values = mustResponse(t, response).(*pdu.ReadRegistersResponse).Registers
	if len(values) != 1 || values[0] != 7 {
		t.Fatalf("unit 2 values = %v", values)
	}

	missing := mustHandle(t, slave, framing.Frame{TransactionID: 3, UnitID: 9, PDU: single})
	exception := mustResponse(t, missing).(*pdu.ExceptionResponse)
	if exception.Code != pdu.ExceptionGatewayTargetFailed {
		t.Fatalf("missing unit exception = 0x%02X", uint8(exception.Code))
	}
}

// Covers: REQ-SCOPE-01
func TestSlaveRecordsRequests(t *testing.T) {
	slave := New(map[uint8]*Unit{1: {Holding: map[uint16]uint16{}}})
	mustHandle(t, slave, framing.Frame{TransactionID: 1, UnitID: 1, PDU: mustRequest(t, &pdu.ReadRegistersRequest{Func: pdu.FuncReadHoldingRegisters, Quantity: 1})})
	mustHandle(t, slave, framing.Frame{TransactionID: 2, UnitID: 1, PDU: mustRequest(t, &pdu.WriteSingleRegisterRequest{Address: 2, Value: 50})})
	mustHandle(t, slave, framing.Frame{TransactionID: 3, UnitID: 1, PDU: mustRequest(t, &pdu.WriteMultipleRegistersRequest{Address: 3, Values: []uint16{1, 2}})})

	requests := slave.Requests()
	if len(requests) != 3 {
		t.Fatalf("len(Requests) = %d, want 3", len(requests))
	}
	if requests[0].Func != pdu.FuncReadHoldingRegisters || requests[0].Quantity != 1 {
		t.Fatalf("request 0 = %#v", requests[0])
	}
	if requests[1].Func != pdu.FuncWriteSingleRegister || requests[1].Address != 2 || requests[1].Values[0] != 50 {
		t.Fatalf("request 1 = %#v", requests[1])
	}
	if requests[2].Func != pdu.FuncWriteMultipleRegisters || requests[2].Quantity != 2 || requests[2].Values[1] != 2 {
		t.Fatalf("request 2 = %#v", requests[2])
	}
}

// Covers: REQ-SCOPE-01
func TestSlaveUnknownFunctionMBAP(t *testing.T) {
	slave := New(map[uint8]*Unit{1: {Holding: map[uint16]uint16{}}})
	response := mustHandle(t, slave, framing.Frame{TransactionID: 1, UnitID: 1, PDU: []byte{0x11, 0x00, 0x00}})
	exception := mustResponse(t, response).(*pdu.ExceptionResponse)
	if exception.Func != 0x11 || exception.Code != pdu.ExceptionIllegalFunction {
		t.Fatalf("exception = %#v", exception)
	}
}

// Covers: REQ-SCOPE-01
func TestSlaveSetException(t *testing.T) {
	slave := New(map[uint8]*Unit{1: {Holding: map[uint16]uint16{0: 1}}})
	slave.SetException(1, pdu.FuncReadHoldingRegisters, pdu.ExceptionIllegalDataAddress)
	response := mustHandle(t, slave, framing.Frame{TransactionID: 1, UnitID: 1, PDU: mustRequest(t, &pdu.ReadRegistersRequest{Func: pdu.FuncReadHoldingRegisters, Quantity: 1})})
	exception := mustResponse(t, response).(*pdu.ExceptionResponse)
	if exception.Code != pdu.ExceptionIllegalDataAddress {
		t.Fatalf("exception = %#v", exception)
	}
}

// Covers: REQ-SCOPE-01
func TestSlaveWritesRegisters(t *testing.T) {
	unit := &Unit{Holding: map[uint16]uint16{}}
	slave := New(map[uint8]*Unit{1: unit})
	mustHandle(t, slave, framing.Frame{TransactionID: 1, UnitID: 1, PDU: mustRequest(t, &pdu.WriteSingleRegisterRequest{Address: 2, Value: 50})})
	if unit.Holding[2] != 50 {
		t.Fatalf("holding[2] = %d, want 50", unit.Holding[2])
	}
	mustHandle(t, slave, framing.Frame{TransactionID: 2, UnitID: 1, PDU: mustRequest(t, &pdu.WriteMultipleRegistersRequest{Address: 3, Values: []uint16{7, 8}})})
	if unit.Holding[3] != 7 || unit.Holding[4] != 8 {
		t.Fatalf("holding after multiple = %v", unit.Holding)
	}
}

// Covers: REQ-SCOPE-01
func TestSlaveReadMissingAddress(t *testing.T) {
	slave := New(map[uint8]*Unit{1: {Holding: map[uint16]uint16{0: 1}}})
	response := mustHandle(t, slave, framing.Frame{TransactionID: 1, UnitID: 1, PDU: mustRequest(t, &pdu.ReadRegistersRequest{Func: pdu.FuncReadHoldingRegisters, Address: 1, Quantity: 1})})
	exception := mustResponse(t, response).(*pdu.ExceptionResponse)
	if exception.Code != pdu.ExceptionIllegalDataAddress {
		t.Fatalf("exception = %#v", exception)
	}
}

// Covers: REQ-SCOPE-01
func TestSlaveServeConnExitsOnClose(t *testing.T) {
	server, client := net.Pipe()
	slave := New(map[uint8]*Unit{1: {Holding: map[uint16]uint16{}}})
	done := make(chan error, 1)
	go func() {
		done <- slave.ServeConn(server, framing.NewMBAP())
	}()
	if err := client.Close(); err != nil {
		t.Fatalf("client Close: %v", err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, io.EOF) {
			t.Fatalf("ServeConn err = %v, want io.EOF", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ServeConn did not exit after close")
	}
}

func mustRequest(t *testing.T, req pdu.Request) []byte {
	t.Helper()
	raw, err := pdu.EncodeRequest(req)
	if err != nil {
		t.Fatalf("EncodeRequest: %v", err)
	}
	return raw
}

func mustHandle(t *testing.T, slave *Slave, frame framing.Frame) []byte {
	t.Helper()
	response, err := slave.handle(frame)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	return response
}

func mustResponse(t *testing.T, raw []byte) pdu.Response {
	t.Helper()
	response, err := pdu.DecodeResponse(raw)
	if err != nil {
		t.Fatalf("DecodeResponse: %v", err)
	}
	return response
}
