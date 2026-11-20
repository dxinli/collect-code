package sim_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

var simulateTestBinary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "collect-code-sim-bin-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "temp dir: %v\n", err)
		os.Exit(1)
	}
	simulateTestBinary = filepath.Join(dir, "collect-code")
	root, err := filepath.Abs("../..")
	if err != nil {
		fmt.Fprintf(os.Stderr, "root: %v\n", err)
		os.Exit(1)
	}
	build := exec.Command("go", "build", "-o", simulateTestBinary, "./cmd/collect-code")
	build.Dir = root
	build.Env = append(os.Environ(), "CGO_ENABLED=0", "GOFLAGS=")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build failed: %v\n%s", err, out)
		_ = os.RemoveAll(dir)
		os.Exit(1)
	}

	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func TestSimulateProcessSignals(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("signal test requires linux")
	}
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	cmd := exec.Command(simulateTestBinary, "simulate", "--manifest", manifestPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	exited := false
	defer func() {
		if !exited {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}()

	data := waitManifestProcess(t, cmd, manifestPath, 8*time.Second, &stderr)
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("manifest JSON: %v", err)
	}
	httpSection, _ := doc["http"].(map[string]any)
	baseURL, _ := httpSection["base_url"].(string)
	mqttSection, _ := doc["mqtt"].(map[string]any)
	mqttURL, _ := mqttSection["url"].(string)
	if baseURL == "" || mqttURL == "" {
		t.Fatalf("manifest missing endpoints: %v", doc)
	}

	status, captured := getJSON(t, baseURL+"/_sim/captured")
	if status != 200 || captured["ok"] != true {
		t.Fatalf("captured status=%d doc=%v", status, captured)
	}

	client := dialMQTT(t, strings.TrimPrefix(mqttURL, "mqtt://"), "process-1")
	client.subscribe("v1/#", 1)
	client.publish("v1/process", []byte(`{"process":1}`), 1, true)
	msg, err := client.readPublish(3 * time.Second)
	if err != nil {
		t.Fatalf("read publish: %v", err)
	}
	if msg.Topic != "v1/process" || string(msg.Payload) != `{"process":1}` {
		t.Fatalf("message = %+v", msg)
	}
	waitHTTPCaptured(t, baseURL+"/_sim/captured", 1, 3*time.Second)

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("signal: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("wait: %v (stderr=%s)", err, stderr.String())
	}
	exited = true
	if code := cmd.ProcessState.ExitCode(); code != 0 {
		t.Fatalf("exit code = %d, stderr=%s", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), `"reason":"signal"`) {
		t.Fatalf("missing signal shutdown log:\n%s", stderr.String())
	}

	if conn, err := net.DialTimeout("tcp", strings.TrimPrefix(mqttURL, "mqtt://"), 200*time.Millisecond); err == nil {
		_ = conn.Close()
		t.Fatal("mqtt port is still open after shutdown")
	}
	if conn, err := net.DialTimeout("tcp", strings.TrimPrefix(baseURL, "http://"), 200*time.Millisecond); err == nil {
		_ = conn.Close()
		t.Fatal("http port is still open after shutdown")
	}
}

func waitManifestProcess(t *testing.T, cmd *exec.Cmd, path string, timeout time.Duration, stderr *bytes.Buffer) []byte {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		data, err := os.ReadFile(path)
		if err == nil && len(data) > 0 {
			return data
		}
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			t.Fatalf("manifest was not written; stderr=%s", stderr.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
}
