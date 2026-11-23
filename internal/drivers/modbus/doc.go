// Package modbus implements the Modbus client exchange layer for three
// transports: Modbus TCP (MBAP), serial RTU and RTU-over-TCP.
//
// The PDU codec lives in modbus/pdu and the ADU framing in modbus/framing.
// This package adds a Conn seam, a serialized request/response exchange and
// error classification. It never reads the runtime clock: callers pass an
// absolute deadline and the Client compares it with the injected Clock.
// 04b builds interfaces.Driver on top of Client; 06a implements its own
// server in internal/sim and must not reuse these types as a production slave.
package modbus
