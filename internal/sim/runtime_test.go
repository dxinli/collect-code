package sim

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRuntimeStartServeClose(t *testing.T) {
	r, err := NewRuntime(RuntimeOptions{HTTPAddr: "127.0.0.1:0", CaptureCapacity: 4})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	r.Handle("GET /hello", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "hello")
	})
	if r.BaseURL() != "" {
		t.Fatalf("BaseURL before Start = %q", r.BaseURL())
	}
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}

	addr := r.HTTPAddress()
	if !strings.HasPrefix(addr, "127.0.0.1:") {
		t.Fatalf("HTTPAddress = %q", addr)
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(r.BaseURL() + "/hello")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || string(body) != "hello" {
		t.Fatalf("status=%d body=%q", resp.StatusCode, body)
	}

	if err := r.Close(context.Background()); err != nil {
		t.Fatalf("close: %v", err)
	}
	conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		t.Fatal("port is still open after Close")
	}
}

func TestRuntimeCloseIdempotent(t *testing.T) {
	r, err := NewRuntime(RuntimeOptions{HTTPAddr: "127.0.0.1:0", CaptureCapacity: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(context.Background()); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

func TestRuntimeStartTwice(t *testing.T) {
	r, err := NewRuntime(RuntimeOptions{HTTPAddr: "127.0.0.1:0", CaptureCapacity: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close(context.Background()) }()
	if err := r.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := r.Start(context.Background()); err == nil {
		t.Fatal("second Start accepted")
	}
}

func TestRuntimeHandleAfterStartPanics(t *testing.T) {
	r, err := NewRuntime(RuntimeOptions{HTTPAddr: "127.0.0.1:0", CaptureCapacity: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close(context.Background()) }()
	if err := r.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("Handle after Start did not panic")
		}
	}()
	r.Handle("GET /late", func(http.ResponseWriter, *http.Request) {})
}

func TestRuntimeSetEndpointValidation(t *testing.T) {
	r, err := NewRuntime(RuntimeOptions{HTTPAddr: "127.0.0.1:0", CaptureCapacity: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.SetEndpoint("", "/x"); err == nil {
		t.Fatal("empty name accepted")
	}
	if err := r.SetEndpoint("x", ""); err == nil {
		t.Fatal("empty path accepted")
	}
	if err := r.SetEndpoint("x", "nope"); err == nil {
		t.Fatal("relative path accepted")
	}
	if err := r.SetEndpoint("x", "/_sim/x"); err != nil {
		t.Fatal(err)
	}
	if err := r.SetEndpoint("x", "/_sim/y"); err == nil {
		t.Fatal("duplicate endpoint accepted")
	}
}

func TestRuntimeSetSectionValidation(t *testing.T) {
	r, err := NewRuntime(RuntimeOptions{HTTPAddr: "127.0.0.1:0", CaptureCapacity: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.SetSection("", map[string]any{}); err == nil {
		t.Fatal("empty section accepted")
	}
	if err := r.SetSection("http", map[string]any{}); err == nil {
		t.Fatal("reserved section accepted")
	}
	if err := r.SetSection("mqtt", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if err := r.SetSection("mqtt", map[string]any{}); err == nil {
		t.Fatal("duplicate section accepted")
	}
}

func TestRuntimeRegisterControlValidation(t *testing.T) {
	r, err := NewRuntime(RuntimeOptions{HTTPAddr: "127.0.0.1:0", CaptureCapacity: 1})
	if err != nil {
		t.Fatal(err)
	}
	fn := func(ControlRequest) (any, error) { return nil, nil }
	if err := r.RegisterControl("", "", fn); err == nil {
		t.Fatal("empty action accepted")
	}
	if err := r.RegisterControl("", "a", nil); err == nil {
		t.Fatal("nil handler accepted")
	}
	if err := r.RegisterControl("", "a", fn); err != nil {
		t.Fatal(err)
	}
	if err := r.RegisterControl("", "a", fn); err == nil {
		t.Fatal("duplicate action accepted")
	}
}

func TestRuntimeNewValidation(t *testing.T) {
	if _, err := NewRuntime(RuntimeOptions{HTTPAddr: "127.0.0.1:0", CaptureCapacity: -1}); err == nil {
		t.Fatal("negative capacity accepted")
	}
	if _, err := NewRuntime(RuntimeOptions{HTTPAddr: "127.0.0.1:0", CaptureCapacity: maxCaptureCapacity + 1}); err == nil {
		t.Fatal("oversized capacity accepted")
	}
}

func TestRuntimeCapturedEmpty(t *testing.T) {
	r, err := NewRuntime(RuntimeOptions{HTTPAddr: "127.0.0.1:0", CaptureCapacity: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close(context.Background()) }()
	r.Handle("GET /_sim/captured", r.handleCaptured)
	if err := r.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	resp, err := (&http.Client{Timeout: 2 * time.Second}).Get(r.BaseURL() + "/_sim/captured")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var doc struct {
		OK       bool              `json:"ok"`
		Messages []json.RawMessage `json:"messages"`
		Dropped  uint64            `json:"dropped"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if !doc.OK || doc.Messages == nil || len(doc.Messages) != 0 || doc.Dropped != 0 {
		t.Fatalf("doc = %+v", doc)
	}
}

func TestRuntimeNotFound(t *testing.T) {
	r, err := NewRuntime(RuntimeOptions{HTTPAddr: "127.0.0.1:0", CaptureCapacity: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close(context.Background()) }()
	if err := r.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	resp, err := (&http.Client{Timeout: 2 * time.Second}).Get(r.BaseURL() + "/nope")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}
