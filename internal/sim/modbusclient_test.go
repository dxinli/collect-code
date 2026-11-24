package sim_test

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

// testModbusClient is an independent Modbus client. It deliberately does not
// import internal/drivers/modbus: the simulator must be validated by a second
// implementation, not by the code under test.
type testModbusClient struct {
	conn      net.Conn
	reader    *bufio.Reader
	transport string
	txn       uint16
}

type testModbusResponse struct {
	Function  uint8
	Exception uint8
	Values    []uint16
	Address   uint16
	Value     uint16
	Quantity  uint16
}

type testModbusException struct {
	code uint8
}

func (e *testModbusException) Error() string {
	return fmt.Sprintf("modbus exception 0x%02X", e.code)
}

func dialTestModbusTCP(t *testing.T, addr string) *testModbusClient {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial modbus tcp %s: %v", addr, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return &testModbusClient{conn: conn, reader: bufio.NewReader(conn), transport: "tcp"}
}

func dialTestModbusRTU(t *testing.T, addr string) *testModbusClient {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial modbus rtu-over-tcp %s: %v", addr, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return &testModbusClient{conn: conn, reader: bufio.NewReader(conn), transport: "rtu_over_tcp"}
}

func (c *testModbusClient) exchange(unit uint8, pdu []byte, timeout time.Duration) (testModbusResponse, error) {
	if err := c.conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return testModbusResponse{}, err
	}
	switch c.transport {
	case "tcp":
		c.txn++
		raw := make([]byte, 7+len(pdu))
		binary.BigEndian.PutUint16(raw[0:2], c.txn)
		binary.BigEndian.PutUint16(raw[2:4], 0)
		binary.BigEndian.PutUint16(raw[4:6], uint16(len(pdu)+1))
		raw[6] = unit
		copy(raw[7:], pdu)
		if _, err := c.conn.Write(raw); err != nil {
			return testModbusResponse{}, err
		}
		return c.readTCP(unit)
	case "rtu_over_tcp":
		raw := testRTUFrame(unit, pdu)
		if _, err := c.conn.Write(raw); err != nil {
			return testModbusResponse{}, err
		}
		return c.readRTU(unit)
	default:
		return testModbusResponse{}, fmt.Errorf("unknown transport %q", c.transport)
	}
}

func (c *testModbusClient) readTCP(unit uint8) (testModbusResponse, error) {
	var header [7]byte
	if _, err := io.ReadFull(c.reader, header[:]); err != nil {
		return testModbusResponse{}, err
	}
	if binary.BigEndian.Uint16(header[0:2]) != c.txn {
		return testModbusResponse{}, fmt.Errorf("transaction mismatch")
	}
	if binary.BigEndian.Uint16(header[2:4]) != 0 {
		return testModbusResponse{}, fmt.Errorf("protocol id mismatch")
	}
	length := binary.BigEndian.Uint16(header[4:6])
	if length < 2 || length > 254 {
		return testModbusResponse{}, fmt.Errorf("bad MBAP length %d", length)
	}
	if header[6] != unit {
		return testModbusResponse{}, fmt.Errorf("unit mismatch")
	}
	pdu := make([]byte, int(length)-1)
	if _, err := io.ReadFull(c.reader, pdu); err != nil {
		return testModbusResponse{}, err
	}
	return testParseResponse(pdu)
}

func (c *testModbusClient) readRTU(unit uint8) (testModbusResponse, error) {
	var head [2]byte
	if _, err := io.ReadFull(c.reader, head[:]); err != nil {
		return testModbusResponse{}, err
	}
	if head[0] != unit {
		return testModbusResponse{}, fmt.Errorf("unit mismatch: got %d want %d", head[0], unit)
	}
	function := head[1]
	if function&0x80 != 0 {
		rest := make([]byte, 3)
		if _, err := io.ReadFull(c.reader, rest); err != nil {
			return testModbusResponse{}, err
		}
		raw := append(head[:], rest...)
		if !testCRCOK(raw) {
			return testModbusResponse{}, errors.New("bad response crc")
		}
		return testModbusResponse{Function: function & 0x7F, Exception: rest[0]}, nil
	}
	switch function {
	case 0x03, 0x04:
		var count [1]byte
		if _, err := io.ReadFull(c.reader, count[:]); err != nil {
			return testModbusResponse{}, err
		}
		body := make([]byte, int(count[0])+2)
		if _, err := io.ReadFull(c.reader, body); err != nil {
			return testModbusResponse{}, err
		}
		raw := append(append(head[:], count[0]), body...)
		if !testCRCOK(raw) {
			return testModbusResponse{}, errors.New("bad response crc")
		}
		values := make([]uint16, int(count[0])/2)
		for i := range values {
			values[i] = binary.BigEndian.Uint16(body[i*2:])
		}
		return testModbusResponse{Function: function, Values: values}, nil
	case 0x06:
		rest := make([]byte, 6)
		if _, err := io.ReadFull(c.reader, rest); err != nil {
			return testModbusResponse{}, err
		}
		raw := append(head[:], rest...)
		if !testCRCOK(raw) {
			return testModbusResponse{}, errors.New("bad response crc")
		}
		return testModbusResponse{
			Function: function,
			Address:  binary.BigEndian.Uint16(rest[0:2]),
			Value:    binary.BigEndian.Uint16(rest[2:4]),
		}, nil
	case 0x10:
		rest := make([]byte, 6)
		if _, err := io.ReadFull(c.reader, rest); err != nil {
			return testModbusResponse{}, err
		}
		raw := append(head[:], rest...)
		if !testCRCOK(raw) {
			return testModbusResponse{}, errors.New("bad response crc")
		}
		return testModbusResponse{
			Function: function,
			Address:  binary.BigEndian.Uint16(rest[0:2]),
			Quantity: binary.BigEndian.Uint16(rest[2:4]),
		}, nil
	default:
		return testModbusResponse{}, fmt.Errorf("unexpected function 0x%02X", function)
	}
}

func testParseResponse(pdu []byte) (testModbusResponse, error) {
	if len(pdu) < 2 {
		return testModbusResponse{}, fmt.Errorf("short PDU")
	}
	function := pdu[0]
	if function&0x80 != 0 {
		return testModbusResponse{Function: function & 0x7F, Exception: pdu[1]}, nil
	}
	switch function {
	case 0x03, 0x04:
		if len(pdu) < 2 || int(pdu[1]) != len(pdu)-2 {
			return testModbusResponse{}, fmt.Errorf("bad read response length")
		}
		values := make([]uint16, int(pdu[1])/2)
		for i := range values {
			values[i] = binary.BigEndian.Uint16(pdu[2+i*2:])
		}
		return testModbusResponse{Function: function, Values: values}, nil
	case 0x06:
		if len(pdu) != 5 {
			return testModbusResponse{}, fmt.Errorf("bad write response length")
		}
		return testModbusResponse{
			Function: function,
			Address:  binary.BigEndian.Uint16(pdu[1:3]),
			Value:    binary.BigEndian.Uint16(pdu[3:5]),
		}, nil
	case 0x10:
		if len(pdu) != 5 {
			return testModbusResponse{}, fmt.Errorf("bad write-multiple response length")
		}
		return testModbusResponse{
			Function: function,
			Address:  binary.BigEndian.Uint16(pdu[1:3]),
			Quantity: binary.BigEndian.Uint16(pdu[3:5]),
		}, nil
	default:
		return testModbusResponse{}, fmt.Errorf("unexpected function 0x%02X", function)
	}
}

func (c *testModbusClient) readHolding(unit uint8, address, quantity uint16) ([]uint16, error) {
	resp, err := c.exchange(unit, testReadPDU(0x03, address, quantity), time.Second)
	if err != nil {
		return nil, err
	}
	if resp.Exception != 0 {
		return nil, &testModbusException{code: resp.Exception}
	}
	return resp.Values, nil
}

func (c *testModbusClient) readInput(unit uint8, address, quantity uint16) ([]uint16, error) {
	resp, err := c.exchange(unit, testReadPDU(0x04, address, quantity), time.Second)
	if err != nil {
		return nil, err
	}
	if resp.Exception != 0 {
		return nil, &testModbusException{code: resp.Exception}
	}
	return resp.Values, nil
}

func (c *testModbusClient) writeSingle(unit uint8, address, value uint16) error {
	pdu := make([]byte, 5)
	pdu[0] = 0x06
	binary.BigEndian.PutUint16(pdu[1:3], address)
	binary.BigEndian.PutUint16(pdu[3:5], value)
	resp, err := c.exchange(unit, pdu, time.Second)
	if err != nil {
		return err
	}
	if resp.Exception != 0 {
		return &testModbusException{code: resp.Exception}
	}
	if resp.Address != address || resp.Value != value {
		return fmt.Errorf("write echo = %d/%d, want %d/%d", resp.Address, resp.Value, address, value)
	}
	return nil
}

func (c *testModbusClient) writeMultiple(unit uint8, address uint16, values []uint16) error {
	pdu := make([]byte, 6+len(values)*2)
	pdu[0] = 0x10
	binary.BigEndian.PutUint16(pdu[1:3], address)
	binary.BigEndian.PutUint16(pdu[3:5], uint16(len(values)))
	pdu[5] = byte(len(values) * 2)
	for i, value := range values {
		binary.BigEndian.PutUint16(pdu[6+i*2:], value)
	}
	resp, err := c.exchange(unit, pdu, time.Second)
	if err != nil {
		return err
	}
	if resp.Exception != 0 {
		return &testModbusException{code: resp.Exception}
	}
	if resp.Address != address || resp.Quantity != uint16(len(values)) {
		return fmt.Errorf("write-multiple echo = %d/%d", resp.Address, resp.Quantity)
	}
	return nil
}

func (c *testModbusClient) sendRaw(raw []byte) error {
	return writeTestAll(c.conn, raw)
}

func (c *testModbusClient) readRaw(timeout time.Duration) ([]byte, error) {
	if err := c.conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}
	var buf [512]byte
	n, err := c.reader.Read(buf[:])
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), buf[:n]...), nil
}

func testReadPDU(function uint8, address, quantity uint16) []byte {
	pdu := make([]byte, 5)
	pdu[0] = function
	binary.BigEndian.PutUint16(pdu[1:3], address)
	binary.BigEndian.PutUint16(pdu[3:5], quantity)
	return pdu
}

func testRTUFrame(unit uint8, pdu []byte) []byte {
	raw := make([]byte, 1+len(pdu)+2)
	raw[0] = unit
	copy(raw[1:], pdu)
	crc := testCRC16(raw[:len(raw)-2])
	binary.LittleEndian.PutUint16(raw[len(raw)-2:], crc)
	return raw
}

func testCRCOK(raw []byte) bool {
	if len(raw) < 4 {
		return false
	}
	want := binary.LittleEndian.Uint16(raw[len(raw)-2:])
	return testCRC16(raw[:len(raw)-2]) == want
}

func testCRC16(data []byte) uint16 {
	crc := uint16(0xFFFF)
	for _, b := range data {
		crc ^= uint16(b)
		for range 8 {
			if crc&1 != 0 {
				crc = (crc >> 1) ^ 0xA001
			} else {
				crc >>= 1
			}
		}
	}
	return crc
}

func writeTestAll(conn net.Conn, raw []byte) error {
	for len(raw) > 0 {
		n, err := conn.Write(raw)
		if err != nil {
			return err
		}
		raw = raw[n:]
	}
	return nil
}
