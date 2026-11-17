package logging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

func newTestLogger(t *testing.T, opts Options) (*slog.Logger, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	opts.Output = &buf
	if opts.Level == 0 {
		opts.Level = slog.LevelInfo
	}
	return New(opts), &buf
}

func decodeOne(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("decode log line %q: %v", buf.String(), err)
	}
	return m
}

func TestFieldsEmittedWithSeqZero(t *testing.T) {
	logger, buf := newTestLogger(t, Options{})
	zero := uint64(0)
	logger.Info("collected", Fields{
		Device: "VALVE-1", Point: "opening", Link: "valves", Reason: "poll", Seq: &zero,
	}.Attrs()...)

	m := decodeOne(t, buf)
	if m["device"] != "VALVE-1" || m["point"] != "opening" || m["link"] != "valves" || m["reason"] != "poll" {
		t.Fatalf("fields missing: %v", m)
	}
	if seq, ok := m["seq"].(float64); !ok || seq != 0 {
		t.Fatalf("seq = %v, want 0", m["seq"])
	}
	if _, ok := m["device_sn"]; ok {
		t.Fatal("unexpected alias device_sn")
	}
}

func TestKeyRedaction(t *testing.T) {
	logger, buf := newTestLogger(t, Options{
		RedactFields: []string{"password", "token", "authorization", "data"},
	})
	logger.Info("login",
		"username", "alice",
		"password", "p1",
		"mqtt_password", "p2",
		"token", "t1",
		"authorization", "Bearer x",
		"data", "payload",
	)
	m := decodeOne(t, buf)
	for _, key := range []string{"password", "mqtt_password", "token", "authorization", "data"} {
		if m[key] != redacted {
			t.Errorf("%s = %v, want %s", key, m[key], redacted)
		}
	}
	if m["username"] != "alice" {
		t.Errorf("username should be preserved, got %v", m["username"])
	}
}

func TestValueFallback(t *testing.T) {
	logger, buf := newTestLogger(t, Options{RedactValues: []string{"s3cr3t"}})
	logger.Info("m", "foo", "s3cr3t")
	m := decodeOne(t, buf)
	if m["foo"] != redacted {
		t.Fatalf("value fallback failed: %v", m["foo"])
	}

	buf.Reset()
	logger.Info("m", "err", fmt.Errorf("boom s3cr3t"))
	m = decodeOne(t, buf)
	if m["err"] != redacted {
		t.Fatalf("error value fallback failed: %v", m["err"])
	}

	buf.Reset()
	logger.Info("leaking s3cr3t in message")
	m = decodeOne(t, buf)
	if strings.Contains(fmt.Sprint(m["msg"]), "s3cr3t") {
		t.Fatalf("message leaked secret: %v", m["msg"])
	}
}

func TestURLUserinfoRedacted(t *testing.T) {
	logger, buf := newTestLogger(t, Options{})
	logger.Info("broker", "url", "mqtt://user:pass@host:1883")
	m := decodeOne(t, buf)
	got := fmt.Sprint(m["url"])
	if strings.Contains(got, "pass") {
		t.Fatalf("userinfo leaked: %v", got)
	}
	if !strings.Contains(got, "host:1883") {
		t.Fatalf("host lost: %v", got)
	}
}

func TestGroupAndWithAttrsRedaction(t *testing.T) {
	logger, buf := newTestLogger(t, Options{RedactFields: []string{"password"}})
	logger.With("password", "pre").WithGroup("g").Info("m", "password", "inner")
	m := decodeOne(t, buf)
	if m["password"] != redacted {
		t.Fatalf("WithAttrs not redacted: %v", m["password"])
	}
	g, ok := m["g"].(map[string]any)
	if !ok || g["password"] != redacted {
		t.Fatalf("group not redacted: %v", m["g"])
	}
}

func TestLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Options{Level: slog.LevelWarn, Output: &buf})
	logger.Info("hidden")
	if buf.Len() != 0 {
		t.Fatalf("info should be filtered, got %s", buf.String())
	}
	logger.Warn("shown")
	if !strings.Contains(buf.String(), "shown") {
		t.Fatalf("warn missing: %s", buf.String())
	}
}

func TestParseOptions(t *testing.T) {
	if _, err := ParseOptions("info", "json", nil); err != nil {
		t.Fatalf("valid options rejected: %v", err)
	}
	if _, err := ParseOptions("verbose", "json", nil); err == nil {
		t.Fatal("invalid level accepted")
	}
	if _, err := ParseOptions("info", "text", nil); err == nil {
		t.Fatal("invalid format accepted")
	}
}
