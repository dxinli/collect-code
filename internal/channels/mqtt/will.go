package mqtt

import (
	"bytes"
	"encoding/json"
)

// willPayload builds the node-level last-will payload. The byte shape is
// deliberately stable: {"online":false,"node_id":"..."} with no trailing
// newline and no HTML escaping.
func willPayload(nodeID string) []byte {
	doc := struct {
		Online bool   `json:"online"`
		NodeID string `json:"node_id"`
	}{Online: false, NodeID: nodeID}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(doc); err != nil {
		// Encoding a struct of bool/string cannot fail; keep the invariant
		// explicit rather than returning an error that callers must handle.
		panic("mqtt: encode will payload: " + err.Error())
	}
	payload := bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
	return append([]byte(nil), payload...)
}
