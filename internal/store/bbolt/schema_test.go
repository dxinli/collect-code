package bbolt_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dxinli/collect-code/internal/interfaces"
	storebbolt "github.com/dxinli/collect-code/internal/store/bbolt"
	bolt "go.etcd.io/bbolt"
)

func seqKey(seq uint64) []byte {
	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key, seq)
	return key
}

func openStoreAt(t *testing.T, path string) *storebbolt.Store {
	t.Helper()
	st, err := storebbolt.Open(path)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	return st
}

func closeStore(t *testing.T, st *storebbolt.Store) {
	t.Helper()
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestBboltOnDiskSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	st := openStoreAt(t, path)
	ctx := context.Background()
	at := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)

	if err := st.BeginAccepted(ctx, interfaces.AcceptedRecord{Device: "dev-a", Seq: 5, ServiceID: "svc-open", At: at}); err != nil {
		t.Fatalf("BeginAccepted: %v", err)
	}
	if err := st.Finish(ctx, interfaces.FinalRecord{Device: "dev-a", Seq: 5, Code: 200, Msg: "ok", Data: map[string]any{"target": 45}, At: at}); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if err := st.SaveDevices(ctx, []interfaces.DeviceState{{Device: "dev-a", Online: true, LastSeq: 5, UpdatedAt: at}}); err != nil {
		t.Fatalf("SaveDevices: %v", err)
	}
	if err := st.SaveSnapshot(ctx, "dev-a", []interfaces.Property{{Key: "p", Value: 1.0, Unit: "u", Quality: interfaces.QualityGood, TS: at}}); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}
	if err := st.SaveSnapshot(ctx, "dev-empty", nil); err != nil {
		t.Fatalf("SaveSnapshot(nil): %v", err)
	}
	closeStore(t, st)

	db := rawOpen(t, path)
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("close raw db: %v", err)
		}
	}()

	if err := db.View(func(tx *bolt.Tx) error {
		for _, name := range []string{"meta", "records", "last_seq", "devices", "snapshots"} {
			if tx.Bucket([]byte(name)) == nil {
				t.Errorf("top-level bucket %q is missing", name)
			}
		}

		meta := tx.Bucket([]byte("meta"))
		if meta == nil {
			return nil
		}
		version := meta.Get([]byte("schema_version"))
		if len(version) != 8 || binary.BigEndian.Uint64(version) != 1 {
			t.Errorf("schema_version = %v, want 8-byte BE 1", version)
		}

		records := tx.Bucket([]byte("records"))
		if records == nil {
			return nil
		}
		rb := records.Bucket([]byte("dev-a"))
		if rb == nil {
			t.Errorf("records/dev-a bucket is missing")
		} else {
			raw := rb.Get(seqKey(5))
			if raw == nil {
				t.Errorf("records/dev-a/%d is missing", 5)
			}
			text := string(raw)
			for _, want := range []string{`"phase":"final"`, `"service_id":"svc-open"`, `"accepted_at"`, `"final_at"`, `"data"`} {
				if !strings.Contains(text, want) {
					t.Errorf("record JSON missing %s: %s", want, text)
				}
			}
			if rb.Get([]byte("5")) != nil {
				t.Errorf("record key is a decimal string, want 8-byte BE seq")
			}
		}

		watermarks := tx.Bucket([]byte("last_seq"))
		if watermarks == nil {
			return nil
		}
		watermark := watermarks.Get([]byte("dev-a"))
		if len(watermark) != 8 || binary.BigEndian.Uint64(watermark) != 5 {
			t.Errorf("last_seq/dev-a = %v, want 8-byte BE 5", watermark)
		}

		devices := tx.Bucket([]byte("devices"))
		if devices == nil {
			return nil
		}
		deviceJSON := string(devices.Get([]byte("dev-a")))
		for _, want := range []string{`"last_success_at"`, `"last_seq"`, `"updated_at"`, `"online"`} {
			if !strings.Contains(deviceJSON, want) {
				t.Errorf("device JSON missing %s: %s", want, deviceJSON)
			}
		}

		snapshots := tx.Bucket([]byte("snapshots"))
		if snapshots == nil {
			return nil
		}
		snapshot := snapshots.Get([]byte("dev-a"))
		if len(snapshot) == 0 || snapshot[0] != '[' {
			t.Errorf("snapshot value = %s, want a JSON array", snapshot)
		}
		if empty := snapshots.Get([]byte("dev-empty")); !bytes.Equal(empty, []byte("[]")) {
			t.Errorf("nil snapshot value = %s, want []", empty)
		}
		return nil
	}); err != nil {
		t.Fatalf("view raw db: %v", err)
	}
}

func TestBboltRejectsCorruptLastSeq(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	st := openStoreAt(t, path)
	ctx := context.Background()
	at := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	if err := st.BeginAccepted(ctx, interfaces.AcceptedRecord{Device: "dev-a", Seq: 1, At: at}); err != nil {
		t.Fatalf("BeginAccepted: %v", err)
	}
	closeStore(t, st)

	db := rawOpen(t, path)
	if err := db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("last_seq")).Put([]byte("dev-a"), []byte{1, 2, 3})
	}); err != nil {
		t.Fatalf("corrupt last_seq: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close raw db: %v", err)
	}

	st = openStoreAt(t, path)
	defer func() {
		if err := st.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()
	if _, err := st.LastSeq(ctx, "dev-a"); err == nil {
		t.Fatal("LastSeq with a 3-byte watermark succeeded, want error")
	}
	if err := st.BeginAccepted(ctx, interfaces.AcceptedRecord{Device: "dev-a", Seq: 2, At: at}); err == nil {
		t.Fatal("BeginAccepted with a corrupt watermark succeeded, want error")
	}
	if _, found, err := st.Lookup(ctx, "dev-a", 2); err != nil || found {
		t.Fatalf("corrupt watermark wrote a record: found %v, err %v", found, err)
	}
}

func TestBboltDeleteExpiredRemovesEmptyDeviceBucket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	st := openStoreAt(t, path)
	ctx := context.Background()
	cutoff := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)

	for _, seq := range []uint64{1, 2} {
		if err := st.BeginAccepted(ctx, interfaces.AcceptedRecord{Device: "dev-a", Seq: seq, At: cutoff}); err != nil {
			t.Fatalf("BeginAccepted: %v", err)
		}
		if err := st.Finish(ctx, interfaces.FinalRecord{Device: "dev-a", Seq: seq, At: cutoff.Add(-time.Hour)}); err != nil {
			t.Fatalf("Finish: %v", err)
		}
	}
	if err := st.BeginAccepted(ctx, interfaces.AcceptedRecord{Device: "dev-b", Seq: 1, At: cutoff}); err != nil {
		t.Fatalf("BeginAccepted: %v", err)
	}
	if err := st.Finish(ctx, interfaces.FinalRecord{Device: "dev-b", Seq: 1, At: cutoff.Add(-time.Hour)}); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if err := st.BeginAccepted(ctx, interfaces.AcceptedRecord{Device: "dev-b", Seq: 2, At: cutoff}); err != nil {
		t.Fatalf("BeginAccepted: %v", err)
	}
	if err := st.Finish(ctx, interfaces.FinalRecord{Device: "dev-b", Seq: 2, At: cutoff.Add(time.Hour)}); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	count, err := st.DeleteExpired(ctx, cutoff)
	if err != nil || count != 3 {
		t.Fatalf("DeleteExpired = (%d, %v), want (3, nil)", count, err)
	}
	closeStore(t, st)

	db := rawOpen(t, path)
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("close raw db: %v", err)
		}
	}()
	if err := db.View(func(tx *bolt.Tx) error {
		records := tx.Bucket([]byte("records"))
		if records == nil {
			t.Error("records bucket is missing")
			return nil
		}
		if records.Bucket([]byte("dev-a")) != nil {
			t.Error("empty records/dev-a bucket was not removed")
		}
		if records.Bucket([]byte("dev-b")) == nil {
			t.Error("records/dev-b bucket was removed, want it kept")
		}
		return nil
	}); err != nil {
		t.Fatalf("view raw db: %v", err)
	}
}

func TestBboltDeleteExpiredCorruptRecordRollsBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	st := openStoreAt(t, path)
	ctx := context.Background()
	cutoff := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)

	for _, seq := range []uint64{1, 2} {
		if err := st.BeginAccepted(ctx, interfaces.AcceptedRecord{Device: "dev-a", Seq: seq, At: cutoff}); err != nil {
			t.Fatalf("BeginAccepted: %v", err)
		}
		if err := st.Finish(ctx, interfaces.FinalRecord{Device: "dev-a", Seq: seq, At: cutoff.Add(-time.Hour)}); err != nil {
			t.Fatalf("Finish: %v", err)
		}
	}
	closeStore(t, st)

	db := rawOpen(t, path)
	if err := db.Update(func(tx *bolt.Tx) error {
		rb := tx.Bucket([]byte("records")).Bucket([]byte("dev-a"))
		return rb.Put(seqKey(2), []byte("{not-json"))
	}); err != nil {
		t.Fatalf("corrupt record: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close raw db: %v", err)
	}

	st = openStoreAt(t, path)
	defer func() {
		if err := st.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()
	count, err := st.DeleteExpired(ctx, cutoff)
	if err == nil {
		t.Fatal("DeleteExpired with a corrupt record succeeded, want error")
	}
	if count != 0 {
		t.Fatalf("DeleteExpired count = %d, want 0 after rollback", count)
	}
	rec, found, err := st.Lookup(ctx, "dev-a", 1)
	if err != nil || !found {
		t.Fatalf("healthy record was deleted by a rolled-back transaction: found %v, err %v", found, err)
	}
	if rec.Phase != interfaces.PhaseFinal {
		t.Fatalf("healthy record phase = %q, want final", rec.Phase)
	}
	if _, _, err := st.Lookup(ctx, "dev-a", 2); err == nil {
		t.Fatal("Lookup of a corrupt record succeeded, want error")
	}
}

func TestBboltCorruptRecordIsRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	st := openStoreAt(t, path)
	ctx := context.Background()
	at := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	for _, seq := range []uint64{1, 2} {
		if err := st.BeginAccepted(ctx, interfaces.AcceptedRecord{Device: "dev-a", Seq: seq, At: at}); err != nil {
			t.Fatalf("BeginAccepted: %v", err)
		}
	}
	closeStore(t, st)

	db := rawOpen(t, path)
	if err := db.Update(func(tx *bolt.Tx) error {
		rb := tx.Bucket([]byte("records")).Bucket([]byte("dev-a"))
		if err := rb.Put(seqKey(1), []byte(`{"device":"dev-a","seq":1,"phase":"weird","accepted_at":"2026-11-23T10:00:00Z"}`)); err != nil {
			return err
		}
		return rb.Put(seqKey(2), []byte("{not-json"))
	}); err != nil {
		t.Fatalf("corrupt records: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close raw db: %v", err)
	}

	st = openStoreAt(t, path)
	defer func() {
		if err := st.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()
	if _, _, err := st.Lookup(ctx, "dev-a", 1); err == nil {
		t.Fatal("Lookup accepted an unknown phase, want error")
	}
	if _, _, err := st.Lookup(ctx, "dev-a", 2); err == nil {
		t.Fatal("Lookup accepted invalid JSON, want error")
	}
}
