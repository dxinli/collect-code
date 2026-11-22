package sim

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

const (
	maxRequestBody = 1 << 20 // 1 MiB

	pathCaptured = "/_sim/captured"
	pathInvoke   = "/_sim/invoke"
	pathControl  = "/_sim/control"
)

type controlEnvelope struct {
	Module string `json:"module"`
	Action string `json:"action"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"ok":false,"error":"encode response"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func writeAPIError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"ok": false, "error": message})
}

func readJSONBody(w http.ResponseWriter, r *http.Request) (json.RawMessage, error) {
	if r.Body == nil {
		return nil, errors.New("missing request body")
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBody))
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, errors.New("empty request body")
	}
	return json.RawMessage(body), nil
}

func decodeStrict(body []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		return errors.New("unexpected trailing data")
	}
	return nil
}

func (r *Runtime) handleCaptured(w http.ResponseWriter, _ *http.Request) {
	messages, dropped := r.capture.Snapshot()
	if messages == nil {
		messages = []CapturedMessage{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"messages": messages,
		"dropped":  dropped,
	})
}

func (r *Runtime) handleControl(w http.ResponseWriter, req *http.Request) {
	body, err := readJSONBody(w, req)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}

	var env controlEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid control request: "+err.Error())
		return
	}
	if env.Action == "" {
		writeAPIError(w, http.StatusBadRequest, "action is required")
		return
	}

	fn, ok := r.controls.get(env.Module, env.Action)
	if !ok {
		writeAPIError(w, http.StatusBadRequest, fmt.Sprintf("unknown control action %q", env.Action))
		return
	}

	result, err := fn(ControlRequest{Module: env.Module, Action: env.Action, Body: body})
	if err != nil {
		if bad, ok := errors.AsType[*BadRequestError](err); ok {
			writeAPIError(w, http.StatusBadRequest, bad.Error())
			return
		}
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
