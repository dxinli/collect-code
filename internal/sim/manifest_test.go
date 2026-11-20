package sim

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWriteManifestAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	if err := writeManifestAtomic(path, map[string]any{"schema_version": 1, "hello": "world"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if doc["hello"] != "world" {
		t.Fatalf("doc = %v", doc)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "manifest.json" {
		t.Fatalf("unexpected files: %v", entries)
	}
}

func TestWriteManifestMissingDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "manifest.json")
	if err := writeManifestAtomic(path, map[string]any{"n": 1}); err == nil {
		t.Fatal("want error for missing directory")
	}
}

func TestManifestConcurrentReads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	if err := writeManifestAtomic(path, map[string]any{"n": 0}); err != nil {
		t.Fatalf("initial write: %v", err)
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for range 4 {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				data, err := os.ReadFile(path)
				if err != nil {
					t.Errorf("read: %v", err)
					return
				}
				if !json.Valid(data) {
					t.Errorf("reader saw partial JSON: %q", data)
					return
				}
			}
		})
	}

	for i := 1; i <= 200; i++ {
		if err := writeManifestAtomic(path, map[string]any{"n": i}); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	close(stop)
	wg.Wait()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "manifest.json" {
		t.Fatalf("unexpected files: %v", entries)
	}
}

func TestRuntimeManifest(t *testing.T) {
	fixed := time.Date(2026, 1, 2, 3, 4, 5, 123456789, time.UTC)
	r, err := NewRuntime(RuntimeOptions{
		HTTPAddr:        "127.0.0.1:0",
		CaptureCapacity: 7,
		Now:             func() time.Time { return fixed },
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	defer func() { _ = r.Close(context.Background()) }()

	if err := r.SetEndpoint("captured", "/_sim/captured"); err != nil {
		t.Fatal(err)
	}
	if err := r.SetSection("mqtt", map[string]any{"url": "mqtt://127.0.0.1:1"}); err != nil {
		t.Fatal(err)
	}
	if err := r.RegisterControl("", "broker_disconnect", func(ControlRequest) (any, error) { return nil, nil }); err != nil {
		t.Fatal(err)
	}
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}

	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := r.WriteManifest(path); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if got := doc["schema_version"]; got != float64(manifestSchemaVersion) {
		t.Errorf("schema_version = %v", got)
	}
	if got := doc["generated_at"]; got != fixed.Format(time.RFC3339Nano) {
		t.Errorf("generated_at = %v", got)
	}
	httpSection, ok := doc["http"].(map[string]any)
	if !ok {
		t.Fatalf("http section = %T", doc["http"])
	}
	baseURL, _ := httpSection["base_url"].(string)
	if !strings.HasPrefix(baseURL, "http://127.0.0.1:") {
		t.Errorf("base_url = %q", baseURL)
	}
	if port, _ := httpSection["port"].(float64); port <= 0 {
		t.Errorf("http.port = %v", httpSection["port"])
	}
	endpoints, _ := doc["endpoints"].(map[string]any)
	if endpoints["captured"] != "/_sim/captured" {
		t.Errorf("endpoints = %v", endpoints)
	}
	controls, _ := doc["control_actions"].(map[string]any)
	global, _ := controls["global"].([]any)
	if len(global) != 1 || global[0] != "broker_disconnect" {
		t.Errorf("control_actions = %v", controls)
	}
	modules, _ := doc["modules"].([]any)
	if len(modules) != 1 || modules[0] != "mqtt" {
		t.Errorf("modules = %v", modules)
	}
	mqttSection, _ := doc["mqtt"].(map[string]any)
	if mqttSection["url"] != "mqtt://127.0.0.1:1" {
		t.Errorf("mqtt section = %v", mqttSection)
	}
	if got, _ := doc["pid"].(float64); got <= 0 {
		t.Errorf("pid = %v", doc["pid"])
	}
}

func TestRuntimeManifestRequiresStart(t *testing.T) {
	r, err := NewRuntime(RuntimeOptions{HTTPAddr: "127.0.0.1:0", CaptureCapacity: 1})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := r.WriteManifest(path); err == nil {
		t.Fatal("want error before Start")
	}
}

func TestRuntimeManifestEmptyPathIsNoop(t *testing.T) {
	r, err := NewRuntime(RuntimeOptions{HTTPAddr: "127.0.0.1:0", CaptureCapacity: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.WriteManifest(""); err != nil {
		t.Fatalf("empty path: %v", err)
	}
}
