package sim

import (
	"bufio"
	"bytes"
	"errors"
	"testing"
)

func TestModbusCRC16KnownVector(t *testing.T) {
	got := modbusCRC16([]byte{0x01, 0x03, 0x00, 0x00, 0x00, 0x02})
	if got != 0x0BC4 {
		t.Fatalf("crc = 0x%04X, want 0x0BC4", got)
	}
}

func TestModbusBuiltinInitialValues(t *testing.T) {
	unit := newUnitState(1, nil)
	cases := []struct {
		key  modbusKey
		want uint16
	}{
		{modbusKey{address: 0x0000}, 0},
		{modbusKey{address: 0x0001}, 500},
		{modbusKey{address: 0x0002}, 500},
		{modbusKey{address: 0x0009}, 0},
		{modbusKey{address: 0x000B}, 0x0102},
		{modbusKey{input: true, address: 0x0000}, 185},
		{modbusKey{input: true, address: 0x0001}, 352},
		{modbusKey{input: true, address: 0x0002}, 780},
		{modbusKey{input: true, address: 0x0003}, 680},
		{modbusKey{input: true, address: 0x0004}, 215},
		{modbusKey{input: true, address: 0x0005}, 42},
		{modbusKey{input: true, address: 0x0006}, 0},
		{modbusKey{input: true, address: 0x0007}, 123},
		{modbusKey{input: true, address: 0x000C}, 0},
		{modbusKey{input: true, address: 0x000D}, 1000},
	}
	for _, tc := range cases {
		if got := unit.registers[tc.key].value; got != tc.want {
			t.Errorf("register %+v = %d, want %d", tc.key, got, tc.want)
		}
	}
}

func TestModbusWriteSemanticsDirect(t *testing.T) {
	unit := newUnitState(1, nil)
	if code := unit.writeSingle(0x0002, 750); code != 0 {
		t.Fatalf("write target code = %d", code)
	}
	if got := unit.registers[modbusKey{address: 0x0001}].value; got != 750 {
		t.Fatalf("current = %d, want 750", got)
	}
	if code := unit.writeSingle(0x0001, 1); code != modbusExceptionIllegalDataAddress {
		t.Fatalf("write read-only code = %d, want 0x02", code)
	}
	if code := unit.writeMultiple(0x0002, []uint16{100, 0}); code != 0 {
		t.Fatalf("write multiple code = %d", code)
	}
	if got := unit.registers[modbusKey{address: 0x0001}].value; got != 100 {
		t.Fatalf("current = %d, want 100", got)
	}
	if code := unit.writeMultiple(0x0002, []uint16{0, 0, 0}); code != modbusExceptionIllegalDataAddress {
		t.Fatalf("write past writable range code = %d, want 0x02", code)
	}
}

func TestModbusRequestLogDropsOldest(t *testing.T) {
	log := newModbusRequestLog(2)
	log.add(modbusRequestEntry{Outcome: modbusOutcomeOK})
	log.add(modbusRequestEntry{Outcome: modbusOutcomeOK})
	log.add(modbusRequestEntry{Outcome: modbusOutcomeOK})

	entries, dropped := log.snapshot()
	if dropped != 1 {
		t.Fatalf("dropped = %d, want 1", dropped)
	}
	if len(entries) != 2 || entries[0].Seq != 2 || entries[1].Seq != 3 {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestModbusParseRequestBounds(t *testing.T) {
	req, err := parseModbusRequest([]byte{0x03, 0x00, 0x01, 0x00, 0x02})
	if err != nil {
		t.Fatalf("parse read: %v", err)
	}
	if req.address != 1 || req.quantity != 2 {
		t.Fatalf("req = %+v", req)
	}
	_, err = parseModbusRequest([]byte{0x03, 0x00, 0x01, 0x00, 0x00})
	if code := pduErrorCode(t, err); code != modbusExceptionIllegalDataValue {
		t.Fatalf("zero quantity code = %d", code)
	}
	_, err = parseModbusRequest([]byte{0x2B})
	if code := pduErrorCode(t, err); code != modbusExceptionIllegalFunction {
		t.Fatalf("unknown function code = %d", code)
	}
	_, err = parseModbusRequest([]byte{0x10, 0x00, 0x01, 0x00, 0x02, 0x02, 0x00, 0x01})
	if code := pduErrorCode(t, err); code != modbusExceptionIllegalDataValue {
		t.Fatalf("byte count mismatch code = %d", code)
	}
}

func pduErrorCode(t *testing.T, err error) uint8 {
	t.Helper()
	var pduErr *modbusPDUError
	if !errors.As(err, &pduErr) {
		t.Fatalf("want modbusPDUError, got %v", err)
	}
	return pduErr.code
}

func TestModbusReadRTUUnknownFunctionIsFatal(t *testing.T) {
	reader := bufio.NewReader(bytes.NewReader([]byte{0x01, 0x2B}))
	adu, _, err := readRTU(reader)
	var frameErr *modbusFrameError
	if !errors.As(err, &frameErr) {
		t.Fatalf("want frame error, got %v", err)
	}
	if !frameErr.fatal {
		t.Fatal("unknown RTU function must be fatal")
	}
	if adu.unitID != 1 {
		t.Fatalf("unit = %d", adu.unitID)
	}
}

func TestModbusReadRTUBadCRCIsNotFatal(t *testing.T) {
	reader := bufio.NewReader(bytes.NewReader([]byte{0x01, 0x03, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00}))
	_, _, err := readRTU(reader)
	var frameErr *modbusFrameError
	if !errors.As(err, &frameErr) {
		t.Fatalf("want frame error, got %v", err)
	}
	if frameErr.fatal {
		t.Fatal("bad CRC frame boundary is known; must not be fatal")
	}
}

func TestModbusReadMBAPBadProtocolIsFatal(t *testing.T) {
	raw := []byte{0x00, 0x01, 0x00, 0x01, 0x00, 0x02, 0x01}
	_, err := readMBAP(bytes.NewReader(raw))
	var frameErr *modbusFrameError
	if !errors.As(err, &frameErr) {
		t.Fatalf("want frame error, got %v", err)
	}
	if !frameErr.fatal {
		t.Fatal("bad MBAP protocol id must be fatal")
	}
}
