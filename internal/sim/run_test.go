package sim_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dxinli/collect-code/internal/sim"
)

func TestRunHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := sim.Run(context.Background(), []string{"--help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Usage:") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunUsageErrors(t *testing.T) {
	cases := [][]string{
		{"--nope"},
		{"extra"},
		{"--capture-capacity", "0"},
		{"--capture-capacity", "1000001"},
		{"--capture-capacity"},
		{"--http-addr", "bad"},
		{"--mqtt-addr", "127.0.0.1:notaport"},
		{"--topic-filter", "v1/#/x"},
		{"--manifest"},
	}
	for _, args := range cases {
		var stdout, stderr bytes.Buffer
		code := sim.Run(context.Background(), args, &stdout, &stderr)
		if code != 2 {
			t.Errorf("args %v: code = %d, want 2 (stderr=%s)", args, code, stderr.String())
		}
		if stderr.Len() == 0 {
			t.Errorf("args %v: stderr is empty", args)
		}
	}
}

func TestRunManifestAndShutdown(t *testing.T) {
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var stdout, stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- sim.Run(ctx, []string{"--manifest", manifestPath}, &stdout, &stderr)
	}()

	data := waitForManifest(t, done, manifestPath, 5*time.Second, &stderr)
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("manifest JSON: %v\n%s", err, data)
	}
	httpSection, _ := doc["http"].(map[string]any)
	baseURL, _ := httpSection["base_url"].(string)
	if baseURL == "" {
		t.Fatalf("http.base_url missing: %v", doc)
	}
	mqttSection, _ := doc["mqtt"].(map[string]any)
	mqttURL, _ := mqttSection["url"].(string)
	if mqttURL == "" {
		t.Fatalf("mqtt.url missing: %v", doc)
	}
	mqttAddr := strings.TrimPrefix(mqttURL, "mqtt://")

	status, captured := getJSON(t, baseURL+"/_sim/captured")
	if status != 200 || captured["ok"] != true {
		t.Fatalf("captured status=%d doc=%v", status, captured)
	}

	client := dialMQTT(t, mqttAddr, "run-1")
	client.Subscribe("v1/#", 1)
	client.Publish("v1/run", []byte(`{"run":1}`), 1, false)
	msg, err := client.ReadPublish(3 * time.Second)
	if err != nil {
		t.Fatalf("read publish: %v", err)
	}
	if msg.Topic != "v1/run" || string(msg.Payload) != `{"run":1}` {
		t.Fatalf("message = %+v", msg)
	}
	waitHTTPCaptured(t, baseURL+"/_sim/captured", 1, 3*time.Second)

	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("Run = %d, stderr=%s", code, stderr.String())
		}
	case <-time.After(6 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}

	if conn, err := net.DialTimeout("tcp", mqttAddr, 200*time.Millisecond); err == nil {
		_ = conn.Close()
		t.Fatal("mqtt port is still open after shutdown")
	}
	httpAddr := strings.TrimPrefix(baseURL, "http://")
	if conn, err := net.DialTimeout("tcp", httpAddr, 200*time.Millisecond); err == nil {
		_ = conn.Close()
		t.Fatal("http port is still open after shutdown")
	}
}

func TestRunManifestFailure(t *testing.T) {
	manifestPath := filepath.Join(t.TempDir(), "missing", "manifest.json")
	var stdout, stderr bytes.Buffer
	code := sim.Run(context.Background(), []string{"--manifest", manifestPath}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code = %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "manifest") {
		t.Fatalf("stderr does not mention manifest: %s", stderr.String())
	}
}

func waitForManifest(t *testing.T, done <-chan int, path string, timeout time.Duration, stderr *bytes.Buffer) []byte {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		select {
		case code := <-done:
			t.Fatalf("Run exited early code=%d stderr=%s", code, stderr.String())
		default:
		}
		data, err := os.ReadFile(path)
		if err == nil && len(data) > 0 {
			return data
		}
		if time.Now().After(deadline) {
			t.Fatalf("manifest was not written: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitHTTPCaptured(t *testing.T, url string, want int, timeout time.Duration) map[string]any {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		status, doc := getJSON(t, url)
		if status == 200 {
			messages, _ := doc["messages"].([]any)
			if len(messages) >= want {
				return doc
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("captured messages never reached %d: status=%d doc=%v", want, status, doc)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
