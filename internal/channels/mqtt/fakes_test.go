package mqtt

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/dxinli/collect-code/internal/interfaces"
	"github.com/dxinli/collect-code/internal/testutil/fakeclock"
)

type fakeDialer struct {
	mu      sync.Mutex
	calls   []dialConfig
	dialed  chan dialConfig
	fn      func(ctx context.Context, cfg dialConfig, attempt int) (transport, error)
	attempt int
}

func newFakeDialer(fn func(ctx context.Context, cfg dialConfig, attempt int) (transport, error)) *fakeDialer {
	return &fakeDialer{dialed: make(chan dialConfig, 16), fn: fn}
}

func (d *fakeDialer) Dial(ctx context.Context, cfg dialConfig) (transport, error) {
	d.mu.Lock()
	attempt := d.attempt
	d.attempt++
	d.calls = append(d.calls, cfg)
	d.mu.Unlock()
	select {
	case d.dialed <- cfg:
	default:
	}
	if d.fn == nil {
		return nil, errors.New("fake dialer: no result configured")
	}
	return d.fn(ctx, cfg, attempt)
}

func (d *fakeDialer) Calls() []dialConfig {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]dialConfig(nil), d.calls...)
}

type fakeTransport struct {
	mu           sync.Mutex
	connected    bool
	lost         chan error
	published    []interfaces.Message
	subs         map[string]func(interfaces.Message)
	subscribeErr error
	publishErr   error
	publishFn    func(ctx context.Context, msg interfaces.Message) error
	subscribeFn  func(ctx context.Context, filter string, qos interfaces.QoS, deliver func(interfaces.Message)) error
	closeCount   int
}

func newFakeTransport() *fakeTransport {
	return &fakeTransport{
		connected: true,
		lost:      make(chan error, 1),
		subs:      make(map[string]func(interfaces.Message)),
	}
}

func (t *fakeTransport) Publish(ctx context.Context, msg interfaces.Message) error {
	t.mu.Lock()
	fn := t.publishFn
	connected := t.connected
	err := t.publishErr
	t.mu.Unlock()
	if fn != nil {
		return fn(ctx, msg)
	}
	if !connected {
		return interfaces.ErrNotConnected
	}
	t.mu.Lock()
	t.published = append(t.published, msg)
	t.mu.Unlock()
	return err
}

func (t *fakeTransport) Subscribe(ctx context.Context, filter string, qos interfaces.QoS, deliver func(interfaces.Message)) error {
	t.mu.Lock()
	fn := t.subscribeFn
	err := t.subscribeErr
	t.mu.Unlock()
	if fn != nil {
		return fn(ctx, filter, qos, deliver)
	}
	if err != nil {
		return err
	}
	t.mu.Lock()
	t.subs[filter] = deliver
	t.mu.Unlock()
	return nil
}

func (t *fakeTransport) Lost() <-chan error { return t.lost }

func (t *fakeTransport) IsConnected() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.connected
}

func (t *fakeTransport) Close() error {
	t.mu.Lock()
	t.connected = false
	t.closeCount++
	t.mu.Unlock()
	return nil
}

func (t *fakeTransport) setConnected(v bool) {
	t.mu.Lock()
	t.connected = v
	t.mu.Unlock()
}

func (t *fakeTransport) lose(err error) {
	t.mu.Lock()
	t.connected = false
	t.mu.Unlock()
	t.lost <- err
}

func (t *fakeTransport) deliver(filter string, msg interfaces.Message) bool {
	t.mu.Lock()
	deliver := t.subs[filter]
	t.mu.Unlock()
	if deliver == nil {
		return false
	}
	deliver(msg)
	return true
}

func (t *fakeTransport) publishedMessages() []interfaces.Message {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]interfaces.Message(nil), t.published...)
}

func (t *fakeTransport) subscribed(filters ...string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, filter := range filters {
		if _, ok := t.subs[filter]; !ok {
			return false
		}
	}
	return true
}

func (t *fakeTransport) closes() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.closeCount
}

func channelOptions(clk interfaces.Clock) Options {
	return Options{
		BrokerURL:         "mqtt://broker.example:1883",
		ClientIDTemplate:  "collector-{node.id}",
		QoS:               interfaces.QoS1,
		KeepAlive:         60 * time.Second,
		Reconnect:         ReconnectOptions{Min: time.Second, Max: 8 * time.Second},
		NodeID:            "gateway-001",
		ProductKey:        "irrigation",
		WillTopicTemplate: "v1/{product_key}/$nodes/{node_id}/state",
		Clock:             clk,
	}
}

func newTestChannel(t *testing.T, d dialer, clk *fakeclock.Clock) *Channel {
	t.Helper()
	opts := channelOptions(clk)
	c, err := newChannel(opts, d)
	if err != nil {
		t.Fatalf("newChannel: %v", err)
	}
	c.jitter = func(string, int) float64 { return 0 }
	t.Cleanup(func() {
		_ = c.Close()
		clk.Close()
	})
	return c
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func waitStatus(t *testing.T, c *Channel, connected bool) {
	t.Helper()
	waitFor(t, "status connected", func() bool { return c.Status().Connected == connected })
}

func recvDial(t *testing.T, d *fakeDialer) dialConfig {
	t.Helper()
	select {
	case cfg := <-d.dialed:
		return cfg
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for dial")
		return dialConfig{}
	}
}
