package mqtt

// L2 integration tests use the 06b embedded broker and the simulator's HTTP
// capture/invoke/control endpoints. The broker is an independent peer; the
// channel is the system under test.

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dxinli/collect-code/internal/clock/realclock"
	"github.com/dxinli/collect-code/internal/interfaces"
	"github.com/dxinli/collect-code/internal/sim"
)

func newL2Broker(t *testing.T) (*sim.Runtime, *sim.MQTTBroker) {
	t.Helper()
	r, err := sim.NewRuntime(sim.RuntimeOptions{
		HTTPAddr:        "127.0.0.1:0",
		CaptureCapacity: 64,
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	b, err := sim.NewMQTTBroker(sim.MQTTBrokerOptions{Addr: "127.0.0.1:0", Runtime: r})
	if err != nil {
		t.Fatalf("new broker: %v", err)
	}
	if err := b.Register(r); err != nil {
		t.Fatalf("register broker: %v", err)
	}
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("start runtime: %v", err)
	}
	if err := b.Start(); err != nil {
		t.Fatalf("start broker: %v", err)
	}
	t.Cleanup(func() {
		// Let any in-flight CONNECT finish before mochi tears the listener
		// down; mochi v2.7.9 has a Close/EstablishConnection race otherwise.
		time.Sleep(150 * time.Millisecond)
		_ = b.Close()
		_ = r.Close(context.Background())
	})
	return r, b
}

func l2Options(brokerURL, nodeID string) Options {
	return Options{
		BrokerURL:         brokerURL,
		ClientIDTemplate:  "test-{node.id}",
		QoS:               interfaces.QoS1,
		KeepAlive:         2 * time.Second,
		Reconnect:         ReconnectOptions{Min: 20 * time.Millisecond, Max: 100 * time.Millisecond},
		NodeID:            nodeID,
		ProductKey:        "irrigation",
		WillTopicTemplate: "v1/{product_key}/$nodes/{node_id}/state",
		Clock:             realclock.New(),
	}
}

func newL2Channel(t *testing.T, brokerURL, nodeID string) *Channel {
	t.Helper()
	c, err := New(l2Options(brokerURL, nodeID))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func waitL2Connected(t *testing.T, c *Channel) {
	t.Helper()
	waitFor(t, "channel connected", func() bool { return c.Status().Connected })
}

func waitL2Status(t *testing.T, c *Channel, connected bool) {
	t.Helper()
	waitFor(t, "channel status", func() bool { return c.Status().Connected == connected })
}

func waitL2Capture(t *testing.T, r *sim.Runtime, cond func([]sim.CapturedMessage) bool) []sim.CapturedMessage {
	t.Helper()
	var found []sim.CapturedMessage
	waitFor(t, "capture condition", func() bool {
		messages, _ := r.Capture().Snapshot()
		if cond(messages) {
			found = messages
			return true
		}
		return false
	})
	return found
}

func postL2(t *testing.T, url, body string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST %s: status=%d body=%s", url, resp.StatusCode, data)
	}
}

func postL2Control(t *testing.T, r *sim.Runtime, body string) {
	t.Helper()
	postL2(t, r.BaseURL()+"/_sim/control", body)
}

func postL2Invoke(t *testing.T, r *sim.Runtime, topic string, payload string) {
	t.Helper()
	postL2(t, r.BaseURL()+"/_sim/invoke", fmt.Sprintf(`{"topic":%q,"payload":%s}`, topic, payload))
}

func containsTopic(messages []sim.CapturedMessage, topic string) bool {
	for _, m := range messages {
		if m.Topic == topic {
			return true
		}
	}
	return false
}

func findTopic(messages []sim.CapturedMessage, topic string) (sim.CapturedMessage, bool) {
	for _, m := range messages {
		if m.Topic == topic {
			return m, true
		}
	}
	return sim.CapturedMessage{}, false
}

func TestL2ConnectAndPublish(t *testing.T) {
	r, b := newL2Broker(t)
	c := newL2Channel(t, b.URL(), "node-l2-01")
	waitL2Connected(t, c)

	topic := "v1/irrigation/VALVE-0001/property/post"
	payload := []byte(`{"opening":42.5}`)
	if err := c.Publish(context.Background(), interfaces.Message{Topic: topic, Payload: payload, QoS: interfaces.QoS1}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	messages := waitL2Capture(t, r, func(ms []sim.CapturedMessage) bool { return containsTopic(ms, topic) })
	msg, _ := findTopic(messages, topic)
	if !bytes.Equal(msg.Payload, payload) {
		t.Fatalf("payload = %s, want %s", msg.Payload, payload)
	}
	if msg.QoS != 1 || msg.Retain {
		t.Fatalf("qos=%d retain=%t, want 1/false", msg.QoS, msg.Retain)
	}
}

func TestL2KeepAlive(t *testing.T) {
	_, b := newL2Broker(t)
	c := newL2Channel(t, b.URL(), "node-l2-02")
	waitL2Connected(t, c)

	time.Sleep(4 * time.Second)
	if !c.Status().Connected {
		t.Fatal("channel dropped while idle below the broker keepalive threshold")
	}
	if err := c.Publish(context.Background(), interfaces.Message{Topic: "v1/irrigation/x", QoS: interfaces.QoS1}); err != nil {
		t.Fatalf("Publish after idle: %v", err)
	}
}

func TestL2LWTExact(t *testing.T) {
	r, b := newL2Broker(t)
	c := newL2Channel(t, b.URL(), "node-l2-03")
	waitL2Connected(t, c)

	postL2Control(t, r, `{"action":"broker_disconnect"}`)
	willTopic := "v1/irrigation/$nodes/node-l2-03/state"
	messages := waitL2Capture(t, r, func(ms []sim.CapturedMessage) bool { return containsTopic(ms, willTopic) })
	msg, _ := findTopic(messages, willTopic)
	wantPayload := []byte(`{"online":false,"node_id":"node-l2-03"}`)
	if !bytes.Equal(msg.Payload, wantPayload) {
		t.Fatalf("will payload = %s, want %s", msg.Payload, wantPayload)
	}
	if msg.Retain {
		t.Fatal("will retain = true, want false")
	}
	if msg.QoS != 1 {
		t.Fatalf("will qos = %d, want 1", msg.QoS)
	}
}

func TestL2CloseNoLWT(t *testing.T) {
	r, b := newL2Broker(t)
	c := newL2Channel(t, b.URL(), "node-l2-04")
	waitL2Connected(t, c)
	willTopic := "v1/irrigation/$nodes/node-l2-04/state"

	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	time.Sleep(400 * time.Millisecond)
	messages, _ := r.Capture().Snapshot()
	if containsTopic(messages, willTopic) {
		t.Fatalf("graceful Close triggered the will: %+v", messages)
	}
}

func TestL2ReconnectResubscribe(t *testing.T) {
	r, b := newL2Broker(t)
	c := newL2Channel(t, b.URL(), "node-l2-05")
	filter := "v1/irrigation/+/service/invoke"
	sub, err := c.Subscribe(context.Background(), filter)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	waitL2Connected(t, c)
	time.Sleep(100 * time.Millisecond)

	postL2Control(t, r, `{"action":"broker_disconnect"}`)
	waitL2Connected(t, c)

	downTopic := "v1/irrigation/VALVE-0001/service/invoke"
	deadline := time.Now().Add(3 * time.Second)
	for attempt := 0; time.Now().Before(deadline); attempt++ {
		postL2Invoke(t, r, downTopic, fmt.Sprintf(`{"attempt":%d}`, attempt))
		select {
		case msg := <-sub.C():
			if msg.Topic != downTopic {
				t.Fatalf("downlink topic = %q", msg.Topic)
			}
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
	t.Fatal("subscription did not survive the reconnect")
}

func TestL2BrokerHoldRejectsAndRecovers(t *testing.T) {
	r, b := newL2Broker(t)
	c := newL2Channel(t, b.URL(), "node-l2-06")
	waitL2Connected(t, c)

	postL2Control(t, r, `{"action":"broker_disconnect","duration_ms":1000}`)
	waitL2Status(t, c, false)
	err := c.Publish(context.Background(), interfaces.Message{Topic: "v1/irrigation/x", QoS: interfaces.QoS1})
	if !errors.Is(err, interfaces.ErrNotConnected) {
		t.Fatalf("publish while disconnected = %v, want ErrNotConnected", err)
	}
	waitL2Connected(t, c)
	if err := c.Publish(context.Background(), interfaces.Message{Topic: "v1/irrigation/x", QoS: interfaces.QoS1}); err != nil {
		t.Fatalf("publish after recovery: %v", err)
	}
}

func TestL2ConcurrentPublishWhileDown(t *testing.T) {
	r, b := newL2Broker(t)
	c := newL2Channel(t, b.URL(), "node-l2-07")
	waitL2Connected(t, c)

	postL2Control(t, r, `{"action":"broker_disconnect","duration_ms":1000}`)
	waitL2Status(t, c, false)

	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for range 32 {
		wg.Go(func() {
			errs <- c.Publish(context.Background(), interfaces.Message{Topic: "v1/irrigation/x", QoS: interfaces.QoS1})
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if !errors.Is(err, interfaces.ErrNotConnected) {
			t.Fatalf("publish while down = %v, want ErrNotConnected", err)
		}
	}
}

func TestL2NonDefaultWillTemplate(t *testing.T) {
	r, b := newL2Broker(t)
	opts := l2Options(b.URL(), "node-l2-08")
	opts.ProductKey = "farm-a"
	opts.WillTopicTemplate = "custom/v2/{product_key}/nodes/{node_id}/offline"
	c, err := New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	waitL2Connected(t, c)

	postL2Control(t, r, `{"action":"broker_disconnect"}`)
	willTopic := "custom/v2/farm-a/nodes/node-l2-08/offline"
	messages := waitL2Capture(t, r, func(ms []sim.CapturedMessage) bool { return containsTopic(ms, willTopic) })
	msg, _ := findTopic(messages, willTopic)
	if !bytes.Equal(msg.Payload, []byte(`{"online":false,"node_id":"node-l2-08"}`)) {
		t.Fatalf("will payload = %s", msg.Payload)
	}
}

// TestL2InFlightPublishDisconnect uses a minimal fake broker that accepts
// CONNECT, sends CONNACK, then drops the connection on the first PUBLISH
// without acknowledging it. The channel must map paho's generic in-flight
// error to ErrNotConnected.
func TestL2InFlightPublishDisconnect(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go acceptAndDropPublish(ln)

	c := newL2Channel(t, "mqtt://"+ln.Addr().String(), "node-l2-10")
	waitL2Connected(t, c)

	err = c.Publish(context.Background(), interfaces.Message{Topic: "v1/irrigation/x", QoS: interfaces.QoS1})
	if !errors.Is(err, interfaces.ErrNotConnected) {
		t.Fatalf("in-flight publish = %v, want ErrNotConnected", err)
	}
}

func acceptAndDropPublish(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go func(conn net.Conn) {
			defer func() { _ = conn.Close() }()
			_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			r := bufio.NewReader(conn)
			if _, _, err := readMQTTPacket(r); err != nil {
				return
			}
			if _, err := conn.Write([]byte{0x20, 0x02, 0x00, 0x00}); err != nil {
				return
			}
			// Read the PUBLISH, then vanish without PUBACK.
			_, _, _ = readMQTTPacket(r)
		}(conn)
	}
}

func TestL2CloseDuringConnect(t *testing.T) {
	old := connectTimeout
	connectTimeout = 500 * time.Millisecond
	t.Cleanup(func() { connectTimeout = old })

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	closedByPeer := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		r := bufio.NewReader(conn)
		if _, _, err := readMQTTPacket(r); err != nil {
			return
		}
		// Let Close cancel the caller before the CONNACK arrives, so the
		// cleanup path has to disconnect a late success.
		time.Sleep(200 * time.Millisecond)
		if _, err := conn.Write([]byte{0x20, 0x02, 0x00, 0x00}); err != nil {
			return
		}
		for {
			if _, _, err := readMQTTPacket(r); err != nil {
				close(closedByPeer)
				return
			}
		}
	}()

	c, err := New(l2Options("mqtt://"+ln.Addr().String(), "node-l2-11"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	done := make(chan struct{})
	go func() {
		_ = c.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Close did not finish while connecting")
	}
	select {
	case <-closedByPeer:
	case <-time.After(3 * time.Second):
		t.Fatal("late successful connect was not disconnected")
	}
}

func readMQTTPacket(r *bufio.Reader) (byte, []byte, error) {
	hdr, err := r.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	mult := 1
	value := 0
	for range 4 {
		b, err := r.ReadByte()
		if err != nil {
			return 0, nil, err
		}
		value += int(b&0x7f) * mult
		if b&0x80 == 0 {
			break
		}
		mult *= 128
	}
	body := make([]byte, value)
	if _, err := io.ReadFull(r, body); err != nil {
		return 0, nil, err
	}
	return hdr, body, nil
}
