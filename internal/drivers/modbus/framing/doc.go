// Package framing encodes and decodes Modbus application data units (ADUs).
//
// It supports two framings over an io.Reader/Writer seam:
//
//   - MBAP (Modbus TCP): transaction id, protocol id, length, unit id, PDU.
//   - RTU (serial and RTU-over-TCP): unit id, PDU, CRC16.
//
// RTU-over-TCP reuses the RTU framing unchanged: the RTU ADU is carried
// directly on a TCP stream. The package performs no I/O scheduling and reads
// no clock. Decode errors that leave the stream boundary unknown wrap
// ErrUnrecoverable; the caller must close and rebuild the connection.
package framing
