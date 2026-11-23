package modbustest

import (
	"context"
	"errors"
	"io"
	"net"
	"slices"
	"sync"

	"github.com/dxinli/collect-code/internal/drivers/modbus/framing"
	"github.com/dxinli/collect-code/internal/drivers/modbus/pdu"
)

// Unit is one unit id's register file.
type Unit struct {
	Holding map[uint16]uint16
	Input   map[uint16]uint16
}

// Request is one request recorded by the slave.
type Request struct {
	TransactionID uint16
	UnitID        uint8
	Func          pdu.FuncCode
	Address       uint16
	Quantity      uint16
	Values        []uint16
}

// Slave is an in-memory Modbus server for tests.
type Slave struct {
	mu         sync.Mutex
	units      map[uint8]*Unit
	requests   []Request
	exceptions map[exceptionKey]pdu.ExceptionCode
}

type exceptionKey struct {
	unitID uint8
	fn     pdu.FuncCode
}

// New returns a slave serving the given units. It takes ownership of the map.
func New(units map[uint8]*Unit) *Slave {
	if units == nil {
		units = map[uint8]*Unit{}
	}
	return &Slave{units: units, exceptions: map[exceptionKey]pdu.ExceptionCode{}}
}

// ServeConn decodes requests until the stream fails. Framing errors are
// returned unchanged; callers must close and rebuild the connection.
func (s *Slave) ServeConn(conn io.ReadWriteCloser, fr framing.Framer) error {
	for {
		frame, err := fr.DecodeRequest(conn)
		if err != nil {
			return err
		}
		response, err := s.handle(frame)
		if err != nil {
			return err
		}
		encoded, err := fr.Encode(framing.Frame{
			TransactionID: frame.TransactionID,
			UnitID:        frame.UnitID,
			PDU:           response,
		})
		if err != nil {
			return err
		}
		if err := writeFull(conn, encoded); err != nil {
			return err
		}
	}
}

// ServeTCP accepts connections until the listener is closed or ctx is done.
// The caller closes the listener to stop the accept loop.
func (s *Slave) ServeTCP(ctx context.Context, listener net.Listener, fr framing.Framer) error {
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = listener.Close()
		case <-done:
		}
	}()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		go func() {
			defer func() { _ = conn.Close() }()
			_ = s.ServeConn(conn, fr)
		}()
	}
}

// Requests returns a copy of the request log in arrival order.
func (s *Slave) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Request, len(s.requests))
	for i, request := range s.requests {
		out[i] = request
		out[i].Values = append([]uint16(nil), request.Values...)
	}
	return out
}

// Units returns the served unit ids in ascending order.
func (s *Slave) Units() []uint8 {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]uint8, 0, len(s.units))
	for id := range s.units {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// SetException makes the slave answer fn on unitID with code.
func (s *Slave) SetException(unitID uint8, fn pdu.FuncCode, code pdu.ExceptionCode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.exceptions[exceptionKey{unitID: unitID, fn: fn}] = code
}

func (s *Slave) handle(frame framing.Frame) ([]byte, error) {
	if len(frame.PDU) == 0 {
		return nil, pdu.ErrMalformed
	}
	fn := pdu.FuncCode(frame.PDU[0])
	req, err := pdu.DecodeRequest(frame.PDU)
	if err != nil {
		if errors.Is(err, pdu.ErrUnsupportedFunction) {
			return exception(fn, pdu.ExceptionIllegalFunction)
		}
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.recordLocked(frame, req)

	unit := s.units[frame.UnitID]
	if unit == nil {
		// Gateway-style test convention; 06a must not copy this behaviour.
		return exception(fn, pdu.ExceptionGatewayTargetFailed)
	}
	if code, ok := s.exceptions[exceptionKey{unitID: frame.UnitID, fn: fn}]; ok {
		return exception(fn, code)
	}

	switch request := req.(type) {
	case *pdu.ReadRegistersRequest:
		return s.readLocked(unit, request)
	case *pdu.WriteSingleRegisterRequest:
		s.ensureHolding(unit)[request.Address] = request.Value
		return pdu.EncodeResponse(&pdu.WriteSingleRegisterResponse{
			Address: request.Address,
			Value:   request.Value,
		})
	case *pdu.WriteMultipleRegistersRequest:
		holding := s.ensureHolding(unit)
		for i, value := range request.Values {
			holding[request.Address+uint16(i)] = value
		}
		return pdu.EncodeResponse(&pdu.WriteMultipleRegistersResponse{
			Address:  request.Address,
			Quantity: uint16(len(request.Values)),
		})
	default:
		return exception(fn, pdu.ExceptionIllegalFunction)
	}
}

func (s *Slave) readLocked(unit *Unit, req *pdu.ReadRegistersRequest) ([]byte, error) {
	registers := unit.Holding
	if req.Func == pdu.FuncReadInputRegisters {
		registers = unit.Input
	}
	values := make([]uint16, req.Quantity)
	for i := range values {
		value, ok := registers[req.Address+uint16(i)]
		if !ok {
			return exception(req.Func, pdu.ExceptionIllegalDataAddress)
		}
		values[i] = value
	}
	return pdu.EncodeResponse(&pdu.ReadRegistersResponse{Func: req.Func, Registers: values})
}

func (s *Slave) recordLocked(frame framing.Frame, req pdu.Request) {
	entry := Request{TransactionID: frame.TransactionID, UnitID: frame.UnitID}
	switch request := req.(type) {
	case *pdu.ReadRegistersRequest:
		entry.Func = request.Func
		entry.Address = request.Address
		entry.Quantity = request.Quantity
	case *pdu.WriteSingleRegisterRequest:
		entry.Func = pdu.FuncWriteSingleRegister
		entry.Address = request.Address
		entry.Quantity = 1
		entry.Values = []uint16{request.Value}
	case *pdu.WriteMultipleRegistersRequest:
		entry.Func = pdu.FuncWriteMultipleRegisters
		entry.Address = request.Address
		entry.Quantity = uint16(len(request.Values))
		entry.Values = append([]uint16(nil), request.Values...)
	}
	s.requests = append(s.requests, entry)
}

func (s *Slave) ensureHolding(unit *Unit) map[uint16]uint16 {
	if unit.Holding == nil {
		unit.Holding = map[uint16]uint16{}
	}
	return unit.Holding
}

func exception(fn pdu.FuncCode, code pdu.ExceptionCode) ([]byte, error) {
	return pdu.EncodeResponse(&pdu.ExceptionResponse{Func: fn, Code: code})
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
