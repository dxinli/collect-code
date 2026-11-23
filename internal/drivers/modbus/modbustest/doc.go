// Package modbustest provides a minimal Modbus slave peer for tests.
//
// It is test support only: production packages must not import it. It is not
// the 06a simulator. The 06a server lives in internal/sim and must implement
// its own unit routing, fault controls and HTTP API; it must not reuse this
// package to satisfy its acceptance criteria.
//
// The slave supports functions 03, 04, 06 and 16, multiple unit ids, request
// logging and exception injection. Framing errors are returned to the caller;
// for RTU that means the stream must be rebuilt.
package modbustest
