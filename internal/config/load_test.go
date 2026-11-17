package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validTree() map[string]string {
	return map[string]string{
		"config.yaml": "node:\n  id: node-1\n  product_key: irrigation\n" +
			"northbound:\n  mqtt:\n    broker: mqtt://localhost:1883\n",
		"links.yaml": "links:\n  - id: sensors\n    driver: modbus-rtu\n" +
			"    serial:\n      device: /dev/ttyS3\n",
		"devices.yaml": "devices:\n  - sn: S1\n    device_type: sensor\n" +
			"    profile: sensor-soil\n    link: sensors\n    address:\n      unit_id: 1\n",
		"profiles/sensor-soil.yaml": "profile: sensor-soil\ndevice_type: sensor\n" +
			"points:\n  - key: moisture\n",
	}
}

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

func loadTree(t *testing.T, files map[string]string, opts ...Option) (*Bundle, error) {
	t.Helper()
	return Load(writeTree(t, files), opts...)
}

func TestLoadValidAndDefaults(t *testing.T) {
	b, err := loadTree(t, validTree())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := b.Config.Timing.SnapshotIntervalS; got != 60 {
		t.Errorf("snapshot interval = %d, want 60", got)
	}
	if got := b.Config.Northbound.MQTT.QoS; got != 1 {
		t.Errorf("qos = %d, want 1", got)
	}
	if got := b.Config.Queue.PendingMax; got != 4 {
		t.Errorf("pending_max = %d, want 4", got)
	}
	if got := len(b.Config.Log.RedactFields); got != 4 {
		t.Errorf("redact fields = %d, want 4", got)
	}
	if b.Config.Adapter.Topics.PropertyPost == "" {
		t.Error("default property topic missing")
	}
	if len(b.Links) != 1 || len(b.Devices) != 1 || len(b.Profiles) != 1 {
		t.Errorf("counts = %d/%d/%d, want 1/1/1", len(b.Links), len(b.Devices), len(b.Profiles))
	}
	if want := "config ok: node=node-1 product_key=irrigation links=1 devices=1 profiles=1"; b.Summary() != want {
		t.Errorf("summary = %q, want %q", b.Summary(), want)
	}
}

func TestUnknownKeys(t *testing.T) {
	cases := map[string]string{
		"config.yaml":               "node:\n  id: n\n  product_key: p\nnodex: 1\n",
		"links.yaml":                "links: []\nlinkx: 1\n",
		"devices.yaml":              "devices: []\ndevicex: 1\n",
		"profiles/sensor-soil.yaml": "profile: sensor-soil\nunknown: 1\n",
	}
	for file, content := range cases {
		t.Run(file, func(t *testing.T) {
			files := validTree()
			files[file] = content
			_, err := loadTree(t, files)
			if err == nil {
				t.Fatalf("expected error for %s", file)
			}
			msg := err.Error()
			if !strings.Contains(msg, filepath.Base(file)) {
				t.Errorf("error does not name file: %v", err)
			}
			if !strings.Contains(msg, "not found") {
				t.Errorf("error does not report unknown field: %v", err)
			}
		})
	}
}

func TestTopicsAreNotOpenMap(t *testing.T) {
	files := validTree()
	files["config.yaml"] = "node:\n  id: n\n  product_key: p\n" +
		"northbound:\n  mqtt:\n    broker: mqtt://x\n" +
		"adapter:\n  topics:\n    property_posts: v1/x\n"
	_, err := loadTree(t, files)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("misspelled topic should fail, got %v", err)
	}
}

func TestEmptyDocuments(t *testing.T) {
	for _, content := range []string{"", "# only a comment\n", "---\n", "null\n"} {
		t.Run("links:"+strings.ReplaceAll(content, "\n", "_"), func(t *testing.T) {
			files := validTree()
			files["links.yaml"] = content
			_, err := loadTree(t, files)
			if err == nil || !strings.Contains(err.Error(), "empty document") {
				t.Fatalf("want empty document, got %v", err)
			}
		})
	}
}

func TestEmptyContainersAreValid(t *testing.T) {
	for _, content := range []string{"{}\n", "links: []\n"} {
		files := validTree()
		files["links.yaml"] = content
		if _, err := loadTree(t, files); err != nil {
			t.Fatalf("empty container %q should load, got %v", content, err)
		}
	}
}

func TestMultiDocumentRejected(t *testing.T) {
	files := validTree()
	files["links.yaml"] = "links: []\n---\nlinks: []\n"
	_, err := loadTree(t, files)
	if err == nil || !strings.Contains(err.Error(), "multiple") {
		t.Fatalf("want multiple document error, got %v", err)
	}
}

func TestDuplicateKeyRejected(t *testing.T) {
	files := validTree()
	files["devices.yaml"] = "devices:\n  - sn: A\n    sn: B\n"
	_, err := loadTree(t, files)
	if err == nil || !strings.Contains(err.Error(), "already defined") {
		t.Fatalf("want duplicate key error, got %v", err)
	}
}

func TestNullBlocksKeepDefaults(t *testing.T) {
	files := validTree()
	files["config.yaml"] = "node:\n  id: n\n  product_key: p\n" +
		"northbound:\n  mqtt:\n    broker: mqtt://x\n    password: null\n" +
		"log: null\ntiming: null\n"
	b, err := loadTree(t, files)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if b.Config.Log.Level != "info" || b.Config.Log.Format != "json" {
		t.Errorf("log defaults lost: %+v", b.Config.Log)
	}
	if b.Config.Timing.SnapshotIntervalS != 60 {
		t.Errorf("timing defaults lost: %d", b.Config.Timing.SnapshotIntervalS)
	}
	if b.Config.Northbound.MQTT.Password.IsSet() {
		t.Error("null password should be unset")
	}
}

func TestRequiredFields(t *testing.T) {
	files := validTree()
	files["config.yaml"] = "node:\n  product_key: p\nnorthbound:\n  mqtt:\n    broker: mqtt://x\n"
	_, err := loadTree(t, files)
	if err == nil || !strings.Contains(err.Error(), "config.yaml: node.id: required") {
		t.Fatalf("want node.id required, got %v", err)
	}

	files = validTree()
	files["devices.yaml"] = "devices:\n  - sn: S1\n    device_type: sensor\n" +
		"    profile: sensor-soil\n    link: sensors\n"
	_, err = loadTree(t, files)
	if err == nil || !strings.Contains(err.Error(), "devices[0].address: required") {
		t.Fatalf("want address required, got %v", err)
	}
}

func TestUnitIDZeroIsAValidAddress(t *testing.T) {
	files := validTree()
	files["devices.yaml"] = "devices:\n  - sn: S1\n    device_type: sensor\n" +
		"    profile: sensor-soil\n    link: sensors\n    address:\n      unit_id: 0\n"
	if _, err := loadTree(t, files); err != nil {
		t.Fatalf("unit_id 0 should be valid: %v", err)
	}
}

func TestDuplicateProfileRejected(t *testing.T) {
	files := validTree()
	files["profiles/another.yaml"] = "profile: sensor-soil\ndevice_type: sensor\n"
	_, err := loadTree(t, files)
	if err == nil || !strings.Contains(err.Error(), "duplicate profile") {
		t.Fatalf("want duplicate profile error, got %v", err)
	}
}

func TestYMLProfileIsLoaded(t *testing.T) {
	files := validTree()
	delete(files, "profiles/sensor-soil.yaml")
	files["profiles/sensor-soil.yml"] = "profile: sensor-soil\ndevice_type: sensor\n"
	b, err := loadTree(t, files)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, ok := b.Profiles["sensor-soil"]; !ok {
		t.Error("yml profile not loaded")
	}
}

func TestSecretEnvResolution(t *testing.T) {
	files := validTree()
	files["config.yaml"] = "node:\n  id: n\n  product_key: p\n" +
		"northbound:\n  mqtt:\n    broker: mqtt://x\n    password: env:MQTT_PASS\n"

	if _, err := loadTree(t, files); err == nil || !strings.Contains(err.Error(), "MQTT_PASS") {
		t.Fatalf("want missing env error naming the variable, got %v", err)
	}

	b, err := loadTree(t, files, WithEnv(func(name string) (string, bool) {
		if name == "MQTT_PASS" {
			return "s3cr3t-value", true
		}
		return "", false
	}))
	if err != nil {
		t.Fatalf("load with env: %v", err)
	}
	if got := b.Config.Northbound.MQTT.Password.Reveal(); got != "s3cr3t-value" {
		t.Errorf("revealed = %q", got)
	}
	if got := b.Config.Northbound.MQTT.Password.Masked(); got != "[set via env MQTT_PASS]" {
		t.Errorf("masked = %q", got)
	}
	values := b.SecretValues()
	if len(values) != 1 || values[0] != "s3cr3t-value" {
		t.Errorf("secret values = %v", values)
	}
}

func TestSecretInvalidEnvNameIsNotEchoed(t *testing.T) {
	files := validTree()
	files["config.yaml"] = "node:\n  id: n\n  product_key: p\n" +
		"northbound:\n  mqtt:\n    broker: mqtt://x\n    password: env:not a name\n"
	_, err := loadTree(t, files)
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "not a name") {
		t.Fatalf("invalid env name leaked into error: %v", err)
	}
}

func TestSecretMappingRejectedWithoutValue(t *testing.T) {
	files := validTree()
	files["config.yaml"] = "node:\n  id: n\n  product_key: p\n" +
		"northbound:\n  mqtt:\n    broker: mqtt://x\n    password: {a: b}\n"
	_, err := loadTree(t, files)
	if err == nil || !strings.Contains(err.Error(), "secret must be a scalar") {
		t.Fatalf("want scalar error, got %v", err)
	}
	if strings.Contains(err.Error(), "a: b") {
		t.Fatalf("mapping content leaked: %v", err)
	}
}

func TestChirpstackTokenEnv(t *testing.T) {
	files := validTree()
	files["links.yaml"] = "links:\n  - id: lora-gw\n    driver: lorawan\n" +
		"    chirpstack:\n      grpc_addr: 127.0.0.1:8080\n" +
		"      api_token_env: CHIRPSTACK_TOKEN\n"
	if _, err := loadTree(t, files); err == nil || !strings.Contains(err.Error(), "link lora-gw") {
		t.Fatalf("want link-scoped token error, got %v", err)
	}

	b, err := loadTree(t, files, WithEnv(func(name string) (string, bool) {
		if name == "CHIRPSTACK_TOKEN" {
			return "token-value", true
		}
		return "", false
	}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := b.Links[0].Chirpstack.TokenMasked(); got != "[set via env CHIRPSTACK_TOKEN]" {
		t.Errorf("masked = %q", got)
	}
}
