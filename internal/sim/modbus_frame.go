package sim

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	modbusTCPTransport = "tcp"
	modbusRTUTransport = "rtu_over_tcp"

	modbusMaxPDU        = 253
	modbusMaxMBAPLength = 254
)

var errModbusBadFrame = errors.New("sim: modbus bad frame")

// modbusADU is one transport-independent Modbus application data unit. The
// transaction ID is MBAP-only; RTU ignores it.
type modbusADU struct {
	unitID uint8
	pdu    []byte
	txnID  uint16
}

// modbusFrameError carries a parsed ADU alongside a framing error so the
// request log can still record what was seen. fatal means the stream boundary
// is no longer trustworthy and the connection must be closed.
type modbusFrameError struct {
	msg   string
	fatal bool
	adu   modbusADU
	raw   []byte
}

func (e *modbusFrameError) Error() string { return e.msg }

func (e *modbusFrameError) Unwrap() error { return errModbusBadFrame }

// readMBAP reads one Modbus TCP frame. Header-level failures are fatal
// because the stream cannot be resynchronised.
func readMBAP(r io.Reader) (modbusADU, error) {
	var header [7]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return modbusADU{}, err
	}
	txn := binary.BigEndian.Uint16(header[0:2])
	proto := binary.BigEndian.Uint16(header[2:4])
	length := binary.BigEndian.Uint16(header[4:6])
	unitID := header[6]
	if proto != 0 {
		return modbusADU{unitID: unitID, txnID: txn}, &modbusFrameError{
			msg:   fmt.Sprintf("modbus: MBAP protocol id %d", proto),
			fatal: true,
			adu:   modbusADU{unitID: unitID, txnID: txn},
			raw:   header[:],
		}
	}
	if length < 2 || length > modbusMaxMBAPLength {
		return modbusADU{unitID: unitID, txnID: txn}, &modbusFrameError{
			msg:   fmt.Sprintf("modbus: MBAP length %d", length),
			fatal: true,
			adu:   modbusADU{unitID: unitID, txnID: txn},
			raw:   header[:],
		}
	}
	pdu := make([]byte, int(length)-1)
	if _, err := io.ReadFull(r, pdu); err != nil {
		return modbusADU{}, &modbusFrameError{
			msg:   fmt.Sprintf("modbus: short MBAP frame: %v", err),
			fatal: true,
			adu:   modbusADU{unitID: unitID, txnID: txn},
			raw:   header[:],
		}
	}
	return modbusADU{unitID: unitID, pdu: pdu, txnID: txn}, nil
}

func encodeMBAP(adu modbusADU) ([]byte, error) {
	if len(adu.pdu) < 1 || len(adu.pdu) > modbusMaxPDU {
		return nil, fmt.Errorf("%w: PDU length %d", errModbusBadFrame, len(adu.pdu))
	}
	raw := make([]byte, 7+len(adu.pdu))
	binary.BigEndian.PutUint16(raw[0:2], adu.txnID)
	binary.BigEndian.PutUint16(raw[2:4], 0)
	binary.BigEndian.PutUint16(raw[4:6], uint16(len(adu.pdu)+1))
	raw[6] = adu.unitID
	copy(raw[7:], adu.pdu)
	return raw, nil
}

// readRTU reads one RTU ADU from a TCP stream. Fixed-length functions are read
// exactly; function 16 uses its byte count. An unknown function cannot be
// length-delimited and is fatal.
func readRTU(r *bufio.Reader) (modbusADU, []byte, error) {
	var head [2]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return modbusADU{}, nil, err
	}
	unitID, function := head[0], head[1]

	switch function {
	case 0x03, 0x04, 0x06:
		rest := make([]byte, 6)
		if _, err := io.ReadFull(r, rest); err != nil {
			return modbusADU{}, append(head[:], rest...), &modbusFrameError{
				msg:   fmt.Sprintf("modbus: short RTU frame: %v", err),
				fatal: true,
				adu:   modbusADU{unitID: unitID, pdu: []byte{function}},
				raw:   append(head[:], rest...),
			}
		}
		raw := append(append([]byte(nil), head[:]...), rest...)
		adu := modbusADU{unitID: unitID, pdu: raw[1 : len(raw)-2]}
		if !modbusCRCValid(raw) {
			return adu, raw, &modbusFrameError{
				msg:   "modbus: bad RTU request CRC",
				fatal: false,
				adu:   adu,
				raw:   raw,
			}
		}
		return adu, raw, nil

	case 0x10:
		var mid [5]byte
		if _, err := io.ReadFull(r, mid[:]); err != nil {
			return modbusADU{}, append(head[:], mid[:]...), &modbusFrameError{
				msg:   fmt.Sprintf("modbus: short RTU frame: %v", err),
				fatal: true,
				adu:   modbusADU{unitID: unitID, pdu: []byte{function}},
				raw:   append(head[:], mid[:]...),
			}
		}
		byteCount := int(mid[4])
		data := make([]byte, byteCount+2)
		if _, err := io.ReadFull(r, data); err != nil {
			return modbusADU{}, append(append(head[:], mid[:]...), data...), &modbusFrameError{
				msg:   fmt.Sprintf("modbus: short RTU frame: %v", err),
				fatal: true,
				adu:   modbusADU{unitID: unitID, pdu: []byte{function}},
				raw:   append(append(head[:], mid[:]...), data...),
			}
		}
		raw := append(append(append([]byte(nil), head[:]...), mid[:]...), data...)
		adu := modbusADU{unitID: unitID, pdu: raw[1 : len(raw)-2]}
		if !modbusCRCValid(raw) {
			return adu, raw, &modbusFrameError{
				msg:   "modbus: bad RTU request CRC",
				fatal: false,
				adu:   adu,
				raw:   raw,
			}
		}
		return adu, raw, nil

	default:
		return modbusADU{unitID: unitID, pdu: []byte{function}}, head[:], &modbusFrameError{
			msg:   fmt.Sprintf("modbus: unsupported function 0x%02X on RTU", function),
			fatal: true,
			adu:   modbusADU{unitID: unitID, pdu: []byte{function}},
			raw:   head[:],
		}
	}
}

func encodeRTU(adu modbusADU, corruptCRC bool) ([]byte, error) {
	if len(adu.pdu) < 1 || len(adu.pdu) > modbusMaxPDU {
		return nil, fmt.Errorf("%w: PDU length %d", errModbusBadFrame, len(adu.pdu))
	}
	raw := make([]byte, 1+len(adu.pdu)+2)
	raw[0] = adu.unitID
	copy(raw[1:], adu.pdu)
	crc := modbusCRC16(raw[:len(raw)-2])
	if corruptCRC {
		crc ^= 0xFFFF
	}
	binary.LittleEndian.PutUint16(raw[len(raw)-2:], crc)
	return raw, nil
}

func modbusCRCValid(raw []byte) bool {
	if len(raw) < 4 {
		return false
	}
	want := binary.LittleEndian.Uint16(raw[len(raw)-2:])
	return modbusCRC16(raw[:len(raw)-2]) == want
}

// modbusCRC16 implements CRC-16/MODBUS (reflected 0xA001, init 0xFFFF).
func modbusCRC16(data []byte) uint16 {
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
