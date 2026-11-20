package sim

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestControlRegistryGrouping(t *testing.T) {
	reg := newControlRegistry()
	fn := func(ControlRequest) (any, error) { return nil, nil }
	if err := reg.add("", "a", fn); err != nil {
		t.Fatal(err)
	}
	if err := reg.add("mod", "b", fn); err != nil {
		t.Fatal(err)
	}
	if err := reg.add("", "a", fn); err == nil {
		t.Fatal("duplicate action accepted")
	}
	if err := reg.add("", "", fn); err == nil {
		t.Fatal("empty action accepted")
	}
	if err := reg.add("", "nil", nil); err == nil {
		t.Fatal("nil handler accepted")
	}
	if _, ok := reg.get("", "a"); !ok {
		t.Fatal("global action not found")
	}
	if _, ok := reg.get("mod", "b"); !ok {
		t.Fatal("module action not found")
	}
	got := reg.byModule()
	if len(got["global"]) != 1 || got["global"][0] != "a" {
		t.Fatalf("global = %v", got["global"])
	}
	if len(got["mod"]) != 1 || got["mod"][0] != "b" {
		t.Fatalf("mod = %v", got["mod"])
	}
}

func TestControlHTTP(t *testing.T) {
	r, err := NewRuntime(RuntimeOptions{HTTPAddr: "127.0.0.1:0", CaptureCapacity: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close(context.Background()) }()

	type echoBody struct {
		Action string `json:"action"`
		Module string `json:"module"`
		Value  string `json:"value"`
	}
	if err := r.RegisterControl("", "echo", func(req ControlRequest) (any, error) {
		var body echoBody
		if err := decodeStrict(req.Body, &body); err != nil {
			return nil, BadRequestf("%v", err)
		}
		return map[string]any{"ok": true, "value": body.Value}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := r.RegisterControl("", "bad", func(ControlRequest) (any, error) {
		return nil, BadRequestf("bad request")
	}); err != nil {
		t.Fatal(err)
	}
	if err := r.RegisterControl("", "boom", func(ControlRequest) (any, error) {
		return nil, errors.New("boom")
	}); err != nil {
		t.Fatal(err)
	}
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	client := &http.Client{Timeout: 2 * time.Second}

	cases := []struct {
		name   string
		body   string
		status int
		check  func(*testing.T, map[string]any)
	}{
		{
			name:   "ok",
			body:   `{"action":"echo","value":"x"}`,
			status: http.StatusOK,
			check: func(t *testing.T, doc map[string]any) {
				if doc["value"] != "x" {
					t.Errorf("value = %v", doc["value"])
				}
			},
		},
		{
			name:   "unknown action",
			body:   `{"action":"nope"}`,
			status: http.StatusBadRequest,
		},
		{
			name:   "missing action",
			body:   `{}`,
			status: http.StatusBadRequest,
		},
		{
			name:   "bad json",
			body:   `{`,
			status: http.StatusBadRequest,
		},
		{
			name:   "unknown field",
			body:   `{"action":"echo","value":"x","extra":1}`,
			status: http.StatusBadRequest,
		},
		{
			name:   "bad request error",
			body:   `{"action":"bad"}`,
			status: http.StatusBadRequest,
		},
		{
			name:   "internal error",
			body:   `{"action":"boom"}`,
			status: http.StatusInternalServerError,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := client.Post(r.BaseURL()+"/_sim/control", "application/json", strings.NewReader(tc.body))
			if err != nil {
				t.Fatalf("post: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != tc.status {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("status = %d, want %d, body=%s", resp.StatusCode, tc.status, body)
			}
			var doc map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if tc.status == http.StatusOK {
				if doc["ok"] != true {
					t.Errorf("ok = %v", doc["ok"])
				}
			} else if doc["ok"] != false {
				t.Errorf("ok = %v, want false", doc["ok"])
			}
			if tc.check != nil {
				tc.check(t, doc)
			}
		})
	}

	resp, err := client.Get(r.BaseURL() + "/_sim/control")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET status = %d, want 405", resp.StatusCode)
	}
}
