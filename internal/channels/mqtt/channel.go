// Package mqtt is the production northbound MQTT channel. It implements
// interfaces.Channel on top of paho, with repository-controlled reconnect,
// last-will and subscription re-establishment.
package mqtt

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/dxinli/collect-code/internal/interfaces"
	"github.com/dxinli/collect-code/internal/topics"
)

const (
	// writeTimeout bounds paho's outbound queue wait.
	writeTimeout = 5 * time.Second
	// disconnectQuiesce lets paho write DISCONNECT before dropping the socket.
	// A zero quiesce races with the comms worker and occasionally triggers the
	// broker-side will.
	disconnectQuiesce = 250 * time.Millisecond
	// subscribeBuffer is the per-subscription delivery buffer.
	subscribeBuffer = 16
	// deliveryBlockTimeout bounds one slow delivery before the subscription is
	// closed as overflowed; staying well below the paho ping timeout avoids a
	// self-inflicted reconnect.
	deliveryBlockTimeout = 250 * time.Millisecond
	// subscribeRetry re-attempts a failed SUBACK while the connection lives.
	subscribeRetry = 5 * time.Second
)

var (
	errDuplicateFilter = errors.New("mqtt: filter is already subscribed")

	// connectTimeout bounds one TCP/TLS connect and the post-cancel cleanup.
	// It is a variable only so integration tests can shorten a silent connect.
	connectTimeout = 10 * time.Second
	// publishWait bounds a connected QoS1 publish that never receives PUBACK.
	publishWait = 10 * time.Second
)

// Channel is the production northbound transport.
type Channel struct {
	opts   Options
	deriv  derivedOptions
	clock  interfaces.Clock
	log    *slog.Logger
	dial   dialer
	jitter jitterFunc

	mu         sync.Mutex
	closed     bool
	started    bool
	closedCh   chan struct{}
	runDone    chan struct{}
	transport  transport
	subs       map[string]*subscription
	failedSubs map[string]struct{}
	status     interfaces.ChannelStatus
	dialCancel context.CancelFunc
	connCancel context.CancelFunc

	subOpMu   sync.Mutex
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
		jitter:     fnvJitter,
		closedCh:   make(chan struct{}),
		runDone:    make(chan struct{}),
		subs:       make(map[string]*subscription),
		failedSubs: make(map[string]struct{}),
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

func (c *Channel) run() {
	defer close(c.runDone)
	attempt := 0
	for {
		if c.isClosed() {
			return
		}

		dialCtx, dialCancel := context.WithCancel(context.Background())
		c.setDialCancel(dialCancel)
		tr, err := c.dial.Dial(dialCtx, c.dialConfig())
		dialCancel()
		if err != nil {
			if c.isClosed() {
				return
			}
			c.setDisconnected(err)
			delay := c.backoff(attempt)
			c.log.Warn("connect failed",
				"attempt", attempt,
				"backoff_ms", delay.Milliseconds(),
				"reason", c.redact(err.Error()))
			if !c.wait(delay) {
				return
			}
			attempt++
			continue
		}

		if c.isClosed() {
			_ = tr.Close()
			return
		}
		c.setTransport(tr)
		connCtx, connCancel := context.WithCancel(context.Background())
		c.setConnCancel(connCancel)
		c.clearSubFailures()
		c.subscribeRegistered(connCtx, tr)
		if !tr.IsConnected() || c.isClosed() {
			connCancel()
			_ = tr.Close()
			c.clearTransport()
			if c.isClosed() {
				return
			}
			continue
		}
		c.setConnected()
		c.log.Info("connected")
		attempt = 0

		c.connectedLoop(connCtx, tr)
		connCancel()
		_ = tr.Close()
		c.clearTransport()
		if c.isClosed() {
			return
		}
	}
}

func (c *Channel) connectedLoop(ctx context.Context, tr transport) {
	for {
		var retryTimer interfaces.Timer
		var retryC <-chan time.Time
		if c.hasFailedSubs() {
			retryTimer = c.clock.NewTimer(subscribeRetry)
			retryC = retryTimer.C()
		}
		select {
		case err := <-tr.Lost():
			if retryTimer != nil {
				retryTimer.Stop()
			}
			c.setDisconnected(err)
			c.log.Warn("connection lost", "reason", c.redact(errString(err)))
			return
		case <-c.closedCh:
			if retryTimer != nil {
				retryTimer.Stop()
			}
			return
		case <-retryC:
			retryTimer.Stop()
			c.retryFailedSubscriptions(ctx, tr)
		}
	}
}

func (c *Channel) wait(d time.Duration) bool {
	timer := c.clock.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C():
		return true
	case <-c.closedCh:
		return false
	}
}

// Publish sends one northbound message. The error priority is closed,
// transport unavailable, message shape, then the underlying error.
func (c *Channel) Publish(ctx context.Context, msg interfaces.Message) error {
	if c.isClosed() {
		return fmt.Errorf("mqtt: publish %q: %w", msg.Topic, interfaces.ErrClosed)
	}
	tr := c.currentTransport()
	if tr == nil || !tr.IsConnected() {
		return fmt.Errorf("mqtt: publish %q: %w", msg.Topic, interfaces.ErrNotConnected)
	}
	if msg.QoS > interfaces.QoS1 {
		return fmt.Errorf("mqtt: publish %q: qos %d: %w", msg.Topic, msg.QoS, interfaces.ErrUnsupported)
	}
	if err := topics.ValidatePublishTopic(msg.Topic); err != nil {
		return fmt.Errorf("mqtt: publish: %w", err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	out := msg
	out.Payload = append([]byte(nil), msg.Payload...)
	err := tr.Publish(ctx, out)
	if err == nil {
		return nil
	}
	if c.isClosed() {
		return fmt.Errorf("mqtt: publish %q: %w", msg.Topic, interfaces.ErrClosed)
	}
	if errors.Is(err, interfaces.ErrNotConnected) || !tr.IsConnected() {
		return fmt.Errorf("mqtt: publish %q: %w", msg.Topic, interfaces.ErrNotConnected)
	}
	return fmt.Errorf("mqtt: publish %q: %w", msg.Topic, err)
}

// Subscribe registers an inbound filter. The subscription survives reconnects;
// the connection loop re-establishes it after every dial.
func (c *Channel) Subscribe(ctx context.Context, filter string) (interfaces.Subscription, error) {
	if c.isClosed() {
		return nil, fmt.Errorf("mqtt: subscribe %q: %w", filter, interfaces.ErrClosed)
	}
	if err := topics.ValidateFilter(filter); err != nil {
		return nil, fmt.Errorf("mqtt: subscribe: %w", err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("mqtt: subscribe %q: %w", filter, err)
	}

	sub := newSubscription(c, filter)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, fmt.Errorf("mqtt: subscribe %q: %w", filter, interfaces.ErrClosed)
	}
	if _, ok := c.subs[filter]; ok {
		c.mu.Unlock()
		return nil, fmt.Errorf("mqtt: subscribe %q: %w", filter, errDuplicateFilter)
	}
	c.subs[filter] = sub
	tr := c.transport
	connected := tr != nil && tr.IsConnected()
	c.mu.Unlock()

	if connected {
		c.subOpMu.Lock()
		err := tr.Subscribe(ctx, filter, c.opts.QoS, sub.deliver)
		c.subOpMu.Unlock()
		if err != nil {
			_ = sub.Close()
			if c.isClosed() {
				return nil, fmt.Errorf("mqtt: subscribe %q: %w", filter, interfaces.ErrClosed)
			}
			return nil, fmt.Errorf("mqtt: subscribe %q: %w", filter, err)
		}
		c.clearSubFailed(filter)
	}

	if ctx.Done() != nil {
		go func() {
			select {
			case <-ctx.Done():
				_ = sub.Close()
			case <-sub.done:
			}
		}()
	}
	return sub, nil
}

// Status returns a copy of the current channel state.
func (c *Channel) Status() interfaces.ChannelStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

// Close stops the channel. It is idempotent and waits for the connection loop
// and any canceled-connect cleanup to finish.
func (c *Channel) Close() error {
	c.mu.Lock()
	first := !c.closed
	if first {
		c.closed = true
		close(c.closedCh)
		if c.dialCancel != nil {
			c.dialCancel()
		}
		if c.connCancel != nil {
			c.connCancel()
		}
	}
	started := c.started
	subs := c.snapshotSubsLocked()
	c.mu.Unlock()

	for _, sub := range subs {
		_ = sub.Close()
	}
	c.cleanupWG.Wait()
	if started {
		<-c.runDone
	}

	c.mu.Lock()
	c.status = interfaces.ChannelStatus{
		Connected: false,
		LastError: "channel closed",
		ChangedAt: c.clock.Now(),
	}
	c.mu.Unlock()
	if first {
		c.log.Info("closed")
	}
	return nil
}

func (c *Channel) setDialCancel(cancel context.CancelFunc) {
	c.mu.Lock()
	c.dialCancel = cancel
	c.mu.Unlock()
}

func (c *Channel) setConnCancel(cancel context.CancelFunc) {
	c.mu.Lock()
	c.connCancel = cancel
	c.mu.Unlock()
}

func (c *Channel) setTransport(tr transport) {
	c.mu.Lock()
	c.transport = tr
	c.mu.Unlock()
}

func (c *Channel) clearTransport() {
	c.mu.Lock()
	c.transport = nil
	c.mu.Unlock()
}

func (c *Channel) currentTransport() transport {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.transport
}

func (c *Channel) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

func (c *Channel) setConnected() {
	c.mu.Lock()
	c.status = interfaces.ChannelStatus{Connected: true, ChangedAt: c.clock.Now()}
	c.mu.Unlock()
}

func (c *Channel) setDisconnected(err error) {
	c.mu.Lock()
	c.status = interfaces.ChannelStatus{
		Connected: false,
		LastError: c.redact(errString(err)),
		ChangedAt: c.clock.Now(),
	}
	c.mu.Unlock()
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (c *Channel) dialConfig() dialConfig {
	return dialConfig{
		BrokerURL:   c.deriv.brokerURL,
		ClientID:    c.deriv.clientID,
		Username:    c.deriv.username,
		Password:    c.deriv.password,
		QoS:         c.opts.QoS,
		KeepAlive:   c.opts.KeepAlive,
		TLSConfig:   c.deriv.tlsConfig,
		WillTopic:   c.deriv.willTopic,
		WillPayload: c.deriv.willPayload,
		WillQoS:     byte(c.opts.QoS),
		WillRetain:  false,
	}
}

func (c *Channel) subscribeRegistered(ctx context.Context, tr transport) {
	for _, sub := range c.snapshotSubs() {
		if c.isClosed() {
			return
		}
		c.subOpMu.Lock()
		err := tr.Subscribe(ctx, sub.filter, c.opts.QoS, sub.deliver)
		c.subOpMu.Unlock()
		if err != nil {
			c.markSubFailed(sub.filter, err)
			continue
		}
		c.clearSubFailed(sub.filter)
	}
}

func (c *Channel) retryFailedSubscriptions(ctx context.Context, tr transport) {
	for _, filter := range c.failedFilters() {
		sub := c.subscription(filter)
		if sub == nil {
			c.clearSubFailed(filter)
			continue
		}
		c.subOpMu.Lock()
		err := tr.Subscribe(ctx, filter, c.opts.QoS, sub.deliver)
		c.subOpMu.Unlock()
		if err != nil {
			c.markSubFailed(filter, err)
			continue
		}
		c.clearSubFailed(filter)
	}
}

func (c *Channel) snapshotSubs() []*subscription {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.snapshotSubsLocked()
}

func (c *Channel) snapshotSubsLocked() []*subscription {
	out := make([]*subscription, 0, len(c.subs))
	for _, sub := range c.subs {
		out = append(out, sub)
	}
	return out
}

func (c *Channel) subscription(filter string) *subscription {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.subs[filter]
}

func (c *Channel) hasFailedSubs() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.failedSubs) > 0
}

func (c *Channel) failedFilters() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.failedSubs))
	for filter := range c.failedSubs {
		out = append(out, filter)
	}
	return out
}

func (c *Channel) markSubFailed(filter string, err error) {
	c.mu.Lock()
	if _, ok := c.subs[filter]; !ok {
		c.mu.Unlock()
		return
	}
	c.failedSubs[filter] = struct{}{}
	c.status.LastError = c.redact(err.Error())
	c.status.ChangedAt = c.clock.Now()
	c.mu.Unlock()
	c.log.Warn("subscribe failed", "filter", filter, "reason", c.redact(err.Error()))
}

func (c *Channel) clearSubFailed(filter string) {
	c.mu.Lock()
	delete(c.failedSubs, filter)
	c.mu.Unlock()
}

func (c *Channel) clearSubFailures() {
	c.mu.Lock()
	c.failedSubs = make(map[string]struct{})
	c.mu.Unlock()
}

func (c *Channel) removeSubscription(sub *subscription) {
	c.mu.Lock()
	if c.subs[sub.filter] == sub {
		delete(c.subs, sub.filter)
	}
	delete(c.failedSubs, sub.filter)
	c.mu.Unlock()
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

var _ interfaces.Channel = (*Channel)(nil)
