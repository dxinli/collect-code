package mqtt

import (
	"context"
	"crypto/tls"
	"time"

	"github.com/dxinli/collect-code/internal/interfaces"
)

// dialConfig is the immutable connection attempt configuration derived from
// Options. Each attempt creates a fresh transport.
type dialConfig struct {
	BrokerURL   string
	ClientID    string
	Username    string
	Password    string
	QoS         interfaces.QoS
	KeepAlive   time.Duration
	TLSConfig   *tls.Config
	WillTopic   string
	WillPayload []byte
	WillQoS     byte
	WillRetain  bool
}

// dialer is the connection seam. Production uses paho; L1 tests inject a
// deterministic fake.
type dialer interface {
	Dial(ctx context.Context, cfg dialConfig) (transport, error)
}

// transport is one live MQTT connection. A lost connection is reported once
// on Lost; the channel discards the transport and dials again.
type transport interface {
	Publish(ctx context.Context, msg interfaces.Message) error
	Subscribe(ctx context.Context, filter string, qos interfaces.QoS, deliver func(interfaces.Message)) error
	Lost() <-chan error
	IsConnected() bool
	Close() error
}
