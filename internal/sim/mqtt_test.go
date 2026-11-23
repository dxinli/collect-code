package sim_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dxinli/collect-code/internal/sim"
)

type testSim struct {
	runtime *sim.Runtime
	broker  *sim.MQTTBroker
}

func newTestSim(t *testing.T, capacity int, now func() time.Time) *testSim {
	t.Helper()
	r, err := sim.NewRuntime(sim.RuntimeOptions{
		HTTPAddr:        "127.0.0.1:0",
		CaptureCapacity: capacity,
		Now:             now,
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	b, err := sim.NewMQTTBroker(sim.MQTTBrokerOptions{
		Addr:    "127.0.0.1:0",
		Runtime: r,
		Now:     now,
	})
	if err != nil {
		t.Fatalf("new broker: %v", err)
	}
	if err := b.Register(r); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("start runtime: %v", err)
	}
	if err := b.Start(); err != nil {
		t.Fatalf("start broker: %v", err)
	}
	t.Cleanup(func() {
		_ = b.Close()
		_ = r.Close(context.Background())
	})
	return &testSim{runtime: r, broker: b}
}

func postJSON(t *testing.T, url, body string) (int, map[string]any) {
	t.Helper()
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	var doc map[string]any
	if len(data) > 0 {
		_ = json.Unmarshal(data, &doc)
	}
	return resp.StatusCode, doc
}

func getJSON(t *testing.T, url string) (int, map[string]any) {
	t.Helper()
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	var doc map[string]any
	if len(data) > 0 {
		_ = json.Unmarshal(data, &doc)
	}
	return resp.StatusCode, doc
}

func waitCapture(t *testing.T, s *testSim, cond func([]sim.CapturedMessage, uint64) bool, timeout time.Duration) ([]sim.CapturedMessage, uint64) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		messages, dropped := s.runtime.Capture().Snapshot()
		if cond(messages, dropped) {
			return messages, dropped
		}
		if time.Now().After(deadline) {
			t.Fatalf("capture condition not met: messages=%d dropped=%d", len(messages), dropped)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func waitConnect(t *testing.T, addr, clientID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		code, err := tryConnect(t, addr, clientID, 300*time.Millisecond)
		if err == nil && code == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("connect %s: code=%d err=%v", clientID, code, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestBrokerBindsEphemeralLoopbackAndManifest(t *testing.T) {
	s := newTestSim(t, 8, nil)
	host, portText, err := net.SplitHostPort(s.broker.Address())
	if err != nil {
		t.Fatal(err)
	}
	if host != "127.0.0.1" {
		t.Fatalf("host = %q", host)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port == 0 {
		t.Fatalf("port = %q, err=%v", portText, err)
	}

	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := s.runtime.WriteManifest(path); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	mqttSection, _ := doc["mqtt"].(map[string]any)
	if mqttSection["url"] != s.broker.URL() {
		t.Fatalf("mqtt.url = %v, want %s", mqttSection["url"], s.broker.URL())
	}
	if mqttSection["port"] != float64(port) {
		t.Fatalf("mqtt.port = %v, want %d", mqttSection["port"], port)
	}
	if mqttSection["capture_capacity"] != float64(8) {
		t.Fatalf("capture_capacity = %v", mqttSection["capture_capacity"])
	}
	filters, _ := mqttSection["topic_filters"].([]any)
	if len(filters) != 1 || filters[0] != "#" {
		t.Fatalf("topic_filters = %v", filters)
	}
	endpoints, _ := doc["endpoints"].(map[string]any)
	for _, key := range []string{"captured", "invoke", "control"} {
		if endpoints[key] == nil {
			t.Fatalf("endpoint %q missing: %v", key, endpoints)
		}
	}
	modules, _ := doc["modules"].([]any)
	if len(modules) != 1 || modules[0] != "mqtt" {
		t.Fatalf("modules = %v", modules)
	}
	controls, _ := doc["control_actions"].(map[string]any)
	global, _ := controls["global"].([]any)
	if len(global) != 1 || global[0] != "broker_disconnect" {
		t.Fatalf("control_actions = %v", controls)
	}
	httpSection, _ := doc["http"].(map[string]any)
	if base, _ := httpSection["base_url"].(string); !strings.HasPrefix(base, "http://127.0.0.1:") {
		t.Fatalf("http.base_url = %v", httpSection["base_url"])
	}
}

func TestBrokerAuthAllowsConnectPublishSubscribe(t *testing.T) {
	s := newTestSim(t, 16, nil)
	sub := dialMQTT(t, s.broker.Address(), "sub-1")
	if code := sub.Subscribe("v1/#", 1); code == 0x80 {
		t.Fatalf("SUBACK return code = 0x%02x", code)
	}
	pub := dialMQTT(t, s.broker.Address(), "pub-1")
	pub.Publish("v1/x", []byte(`{"n":1}`), 1, false)

	msg, err := sub.ReadPublish(3 * time.Second)
	if err != nil {
		t.Fatalf("read publish: %v", err)
	}
	if msg.Topic != "v1/x" || string(msg.Payload) != `{"n":1}` || msg.QoS != 1 {
		t.Fatalf("message = %+v", msg)
	}
	waitCapture(t, s, func(messages []sim.CapturedMessage, _ uint64) bool {
		return len(messages) >= 1
	}, 2*time.Second)
}

func TestCaptureQoS0AndQoS1(t *testing.T) {
	s := newTestSim(t, 16, nil)
	c := dialMQTT(t, s.broker.Address(), "capture-1")
	c.Publish("v1/a", []byte(`{"n":0}`), 0, false)
	c.Publish("v1/b", []byte(`{"n":1}`), 1, true)

	messages, dropped := waitCapture(t, s, func(messages []sim.CapturedMessage, _ uint64) bool {
		return len(messages) >= 2
	}, 2*time.Second)
	if dropped != 0 {
		t.Fatalf("dropped = %d", dropped)
	}
	if messages[0].Topic != "v1/a" || messages[0].QoS != 0 || messages[0].Retain {
		t.Fatalf("message[0] = %+v", messages[0])
	}
	if messages[1].Topic != "v1/b" || messages[1].QoS != 1 || !messages[1].Retain {
		t.Fatalf("message[1] = %+v", messages[1])
	}
	if string(messages[1].Payload) != `{"n":1}` {
		t.Fatalf("payload = %s", messages[1].Payload)
	}
}

func TestCapturedExcludesInlineDownlink(t *testing.T) {
	s := newTestSim(t, 8, nil)
	if err := s.broker.Publish("v1/down", []byte(`{"down":1}`), 1, false); err != nil {
		t.Fatalf("publish: %v", err)
	}
	messages, dropped := s.runtime.Capture().Snapshot()
	if len(messages) != 0 || dropped != 0 {
		t.Fatalf("inline downlink captured: %+v dropped=%d", messages, dropped)
	}
}

func TestInvokePublishesExactBytes(t *testing.T) {
	s := newTestSim(t, 16, nil)
	const topic = "v1/irrigation/VALVE-0001/service/invoke"
	sub := dialMQTT(t, s.broker.Address(), "invoke-sub")
	sub.Subscribe(topic, 1)

	payload := `{"msg_id":"m1","type":"service.invoke","device_sn":"VALVE-0001","service_id":"set_opening","seq":17,"params":{"opening":50},"timeout_ms":600000,"issued_ts":1763400000000}`
	body := fmt.Sprintf(`{"topic":%q,"payload":%s,"qos":1}`, topic, payload)
	status, doc := postJSON(t, s.runtime.BaseURL()+"/_sim/invoke", body)
	if status != http.StatusOK {
		t.Fatalf("status = %d, doc=%v", status, doc)
	}
	if doc["bytes"] != float64(len(payload)) {
		t.Fatalf("bytes = %v, want %d", doc["bytes"], len(payload))
	}

	msg, err := sub.ReadPublish(3 * time.Second)
	if err != nil {
		t.Fatalf("read publish: %v", err)
	}
	if msg.Topic != topic || string(msg.Payload) != payload || msg.QoS != 1 || msg.Retain {
		t.Fatalf("message = %+v", msg)
	}

	messages, _ := s.runtime.Capture().Snapshot()
	for _, m := range messages {
		if m.Topic == topic {
			t.Fatalf("inline downlink was captured: %+v", m)
		}
	}
}

func TestInvokeQoSRetain(t *testing.T) {
	s := newTestSim(t, 8, nil)
	const topic = "v1/retain"
	body := fmt.Sprintf(`{"topic":%q,"payload":{"r":true},"qos":0,"retain":true}`, topic)
	status, doc := postJSON(t, s.runtime.BaseURL()+"/_sim/invoke", body)
	if status != http.StatusOK {
		t.Fatalf("status = %d, doc=%v", status, doc)
	}

	sub := dialMQTT(t, s.broker.Address(), "retain-sub")
	sub.Subscribe(topic, 0)
	msg, err := sub.ReadPublish(3 * time.Second)
	if err != nil {
		t.Fatalf("read retained publish: %v", err)
	}
	if msg.Topic != topic || string(msg.Payload) != `{"r":true}` || msg.QoS != 0 || !msg.Retain {
		t.Fatalf("message = %+v", msg)
	}
}

func TestInvokeValidation(t *testing.T) {
	s := newTestSim(t, 8, nil)
	base := s.runtime.BaseURL()
	cases := []struct {
		name   string
		body   string
		status int
	}{
		{"missing topic", `{"payload":{"a":1}}`, http.StatusBadRequest},
		{"empty topic", `{"topic":"","payload":{"a":1}}`, http.StatusBadRequest},
		{"wildcard topic", `{"topic":"v1/+/x","payload":{"a":1}}`, http.StatusBadRequest},
		{"sys topic", `{"topic":"$SYS/x","payload":{"a":1}}`, http.StatusBadRequest},
		{"missing payload", `{"topic":"v1/x"}`, http.StatusBadRequest},
		{"qos 2", `{"topic":"v1/x","payload":{"a":1},"qos":2}`, http.StatusBadRequest},
		{"bad json", `{`, http.StatusBadRequest},
		{"unknown field", `{"topic":"v1/x","payload":{"a":1},"extra":1}`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, doc := postJSON(t, base+"/_sim/invoke", tc.body)
			if status != tc.status {
				t.Fatalf("status = %d, want %d, doc=%v", status, tc.status, doc)
			}
		})
	}

	resp, err := (&http.Client{Timeout: 2 * time.Second}).Get(base + "/_sim/invoke")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET invoke status = %d", resp.StatusCode)
	}
	resp, err = (&http.Client{Timeout: 2 * time.Second}).Get(base + "/nope")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET unknown status = %d", resp.StatusCode)
	}
}

func TestHTTPCaptured(t *testing.T) {
	s := newTestSim(t, 8, nil)
	c := dialMQTT(t, s.broker.Address(), "http-capture")
	c.Publish("v1/http", []byte(`{"n":7}`), 1, false)
	waitCapture(t, s, func(messages []sim.CapturedMessage, _ uint64) bool {
		return len(messages) >= 1
	}, 2*time.Second)

	status, doc := getJSON(t, s.runtime.BaseURL()+"/_sim/captured")
	if status != http.StatusOK || doc["ok"] != true {
		t.Fatalf("status=%d doc=%v", status, doc)
	}
	messages, _ := doc["messages"].([]any)
	if len(messages) != 1 {
		t.Fatalf("messages = %v", doc["messages"])
	}
	first, _ := messages[0].(map[string]any)
	if first["topic"] != "v1/http" || first["qos"] != float64(1) || first["retain"] != false {
		t.Fatalf("message = %v", first)
	}
	payload, _ := first["payload"].(map[string]any)
	if payload["n"] != float64(7) {
		t.Fatalf("payload = %v", first["payload"])
	}
}

func TestCaptureCapacityIntegration(t *testing.T) {
	s := newTestSim(t, 2, nil)
	c := dialMQTT(t, s.broker.Address(), "capacity")
	for i := 1; i <= 5; i++ {
		c.Publish(fmt.Sprintf("v1/%d", i), fmt.Appendf(nil, `{"n":%d}`, i), 0, false)
	}
	messages, _ := waitCapture(t, s, func(messages []sim.CapturedMessage, dropped uint64) bool {
		return len(messages) == 2 && dropped == 3
	}, 2*time.Second)
	if messages[0].Seq != 4 || messages[1].Seq != 5 {
		t.Fatalf("seqs = %d,%d", messages[0].Seq, messages[1].Seq)
	}
}

func TestBrokerDisconnectKicksClients(t *testing.T) {
	s := newTestSim(t, 8, nil)
	client := dialMQTT(t, s.broker.Address(), "kick-1")
	status, doc := postJSON(t, s.runtime.BaseURL()+"/_sim/control", `{"action":"broker_disconnect"}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d, doc=%v", status, doc)
	}
	if n, _ := doc["disconnected"].(float64); n < 1 {
		t.Fatalf("disconnected = %v", doc["disconnected"])
	}
	if doc["hold_until"] != nil {
		t.Fatalf("hold_until = %v, want null", doc["hold_until"])
	}
	client.ExpectClosed(2 * time.Second)
	waitConnect(t, s.broker.Address(), "kick-1", 3*time.Second)
}

func TestBrokerDisconnectHoldAndRecover(t *testing.T) {
	var mu sync.Mutex
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	nowFn := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return now
	}
	s := newTestSim(t, 8, nowFn)

	status, doc := postJSON(t, s.runtime.BaseURL()+"/_sim/control", `{"action":"broker_disconnect","duration_ms":5000}`)
	if status != http.StatusOK || doc["hold_until"] == nil {
		t.Fatalf("status=%d doc=%v", status, doc)
	}
	code, err := tryConnect(t, s.broker.Address(), "hold-1", time.Second)
	if err != nil {
		t.Fatalf("expected CONNACK rejection, got error: %v", err)
	}
	if code != 5 {
		t.Fatalf("CONNACK code = %d, want 5", code)
	}

	mu.Lock()
	now = now.Add(6 * time.Second)
	mu.Unlock()
	code, err = tryConnect(t, s.broker.Address(), "hold-1", time.Second)
	if err != nil || code != 0 {
		t.Fatalf("after hold: code=%d err=%v", code, err)
	}
}

func TestBrokerDisconnectValidation(t *testing.T) {
	s := newTestSim(t, 4, nil)
	cases := []string{
		`{"action":"broker_disconnect","duration_ms":-1}`,
		`{"action":"broker_disconnect","duration_ms":86400001}`,
		`{"action":"broker_disconnect","extra":1}`,
	}
	for _, body := range cases {
		status, doc := postJSON(t, s.runtime.BaseURL()+"/_sim/control", body)
		if status != http.StatusBadRequest {
			t.Fatalf("body %s: status = %d, doc=%v", body, status, doc)
		}
	}
}

func TestBrokerPublishValidation(t *testing.T) {
	s := newTestSim(t, 4, nil)
	cases := []struct {
		topic string
		qos   byte
	}{
		{"", 0},
		{"v1/+/x", 0},
		{"$SYS/x", 0},
		{"v1/x", 2},
	}
	for _, tc := range cases {
		if err := s.broker.Publish(tc.topic, nil, tc.qos, false); err == nil {
			t.Fatalf("Publish(%q, qos=%d) accepted", tc.topic, tc.qos)
		}
	}
}

func TestBrokerCloseReleasesPort(t *testing.T) {
	s := newTestSim(t, 4, nil)
	addr := s.broker.Address()
	if err := s.broker.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		t.Fatal("mqtt port is still open")
	}
}

func TestBrokerInvalidOptions(t *testing.T) {
	r, err := sim.NewRuntime(sim.RuntimeOptions{HTTPAddr: "127.0.0.1:0", CaptureCapacity: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close(context.Background()) }()
	if _, err := sim.NewMQTTBroker(sim.MQTTBrokerOptions{}); err == nil {
		t.Fatal("nil runtime accepted")
	}
	if _, err := sim.NewMQTTBroker(sim.MQTTBrokerOptions{Runtime: r, TopicFilter: "v1/#/x"}); err == nil {
		t.Fatal("invalid topic filter accepted")
	}
	if _, err := sim.NewMQTTBroker(sim.MQTTBrokerOptions{Runtime: r, Addr: "not-an-address"}); err == nil {
		t.Fatal("invalid address accepted")
	}
}

func TestBrokerStartTwice(t *testing.T) {
	s := newTestSim(t, 4, nil)
	if err := s.broker.Start(); err == nil {
		t.Fatal("second Start accepted")
	}
}
