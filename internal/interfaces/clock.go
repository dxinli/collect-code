package interfaces

import "time"

// Clock is the only source of time for kernel and driver code.
// Business deadlines MUST be derived from this interface, never from
// context.WithTimeout or package-level time functions.
type Clock interface {
	Now() time.Time
	NewTicker(d time.Duration) Ticker
	NewTimer(d time.Duration) Timer
	Sleep(d time.Duration)
}

// Ticker is a best-effort periodic source. Stop does not close C and is
// idempotent. Delivery is best-effort: when the receiver is not ready a tick
// may be dropped. The interface makes no promise about channel capacity;
// Go 1.27 time.Ticker delivers on an unbuffered channel.
type Ticker interface {
	C() <-chan time.Time
	Stop()
}

// Timer mirrors the subset of time.Timer used by the kernel. Stop does not
// close C. The kernel never relies on Reset; each deadline creates a new
// Timer so a fake clock can advance deterministically. Go 1.27 time.Timer
// delivers on an unbuffered channel; implementations must not assume
// buffering or that a stopped timer's channel is empty.
type Timer interface {
	C() <-chan time.Time
	Stop()
}
