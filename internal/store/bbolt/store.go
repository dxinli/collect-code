// Package bbolt is the production interfaces.Store implementation, backed by
// the embedded bbolt key-value database.
//
// The on-disk layout is versioned: meta/schema_version must be the 8-byte
// big-endian uint64 schemaVersion, records holds one nested bucket per device
// keyed by the 8-byte big-endian seq, last_seq holds the per-device
// idempotency watermark, devices holds one JSON deviceDTO per device, and
// snapshots holds one JSON property array per device. Records, devices and
// snapshots are JSON so the observable value semantics match the in-memory
// implementation (see docs/contracts/store.md).
//
// Open takes an exclusive file lock. One process should open a given state.db
// once; a second open fails after the lock timeout. Close is idempotent, and
// every method returns interfaces.ErrClosed afterwards.
package bbolt

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/dxinli/collect-code/internal/interfaces"
	bolt "go.etcd.io/bbolt"
)

const schemaVersion uint64 = 1

var (
	bucketMeta      = []byte("meta")
	bucketRecords   = []byte("records")
	bucketLastSeq   = []byte("last_seq")
	bucketDevices   = []byte("devices")
	bucketSnapshots = []byte("snapshots")
	keySchema       = []byte("schema_version")

	knownBuckets = [][]byte{bucketRecords, bucketLastSeq, bucketDevices, bucketSnapshots}
)

var (
	errClosed          = fmt.Errorf("bbolt store: %w", interfaces.ErrClosed)
	errRecordNotFound  = fmt.Errorf("bbolt store: %w", interfaces.ErrRecordNotFound)
	errAlreadyAccepted = fmt.Errorf("bbolt store: %w", interfaces.ErrAlreadyAccepted)
	errAlreadyFinal    = fmt.Errorf("bbolt store: %w", interfaces.ErrAlreadyFinal)
)

// recordDTO is the JSON record written to records/<device>/<seq>.
type recordDTO struct {
	Device     string         `json:"device"`
	Seq        uint64         `json:"seq"`
	ServiceID  string         `json:"service_id"`
	Phase      string         `json:"phase"`
	AcceptedAt time.Time      `json:"accepted_at"`
	FinalAt    time.Time      `json:"final_at"`
	Code       int            `json:"code"`
	Msg        string         `json:"msg"`
	Data       map[string]any `json:"data"`
}

// deviceDTO is the JSON device state written to devices/<device>.
type deviceDTO struct {
	Device        string    `json:"device"`
	Online        bool      `json:"online"`
	LastSuccessAt time.Time `json:"last_success_at"`
	LastSeq       uint64    `json:"last_seq"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// propertyDTO is one entry of the JSON array written to snapshots/<device>.
type propertyDTO struct {
	Key     string    `json:"key"`
	Value   any       `json:"value"`
	Unit    string    `json:"unit"`
	Quality string    `json:"quality"`
	TS      time.Time `json:"ts"`
}

// Store is the bbolt-backed interfaces.Store.
type Store struct {
	mu     sync.RWMutex
	db     *bolt.DB
	closed bool
}

// Open opens or creates the state database at path with a 5s lock timeout.
// It creates the parent directory with mode 0750 when it is missing.
func Open(path string) (*Store, error) {
	return open(path, 5*time.Second)
}

func open(path string, lockTimeout time.Duration) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("bbolt store: empty path")
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("bbolt store: create parent directory %s: %w", dir, err)
		}
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: lockTimeout})
	if err != nil {
		return nil, fmt.Errorf("bbolt store: open %s: %w", path, err)
	}
	if err := ensureSchema(db, path); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func ensureSchema(db *bolt.DB, path string) error {
	return db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket(bucketMeta)
		if meta == nil {
			for _, name := range knownBuckets {
				if tx.Bucket(name) != nil {
					return fmt.Errorf("bbolt store: %s: bucket %q exists without a meta bucket", path, name)
				}
			}
			meta, err := tx.CreateBucket(bucketMeta)
			if err != nil {
				return fmt.Errorf("bbolt store: %s: create meta bucket: %w", path, err)
			}
			for _, name := range knownBuckets {
				if _, err := tx.CreateBucket(name); err != nil {
					return fmt.Errorf("bbolt store: %s: create bucket %q: %w", path, name, err)
				}
			}
			if err := meta.Put(keySchema, seqBE(schemaVersion)); err != nil {
				return fmt.Errorf("bbolt store: %s: write schema_version: %w", path, err)
			}
			return nil
		}

		raw := meta.Get(keySchema)
		if raw == nil {
			return fmt.Errorf("bbolt store: %s: missing schema_version", path)
		}
		if len(raw) != 8 {
			return fmt.Errorf("bbolt store: %s: schema_version has length %d, want 8", path, len(raw))
		}
		if version := binary.BigEndian.Uint64(raw); version != schemaVersion {
			return fmt.Errorf("bbolt store: %s: unsupported schema version %d, want %d", path, version, schemaVersion)
		}
		for _, name := range knownBuckets {
			if _, err := tx.CreateBucketIfNotExists(name); err != nil {
				return fmt.Errorf("bbolt store: %s: create bucket %q: %w", path, name, err)
			}
		}
		return nil
	})
}

// Close closes the database. It is idempotent; the first error is returned but
// not cached, and later calls return nil.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	db := s.db
	s.db = nil
	if err := db.Close(); err != nil {
		return fmt.Errorf("bbolt store: close: %w", err)
	}
	return nil
}

func checkCtx(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

// seqBE encodes a seq as the 8-byte big-endian key used by records buckets.
func seqBE(seq uint64) []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], seq)
	return buf[:]
}

// readWatermark returns the per-device accepted-seq watermark, 0 when absent.
func readWatermark(bucket *bolt.Bucket, device string) (uint64, error) {
	raw := bucket.Get([]byte(device))
	if raw == nil {
		return 0, nil
	}
	if len(raw) != 8 {
		return 0, fmt.Errorf("bbolt store: corrupt last_seq value for %q: length %d, want 8", device, len(raw))
	}
	return binary.BigEndian.Uint64(raw), nil
}

func decodeRecord(raw []byte) (interfaces.Record, error) {
	var dto recordDTO
	if err := json.Unmarshal(raw, &dto); err != nil {
		return interfaces.Record{}, fmt.Errorf("bbolt store: decode record: %w", err)
	}
	phase := interfaces.RecordPhase(dto.Phase)
	switch phase {
	case interfaces.PhaseAccepted, interfaces.PhaseFinal:
	default:
		return interfaces.Record{}, fmt.Errorf("bbolt store: corrupt record for device %q seq %d: unknown phase %q", dto.Device, dto.Seq, dto.Phase)
	}
	return interfaces.Record{
		Device:     dto.Device,
		Seq:        dto.Seq,
		ServiceID:  dto.ServiceID,
		Phase:      phase,
		AcceptedAt: dto.AcceptedAt,
		FinalAt:    dto.FinalAt,
		Code:       dto.Code,
		Msg:        dto.Msg,
		Data:       dto.Data,
	}, nil
}

func encodeRecord(rec interfaces.Record) ([]byte, error) {
	value, err := json.Marshal(recordDTO{
		Device:     rec.Device,
		Seq:        rec.Seq,
		ServiceID:  rec.ServiceID,
		Phase:      string(rec.Phase),
		AcceptedAt: rec.AcceptedAt,
		FinalAt:    rec.FinalAt,
		Code:       rec.Code,
		Msg:        rec.Msg,
		Data:       rec.Data,
	})
	if err != nil {
		return nil, fmt.Errorf("bbolt store: encode record: %w", err)
	}
	return value, nil
}

// BeginAccepted implements interfaces.Store.
func (s *Store) BeginAccepted(ctx context.Context, rec interfaces.AcceptedRecord) error {
	if err := checkCtx(ctx); err != nil {
		return err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return errClosed
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		if err := checkCtx(ctx); err != nil {
			return err
		}
		records := tx.Bucket(bucketRecords)
		if records == nil {
			return fmt.Errorf("bbolt store: missing %q bucket", bucketRecords)
		}
		rb, err := records.CreateBucketIfNotExists([]byte(rec.Device))
		if err != nil {
			return fmt.Errorf("bbolt store: create records bucket for %q: %w", rec.Device, err)
		}
		key := seqBE(rec.Seq)
		if rb.Get(key) != nil {
			return errAlreadyAccepted
		}
		value, err := encodeRecord(interfaces.Record{
			Device:     rec.Device,
			Seq:        rec.Seq,
			ServiceID:  rec.ServiceID,
			Phase:      interfaces.PhaseAccepted,
			AcceptedAt: rec.At,
		})
		if err != nil {
			return err
		}
		if err := rb.Put(key, value); err != nil {
			return fmt.Errorf("bbolt store: write accepted record: %w", err)
		}

		watermarks := tx.Bucket(bucketLastSeq)
		if watermarks == nil {
			return fmt.Errorf("bbolt store: missing %q bucket", bucketLastSeq)
		}
		current, err := readWatermark(watermarks, rec.Device)
		if err != nil {
			return err
		}
		if rec.Seq > current {
			if err := watermarks.Put([]byte(rec.Device), seqBE(rec.Seq)); err != nil {
				return fmt.Errorf("bbolt store: write last_seq: %w", err)
			}
		}
		return nil
	})
}

// Finish implements interfaces.Store.
func (s *Store) Finish(ctx context.Context, rec interfaces.FinalRecord) error {
	if err := checkCtx(ctx); err != nil {
		return err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return errClosed
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		if err := checkCtx(ctx); err != nil {
			return err
		}
		records := tx.Bucket(bucketRecords)
		if records == nil {
			return errRecordNotFound
		}
		rb := records.Bucket([]byte(rec.Device))
		if rb == nil {
			return errRecordNotFound
		}
		key := seqBE(rec.Seq)
		raw := rb.Get(key)
		if raw == nil {
			return errRecordNotFound
		}
		stored, err := decodeRecord(raw)
		if err != nil {
			return err
		}
		if stored.Phase == interfaces.PhaseFinal {
			return errAlreadyFinal
		}
		stored.Phase = interfaces.PhaseFinal
		stored.FinalAt = rec.At
		stored.Code = rec.Code
		stored.Msg = rec.Msg
		stored.Data = rec.Data
		value, err := encodeRecord(stored)
		if err != nil {
			return err
		}
		if err := rb.Put(key, value); err != nil {
			return fmt.Errorf("bbolt store: write final record: %w", err)
		}
		return nil
	})
}

// Lookup implements interfaces.Store.
func (s *Store) Lookup(ctx context.Context, device string, seq uint64) (interfaces.Record, bool, error) {
	if err := checkCtx(ctx); err != nil {
		return interfaces.Record{}, false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return interfaces.Record{}, false, errClosed
	}

	var rec interfaces.Record
	var found bool
	err := s.db.View(func(tx *bolt.Tx) error {
		if err := checkCtx(ctx); err != nil {
			return err
		}
		records := tx.Bucket(bucketRecords)
		if records == nil {
			return fmt.Errorf("bbolt store: missing %q bucket", bucketRecords)
		}
		rb := records.Bucket([]byte(device))
		if rb == nil {
			return nil
		}
		raw := rb.Get(seqBE(seq))
		if raw == nil {
			return nil
		}
		decoded, err := decodeRecord(raw)
		if err != nil {
			return err
		}
		rec = decoded
		found = true
		return nil
	})
	if err != nil {
		return interfaces.Record{}, false, err
	}
	return rec, found, nil
}

// LastSeq implements interfaces.Store.
func (s *Store) LastSeq(ctx context.Context, device string) (uint64, error) {
	if err := checkCtx(ctx); err != nil {
		return 0, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return 0, errClosed
	}

	var watermark uint64
	err := s.db.View(func(tx *bolt.Tx) error {
		if err := checkCtx(ctx); err != nil {
			return err
		}
		watermarks := tx.Bucket(bucketLastSeq)
		if watermarks == nil {
			return fmt.Errorf("bbolt store: missing %q bucket", bucketLastSeq)
		}
		value, err := readWatermark(watermarks, device)
		if err != nil {
			return err
		}
		watermark = value
		return nil
	})
	if err != nil {
		return 0, err
	}
	return watermark, nil
}
