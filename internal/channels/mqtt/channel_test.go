package mqtt

// Covers: REQ-EVENT-03, REQ-CACHE-01, REQ-SEAM-01, REQ-TIME-01,
// EXT-LWT-01 (will parameters), EXT-TOPIC-01 (template rendering),
// REQ-OPS-03 (redaction).

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dxinli/collect-code/internal/interfaces"
	"github.com/dxinli/collect-code/internal/testutil/fakeclock"
)

const testFilter = "v1/irrigation/+/service/invoke"

func TestL1ConnectAndRegisterBeforeConnect(t *testing.T) {
	clk := fakeclock.New(time.Unix(1_700_000_000, 0).UTC())
	tr := newFakeTransport()
	d := newFakeDialer(func(context.Context, dialConfig, int) (transport, error) { return tr, nil })
	c := newTestChannel(t, d, clk)

	sub, err := c.Subscribe(context.Background(), testFilter)
	if err != nil {
		t.Fatalf("Subscribe before connect: %v", err)
	}
	c.start()
	recvDial(t, d)
	waitStatus(t, c, true)
	waitFor(t, "registered filter", func() bool { return tr.subscribed(testFilter) })

	if got := c.Status().ChangedAt; !got.Equal(clk.Now()) {
		t.Fatalf("ChangedAt = %s, want fake now %s", got, clk.Now())
	}
	if got := c.Status().LastError; got != "" {
		t.Fatalf("LastError = %q, want empty", got)
	}

	tr.deliver(testFilter, interfaces.Message{Topic: "down", Payload: []byte("x"), QoS: interfaces.QoS1, Retain: true})
	select {
	case msg := <-sub.C():
		if msg.Topic != "down" || string(msg.Payload) != "x" || msg.QoS != interfaces.QoS1 || !msg.Retain {
			t.Fatalf("delivered message = %+v", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for delivery")
	}

	cfg := d.Calls()[0]
	if cfg.WillTopic != "v1/irrigation/$nodes/gateway-001/state" {
		t.Fatalf("will topic = %q", cfg.WillTopic)
	}
	if string(cfg.WillPayload) != `{"online":false,"node_id":"gateway-001"}` {
		t.Fatalf("will payload = %q", cfg.WillPayload)
	}
	if cfg.WillRetain {
		t.Fatal("will retain must be false")
	}
	if cfg.WillQoS != 1 {
		t.Fatalf("will qos = %d, want 1", cfg.WillQoS)
	}
}

func TestL1BackoffSequence(t *testing.T) {
	start := time.Unix(1_700_000_000, 0).UTC()
	clk := fakeclock.New(start)
	d := newFakeDialer(func(context.Context, dialConfig, int) (transport, error) {
		return nil, errors.New("connection refused")
	})
	c := newTestChannel(t, d, clk)
	c.start()

	recvDial(t, d)
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}
	for _, delta := range want {
		waitFor(t, "backoff timer", func() bool { return clk.NumActive() > 0 })
		clk.Advance(delta)
		recvDial(t, d)
	}
	// attempt 4 is still capped at max, so the next delay is 8s.
	waitFor(t, "backoff timer", func() bool { return clk.NumActive() > 0 })
	clk.Advance(8 * time.Second)
	recvDial(t, d)

	calls := d.Calls()
	if len(calls) < 6 {
		t.Fatalf("dial calls = %d, want at least 6", len(calls))
	}
}

func TestL1ReconnectResubscribes(t *testing.T) {
	clk := fakeclock.New(time.Unix(1_700_000_000, 0).UTC())
	tr1 := newFakeTransport()
	tr2 := newFakeTransport()
	d := newFakeDialer(func(_ context.Context, _ dialConfig, attempt int) (transport, error) {
		if attempt == 0 {
			return tr1, nil
		}
		return tr2, nil
	})
	c := newTestChannel(t, d, clk)
	c.start()
	recvDial(t, d)
	waitStatus(t, c, true)

	sub, err := c.Subscribe(context.Background(), testFilter)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	waitFor(t, "first transport subscribe", func() bool { return tr1.subscribed(testFilter) })

	tr1.lose(errors.New("network down"))
	recvDial(t, d)
	waitStatus(t, c, true)
	waitFor(t, "second transport subscribe", func() bool { return tr2.subscribed(testFilter) })

	tr2.deliver(testFilter, interfaces.Message{Topic: "down", Payload: []byte("y")})
	select {
	case msg := <-sub.C():
		if msg.Topic != "down" || string(msg.Payload) != "y" {
			t.Fatalf("delivered after reconnect = %+v", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for delivery after reconnect")
	}
	if tr1.closes() == 0 {
		t.Fatal("lost transport was not closed")
	}
}

func TestL1SubackRetry(t *testing.T) {
	clk := fakeclock.New(time.Unix(1_700_000_000, 0).UTC())
	tr := newFakeTransport()
	var mu sync.Mutex
	attempts := 0
	tr.subscribeFn = func(_ context.Context, filter string, _ interfaces.QoS, deliver func(interfaces.Message)) error {
		mu.Lock()
		attempts++
		n := attempts
		mu.Unlock()
		if n == 1 {
			return errors.New("SUBACK 0x80")
		}
		tr.mu.Lock()
		tr.subs[filter] = deliver
		tr.mu.Unlock()
		return nil
	}
	d := newFakeDialer(func(context.Context, dialConfig, int) (transport, error) { return tr, nil })
	c := newTestChannel(t, d, clk)
	if _, err := c.Subscribe(context.Background(), testFilter); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	c.start()
	recvDial(t, d)
	waitStatus(t, c, true)
	waitFor(t, "first subscribe attempt", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return attempts == 1
	})
	waitFor(t, "retry timer", func() bool { return clk.NumActive() > 0 })
	clk.Advance(subscribeRetry)
	waitFor(t, "second subscribe attempt", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return attempts == 2
	})
	waitFor(t, "subscription registered", func() bool { return tr.subscribed(testFilter) })
}

func TestL1PublishSemantics(t *testing.T) {
	clk := fakeclock.New(time.Unix(1_700_000_000, 0).UTC())
	tr := newFakeTransport()
	d := newFakeDialer(func(context.Context, dialConfig, int) (transport, error) { return tr, nil })
	c := newTestChannel(t, d, clk)
	c.start()
	recvDial(t, d)
	waitStatus(t, c, true)

	payload := []byte("payload")
	if err := c.Publish(context.Background(), interfaces.Message{Topic: "v1/x/y", Payload: payload, QoS: interfaces.QoS1}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	payload[0] = 'X'
	published := tr.publishedMessages()
	if len(published) != 1 || string(published[0].Payload) != "payload" {
		t.Fatalf("published = %+v", published)
	}

	if err := c.Publish(context.Background(), interfaces.Message{Topic: "v1/x/y", QoS: 2}); !errors.Is(err, interfaces.ErrUnsupported) {
		t.Fatalf("qos2 connected error = %v, want ErrUnsupported", err)
	}
	if err := c.Publish(context.Background(), interfaces.Message{Topic: "v1/+/y", QoS: interfaces.QoS1}); err == nil {
		t.Fatal("wildcard publish topic was accepted")
	}

	tr.setConnected(false)
	if err := c.Publish(context.Background(), interfaces.Message{Topic: "v1/x/y", QoS: interfaces.QoS1}); !errors.Is(err, interfaces.ErrNotConnected) {
		t.Fatalf("disconnected publish error = %v, want ErrNotConnected", err)
	}
	if err := c.Publish(context.Background(), interfaces.Message{Topic: "v1/x/y", QoS: 2}); !errors.Is(err, interfaces.ErrNotConnected) {
		t.Fatalf("disconnected qos2 error = %v, want ErrNotConnected (priority)", err)
	}
}

func TestL1InFlightPublishMapsToNotConnected(t *testing.T) {
	clk := fakeclock.New(time.Unix(1_700_000_000, 0).UTC())
	tr := newFakeTransport()
	tr.publishFn = func(context.Context, interfaces.Message) error {
		tr.setConnected(false)
		return errors.New("connection lost before Publish completed")
	}
	d := newFakeDialer(func(context.Context, dialConfig, int) (transport, error) { return tr, nil })
	c := newTestChannel(t, d, clk)
	c.start()
	recvDial(t, d)
	waitStatus(t, c, true)

	err := c.Publish(context.Background(), interfaces.Message{Topic: "v1/x/y", QoS: interfaces.QoS1})
	if !errors.Is(err, interfaces.ErrNotConnected) {
		t.Fatalf("in-flight publish error = %v, want ErrNotConnected", err)
	}
}

func TestL1PublishContextCancel(t *testing.T) {
	clk := fakeclock.New(time.Unix(1_700_000_000, 0).UTC())
	tr := newFakeTransport()
	tr.publishFn = func(ctx context.Context, _ interfaces.Message) error {
		<-ctx.Done()
		return ctx.Err()
	}
	d := newFakeDialer(func(context.Context, dialConfig, int) (transport, error) { return tr, nil })
	c := newTestChannel(t, d, clk)
	c.start()
	recvDial(t, d)
	waitStatus(t, c, true)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := c.Publish(ctx, interfaces.Message{Topic: "v1/x/y", QoS: interfaces.QoS1})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("publish error = %v, want context.Canceled", err)
	}
}

func TestL1SubscribeRulesAndContext(t *testing.T) {
	clk := fakeclock.New(time.Unix(1_700_000_000, 0).UTC())
	tr := newFakeTransport()
	d := newFakeDialer(func(context.Context, dialConfig, int) (transport, error) { return tr, nil })
	c := newTestChannel(t, d, clk)
	c.start()
	recvDial(t, d)
	waitStatus(t, c, true)

	if _, err := c.Subscribe(context.Background(), "v1/irrigation/"); err == nil {
		t.Fatal("invalid filter was accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	sub, err := c.Subscribe(ctx, testFilter)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if _, err := c.Subscribe(context.Background(), testFilter); err == nil {
		t.Fatal("duplicate filter was accepted")
	}
	cancel()
	waitClosed(t, sub.C())
	// Cancellation removes the registration, so the filter can be reused.
	if _, err := c.Subscribe(context.Background(), testFilter); err != nil {
		t.Fatalf("re-subscribe after ctx cancel: %v", err)
	}
}

func TestL1CloseIdempotentAndClosedErrors(t *testing.T) {
	clk := fakeclock.New(time.Unix(1_700_000_000, 0).UTC())
	tr := newFakeTransport()
	d := newFakeDialer(func(context.Context, dialConfig, int) (transport, error) { return tr, nil })
	c := newTestChannel(t, d, clk)
	c.start()
	recvDial(t, d)
	waitStatus(t, c, true)

	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if tr.closes() != 1 {
		t.Fatalf("transport closes = %d, want 1", tr.closes())
	}
	if err := c.Publish(context.Background(), interfaces.Message{Topic: "v1/x/y"}); !errors.Is(err, interfaces.ErrClosed) {
		t.Fatalf("publish after close = %v, want ErrClosed", err)
	}
	if _, err := c.Subscribe(context.Background(), testFilter); !errors.Is(err, interfaces.ErrClosed) {
		t.Fatalf("subscribe after close error = %v, want ErrClosed", err)
	}
	st := c.Status()
	if st.Connected || st.LastError != "channel closed" {
		t.Fatalf("status after close = %+v", st)
	}
	if !st.ChangedAt.Equal(clk.Now()) {
		t.Fatalf("ChangedAt = %s, want fake now %s", st.ChangedAt, clk.Now())
	}
}

func TestL1DeliveryBackpressureAndOverflow(t *testing.T) {
	clk := fakeclock.New(time.Unix(1_700_000_000, 0).UTC())
	tr := newFakeTransport()
	d := newFakeDialer(func(context.Context, dialConfig, int) (transport, error) { return tr, nil })
	c := newTestChannel(t, d, clk)
	c.start()
	recvDial(t, d)
	waitStatus(t, c, true)

	sub, err := c.Subscribe(context.Background(), testFilter)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	for i := 0; i < subscribeBuffer; i++ {
		if !tr.deliver(testFilter, interfaces.Message{Topic: "down"}) {
			t.Fatal("deliver target missing")
		}
	}
	done := make(chan struct{})
	go func() {
		tr.deliver(testFilter, interfaces.Message{Topic: "overflow"})
		close(done)
	}()
	waitFor(t, "delivery timer", func() bool { return clk.NumActive() > 0 })
	clk.Advance(deliveryBlockTimeout)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("blocked delivery did not return")
	}
	for i := 0; i < subscribeBuffer; i++ {
		select {
		case <-sub.C():
		case <-time.After(2 * time.Second):
			t.Fatal("timed out draining the buffer")
		}
	}
	waitClosed(t, sub.C())
}

func TestL1DeliveryCloseRace(t *testing.T) {
	clk := fakeclock.New(time.Unix(1_700_000_000, 0).UTC())
	tr := newFakeTransport()
	d := newFakeDialer(func(context.Context, dialConfig, int) (transport, error) { return tr, nil })
	c := newTestChannel(t, d, clk)
	c.start()
	recvDial(t, d)
	waitStatus(t, c, true)

	sub, err := c.Subscribe(context.Background(), testFilter)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	for i := 0; i < subscribeBuffer; i++ {
		tr.deliver(testFilter, interfaces.Message{Topic: "down"})
	}
	done := make(chan struct{})
	go func() {
		tr.deliver(testFilter, interfaces.Message{Topic: "blocked"})
		close(done)
	}()
	waitFor(t, "delivery timer", func() bool { return clk.NumActive() > 0 })
	if err := sub.Close(); err != nil {
		t.Fatalf("sub.Close: %v", err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("blocked delivery did not return after Close")
	}
	waitClosed(t, sub.C())
}

func TestL1ConcurrentPublishClose(t *testing.T) {
	clk := fakeclock.New(time.Unix(1_700_000_000, 0).UTC())
	tr := newFakeTransport()
	d := newFakeDialer(func(context.Context, dialConfig, int) (transport, error) { return tr, nil })
	c := newTestChannel(t, d, clk)
	c.start()
	recvDial(t, d)
	waitStatus(t, c, true)

	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- c.Publish(context.Background(), interfaces.Message{Topic: "v1/x/y"})
		}()
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err == nil {
			continue
		}
		if !errors.Is(err, interfaces.ErrClosed) && !errors.Is(err, interfaces.ErrNotConnected) {
			t.Fatalf("unexpected publish error: %v", err)
		}
	}
}

func TestL1StatusRedactsSecrets(t *testing.T) {
	clk := fakeclock.New(time.Unix(1_700_000_000, 0).UTC())
	d := newFakeDialer(func(context.Context, dialConfig, int) (transport, error) {
		return nil, errors.New("auth failed for s3cr3t")
	})
	opts := channelOptions(clk)
	opts.Username = "user"
	opts.Password = "s3cr3t"
	c, err := newChannel(opts, d)
	if err != nil {
		t.Fatalf("newChannel: %v", err)
	}
	c.jitter = func(string, int) float64 { return 0 }
	t.Cleanup(func() {
		_ = c.Close()
		clk.Close()
	})
	c.start()
	waitFor(t, "status error", func() bool { return c.Status().LastError != "" })
	st := c.Status()
	if strings.Contains(st.LastError, "s3cr3t") {
		t.Fatalf("LastError leaks the secret: %q", st.LastError)
	}
	if !strings.Contains(st.LastError, "[REDACTED]") {
		t.Fatalf("LastError = %q, want [REDACTED]", st.LastError)
	}
}

func TestL1CloseDuringConnect(t *testing.T) {
	clk := fakeclock.New(time.Unix(1_700_000_000, 0).UTC())
	started := make(chan struct{})
	d := newFakeDialer(func(ctx context.Context, _ dialConfig, _ int) (transport, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	c := newTestChannel(t, d, clk)
	c.start()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("dial did not start")
	}
	done := make(chan struct{})
	go func() {
		_ = c.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not interrupt the dial")
	}
}

func waitClosed(t *testing.T, ch <-chan interfaces.Message) {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-timeout:
			t.Fatal("timed out waiting for close")
		}
	}
}
