package sim

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestCaptureRingCapacityAndDropped(t *testing.T) {
	fixed := time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC)
	c := NewCaptureBuffer(2, func() time.Time { return fixed })
	for i := 1; i <= 5; i++ {
		c.Add(fmt.Sprintf("t/%d", i), fmt.Appendf(nil, `{"n":%d}`, i), 1, false)
	}

	messages, dropped := c.Snapshot()
	if dropped != 3 {
		t.Fatalf("dropped = %d, want 3", dropped)
	}
	if len(messages) != 2 {
		t.Fatalf("len(messages) = %d, want 2", len(messages))
	}
	if messages[0].Seq != 4 || messages[1].Seq != 5 {
		t.Fatalf("seqs = %d,%d, want 4,5", messages[0].Seq, messages[1].Seq)
	}
	if messages[0].Topic != "t/4" || messages[1].Topic != "t/5" {
		t.Fatalf("topics = %q,%q", messages[0].Topic, messages[1].Topic)
	}
	if !messages[0].ReceivedAt.Equal(fixed) {
		t.Fatalf("received_at = %s, want %s", messages[0].ReceivedAt, fixed)
	}
}

func TestCapturePayloadRepresentation(t *testing.T) {
	c := NewCaptureBuffer(4, nil)
	c.Add("valid", []byte(`{"a":1}`), 1, false)
	c.Add("text", []byte(`not-json`), 0, true)
	c.Add("empty", nil, 0, false)

	messages, _ := c.Snapshot()
	if got := string(messages[0].Payload); got != `{"a":1}` {
		t.Errorf("valid payload = %s", got)
	}
	if got := string(messages[1].Payload); got != `"not-json"` {
		t.Errorf("text payload = %s", got)
	}
	if got := string(messages[2].Payload); got != `""` {
		t.Errorf("empty payload = %s", got)
	}
	if messages[1].Retain != true || messages[1].QoS != 0 {
		t.Errorf("qos/retain = %d/%v", messages[1].QoS, messages[1].Retain)
	}
}

func TestCaptureCopiesPayload(t *testing.T) {
	c := NewCaptureBuffer(1, nil)
	payload := []byte(`{"a":1}`)
	c.Add("t", payload, 0, false)
	copy(payload, []byte(`{"a":2}`))

	messages, _ := c.Snapshot()
	if got := string(messages[0].Payload); got != `{"a":1}` {
		t.Fatalf("payload changed after Add: %s", got)
	}
	// The snapshot itself must not alias the buffer's payload.
	messages[0].Payload[0] = '['
	again, _ := c.Snapshot()
	if got := string(again[0].Payload); got != `{"a":1}` {
		t.Fatalf("snapshot aliases buffer payload: %s", got)
	}
}

func TestCaptureSnapshotEmpty(t *testing.T) {
	c := NewCaptureBuffer(2, nil)
	messages, dropped := c.Snapshot()
	if messages == nil {
		t.Fatal("messages = nil, want empty slice")
	}
	if len(messages) != 0 || dropped != 0 {
		t.Fatalf("len=%d dropped=%d", len(messages), dropped)
	}
}

func TestCaptureConcurrent(t *testing.T) {
	const (
		writers   = 8
		perWriter = 200
		capacity  = 64
		total     = writers * perWriter
	)
	c := NewCaptureBuffer(capacity, nil)

	var wg sync.WaitGroup
	stop := make(chan struct{})
	var readerWG sync.WaitGroup
	readerWG.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
				c.Snapshot()
			}
		}
	})

	for w := range writers {
		wg.Go(func() {
			for range perWriter {
				c.Add(fmt.Sprintf("w/%d", w), []byte(`{"ok":true}`), 1, false)
			}
		})
	}
	wg.Wait()
	close(stop)
	readerWG.Wait()

	messages, dropped := c.Snapshot()
	if len(messages) != capacity {
		t.Fatalf("len(messages) = %d, want %d", len(messages), capacity)
	}
	if dropped != total-capacity {
		t.Fatalf("dropped = %d, want %d", dropped, total-capacity)
	}
	if messages[len(messages)-1].Seq != total {
		t.Fatalf("last seq = %d, want %d", messages[len(messages)-1].Seq, total)
	}
	for i := 1; i < len(messages); i++ {
		if messages[i].Seq != messages[i-1].Seq+1 {
			t.Fatalf("seq gap at %d: %d then %d", i, messages[i-1].Seq, messages[i].Seq)
		}
	}
}
