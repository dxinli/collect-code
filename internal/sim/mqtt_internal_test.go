package sim

import (
	"context"
	"testing"
	"time"

	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/packets"
)

func TestIsSysTopic(t *testing.T) {
	cases := map[string]bool{
		"$SYS":             true,
		"$SYS/":            true,
		"$SYS/broker/load": true,
		"v1/$SYS/x":        false,
		"":                 false,
	}
	for topic, want := range cases {
		if got := isSysTopic(topic); got != want {
			t.Errorf("isSysTopic(%q) = %v, want %v", topic, got, want)
		}
	}
}

func TestValidatePublishTopic(t *testing.T) {
	for _, topic := range []string{"v1/a", "a/b/c", "v1/x/y"} {
		if err := validatePublishTopic(topic); err != nil {
			t.Errorf("valid topic %q: %v", topic, err)
		}
	}
	for _, topic := range []string{"", "v1/+", "v1/#", "$SYS/x"} {
		if err := validatePublishTopic(topic); err == nil {
			t.Errorf("invalid topic %q accepted", topic)
		}
	}
}

func TestAuthHookHold(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	h := &authHook{now: func() time.Time { return now }}
	if !h.Provides(mqtt.OnConnectAuthenticate) || !h.Provides(mqtt.OnACLCheck) {
		t.Fatal("hook does not provide both auth and ACL")
	}
	if !h.OnConnectAuthenticate(nil, packets.Packet{}) {
		t.Fatal("connection rejected without hold")
	}
	if !h.OnACLCheck(nil, "v1/x", true) {
		t.Fatal("ACL rejected without hold")
	}
	h.setHold(now.Add(time.Second))
	if h.OnConnectAuthenticate(nil, packets.Packet{}) {
		t.Fatal("connection accepted during hold")
	}
	now = now.Add(2 * time.Second)
	if !h.OnConnectAuthenticate(nil, packets.Packet{}) {
		t.Fatal("connection rejected after hold expired")
	}
}

func TestCaptureInlineSkipsInlineAndSys(t *testing.T) {
	r, err := NewRuntime(RuntimeOptions{HTTPAddr: "127.0.0.1:0", CaptureCapacity: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close(context.Background()) }()
	b := &MQTTBroker{runtime: r}

	b.captureInline(nil, packets.Subscription{}, packets.Packet{
		TopicName: "v1/a",
		Payload:   []byte(`{"ok":true}`),
		Origin:    "client-1",
	})
	b.captureInline(nil, packets.Subscription{}, packets.Packet{
		TopicName: "v1/down",
		Origin:    mqtt.InlineClientId,
	})
	b.captureInline(nil, packets.Subscription{}, packets.Packet{
		TopicName: "$SYS/broker/load",
		Origin:    "sys",
	})

	messages, dropped := r.Capture().Snapshot()
	if len(messages) != 1 || dropped != 0 {
		t.Fatalf("messages=%d dropped=%d", len(messages), dropped)
	}
	if messages[0].Topic != "v1/a" {
		t.Fatalf("topic = %q", messages[0].Topic)
	}
}
