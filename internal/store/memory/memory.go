// Package memory is the in-memory, test-only implementation of
// interfaces.Store.
//
// It is not part of any production binary: no production package may import
// it, and scripts/check-deps.sh enforces that. core tests use it as the L1
// store; docs/contracts/store.md defines the semantics it must share with the
// bbolt implementation.
//
// Values are stored as JSON bytes exactly like bbolt stores them, so callers
// observe the same JSON semantics from both implementations: numbers in
// Record.Data and Property.Value decode as float64, nil stays nil, and time
// values inside Data or Value become RFC3339 strings. Close marks the handle
// closed but keeps the data; Reopen clears the flag. Reopen is a testing hook
// and is not part of interfaces.Store.
package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/dxinli/collect-code/internal/interfaces"
)

var (
	errClosed          = fmt.Errorf("memory store: %w", interfaces.ErrClosed)
	errRecordNotFound  = fmt.Errorf("memory store: %w", interfaces.ErrRecordNotFound)
	errAlreadyAccepted = fmt.Errorf("memory store: %w", interfaces.ErrAlreadyAccepted)
	errAlreadyFinal    = fmt.Errorf("memory store: %w", interfaces.ErrAlreadyFinal)
)

// Store is an in-memory interfaces.Store.
type Store struct {
	mu        sync.RWMutex
	records   map[string]map[uint64][]byte
	lastSeq   map[string]uint64
	devices   map[string][]byte
	snapshots map[string][]byte
	closed    bool
}

var _ interfaces.Store = (*Store)(nil)

// New returns an empty, open store.
func New() *Store {
	return &Store{
		records:   make(map[string]map[uint64][]byte),
		lastSeq:   make(map[string]uint64),
		devices:   make(map[string][]byte),
		snapshots: make(map[string][]byte),
	}
}

func checkCtx(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

// Close marks the store closed. It is idempotent and keeps the data so a
// test can call Reopen.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

// Reopen clears the closed flag. It is a testing hook, not part of
// interfaces.Store, and is a no-op when the store is already open.
func (s *Store) Reopen() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = false
}

// BeginAccepted implements interfaces.Store.
func (s *Store) BeginAccepted(ctx context.Context, rec interfaces.AcceptedRecord) error {
	if err := checkCtx(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errClosed
	}
	if err := checkCtx(ctx); err != nil {
		return err
	}

	if bucket := s.records[rec.Device]; bucket != nil {
		if _, ok := bucket[rec.Seq]; ok {
			return errAlreadyAccepted
		}
	}
	value, err := json.Marshal(interfaces.Record{
		Device:     rec.Device,
		Seq:        rec.Seq,
		ServiceID:  rec.ServiceID,
		Phase:      interfaces.PhaseAccepted,
		AcceptedAt: rec.At,
	})
	if err != nil {
		return fmt.Errorf("memory store: encode accepted record: %w", err)
	}
	bucket := s.records[rec.Device]
	if bucket == nil {
		bucket = make(map[uint64][]byte)
		s.records[rec.Device] = bucket
	}
	bucket[rec.Seq] = value
	if rec.Seq > s.lastSeq[rec.Device] {
		s.lastSeq[rec.Device] = rec.Seq
	}
	return nil
}

// Finish implements interfaces.Store.
func (s *Store) Finish(ctx context.Context, rec interfaces.FinalRecord) error {
	if err := checkCtx(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errClosed
	}
	if err := checkCtx(ctx); err != nil {
		return err
	}

	bucket := s.records[rec.Device]
	if bucket == nil {
		return errRecordNotFound
	}
	raw, ok := bucket[rec.Seq]
	if !ok {
		return errRecordNotFound
	}
	var stored interfaces.Record
	if err := json.Unmarshal(raw, &stored); err != nil {
		return fmt.Errorf("memory store: decode record: %w", err)
	}
	if stored.Phase == interfaces.PhaseFinal {
		return errAlreadyFinal
	}
	stored.Phase = interfaces.PhaseFinal
	stored.FinalAt = rec.At
	stored.Code = rec.Code
	stored.Msg = rec.Msg
	stored.Data = rec.Data
	value, err := json.Marshal(stored)
	if err != nil {
		return fmt.Errorf("memory store: encode final record: %w", err)
	}
	bucket[rec.Seq] = value
	return nil
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

	bucket := s.records[device]
	if bucket == nil {
		return interfaces.Record{}, false, nil
	}
	raw, ok := bucket[seq]
	if !ok {
		return interfaces.Record{}, false, nil
	}
	var rec interfaces.Record
	if err := json.Unmarshal(raw, &rec); err != nil {
		return interfaces.Record{}, false, fmt.Errorf("memory store: decode record: %w", err)
	}
	return rec, true, nil
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
	return s.lastSeq[device], nil
}

// LoadDevices implements interfaces.Store.
func (s *Store) LoadDevices(ctx context.Context) ([]interfaces.DeviceState, error) {
	if err := checkCtx(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, errClosed
	}

	states := make([]interfaces.DeviceState, 0, len(s.devices))
	for device, raw := range s.devices {
		if err := checkCtx(ctx); err != nil {
			return nil, err
		}
		var state interfaces.DeviceState
		if err := json.Unmarshal(raw, &state); err != nil {
			return nil, fmt.Errorf("memory store: decode device %q: %w", device, err)
		}
		states = append(states, state)
	}
	return states, nil
}

// SaveDevices implements interfaces.Store.
func (s *Store) SaveDevices(ctx context.Context, states []interfaces.DeviceState) error {
	if err := checkCtx(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errClosed
	}

	type entry struct {
		device string
		value  []byte
	}
	encoded := make([]entry, 0, len(states))
	for _, state := range states {
		if err := checkCtx(ctx); err != nil {
			return err
		}
		value, err := json.Marshal(state)
		if err != nil {
			return fmt.Errorf("memory store: encode device %q: %w", state.Device, err)
		}
		encoded = append(encoded, entry{device: state.Device, value: value})
	}
	for _, e := range encoded {
		if err := checkCtx(ctx); err != nil {
			return err
		}
		s.devices[e.device] = e.value
	}
	return nil
}

// LoadSnapshot implements interfaces.Store.
func (s *Store) LoadSnapshot(ctx context.Context, device string) ([]interfaces.Property, bool, error) {
	if err := checkCtx(ctx); err != nil {
		return nil, false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, false, errClosed
	}

	raw, ok := s.snapshots[device]
	if !ok {
		return nil, false, nil
	}
	var props []interfaces.Property
	if err := json.Unmarshal(raw, &props); err != nil {
		return nil, false, fmt.Errorf("memory store: decode snapshot %q: %w", device, err)
	}
	if props == nil {
		props = []interfaces.Property{}
	}
	return props, true, nil
}

// SaveSnapshot implements interfaces.Store.
func (s *Store) SaveSnapshot(ctx context.Context, device string, props []interfaces.Property) error {
	if err := checkCtx(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errClosed
	}

	if props == nil {
		props = []interfaces.Property{}
	}
	value, err := json.Marshal(props)
	if err != nil {
		return fmt.Errorf("memory store: encode snapshot %q: %w", device, err)
	}
	s.snapshots[device] = value
	return nil
}

// DeleteExpired implements interfaces.Store.
func (s *Store) DeleteExpired(ctx context.Context, before time.Time) (int, error) {
	if err := checkCtx(ctx); err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, errClosed
	}

	type recordKey struct {
		device string
		seq    uint64
	}
	var toDelete []recordKey
	affected := make(map[string]struct{})
	for device, bucket := range s.records {
		for seq, raw := range bucket {
			if err := checkCtx(ctx); err != nil {
				return 0, err
			}
			var rec interfaces.Record
			if err := json.Unmarshal(raw, &rec); err != nil {
				return 0, fmt.Errorf("memory store: decode record: %w", err)
			}
			if rec.Phase == interfaces.PhaseFinal && rec.FinalAt.Before(before) {
				toDelete = append(toDelete, recordKey{device: device, seq: seq})
				affected[device] = struct{}{}
			}
		}
	}
	for _, key := range toDelete {
		if err := checkCtx(ctx); err != nil {
			return 0, err
		}
		delete(s.records[key.device], key.seq)
	}
	for device := range affected {
		if len(s.records[device]) == 0 {
			delete(s.records, device)
		}
	}
	return len(toDelete), nil
}
