package sim

import (
	"encoding/json"
	"sync"
	"time"
)

// CapturedMessage is one uplink message observed by the cloud stub. Payload
// is the original byte sequence represented as a JSON value when it is valid
// JSON, or as a JSON string otherwise.
type CapturedMessage struct {
	Seq        uint64          `json:"seq"`
	Topic      string          `json:"topic"`
	Payload    json.RawMessage `json:"payload"`
	QoS        byte            `json:"qos"`
	Retain     bool            `json:"retain"`
	ReceivedAt time.Time       `json:"received_at"`
}

// CaptureBuffer is a bounded, concurrency-safe ring of captured messages.
// When full it drops the oldest message and increments dropped; it never
// blocks the broker.
type CaptureBuffer struct {
	mu       sync.Mutex
	capacity int
	now      func() time.Time
	items    []CapturedMessage
	start    int
	count    int
	seq      uint64
	dropped  uint64
}

// NewCaptureBuffer creates a ring with the given capacity. Capacity must be
// positive; a nil now defaults to time.Now.
func NewCaptureBuffer(capacity int, now func() time.Time) *CaptureBuffer {
	if capacity <= 0 {
		panic("sim: capture capacity must be positive")
	}
	if now == nil {
		now = time.Now
	}
	return &CaptureBuffer{
		capacity: capacity,
		now:      now,
		items:    make([]CapturedMessage, capacity),
	}
}

// Capacity returns the fixed ring capacity.
func (c *CaptureBuffer) Capacity() int { return c.capacity }

// Add records one message. The payload is copied; callers may reuse the
// input slice immediately after Add returns.
func (c *CaptureBuffer) Add(topic string, payload []byte, qos byte, retain bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.seq++
	msg := CapturedMessage{
		Seq:        c.seq,
		Topic:      topic,
		Payload:    encodePayload(payload),
		QoS:        qos,
		Retain:     retain,
		ReceivedAt: c.now().UTC(),
	}

	if c.count < c.capacity {
		c.items[(c.start+c.count)%c.capacity] = msg
		c.count++
		return
	}

	c.items[c.start] = msg
	c.start = (c.start + 1) % c.capacity
	c.dropped++
}

// Snapshot returns the retained messages in arrival order and the number of
// messages dropped because the ring was full.
func (c *CaptureBuffer) Snapshot() ([]CapturedMessage, uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	out := make([]CapturedMessage, 0, c.count)
	for i := 0; i < c.count; i++ {
		msg := c.items[(c.start+i)%c.capacity]
		msg.Payload = append(json.RawMessage(nil), msg.Payload...)
		out = append(out, msg)
	}
	return out, c.dropped
}

func encodePayload(payload []byte) json.RawMessage {
	if json.Valid(payload) {
		return append(json.RawMessage(nil), payload...)
	}
	// json.Marshal replaces invalid UTF-8 with U+FFFD; this representation is
	// for diagnosis only and is not part of the uplink contract.
	encoded, err := json.Marshal(string(payload))
	if err != nil {
		return json.RawMessage(`""`)
	}
	return encoded
}
