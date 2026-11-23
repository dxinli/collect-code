package framing

import (
	"errors"
	"fmt"
)

// CRC16 returns the CRC-16/MODBUS of data: reflected polynomial 0xA001,
// initial value 0xFFFF, no final XOR.
func CRC16(data []byte) uint16 {
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

// checkCRC validates an RTU ADU whose last two bytes are the CRC, low byte
// first. Every failure is unrecoverable from the framing layer.
func checkCRC(raw []byte) error {
	if len(raw) < 4 {
		return unrecoverable(errors.Join(ErrShortFrame, fmt.Errorf("frame length %d", len(raw))))
	}
	want := uint16(raw[len(raw)-2]) | uint16(raw[len(raw)-1])<<8
	got := CRC16(raw[:len(raw)-2])
	if got != want {
		return unrecoverable(fmt.Errorf("%w: got 0x%04X want 0x%04X", ErrBadCRC, got, want))
	}
	return nil
}

func appendCRC(adu []byte) []byte {
	crc := CRC16(adu)
	return append(adu, byte(crc), byte(crc>>8))
}
