package sim_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/dxinli/collect-code/internal/sim"
)

type testModbusServer struct {
	runtime *sim.Runtime
	modbus  *sim.Modbus
	baseURL string
}

func newTestModbus(t *testing.T, now func() time.Time, configDir string) *testModbusServer {
	t.Helper()
	rt, err := sim.NewRuntime(sim.RuntimeOptions{
		HTTPAddr: "127.0.0.1:0",
		Now:      now,
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	mb, err := sim.NewModbus(sim.ModbusOptions{
		TCPAddr:   "127.0.0.1:0",
		RTUAddr:   "127.0.0.1:0",
		ConfigDir: configDir,
		Now:       now,
	})
	if err != nil {
		t.Fatalf("new modbus: %v", err)
	}
	if err := mb.Register(rt); err != nil {
		t.Fatalf("register modbus: %v", err)
	}
	if err := mb.Start(); err != nil {
		t.Fatalf("start modbus: %v", err)
	}
	if err := rt.Start(context.Background()); err != nil {
		t.Fatalf("start runtime: %v", err)
	}
	t.Cleanup(func() {
		_ = mb.Close()
		_ = rt.Close(context.Background())
	})
	return &testModbusServer{runtime: rt, modbus: mb, baseURL: rt.BaseURL()}
}

type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func newTestClock() *testClock {
	return &testClock{t: time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)}
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func requireException(t *testing.T, err error, code uint8) {
	t.Helper()
	var exception *testModbusException
	if !errors.As(err, &exception) {
		t.Fatalf("want exception 0x%02X, got %v", code, err)
	}
	if exception.code != code {
		t.Fatalf("exception = 0x%02X, want 0x%02X", exception.code, code)
	}
}

func requireTimeout(t *testing.T, err error) {
	t.Helper()
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("want timeout, got %v", err)
	}
}

func requireValues(t *testing.T, got, want []uint16) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("values = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("values = %v, want %v", got, want)
		}
	}
}

func TestModbusBuiltinReadsTCP(t *testing.T) {
	server := newTestModbus(t, nil, "")
	client := dialTestModbusTCP(t, server.modbus.TCPAddress())

	holding := []struct {
		address uint16
		want    uint16
	}{
		{0x0000, 0},
		{0x0001, 500},
		{0x0002, 500},
		{0x0009, 0},
		{0x000A, 0},
		{0x000B, 0x0102},
	}
	for _, tc := range holding {
		got, err := client.readHolding(1, tc.address, 1)
		if err != nil {
			t.Fatalf("read holding 0x%04X: %v", tc.address, err)
		}
		requireValues(t, got, []uint16{tc.want})
	}

	input, err := client.readInput(1, 0x0000, 4)
	if err != nil {
		t.Fatalf("read input 0x0000: %v", err)
	}
	requireValues(t, input, []uint16{185, 352, 780, 680})

	env, err := client.readInput(1, 0x0004, 2)
	if err != nil {
		t.Fatalf("read input 0x0004: %v", err)
	}
	requireValues(t, env, []uint16{215, 42})

	flow, err := client.readInput(1, 0x0006, 2)
	if err != nil {
		t.Fatalf("read input 0x0006: %v", err)
	}
	requireValues(t, flow, []uint16{0, 123})

	total, err := client.readInput(1, 0x000C, 2)
	if err != nil {
		t.Fatalf("read input 0x000C: %v", err)
	}
	requireValues(t, total, []uint16{0, 1000})

	other, err := client.readHolding(2, 0x0001, 1)
	if err != nil {
		t.Fatalf("read unit 2: %v", err)
	}
	requireValues(t, other, []uint16{500})
}

func TestModbusRTUOverTCP(t *testing.T) {
	server := newTestModbus(t, nil, "")
	client := dialTestModbusRTU(t, server.modbus.RTUAddress())

	got, err := client.readInput(1, 0x0000, 4)
	if err != nil {
		t.Fatalf("read input over rtu: %v", err)
	}
	requireValues(t, got, []uint16{185, 352, 780, 680})

	if err := client.writeSingle(1, 0x0002, 750); err != nil {
		t.Fatalf("write target over rtu: %v", err)
	}
	current, err := client.readHolding(1, 0x0001, 1)
	if err != nil {
		t.Fatalf("read current over rtu: %v", err)
	}
	requireValues(t, current, []uint16{750})
}

func TestModbusWriteSemantics(t *testing.T) {
	server := newTestModbus(t, nil, "")
	client := dialTestModbusTCP(t, server.modbus.TCPAddress())

	if err := client.writeSingle(1, 0x0002, 750); err != nil {
		t.Fatalf("write target: %v", err)
	}
	current, err := client.readHolding(1, 0x0001, 1)
	if err != nil {
		t.Fatalf("read current: %v", err)
	}
	requireValues(t, current, []uint16{750})
	target, err := client.readHolding(1, 0x0002, 1)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	requireValues(t, target, []uint16{750})
	state, err := client.readHolding(1, 0x0000, 1)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	requireValues(t, state, []uint16{0})

	if err := client.writeSingle(1, 0x0003, 1); err != nil {
		t.Fatalf("open command: %v", err)
	}
	current, err = client.readHolding(1, 0x0001, 1)
	if err != nil {
		t.Fatalf("read current after open: %v", err)
	}
	requireValues(t, current, []uint16{1000})

	if err := client.writeMultiple(1, 0x0002, []uint16{800, 0}); err != nil {
		t.Fatalf("write multiple: %v", err)
	}
	current, err = client.readHolding(1, 0x0001, 1)
	if err != nil {
		t.Fatalf("read current after multiple: %v", err)
	}
	requireValues(t, current, []uint16{800})

	err = client.writeSingle(1, 0x0001, 10)
	requireException(t, err, 0x02)
	err = client.writeMultiple(1, 0x0001, []uint16{1})
	requireException(t, err, 0x02)

	_, err = client.readHolding(1, 0x0003, 1)
	requireException(t, err, 0x02)
}

func TestModbusExceptions(t *testing.T) {
	server := newTestModbus(t, nil, "")
	tcp := dialTestModbusTCP(t, server.modbus.TCPAddress())

	_, err := tcp.readHolding(1, 0x0000, 0)
	requireException(t, err, 0x03)
	_, err = tcp.readHolding(1, 0x0000, 126)
	requireException(t, err, 0x03)
	_, err = tcp.readHolding(1, 0x0010, 1)
	requireException(t, err, 0x02)
	_, err = tcp.readHolding(1, 0x0002, 2)
	requireException(t, err, 0x02)

	resp, err := tcp.exchange(1, []byte{0x2B, 0x0E, 0x01, 0x00}, time.Second)
	if err != nil {
		t.Fatalf("unknown function: %v", err)
	}
	if resp.Exception != 0x01 {
		t.Fatalf("unknown function exception = 0x%02X, want 0x01", resp.Exception)
	}

	_, err = tcp.exchange(9, testReadPDU(0x03, 0x0001, 1), 100*time.Millisecond)
	requireTimeout(t, err)

	rtu := dialTestModbusRTU(t, server.modbus.RTUAddress())
	bad := testRTUFrame(1, testReadPDU(0x03, 0x0001, 1))
	bad[len(bad)-1] ^= 0xFF
	if err := rtu.sendRaw(bad); err != nil {
		t.Fatalf("send bad crc: %v", err)
	}
	if _, err := rtu.readRaw(100 * time.Millisecond); err == nil {
		t.Fatal("server responded to a bad request CRC")
	}
	got, err := rtu.readHolding(1, 0x0001, 1)
	if err != nil {
		t.Fatalf("connection unusable after bad CRC: %v", err)
	}
	requireValues(t, got, []uint16{500})

	unknown := rtu
	if err := unknown.sendRaw([]byte{0x01, 0x2B}); err != nil {
		t.Fatalf("send unknown function: %v", err)
	}
	if _, err := unknown.readRaw(100 * time.Millisecond); err == nil {
		t.Fatal("unknown RTU function did not close the connection")
	}
}

func TestModbusControlOfflineAndRecovery(t *testing.T) {
	clock := newTestClock()
	server := newTestModbus(t, clock.Now, "")
	client := dialTestModbusTCP(t, server.modbus.TCPAddress())

	status, doc := postJSON(t, server.baseURL+"/_sim/control",
		`{"module":"modbus","action":"unit_offline","unit_id":1,"duration_ms":1000}`)
	if status != 200 || doc["ok"] != true {
		t.Fatalf("control status=%d doc=%v", status, doc)
	}
	if doc["until"] == nil {
		t.Fatalf("until missing: %v", doc)
	}

	_, err := client.exchange(1, testReadPDU(0x03, 0x0001, 1), 100*time.Millisecond)
	requireTimeout(t, err)

	status, doc = getJSON(t, server.baseURL+"/_sim/requests")
	if status != 200 {
		t.Fatalf("requests status=%d", status)
	}
	entries, _ := doc["requests"].([]any)
	last, _ := entries[len(entries)-1].(map[string]any)
	if last["outcome"] != "no_response" {
		t.Fatalf("last outcome = %v, want no_response", last["outcome"])
	}

	clock.Advance(1001 * time.Millisecond)
	got, err := client.readHolding(1, 0x0001, 1)
	if err != nil {
		t.Fatalf("read after recovery: %v", err)
	}
	requireValues(t, got, []uint16{500})
}

func TestModbusControlSlow(t *testing.T) {
	clock := newTestClock()
	server := newTestModbus(t, clock.Now, "")
	client := dialTestModbusTCP(t, server.modbus.TCPAddress())

	status, doc := postJSON(t, server.baseURL+"/_sim/control",
		`{"module":"modbus","action":"unit_slow","unit_id":1,"delay_ms":50,"duration_ms":1000}`)
	if status != 200 || doc["ok"] != true {
		t.Fatalf("control status=%d doc=%v", status, doc)
	}

	start := time.Now()
	got, err := client.readHolding(1, 0x0001, 1)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("slow read: %v", err)
	}
	requireValues(t, got, []uint16{500})
	if elapsed < 25*time.Millisecond {
		t.Fatalf("slow read returned after %v, want at least 25ms", elapsed)
	}

	status, doc = getJSON(t, server.baseURL+"/_sim/requests")
	if status != 200 {
		t.Fatalf("requests status=%d", status)
	}
	entries, _ := doc["requests"].([]any)
	last, _ := entries[len(entries)-1].(map[string]any)
	if last["duration_ms"].(float64) < 25 {
		t.Fatalf("duration_ms = %v, want >= 25", last["duration_ms"])
	}

	clock.Advance(1001 * time.Millisecond)
	start = time.Now()
	if _, err := client.readHolding(1, 0x0001, 1); err != nil {
		t.Fatalf("read after slow recovery: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("read still delayed after recovery: %v", elapsed)
	}
}

func TestModbusControlBadCRC(t *testing.T) {
	clock := newTestClock()
	server := newTestModbus(t, clock.Now, "")
	rtu := dialTestModbusRTU(t, server.modbus.RTUAddress())
	tcp := dialTestModbusTCP(t, server.modbus.TCPAddress())

	status, doc := postJSON(t, server.baseURL+"/_sim/control",
		`{"module":"modbus","action":"unit_bad_crc","unit_id":1}`)
	if status != 200 || doc["ok"] != true {
		t.Fatalf("control status=%d doc=%v", status, doc)
	}

	_, err := rtu.readHolding(1, 0x0001, 1)
	if err == nil || !strings.Contains(err.Error(), "bad response crc") {
		t.Fatalf("rtu read = %v, want bad response crc", err)
	}
	got, err := tcp.readHolding(1, 0x0001, 1)
	if err != nil {
		t.Fatalf("tcp read under bad_crc: %v", err)
	}
	requireValues(t, got, []uint16{500})

	status, doc = getJSON(t, server.baseURL+"/_sim/requests")
	if status != 200 {
		t.Fatalf("requests status=%d", status)
	}
	entries, _ := doc["requests"].([]any)
	foundBadCRC := false
	for _, raw := range entries {
		entry, _ := raw.(map[string]any)
		if entry["outcome"] == "bad_crc" && entry["transport"] == "rtu_over_tcp" {
			foundBadCRC = true
		}
	}
	if !foundBadCRC {
		t.Fatalf("no bad_crc request logged: %v", entries)
	}
}

func TestModbusControlValidation(t *testing.T) {
	server := newTestModbus(t, nil, "")
	cases := []string{
		`{"module":"modbus","action":"unit_offline"}`,
		`{"module":"modbus","action":"unit_offline","unit_id":99}`,
		`{"module":"modbus","action":"unit_offline","unit_id":300}`,
		`{"module":"modbus","action":"unit_offline","unit_id":1,"duration_ms":-1}`,
		`{"module":"modbus","action":"unit_offline","unit_id":1,"duration_ms":86400001}`,
		`{"module":"modbus","action":"unit_slow","unit_id":1,"duration_ms":1000}`,
		`{"module":"modbus","action":"unit_slow","unit_id":1,"delay_ms":0}`,
		`{"module":"modbus","action":"unit_offline","unit_id":1,"bogus":true}`,
	}
	for _, body := range cases {
		status, doc := postJSON(t, server.baseURL+"/_sim/control", body)
		if status != 400 {
			t.Errorf("body %s: status=%d doc=%v, want 400", body, status, doc)
		}
	}
}

func TestModbusRequestsEndpoint(t *testing.T) {
	server := newTestModbus(t, nil, "")
	status, doc := getJSON(t, server.baseURL+"/_sim/requests")
	if status != 200 || doc["ok"] != true {
		t.Fatalf("empty requests status=%d doc=%v", status, doc)
	}
	entries, _ := doc["requests"].([]any)
	if len(entries) != 0 {
		t.Fatalf("empty requests = %v", entries)
	}

	client := dialTestModbusTCP(t, server.modbus.TCPAddress())
	if _, err := client.readHolding(1, 0x0001, 1); err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := client.writeSingle(1, 0x0002, 750); err != nil {
		t.Fatalf("write: %v", err)
	}

	status, doc = getJSON(t, server.baseURL+"/_sim/requests")
	if status != 200 {
		t.Fatalf("requests status=%d", status)
	}
	entries, _ = doc["requests"].([]any)
	if len(entries) != 2 {
		t.Fatalf("requests = %v, want 2 entries", entries)
	}
	read, _ := entries[0].(map[string]any)
	if read["function"].(float64) != 3 || read["address"].(float64) != 1 ||
		read["outcome"] != "ok" || read["transport"] != "tcp" {
		t.Fatalf("read entry = %v", read)
	}
	if read["responded_at"] == nil {
		t.Fatalf("read responded_at missing: %v", read)
	}
	write, _ := entries[1].(map[string]any)
	if write["function"].(float64) != 6 || write["outcome"] != "ok" {
		t.Fatalf("write entry = %v", write)
	}
	values, _ := write["values"].([]any)
	if len(values) != 1 || values[0].(float64) != 750 {
		t.Fatalf("write values = %v", values)
	}
	if doc["dropped"].(float64) != 0 {
		t.Fatalf("dropped = %v", doc["dropped"])
	}
}

func TestModbusConfigProfile(t *testing.T) {
	server := newTestModbus(t, nil, filepath.Join("..", "..", "configs", "example"))
	manifest := writeTestModbusManifest(t, server)
	section, _ := manifest["modbus"].(map[string]any)
	if section["register_source"] != "config" {
		t.Fatalf("register_source = %v", section["register_source"])
	}
	units, _ := section["units"].([]any)
	if len(units) != 1 || units[0].(float64) != 1 {
		t.Fatalf("units = %v, want [1]", units)
	}

	client := dialTestModbusTCP(t, server.modbus.TCPAddress())
	opening, err := client.readHolding(1, 0x0001, 1)
	if err != nil {
		t.Fatalf("read opening: %v", err)
	}
	requireValues(t, opening, []uint16{500})
	soil, err := client.readInput(1, 0x0000, 4)
	if err != nil {
		t.Fatalf("read soil: %v", err)
	}
	requireValues(t, soil, []uint16{185, 352, 780, 680})

	if _, err := client.exchange(2, testReadPDU(0x03, 0x0001, 1), 100*time.Millisecond); err == nil {
		t.Fatal("unit 2 (serial sensor, skipped) should not be served")
	}
}

func TestModbusConfigErrors(t *testing.T) {
	if _, err := sim.NewModbus(sim.ModbusOptions{
		TCPAddr:   "127.0.0.1:0",
		RTUAddr:   "127.0.0.1:0",
		ConfigDir: t.TempDir(),
	}); err == nil {
		t.Fatal("NewModbus accepted an empty config directory")
	}
}

func TestModbusConfigAccessConflict(t *testing.T) {
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(filepath.Join("..", "..", "configs", "example"))); err != nil {
		t.Fatalf("copy example config: %v", err)
	}
	profile := `profile: valve-conflict
device_type: valve
poll_interval_s: 5
points:
  - key: conflicting
    access: rw
    modbus: {func: 3, address: 1, type: uint16}
    write: {func: 6, address: 1}
services: []
events: []
`
	if err := os.WriteFile(filepath.Join(dir, "profiles", "valve-conflict.yaml"), []byte(profile), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	devicesPath := filepath.Join(dir, "devices.yaml")
	devices, err := os.ReadFile(devicesPath)
	if err != nil {
		t.Fatalf("read devices: %v", err)
	}
	devices = append(devices, []byte(`
  - sn: VALVE-CONFLICT
    device_type: valve
    profile: valve-conflict
    link: valves-net
    address: {unit_id: 1}
`)...)
	if err := os.WriteFile(devicesPath, devices, 0o644); err != nil {
		t.Fatalf("write devices: %v", err)
	}
	_, err = sim.NewModbus(sim.ModbusOptions{
		TCPAddr:   "127.0.0.1:0",
		RTUAddr:   "127.0.0.1:0",
		ConfigDir: dir,
	})
	if err == nil || !strings.Contains(err.Error(), "conflicting access") {
		t.Fatalf("NewModbus error = %v, want conflicting access", err)
	}
}

func writeTestModbusManifest(t *testing.T, server *testModbusServer) map[string]any {
	t.Helper()
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := server.runtime.WriteManifest(path); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("manifest JSON: %v", err)
	}
	return doc
}

func TestSimulateProcessModbus(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("process test requires linux")
	}
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	cmd := exec.Command(simulateTestBinary, "simulate", "--manifest", manifestPath)
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

	var stderr bytes.Buffer
	data := waitManifestProcess(t, cmd, manifestPath, 8*time.Second, &stderr)
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("manifest JSON: %v", err)
	}
	section, _ := doc["modbus"].(map[string]any)
	tcpSection, _ := section["tcp"].(map[string]any)
	rtuSection, _ := section["rtu_over_tcp"].(map[string]any)
	tcpURL, _ := tcpSection["url"].(string)
	rtuURL, _ := rtuSection["url"].(string)
	if tcpURL == "" || rtuURL == "" {
		t.Fatalf("modbus addresses missing: %v", section)
	}
	tcpAddr := strings.TrimPrefix(tcpURL, "tcp://")
	rtuAddr := strings.TrimPrefix(rtuURL, "tcp://")

	tcp := dialTestModbusTCP(t, tcpAddr)
	got, err := tcp.readHolding(1, 0x0001, 1)
	if err != nil {
		t.Fatalf("process tcp read: %v", err)
	}
	requireValues(t, got, []uint16{500})

	rtu := dialTestModbusRTU(t, rtuAddr)
	got, err = rtu.readInput(1, 0x0000, 4)
	if err != nil {
		t.Fatalf("process rtu read: %v", err)
	}
	requireValues(t, got, []uint16{185, 352, 780, 680})

	baseURL, _ := doc["http"].(map[string]any)["base_url"].(string)
	if baseURL == "" {
		t.Fatalf("http.base_url missing: %v", doc)
	}
	status, control := postJSON(t, baseURL+"/_sim/control",
		`{"module":"modbus","action":"unit_offline","unit_id":1,"duration_ms":50}`)
	if status != 200 || control["ok"] != true {
		t.Fatalf("control status=%d doc=%v", status, control)
	}
	status, requests := getJSON(t, baseURL+"/_sim/requests")
	if status != 200 || requests["ok"] != true {
		t.Fatalf("requests status=%d doc=%v", status, requests)
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("signal: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("wait: %v", err)
	}
	exited = true
	if code := cmd.ProcessState.ExitCode(); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	for _, addr := range []string{tcpAddr, rtuAddr} {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			t.Fatalf("modbus port %s still open after shutdown", addr)
		}
	}
}
