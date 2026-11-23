// Package pdu implements the Modbus application protocol data units for
// functions 03, 04, 06 and 16.
//
// The package is pure: it performs no I/O, reads no clock and does not import
// the kernel interfaces. Multi-byte integers use big-endian byte order per the
// Modbus specification. Register values stay uint16; interpreting combinations
// of registers (32-bit integers, floats, scaling) belongs to the driver.
//
// Requests and responses are sealed concrete types behind Request and
// Response: only this package can implement them, so every encoder and decoder
// shares the same range checks.
package pdu
