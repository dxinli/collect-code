// Package mqtt is the production northbound MQTT channel. It implements
// interfaces.Channel on top of paho, with repository-controlled reconnect,
// last-will and subscription re-establishment.
package mqtt

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/dxinli/collect-code/internal/interfaces"
)

const (
	// connectTimeout bounds one TCP/TLS connect and the post-cancel cleanup.
	connectTimeout = 10 * time.Second
	// writeTimeout bounds paho's outbound queue wait.
	writeTimeout = 5 * time.Second
	// disconnectQuiesce lets paho write DISCONNECT before dropping the socket.
	// A zero quiesce races with the comms worker and occasionally triggers the
	// broker-side will.
	disconnectQuiesce = 250 * time.Millisecond
	// publishWait bounds a connected QoS1 publish that never receives PUBACK.
	publishWait = 10 * time.Second
	// subscribeBuffer is the per-subscription delivery buffer.
	subscribeBuffer = 16
	// deliveryBlockTimeout bounds one slow delivery before the subscription is
	// closed as overflowed; staying well below the paho ping timeout avoids a
	// self-inflicted reconnect.
	deliveryBlockTimeout = 250 * time.Millisecond
	// subscribeRetry re-attempts a failed SUBACK while the connection lives.
	subscribeRetry = 5 * time.Second
)

// Channel is the production northbound transport.
type Channel struct {
	opts  Options
	deriv derivedOptions
	clock interfaces.Clock
	log   *slog.Logger
	dial  dialer

	mu         sync.Mutex
	closed     bool
	started    bool
	closedCh   chan struct{}
	runDone    chan struct{}
	transport  transport
	subs       map[string]*subscription
	failedSubs map[string]struct{}
	status     interfaces.ChannelStatus

	down      chan struct{}
	downOnce  sync.Once
	upOnce    sync.Once
	cleanupWG sync.WaitGroup
}

// New validates opts and starts the channel. It does not wait for the first
// connection: the edge may boot while the cloud is unreachable.
func New(opts Options) (*Channel, error) {
	c, err := newChannel(opts, nil)
	if err != nil {
		return nil, err
	}
	c.start()
	return c, nil
}

// newChannel validates opts and builds a channel without starting it. Tests
// use it to inject a fake dialer.
func newChannel(opts Options, d dialer) (*Channel, error) {
	deriv, err := validateOptions(opts)
	if err != nil {
		return nil, err
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	c := &Channel{
		opts:       opts,
		deriv:      deriv,
		clock:      opts.Clock,
		log:        logger.With("component", "mqtt", "broker", deriv.brokerLabel, "client_id", deriv.clientID),
		dial:       d,
		closedCh:   make(chan struct{}),
		runDone:    make(chan struct{}),
		subs:       make(map[string]*subscription),
		failedSubs: make(map[string]struct{}),
		down:       make(chan struct{}),
		status: interfaces.ChannelStatus{
			Connected: false,
			ChangedAt: opts.Clock.Now(),
		},
	}
	if c.dial == nil {
		c.dial = &pahoDialer{clock: opts.Clock, log: c.log, registerCleanup: c.registerCleanup}
	}
	if deriv.warnUserinfo {
		c.log.Warn("broker userinfo takes precedence over northbound.mqtt.username/password")
	}
	if opts.KeepAlive == 0 {
		c.log.Warn("keepalive disabled")
	}
	return c, nil
}

// start launches the connection loop. It is called once by New.
func (c *Channel) start() {
	c.mu.Lock()
	if c.started || c.closed {
		c.mu.Unlock()
		return
	}
	c.started = true
	c.mu.Unlock()
	go c.run()
}

// Publish sends one northbound message.
func (c *Channel) Publish(ctx context.Context, msg interfaces.Message) error {
	if c.isClosed() {
		return fmt.Errorf("mqtt: publish %q: %w", msg.Topic, interfaces.ErrClosed)
	}
	return fmt.Errorf("mqtt: publish %q: %w", msg.Topic, interfaces.ErrNotConnected)
}

// Subscribe registers an inbound filter.
func (c *Channel) Subscribe(ctx context.Context, filter string) (interfaces.Subscription, error) {
	if c.isClosed() {
		return nil, fmt.Errorf("mqtt: subscribe %q: %w", filter, interfaces.ErrClosed)
	}
	return nil, fmt.Errorf("mqtt: subscribe %q: %w", filter, interfaces.ErrNotConnected)
}

// Status returns a copy of the current channel state.
func (c *Channel) Status() interfaces.ChannelStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

// Close stops the channel. It is idempotent.
func (c *Channel) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
	} else {
		c.closed = true
		close(c.closedCh)
		c.mu.Unlock()
	}
	return nil
}

func (c *Channel) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

// registerCleanup runs fn in a goroutine tracked by Close.
func (c *Channel) registerCleanup(fn func()) {
	c.cleanupWG.Go(fn)
}

func (c *Channel) redact(s string) string {
	for _, secret := range c.deriv.secrets {
		if secret == "" {
			continue
		}
		s = strings.ReplaceAll(s, secret, "[REDACTED]")
	}
	return s
}

// run is filled in by the lifecycle implementation.
func (c *Channel) run() {
	<-c.closedCh
	close(c.runDone)
}

var _ interfaces.Channel = (*Channel)(nil)
