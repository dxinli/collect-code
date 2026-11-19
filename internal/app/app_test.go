package app

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func baseTree() map[string]string {
	return map[string]string{
		"config.yaml": "node:\n  id: node-1\n  product_key: irrigation\n" +
			"northbound:\n  mqtt:\n    broker: mqtt://localhost:1883\n",
		"links.yaml": "links:\n  - id: sensors\n    driver: modbus-rtu\n" +
			"    serial:\n      device: /dev/ttyS3\n",
		"devices.yaml": "devices:\n  - sn: S1\n    device_type: sensor\n" +
			"    profile: sensor-soil\n    link: sensors\n    address:\n      unit_id: 1\n",
		"profiles/sensor-soil.yaml": "profile: sensor-soil\ndevice_type: sensor\n",
	}
}

func writeConfig(t *testing.T, overrides map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	files := baseTree()
	maps.Copy(files, overrides)
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	return dir
}

func run(t *testing.T, args []string, lookup func(string) (string, bool)) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), args, &stdout, &stderr, lookup)
	return code, stdout.String(), stderr.String()
}

func TestVersion(t *testing.T) {
	code, stdout, stderr := run(t, []string{"version"}, nil)
	if code != ExitOK {
		t.Fatalf("code = %d, stderr=%s", code, stderr)
	}
	for _, key := range []string{"binary:", "version:", "commit:", "build_date:", "go:", "platform:"} {
		if !strings.Contains(stdout, key) {
			t.Errorf("version output missing %q:\n%s", key, stdout)
		}
	}
}

func TestUsageErrors(t *testing.T) {
	cases := [][]string{
		{},
		{"unknown"},
		{"config-check", "extra"},
		{"config-check", "--config="},
		{"config-check", "--nope"},
	}
	for _, args := range cases {
		code, _, _ := run(t, args, nil)
		if code != ExitUsage {
			t.Errorf("args %v: code = %d, want %d", args, code, ExitUsage)
		}
	}

	code, stdout, _ := run(t, []string{"--help"}, nil)
	if code != ExitOK || !strings.Contains(stdout, "Usage:") {
		t.Errorf("--help: code=%d out=%q", code, stdout)
	}
}

func TestConfigCheckValid(t *testing.T) {
	dir := writeConfig(t, nil)
	code, stdout, stderr := run(t, []string{"config-check", "--config", dir}, nil)
	if code != ExitOK {
		t.Fatalf("code = %d, stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "config ok: node=node-1") {
		t.Fatalf("summary = %q", stdout)
	}
}

func TestConfigCheckInvalid(t *testing.T) {
	dir := writeConfig(t, map[string]string{
		"config.yaml": "node:\n  product_key: irrigation\n" +
			"northbound:\n  mqtt:\n    broker: mqtt://x\n",
	})
	code, _, stderr := run(t, []string{"config-check", "--config", dir}, nil)
	if code != ExitError {
		t.Fatalf("code = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, "node.id: required") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestConfigCheckRejectsBadLogFormat(t *testing.T) {
	dir := writeConfig(t, map[string]string{
		"config.yaml": "node:\n  id: n\n  product_key: p\n" +
			"northbound:\n  mqtt:\n    broker: mqtt://x\n" +
			"log:\n  format: text\n",
	})
	code, _, stderr := run(t, []string{"config-check", "--config", dir}, nil)
	if code != ExitError || !strings.Contains(stderr, "log.format") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestConfigCheckSecretHandling(t *testing.T) {
	dir := writeConfig(t, map[string]string{
		"config.yaml": "node:\n  id: n\n  product_key: p\n" +
			"northbound:\n  mqtt:\n    broker: mqtt://x\n    password: env:MQTT_PASS\n",
	})

	code, _, stderr := run(t, []string{"config-check", "--config", dir}, func(string) (string, bool) {
		return "", false
	})
	if code != ExitError || !strings.Contains(stderr, "MQTT_PASS") {
		t.Fatalf("missing env: code=%d stderr=%q", code, stderr)
	}

	code, stdout, stderr := run(t, []string{"config-check", "--config", dir}, func(name string) (string, bool) {
		if name == "MQTT_PASS" {
			return "s3cr3t-value", true
		}
		return "", false
	})
	if code != ExitOK {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if strings.Contains(stdout+stderr, "s3cr3t-value") {
		t.Fatalf("secret leaked: %s%s", stdout, stderr)
	}
}

func TestRunWithBadConfig(t *testing.T) {
	dir := writeConfig(t, map[string]string{"links.yaml": ""})
	code, _, stderr := run(t, []string{"run", "--config", dir}, nil)
	if code != ExitError || !strings.Contains(stderr, "empty document") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

var testBinary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "collect-code-bin-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "temp dir: %v\n", err)
		os.Exit(1)
	}
	testBinary = filepath.Join(dir, "collect-code")
	root, err := filepath.Abs("../..")
	if err != nil {
		fmt.Fprintf(os.Stderr, "root: %v\n", err)
		os.Exit(1)
	}
	build := exec.Command("go", "build", "-o", testBinary, "./cmd/collect-code")
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

func TestRunGracefulShutdown(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("signal test requires linux")
	}
	const sentinel = "SENTINEL-run-secret"
	dir := writeConfig(t, map[string]string{
		"config.yaml": "node:\n  id: node-1\n  product_key: irrigation\n" +
			"northbound:\n  mqtt:\n    broker: mqtt://localhost:1883\n" +
			"    password: env:MQTT_PASS\n",
	})

	cmd := exec.Command(testBinary, "run", "--config", dir)
	cmd.Env = append(os.Environ(), "MQTT_PASS="+sentinel)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	lines := make(chan string, 64)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		if err := scanner.Err(); err != nil {
			lines <- `{"msg":"stderr scanner failed: ` + err.Error() + `"}`
		}
	}()

	waitFor := func(sub string) {
		t.Helper()
		deadline := time.After(5 * time.Second)
		var last string
		for {
			select {
			case line, ok := <-lines:
				if !ok {
					t.Fatalf("stderr closed before %q; last=%q", sub, last)
				}
				last = line
				if strings.Contains(line, sub) {
					return
				}
			case <-deadline:
				t.Fatalf("timeout waiting for %q; last=%q", sub, last)
			}
		}
	}

	// run installs the signal handler before logging startup, so seeing the
	// startup line means SIGTERM can no longer hit the default action.
	waitFor(`"msg":"startup"`)

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("signal: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("process exit: %v", err)
		}
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("process did not exit after SIGTERM")
	}

	var tail []string
	for line := range lines {
		tail = append(tail, line)
	}
	found := false
	for _, line := range tail {
		if strings.Contains(line, `"event":"shutdown"`) &&
			strings.Contains(line, `"reason":"signal"`) &&
			strings.Contains(line, `"signal":"terminated"`) {
			found = true
		}
	}
	if !found {
		t.Fatalf("shutdown log missing; tail=%v", tail)
	}
	if _, err := os.Stat(filepath.Join(dir, "state.db")); !os.IsNotExist(err) {
		t.Fatalf("state.db should not be created in 01a")
	}
	for _, line := range tail {
		if strings.Contains(line, sentinel) {
			t.Fatalf("secret leaked in run logs: %s", line)
		}
	}
}

func TestVersionOutputOrder(t *testing.T) {
	code, stdout, _ := run(t, []string{"version"}, nil)
	if code != ExitOK {
		t.Fatalf("code = %d", code)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	want := []string{"binary:", "version:", "commit:", "build_date:", "go:", "platform:"}
	if len(lines) != len(want) {
		t.Fatalf("lines = %d, want %d: %q", len(lines), len(want), stdout)
	}
	for i, prefix := range want {
		if !strings.HasPrefix(lines[i], prefix) {
			t.Fatalf("line %d = %q, want prefix %q", i, lines[i], prefix)
		}
	}
}

func TestConfigCheckParseFailureDoesNotLeakSentinel(t *testing.T) {
	const sentinel = "SENTINEL-parse-secret"
	dir := writeConfig(t, map[string]string{
		"config.yaml": "node:\n  id: n\n  product_key: p\n" +
			"northbound:\n  mqtt:\n    broker: mqtt://x\n" +
			"    password: {value: " + sentinel + "}\n",
	})
	code, stdout, stderr := run(t, []string{"config-check", "--config", dir}, func(string) (string, bool) {
		return "", false
	})
	if code != ExitError {
		t.Fatalf("code = %d, want %d", code, ExitError)
	}
	if strings.Contains(stdout+stderr, sentinel) {
		t.Fatalf("sentinel leaked: %s%s", stdout, stderr)
	}
}
