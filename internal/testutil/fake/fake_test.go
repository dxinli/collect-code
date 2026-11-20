package fake_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/dxinli/collect-code/internal/interfaces"
	"github.com/dxinli/collect-code/internal/testutil/fake"
)

var (
	_ interfaces.Channel         = (*fake.Channel)(nil)
	_ interfaces.UplinkAdapter   = (*fake.UplinkAdapter)(nil)
	_ interfaces.DownlinkAdapter = (*fake.DownlinkAdapter)(nil)
)

func TestFakeChannelPublishSubscribe(t *testing.T) {
	ch := fake.NewChannel()
	sub, err := ch.Subscribe(context.Background(), "v1/+/dev/+")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Close()
	var _ interfaces.Subscription = sub

	match := interfaces.Message{Topic: "v1/p/dev/1", Payload: []byte("a")}
	if err := ch.Publish(context.Background(), match); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	select {
	case got := <-sub.C():
		if got.Topic != match.Topic {
			t.Fatalf("got %q, want %q", got.Topic, match.Topic)
		}
	default:
		t.Fatal("matching message was not delivered")
	}

	if err := ch.Publish(context.Background(), interfaces.Message{Topic: "v1/p/dev/1/extra"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	select {
	case msg := <-sub.C():
		t.Fatalf("non-matching message delivered: %s", msg.Topic)
	default:
	}

	ch.Inject(interfaces.Message{Topic: "v1/p/dev/9"})
	select {
	case got := <-sub.C():
		if got.Topic != "v1/p/dev/9" {
			t.Fatalf("injected topic = %q", got.Topic)
		}
	default:
		t.Fatal("injected message was not delivered")
	}
	if got := len(ch.Published()); got != 2 {
		t.Fatalf("Published = %d, want 2 (Inject must not be recorded)", got)
	}
}

func TestFakeChannelPublishError(t *testing.T) {
	ch := fake.NewChannel()
	boom := errors.New("boom")
	ch.Fail(boom)
	if err := ch.Publish(context.Background(), interfaces.Message{}); !errors.Is(err, boom) {
		t.Fatalf("Publish error = %v, want boom", err)
	}
	ch.Fail(nil)
	if err := ch.Publish(context.Background(), interfaces.Message{}); err != nil {
		t.Fatalf("Publish after clearing failure: %v", err)
	}

	if err := ch.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := ch.Publish(context.Background(), interfaces.Message{}); !errors.Is(err, interfaces.ErrClosed) {
		t.Fatalf("Publish after Close = %v, want ErrClosed", err)
	}
	if _, err := ch.Subscribe(context.Background(), "#"); !errors.Is(err, interfaces.ErrClosed) {
		t.Fatalf("Subscribe after Close = %v, want ErrClosed", err)
	}
}

func TestFakeChannelSubscribeContextCancel(t *testing.T) {
	ch := fake.NewChannel()
	ctx, cancel := context.WithCancel(context.Background())
	sub, err := ch.Subscribe(ctx, "#")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	cancel()
	deadline := time.Now().Add(time.Second)
	for {
		select {
		case _, ok := <-sub.C():
			if !ok {
				return
			}
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("subscription did not close after context cancellation")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestFakeChannelConcurrentPublishInject(t *testing.T) {
	ch := fake.NewChannel()
	sub, err := ch.Subscribe(context.Background(), "#")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Close()

	const n = 50
	var wg sync.WaitGroup
	wg.Add(2 * n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_ = ch.Publish(context.Background(), interfaces.Message{Topic: "t"})
		}()
		go func() {
			defer wg.Done()
			ch.Inject(interfaces.Message{Topic: "t"})
		}()
	}

	received := make(chan int, 1)
	go func() {
		count := 0
		for count < 2*n {
			select {
			case _, ok := <-sub.C():
				if !ok {
					received <- count
					return
				}
				count++
			case <-time.After(time.Second):
				received <- count
				return
			}
		}
		received <- count
	}()

	wg.Wait()
	got := <-received
	if got+ch.Dropped() != 2*n {
		t.Fatalf("received %d + dropped %d = %d, want %d", got, ch.Dropped(), got+ch.Dropped(), 2*n)
	}
	if got := len(ch.Published()); got != n {
		t.Fatalf("Published = %d, want %d", got, n)
	}
}

func TestFakeUplinkAdapter(t *testing.T) {
	a := fake.NewUplinkAdapter()
	at := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	a.SetDecoded([]interfaces.Sample{{PointKey: "x", Value: 1}})

	samples, err := a.DecodePayload("dev", []byte{1}, 10, at)
	if err != nil {
		t.Fatalf("DecodePayload: %v", err)
	}
	if len(samples) != 1 || !samples[0].At.Equal(at) {
		t.Fatalf("samples = %+v, want one sample stamped with %s", samples, at)
	}

	if _, err := a.PropertyReport(interfaces.PropertyReport{MsgID: "m1"}); err != nil {
		t.Fatalf("PropertyReport: %v", err)
	}
	if _, err := a.EventReport(interfaces.EventReport{MsgID: "e1"}); err != nil {
		t.Fatalf("EventReport: %v", err)
	}
	if got := len(a.Reports()); got != 1 {
		t.Fatalf("Reports = %d, want 1", got)
	}
	if got := len(a.Events()); got != 1 {
		t.Fatalf("Events = %d, want 1", got)
	}

	boom := errors.New("boom")
	a.SetError(boom)
	if _, err := a.PropertyReport(interfaces.PropertyReport{}); !errors.Is(err, boom) {
		t.Fatalf("PropertyReport error = %v, want boom", err)
	}
	if _, err := a.DecodePayload("dev", nil, 0, at); !errors.Is(err, boom) {
		t.Fatalf("DecodePayload error = %v, want boom", err)
	}
}

func TestFakeDownlinkAdapter(t *testing.T) {
	a := fake.NewDownlinkAdapter()
	a.QueueInvocation(interfaces.Invocation{ServiceID: "first", Seq: 1})
	a.QueueInvocation(interfaces.Invocation{ServiceID: "second", Seq: 2})

	first, err := a.ParseInvocation(interfaces.Message{})
	if err != nil || first.ServiceID != "first" {
		t.Fatalf("first = %+v, err = %v", first, err)
	}
	second, err := a.ParseInvocation(interfaces.Message{})
	if err != nil || second.ServiceID != "second" {
		t.Fatalf("second = %+v, err = %v", second, err)
	}
	if _, err := a.ParseInvocation(interfaces.Message{}); !errors.Is(err, fake.ErrNoInvocation) {
		t.Fatalf("empty queue error = %v, want ErrNoInvocation", err)
	}

	a.QueueInvocation(interfaces.Invocation{ServiceID: "third"})
	boom := errors.New("boom")
	a.SetParseError(boom)
	if _, err := a.ParseInvocation(interfaces.Message{}); !errors.Is(err, boom) {
		t.Fatalf("ParseInvocation error = %v, want boom", err)
	}

	if _, err := a.ServiceReply(interfaces.ServiceReply{MsgID: "r1", DeviceSN: "dev"}); err != nil {
		t.Fatalf("ServiceReply: %v", err)
	}
	if got := len(a.Replies()); got != 1 {
		t.Fatalf("Replies = %d, want 1", got)
	}
	a.SetReplyError(boom)
	if _, err := a.ServiceReply(interfaces.ServiceReply{}); !errors.Is(err, boom) {
		t.Fatalf("ServiceReply error = %v, want boom", err)
	}
}
