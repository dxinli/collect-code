package sim

import (
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

const (
	modbusRequestLogCapacity = 10000
	modbusRawHexLimit        = 512
)

const (
	modbusOutcomeOK         = "ok"
	modbusOutcomeException  = "exception"
	modbusOutcomeNoResponse = "no_response"
	modbusOutcomeBadFrame   = "bad_frame"
	modbusOutcomeBadCRC     = "bad_crc"
)

// modbusRequestEntry is one line of GET /_sim/requests. Unknown fields are
// null; values is only present for writes.
type modbusRequestEntry struct {
	Seq         uint64     `json:"seq"`
	Transport   string     `json:"transport"`
	Remote      string     `json:"remote"`
	UnitID      *uint8     `json:"unit_id"`
	Function    *uint8     `json:"function"`
	Address     *uint16    `json:"address"`
	Quantity    *uint16    `json:"quantity"`
	Values      []uint16   `json:"values,omitempty"`
	Outcome     string     `json:"outcome"`
	Exception   *uint8     `json:"exception"`
	RawHex      string     `json:"raw_hex,omitempty"`
	ReceivedAt  time.Time  `json:"received_at"`
	RespondedAt *time.Time `json:"responded_at"`
	DurationMS  int64      `json:"duration_ms"`
}

type modbusRequestLog struct {
	mu       sync.Mutex
	capacity int
	items    []modbusRequestEntry
	start    int
	count    int
	seq      uint64
	dropped  uint64
}

func newModbusRequestLog(capacity int) *modbusRequestLog {
	if capacity <= 0 {
		capacity = modbusRequestLogCapacity
	}
	return &modbusRequestLog{
		capacity: capacity,
		items:    make([]modbusRequestEntry, capacity),
	}
}

func (l *modbusRequestLog) add(entry modbusRequestEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.seq++
	entry.Seq = l.seq
	if l.count < l.capacity {
		l.items[(l.start+l.count)%l.capacity] = entry
		l.count++
		return
	}
	l.items[l.start] = entry
	l.start = (l.start + 1) % l.capacity
	l.dropped++
}

func (l *modbusRequestLog) snapshot() ([]modbusRequestEntry, uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()

	out := make([]modbusRequestEntry, 0, l.count)
	for i := 0; i < l.count; i++ {
		entry := l.items[(l.start+i)%l.capacity]
		if entry.Values != nil {
			entry.Values = append([]uint16(nil), entry.Values...)
		}
		out = append(out, entry)
	}
	return out, l.dropped
}

func (m *Modbus) handleRequests(w http.ResponseWriter, _ *http.Request) {
	entries, dropped := m.requests.snapshot()
	if entries == nil {
		entries = []modbusRequestEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"requests": entries,
		"dropped":  dropped,
	})
}

func modbusRawHex(raw []byte) string {
	if len(raw) > modbusRawHexLimit {
		raw = raw[:modbusRawHexLimit]
	}
	return hex.EncodeToString(raw)
}

func modbusUint8Ptr(v uint8) *uint8    { return &v }
func modbusUint16Ptr(v uint16) *uint16 { return &v }
