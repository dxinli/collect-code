package interfaces

import (
	"context"
	"time"
)

// QoS is the northbound publish quality of service.
type QoS uint8

const (
	QoS0 QoS = 0
	QoS1 QoS = 1
)

// Message is one northbound or inbound wire message.
type Message struct {
	Topic    string
	Payload  []byte
	QoS      QoS
	Retain   bool
	DedupKey string
}

// Subscription delivers inbound messages until it is closed.
type Subscription interface {
	C() <-chan Message
	Close() error
}

// ChannelStatus is a snapshot of the northbound transport state.
type ChannelStatus struct {
	Connected bool
	LastError string
	ChangedAt time.Time
}

// Channel is the northbound transport seam.
//
// Publish MUST return ErrNotConnected or ErrClosed (wrapped with %w) when the
// transport is not usable, so callers can classify outages with errors.Is.
type Channel interface {
	Publish(ctx context.Context, msg Message) error
	Subscribe(ctx context.Context, filter string) (Subscription, error)
	Status() ChannelStatus
	Close() error
}
