// Package storetest is the shared contract suite for interfaces.Store.
//
// Every implementation of interfaces.Store must pass Run unchanged. The suite
// is a normal package (not a _test.go file) so both the in-memory and the
// bbolt implementation tests can import it, but no production package may
// import it; scripts/check-deps.sh enforces that boundary.
//
// Observable semantics fixed by this suite:
//
//   - Values cross the seam with JSON semantics. Numbers in Record.Data and
//     Property.Value decode as float64, nil stays nil, nested objects and
//     arrays become map[string]any and []any, and a time.Time stored inside
//     Data or Value comes back as an RFC3339 string. Integers above 2^53 lose
//     precision; callers that need them must store strings. Replay
//     comparison (tickets 02b/10) must use the same JSON semantics and must
//     not use reflect.DeepEqual on Data or Value.
//   - Typed time fields keep the instant only: monotonic readings and
//     location names are dropped. Compare with time.Equal, never with ==.
//     time.Time{} round-trips as the zero time.
//   - LoadDevices order is not part of the contract; tests sort before
//     comparing. LoadSnapshot preserves the save order.
//   - Close is idempotent. After Close every method that takes a context
//     returns interfaces.ErrClosed.
//   - Corrupt records are a fail-fast condition: read paths return an error
//     instead of skipping. The in-memory implementation has no public path
//     that produces one, so only bbolt exercises that case.
//
// Harness lifecycle: Run calls the factory inside each subtest and registers
// Cleanup on that subtest's t, so every subtest owns one fresh store.
package storetest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/dxinli/collect-code/internal/interfaces"
)

// Harness abstracts one store plus its persistence lifecycle.
type Harness interface {
	Store() interfaces.Store
	// Reopen closes the current store and returns one over the same persisted
	// data. The caller must call Store() afterwards to obtain the new handle.
	Reopen(t *testing.T)
	// Cleanup releases resources. Run registers it with t.Cleanup on the
	// subtest that created the harness.
	Cleanup(t *testing.T)
}

// Factory creates a fresh harness for one subtest.
type Factory func(t *testing.T) Harness

// Run executes the full Store contract against the harness returned by
// factory. Each subtest gets a fresh harness.
func Run(t *testing.T, factory Factory) {
	t.Helper()
	run(t, factory, subtests)
}

type subtest struct {
	name string
	fn   func(t *testing.T, h Harness)
}

func run(t *testing.T, factory Factory, cases []subtest) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := factory(t)
			t.Cleanup(func() { h.Cleanup(t) })
			tc.fn(t, h)
		})
	}
}

var baseTime = time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)

const (
	devA = "dev-a"
	devB = "dev-b"
)

func ctx() context.Context { return context.Background() }

func mustNoErr(t *testing.T, err error, what string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: unexpected error: %v", what, err)
	}
}

func wantIs(t *testing.T, err error, target error, what string) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("%s: error = %v, want %v", what, err, target)
	}
}

func mustBegin(t *testing.T, st interfaces.Store, rec interfaces.AcceptedRecord) {
	t.Helper()
	mustNoErr(t, st.BeginAccepted(ctx(), rec), "BeginAccepted")
}

func mustFinish(t *testing.T, st interfaces.Store, rec interfaces.FinalRecord) {
	t.Helper()
	mustNoErr(t, st.Finish(ctx(), rec), "Finish")
}

func mustLookup(t *testing.T, st interfaces.Store, device string, seq uint64) interfaces.Record {
	t.Helper()
	rec, found, err := st.Lookup(ctx(), device, seq)
	mustNoErr(t, err, "Lookup")
	if !found {
		t.Fatalf("Lookup(%q, %d): found = false, want true", device, seq)
	}
	return rec
}

func mustLastSeq(t *testing.T, st interfaces.Store, device string) uint64 {
	t.Helper()
	seq, err := st.LastSeq(ctx(), device)
	mustNoErr(t, err, "LastSeq")
	return seq
}

func mustLoadDevices(t *testing.T, st interfaces.Store) []interfaces.DeviceState {
	t.Helper()
	states, err := st.LoadDevices(ctx())
	mustNoErr(t, err, "LoadDevices")
	if states == nil {
		t.Fatal("LoadDevices: returned nil slice, want non-nil")
	}
	return states
}

func mustLoadSnapshot(t *testing.T, st interfaces.Store, device string) []interfaces.Property {
	t.Helper()
	props, found, err := st.LoadSnapshot(ctx(), device)
	mustNoErr(t, err, "LoadSnapshot")
	if !found {
		t.Fatalf("LoadSnapshot(%q): found = false, want true", device)
	}
	if props == nil {
		t.Fatalf("LoadSnapshot(%q): returned nil slice, want non-nil", device)
	}
	return props
}

func findDevice(t *testing.T, states []interfaces.DeviceState, device string) interfaces.DeviceState {
	t.Helper()
	for _, st := range states {
		if st.Device == device {
			return st
		}
	}
	t.Fatalf("LoadDevices: device %q not found in %d states", device, len(states))
	return interfaces.DeviceState{}
}

// jsonEqual compares two values by their JSON encoding. encoding/json sorts
// map keys, so this compares JSON semantics rather than Go type identity:
// int(1) and float64(1) both encode as 1.
func jsonEqual(t *testing.T, want, got any) bool {
	t.Helper()
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal want: %v", err)
	}
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal got: %v", err)
	}
	return bytes.Equal(wantJSON, gotJSON)
}

func timeEqual(t *testing.T, want, got time.Time) bool {
	t.Helper()
	return got.Equal(want)
}

func sortDevices(states []interfaces.DeviceState) {
	sort.Slice(states, func(i, j int) bool { return states[i].Device < states[j].Device })
}

var subtests = []subtest{
	{"BeginAcceptedAndLookup", testBeginAcceptedAndLookup},
	{"BeginAcceptedDuplicateIsRejected", testBeginAcceptedDuplicateIsRejected},
	{"FinishCompletesAcceptedRecord", testFinishCompletesAcceptedRecord},
	{"FinishUnknownRecordIsRejected", testFinishUnknownRecordIsRejected},
	{"FinishUnknownDeviceVsUnknownSeq", testFinishUnknownDeviceVsUnknownSeq},
	{"FinishAlreadyFinalIsRejected", testFinishAlreadyFinalIsRejected},
	{"LookupMissingReturnsNotFound", testLookupMissingReturnsNotFound},
	{"LastSeqStartsAtZeroAndIsMonotonic", testLastSeqStartsAtZeroAndIsMonotonic},
	{"LastSeqSurvivesDeleteExpired", testLastSeqSurvivesDeleteExpired},
	{"LoadDevicesEmptyReturnsEmptySlice", testLoadDevicesEmptyReturnsEmptySlice},
	{"SaveDevicesUpsertsWithoutDeleting", testSaveDevicesUpsertsWithoutDeleting},
	{"SaveDevicesDuplicateDevice", testSaveDevicesDuplicateDevice},
	{"SaveDevicesDoesNotChangeLastSeqWatermark", testSaveDevicesDoesNotChangeLastSeqWatermark},
	{"LoadSnapshotMissingReturnsNotFound", testLoadSnapshotMissingReturnsNotFound},
	{"SaveSnapshotReplacesWholeDevice", testSaveSnapshotReplacesWholeDevice},
	{"SaveSnapshotNilIsFoundEmpty", testSaveSnapshotNilIsFoundEmpty},
	{"UnknownQualityIsOpaque", testUnknownQualityIsOpaque},
	{"DeleteExpiredOnlyFinalStrictlyBefore", testDeleteExpiredOnlyFinalStrictlyBefore},
	{"DeleteExpiredKeepsAcceptedRecords", testDeleteExpiredKeepsAcceptedRecords},
	{"DeleteExpiredReturnsDeletedCount", testDeleteExpiredReturnsDeletedCount},
	{"DeleteExpiredMultipleRecordsSameDevice", testDeleteExpiredMultipleRecordsSameDevice},
	{"DeleteExpiredEmptyStore", testDeleteExpiredEmptyStore},
	{"DeleteExpiredAfterReopen", testDeleteExpiredAfterReopen},
	{"DeleteExpiredCanceledContextLeavesRecords", testDeleteExpiredCanceledContextLeavesRecords},
	{"ReopenPreservesAcceptedPhase", testReopenPreservesAcceptedPhase},
	{"ReopenPreservesAllData", testReopenPreservesAllData},
	{"ReturnedValuesAreCopies", testReturnedValuesAreCopies},
	{"JSONRoundTripSemantics", testJSONRoundTripSemantics},
	{"TimesCompareByInstant", testTimesCompareByInstant},
	{"ZeroTimesRoundTrip", testZeroTimesRoundTrip},
	{"DataTimeBecomesString", testDataTimeBecomesString},
	{"NilVsEmptyMapSlice", testNilVsEmptyMapSlice},
	{"CanceledContextReturnsPromptly", testCanceledContextReturnsPromptly},
	{"CloseIsIdempotent", testCloseIsIdempotent},
	{"OperationsAfterCloseReturnErrClosed", testOperationsAfterCloseReturnErrClosed},
	{"CloseWhileWritesInFlight", testCloseWhileWritesInFlight},
	{"ConcurrentWritesAndReadsDoNotPanic", testConcurrentWritesAndReadsDoNotPanic},
}

func testBeginAcceptedAndLookup(t *testing.T, h Harness) {
	st := h.Store()
	at := baseTime.Add(time.Second)
	mustBegin(t, st, interfaces.AcceptedRecord{Device: devA, Seq: 7, ServiceID: "svc-open", At: at})

	rec := mustLookup(t, st, devA, 7)
	if rec.Device != devA || rec.Seq != 7 {
		t.Fatalf("Lookup identity = (%q, %d), want (%q, 7)", rec.Device, rec.Seq, devA)
	}
	if rec.ServiceID != "svc-open" {
		t.Fatalf("ServiceID = %q, want svc-open", rec.ServiceID)
	}
	if rec.Phase != interfaces.PhaseAccepted {
		t.Fatalf("Phase = %q, want %q", rec.Phase, interfaces.PhaseAccepted)
	}
	if !timeEqual(t, at, rec.AcceptedAt) {
		t.Fatalf("AcceptedAt = %v, want %v", rec.AcceptedAt, at)
	}
	if !rec.FinalAt.IsZero() {
		t.Fatalf("FinalAt = %v, want zero", rec.FinalAt)
	}
	if got := mustLastSeq(t, st, devA); got != 7 {
		t.Fatalf("LastSeq = %d, want 7", got)
	}
}

func testBeginAcceptedDuplicateIsRejected(t *testing.T, h Harness) {
	st := h.Store()
	first := interfaces.AcceptedRecord{Device: devA, Seq: 1, ServiceID: "first", At: baseTime}
	mustBegin(t, st, first)

	err := st.BeginAccepted(ctx(), interfaces.AcceptedRecord{Device: devA, Seq: 1, ServiceID: "second", At: baseTime.Add(time.Hour)})
	wantIs(t, err, interfaces.ErrAlreadyAccepted, "duplicate accepted BeginAccepted")
	rec := mustLookup(t, st, devA, 1)
	if rec.ServiceID != "first" {
		t.Fatalf("duplicate overwrote ServiceID = %q, want first", rec.ServiceID)
	}
	if !timeEqual(t, first.At, rec.AcceptedAt) {
		t.Fatalf("duplicate overwrote AcceptedAt = %v, want %v", rec.AcceptedAt, first.At)
	}

	second := interfaces.AcceptedRecord{Device: devA, Seq: 2, ServiceID: "svc", At: baseTime}
	mustBegin(t, st, second)
	mustFinish(t, st, interfaces.FinalRecord{Device: devA, Seq: 2, Code: 200, Msg: "ok", At: baseTime.Add(time.Minute)})

	err = st.BeginAccepted(ctx(), interfaces.AcceptedRecord{Device: devA, Seq: 2, ServiceID: "again", At: baseTime.Add(2 * time.Hour)})
	wantIs(t, err, interfaces.ErrAlreadyAccepted, "duplicate final BeginAccepted")
	rec = mustLookup(t, st, devA, 2)
	if rec.Phase != interfaces.PhaseFinal || rec.Code != 200 || rec.Msg != "ok" {
		t.Fatalf("duplicate final overwrote record: %+v", rec)
	}
	if got := mustLastSeq(t, st, devA); got != 2 {
		t.Fatalf("LastSeq = %d, want 2", got)
	}
}

func testFinishCompletesAcceptedRecord(t *testing.T, h Harness) {
	st := h.Store()
	acceptedAt := baseTime
	finalAt := baseTime.Add(time.Minute)
	mustBegin(t, st, interfaces.AcceptedRecord{Device: devA, Seq: 3, ServiceID: "svc-open", At: acceptedAt})
	mustFinish(t, st, interfaces.FinalRecord{
		Device: devA, Seq: 3, Code: 200, Msg: "ok", Data: map[string]any{"target": 45}, At: finalAt,
	})

	rec := mustLookup(t, st, devA, 3)
	if rec.Phase != interfaces.PhaseFinal {
		t.Fatalf("Phase = %q, want final", rec.Phase)
	}
	if rec.Code != 200 || rec.Msg != "ok" {
		t.Fatalf("terminal fields = (%d, %q), want (200, ok)", rec.Code, rec.Msg)
	}
	if !jsonEqual(t, map[string]any{"target": 45}, rec.Data) {
		t.Fatalf("Data = %v, want {target:45}", rec.Data)
	}
	if rec.ServiceID != "svc-open" {
		t.Fatalf("ServiceID = %q, want svc-open", rec.ServiceID)
	}
	if !timeEqual(t, acceptedAt, rec.AcceptedAt) {
		t.Fatalf("AcceptedAt = %v, want %v", rec.AcceptedAt, acceptedAt)
	}
	if !timeEqual(t, finalAt, rec.FinalAt) {
		t.Fatalf("FinalAt = %v, want %v", rec.FinalAt, finalAt)
	}
}

func testFinishUnknownRecordIsRejected(t *testing.T, h Harness) {
	st := h.Store()
	err := st.Finish(ctx(), interfaces.FinalRecord{Device: "dev-missing", Seq: 9, Code: 200, At: baseTime})
	wantIs(t, err, interfaces.ErrRecordNotFound, "Finish unknown record")

	if _, found, err := st.Lookup(ctx(), "dev-missing", 9); err != nil || found {
		t.Fatalf("Lookup after rejected Finish = (found %v, err %v), want (false, nil)", found, err)
	}
	if got := mustLastSeq(t, st, "dev-missing"); got != 0 {
		t.Fatalf("LastSeq = %d, want 0", got)
	}
}

func testFinishUnknownDeviceVsUnknownSeq(t *testing.T, h Harness) {
	st := h.Store()
	err := st.Finish(ctx(), interfaces.FinalRecord{Device: "dev-unknown", Seq: 1, At: baseTime})
	wantIs(t, err, interfaces.ErrRecordNotFound, "Finish unknown device")

	mustBegin(t, st, interfaces.AcceptedRecord{Device: devA, Seq: 1, At: baseTime})
	err = st.Finish(ctx(), interfaces.FinalRecord{Device: devA, Seq: 2, At: baseTime})
	wantIs(t, err, interfaces.ErrRecordNotFound, "Finish unknown seq")

	rec := mustLookup(t, st, devA, 1)
	if rec.Phase != interfaces.PhaseAccepted {
		t.Fatalf("Phase = %q, want accepted", rec.Phase)
	}
}

func testFinishAlreadyFinalIsRejected(t *testing.T, h Harness) {
	st := h.Store()
	mustBegin(t, st, interfaces.AcceptedRecord{Device: devA, Seq: 4, ServiceID: "svc", At: baseTime})
	mustFinish(t, st, interfaces.FinalRecord{Device: devA, Seq: 4, Code: 100, Msg: "first", Data: map[string]any{"v": 1}, At: baseTime.Add(time.Minute)})

	err := st.Finish(ctx(), interfaces.FinalRecord{Device: devA, Seq: 4, Code: 500, Msg: "second", Data: map[string]any{"v": 2}, At: baseTime.Add(2 * time.Minute)})
	wantIs(t, err, interfaces.ErrAlreadyFinal, "second Finish")

	rec := mustLookup(t, st, devA, 4)
	if rec.Phase != interfaces.PhaseFinal || rec.Code != 100 || rec.Msg != "first" {
		t.Fatalf("record changed after rejected Finish: %+v", rec)
	}
	if !jsonEqual(t, map[string]any{"v": 1}, rec.Data) {
		t.Fatalf("Data = %v, want {v:1}", rec.Data)
	}
}

func testLookupMissingReturnsNotFound(t *testing.T, h Harness) {
	st := h.Store()
	rec, found, err := st.Lookup(ctx(), "dev-missing", 42)
	if err != nil {
		t.Fatalf("Lookup: unexpected error: %v", err)
	}
	if found {
		t.Fatalf("Lookup: found = true, want false")
	}
	if rec.Device != "" || rec.Seq != 0 || rec.Phase != "" {
		t.Fatalf("Lookup zero value = %+v, want zero Record", rec)
	}
}

func testLastSeqStartsAtZeroAndIsMonotonic(t *testing.T, h Harness) {
	st := h.Store()
	if got := mustLastSeq(t, st, devA); got != 0 {
		t.Fatalf("initial LastSeq = %d, want 0", got)
	}

	mustBegin(t, st, interfaces.AcceptedRecord{Device: devA, Seq: 5, At: baseTime})
	if got := mustLastSeq(t, st, devA); got != 5 {
		t.Fatalf("LastSeq after seq 5 = %d, want 5", got)
	}
	mustBegin(t, st, interfaces.AcceptedRecord{Device: devA, Seq: 3, At: baseTime})
	if got := mustLastSeq(t, st, devA); got != 5 {
		t.Fatalf("LastSeq after smaller seq = %d, want 5", got)
	}
	mustFinish(t, st, interfaces.FinalRecord{Device: devA, Seq: 5, At: baseTime.Add(time.Minute)})
	if got := mustLastSeq(t, st, devA); got != 5 {
		t.Fatalf("LastSeq after Finish = %d, want 5", got)
	}
	err := st.BeginAccepted(ctx(), interfaces.AcceptedRecord{Device: devA, Seq: 3, At: baseTime})
	wantIs(t, err, interfaces.ErrAlreadyAccepted, "duplicate BeginAccepted")
	if got := mustLastSeq(t, st, devA); got != 5 {
		t.Fatalf("LastSeq after duplicate = %d, want 5", got)
	}

	if got := mustLastSeq(t, st, devB); got != 0 {
		t.Fatalf("other device LastSeq = %d, want 0", got)
	}
	mustBegin(t, st, interfaces.AcceptedRecord{Device: devB, Seq: 9, At: baseTime})
	if got := mustLastSeq(t, st, devB); got != 9 {
		t.Fatalf("other device LastSeq = %d, want 9", got)
	}
	if got := mustLastSeq(t, st, devA); got != 5 {
		t.Fatalf("first device LastSeq = %d, want 5", got)
	}
}

func testLastSeqSurvivesDeleteExpired(t *testing.T, h Harness) {
	st := h.Store()
	mustBegin(t, st, interfaces.AcceptedRecord{Device: devA, Seq: 10, At: baseTime})
	mustFinish(t, st, interfaces.FinalRecord{Device: devA, Seq: 10, At: baseTime.Add(time.Minute)})

	count, err := st.DeleteExpired(ctx(), baseTime.Add(time.Hour))
	mustNoErr(t, err, "DeleteExpired")
	if count != 1 {
		t.Fatalf("DeleteExpired count = %d, want 1", count)
	}
	if got := mustLastSeq(t, st, devA); got != 10 {
		t.Fatalf("LastSeq after delete = %d, want 10", got)
	}

	h.Reopen(t)
	st = h.Store()
	if got := mustLastSeq(t, st, devA); got != 10 {
		t.Fatalf("LastSeq after reopen = %d, want 10", got)
	}
}

func testLoadDevicesEmptyReturnsEmptySlice(t *testing.T, h Harness) {
	st := h.Store()
	states, err := st.LoadDevices(ctx())
	mustNoErr(t, err, "LoadDevices")
	if states == nil {
		t.Fatal("LoadDevices: returned nil slice, want non-nil empty slice")
	}
	if len(states) != 0 {
		t.Fatalf("LoadDevices length = %d, want 0", len(states))
	}
}

func testSaveDevicesUpsertsWithoutDeleting(t *testing.T, h Harness) {
	st := h.Store()
	success := baseTime.Add(-time.Minute)
	updated := baseTime
	states := []interfaces.DeviceState{
		{Device: devA, Online: true, LastSuccessAt: success, LastSeq: 42, UpdatedAt: updated},
		{Device: devB, Online: false, LastSeq: 7, UpdatedAt: updated},
	}
	mustNoErr(t, st.SaveDevices(ctx(), states), "SaveDevices")

	states[0] = interfaces.DeviceState{Device: devA, Online: false, LastSeq: 43, UpdatedAt: updated.Add(time.Minute)}
	mustNoErr(t, st.SaveDevices(ctx(), states[:1]), "SaveDevices update")

	loaded := mustLoadDevices(t, st)
	sortDevices(loaded)
	if len(loaded) != 2 {
		t.Fatalf("LoadDevices length = %d, want 2", len(loaded))
	}
	a := findDevice(t, loaded, devA)
	if a.Online || a.LastSeq != 43 {
		t.Fatalf("device A = %+v, want online=false LastSeq=43", a)
	}
	b := findDevice(t, loaded, devB)
	if b.Online || b.LastSeq != 7 {
		t.Fatalf("device B = %+v, want online=false LastSeq=7", b)
	}
	if !timeEqual(t, updated, b.UpdatedAt) {
		t.Fatalf("device B UpdatedAt = %v, want %v", b.UpdatedAt, updated)
	}

	mustNoErr(t, st.SaveDevices(ctx(), nil), "SaveDevices empty")
	loaded = mustLoadDevices(t, st)
	if len(loaded) != 2 {
		t.Fatalf("LoadDevices after empty save length = %d, want 2", len(loaded))
	}
}

func testSaveDevicesDuplicateDevice(t *testing.T, h Harness) {
	st := h.Store()
	states := []interfaces.DeviceState{
		{Device: devA, LastSeq: 1, UpdatedAt: baseTime},
		{Device: devB, LastSeq: 2, UpdatedAt: baseTime},
		{Device: devA, LastSeq: 3, UpdatedAt: baseTime},
	}
	mustNoErr(t, st.SaveDevices(ctx(), states), "SaveDevices")

	loaded := mustLoadDevices(t, st)
	sortDevices(loaded)
	if len(loaded) != 2 {
		t.Fatalf("LoadDevices length = %d, want 2", len(loaded))
	}
	if got := findDevice(t, loaded, devA).LastSeq; got != 3 {
		t.Fatalf("device A LastSeq = %d, want 3 (last write wins)", got)
	}
	if got := findDevice(t, loaded, devB).LastSeq; got != 2 {
		t.Fatalf("device B LastSeq = %d, want 2", got)
	}
}

func testSaveDevicesDoesNotChangeLastSeqWatermark(t *testing.T, h Harness) {
	st := h.Store()
	mustBegin(t, st, interfaces.AcceptedRecord{Device: devA, Seq: 5, At: baseTime})

	states := []interfaces.DeviceState{{Device: devA, LastSeq: 100, UpdatedAt: baseTime}}
	mustNoErr(t, st.SaveDevices(ctx(), states), "SaveDevices")

	if got := mustLastSeq(t, st, devA); got != 5 {
		t.Fatalf("LastSeq = %d, want 5: SaveDevices must not move the watermark", got)
	}
	loaded := mustLoadDevices(t, st)
	if got := findDevice(t, loaded, devA).LastSeq; got != 100 {
		t.Fatalf("DeviceState.LastSeq = %d, want 100", got)
	}
}

func testLoadSnapshotMissingReturnsNotFound(t *testing.T, h Harness) {
	st := h.Store()
	props, found, err := st.LoadSnapshot(ctx(), "dev-missing")
	if err != nil {
		t.Fatalf("LoadSnapshot: unexpected error: %v", err)
	}
	if found {
		t.Fatal("LoadSnapshot: found = true, want false")
	}
	if props != nil {
		t.Fatalf("LoadSnapshot props = %v, want nil", props)
	}
}

func testSaveSnapshotReplacesWholeDevice(t *testing.T, h Harness) {
	st := h.Store()
	mustNoErr(t, st.SaveSnapshot(ctx(), devA, []interfaces.Property{
		{Key: "p1", Value: 1.0, Unit: "u", Quality: interfaces.QualityGood, TS: baseTime},
		{Key: "p2", Value: 2.0, Unit: "u", Quality: interfaces.QualityGood, TS: baseTime},
	}), "SaveSnapshot")
	mustNoErr(t, st.SaveSnapshot(ctx(), devB, []interfaces.Property{
		{Key: "b1", Value: 9.0, Unit: "u", Quality: interfaces.QualityStale, TS: baseTime},
	}), "SaveSnapshot devB")

	mustNoErr(t, st.SaveSnapshot(ctx(), devA, []interfaces.Property{
		{Key: "p3", Value: 3.0, Unit: "u", Quality: interfaces.QualityGood, TS: baseTime},
	}), "SaveSnapshot replace")

	props := mustLoadSnapshot(t, st, devA)
	if len(props) != 1 || props[0].Key != "p3" {
		t.Fatalf("snapshot A = %+v, want exactly [p3]", props)
	}
	bProps := mustLoadSnapshot(t, st, devB)
	if len(bProps) != 1 || bProps[0].Key != "b1" {
		t.Fatalf("snapshot B = %+v, want [b1]", bProps)
	}

	mustNoErr(t, st.SaveSnapshot(ctx(), "dev-empty", []interfaces.Property{}), "SaveSnapshot empty")
	empty := mustLoadSnapshot(t, st, "dev-empty")
	if len(empty) != 0 {
		t.Fatalf("empty snapshot length = %d, want 0", len(empty))
	}
}

func testSaveSnapshotNilIsFoundEmpty(t *testing.T, h Harness) {
	st := h.Store()
	mustNoErr(t, st.SaveSnapshot(ctx(), devA, nil), "SaveSnapshot(nil)")

	props, found, err := st.LoadSnapshot(ctx(), devA)
	if err != nil {
		t.Fatalf("LoadSnapshot: unexpected error: %v", err)
	}
	if !found {
		t.Fatal("LoadSnapshot after SaveSnapshot(nil): found = false, want true")
	}
	if props == nil {
		t.Fatal("LoadSnapshot after SaveSnapshot(nil): nil slice, want non-nil empty")
	}
	if len(props) != 0 {
		t.Fatalf("LoadSnapshot length = %d, want 0", len(props))
	}
}

func testUnknownQualityIsOpaque(t *testing.T, h Harness) {
	st := h.Store()
	mustNoErr(t, st.SaveSnapshot(ctx(), devA, []interfaces.Property{
		{Key: "p", Value: 1.0, Unit: "u", Quality: interfaces.Quality("weird"), TS: baseTime},
	}), "SaveSnapshot")

	props := mustLoadSnapshot(t, st, devA)
	if len(props) != 1 {
		t.Fatalf("snapshot length = %d, want 1", len(props))
	}
	if props[0].Quality != interfaces.Quality("weird") {
		t.Fatalf("Quality = %q, want weird", props[0].Quality)
	}
}

func testDeleteExpiredOnlyFinalStrictlyBefore(t *testing.T, h Harness) {
	st := h.Store()
	device := "dev-exp"
	// seq 1: final strictly before cutoff -> deleted.
	mustBegin(t, st, interfaces.AcceptedRecord{Device: device, Seq: 1, At: baseTime})
	mustFinish(t, st, interfaces.FinalRecord{Device: device, Seq: 1, At: baseTime.Add(-time.Nanosecond)})
	// seq 2: final exactly at cutoff -> kept.
	mustBegin(t, st, interfaces.AcceptedRecord{Device: device, Seq: 2, At: baseTime})
	mustFinish(t, st, interfaces.FinalRecord{Device: device, Seq: 2, At: baseTime})
	// seq 3: final after cutoff -> kept.
	mustBegin(t, st, interfaces.AcceptedRecord{Device: device, Seq: 3, At: baseTime})
	mustFinish(t, st, interfaces.FinalRecord{Device: device, Seq: 3, At: baseTime.Add(time.Nanosecond)})
	// seq 4: accepted long before cutoff -> kept.
	mustBegin(t, st, interfaces.AcceptedRecord{Device: device, Seq: 4, At: baseTime.Add(-time.Hour)})
	// seq 5: zero FinalAt is literally before any positive cutoff -> deleted.
	mustBegin(t, st, interfaces.AcceptedRecord{Device: device, Seq: 5, At: baseTime})
	mustFinish(t, st, interfaces.FinalRecord{Device: device, Seq: 5, At: time.Time{}})

	count, err := st.DeleteExpired(ctx(), baseTime)
	mustNoErr(t, err, "DeleteExpired")
	if count != 2 {
		t.Fatalf("DeleteExpired count = %d, want 2", count)
	}
	for _, seq := range []uint64{1, 5} {
		if _, found, err := st.Lookup(ctx(), device, seq); err != nil || found {
			t.Fatalf("Lookup(seq %d) after delete = (found %v, err %v), want (false, nil)", seq, found, err)
		}
	}
	if rec := mustLookup(t, st, device, 2); rec.Phase != interfaces.PhaseFinal {
		t.Fatalf("seq 2 phase = %q, want final (cutoff equality is kept)", rec.Phase)
	}
	if rec := mustLookup(t, st, device, 3); rec.Phase != interfaces.PhaseFinal {
		t.Fatalf("seq 3 phase = %q, want final", rec.Phase)
	}
	if rec := mustLookup(t, st, device, 4); rec.Phase != interfaces.PhaseAccepted {
		t.Fatalf("seq 4 phase = %q, want accepted", rec.Phase)
	}
	if got := mustLastSeq(t, st, device); got != 5 {
		t.Fatalf("LastSeq = %d, want 5", got)
	}
}

func testDeleteExpiredKeepsAcceptedRecords(t *testing.T, h Harness) {
	st := h.Store()
	mustBegin(t, st, interfaces.AcceptedRecord{Device: devA, Seq: 1, At: baseTime.Add(-24 * time.Hour)})

	count, err := st.DeleteExpired(ctx(), baseTime.Add(24*time.Hour))
	mustNoErr(t, err, "DeleteExpired")
	if count != 0 {
		t.Fatalf("DeleteExpired count = %d, want 0", count)
	}
	rec := mustLookup(t, st, devA, 1)
	if rec.Phase != interfaces.PhaseAccepted {
		t.Fatalf("Phase = %q, want accepted", rec.Phase)
	}
}

func testDeleteExpiredReturnsDeletedCount(t *testing.T, h Harness) {
	st := h.Store()
	// devA: two expired finals. devB: one expired, one not.
	for _, seq := range []uint64{1, 2} {
		mustBegin(t, st, interfaces.AcceptedRecord{Device: devA, Seq: seq, At: baseTime})
		mustFinish(t, st, interfaces.FinalRecord{Device: devA, Seq: seq, At: baseTime.Add(-time.Hour)})
	}
	mustBegin(t, st, interfaces.AcceptedRecord{Device: devB, Seq: 1, At: baseTime})
	mustFinish(t, st, interfaces.FinalRecord{Device: devB, Seq: 1, At: baseTime.Add(-time.Hour)})
	mustBegin(t, st, interfaces.AcceptedRecord{Device: devB, Seq: 2, At: baseTime})
	mustFinish(t, st, interfaces.FinalRecord{Device: devB, Seq: 2, At: baseTime.Add(time.Hour)})

	count, err := st.DeleteExpired(ctx(), baseTime)
	mustNoErr(t, err, "DeleteExpired")
	if count != 3 {
		t.Fatalf("DeleteExpired count = %d, want 3", count)
	}
	count, err = st.DeleteExpired(ctx(), baseTime)
	mustNoErr(t, err, "second DeleteExpired")
	if count != 0 {
		t.Fatalf("second DeleteExpired count = %d, want 0", count)
	}
	if _, found, err := st.Lookup(ctx(), devB, 2); err != nil || !found {
		t.Fatalf("unexpired record = (found %v, err %v), want (true, nil)", found, err)
	}
}

func testDeleteExpiredMultipleRecordsSameDevice(t *testing.T, h Harness) {
	st := h.Store()
	device := "dev-multi"
	for _, seq := range []uint64{1, 2} {
		mustBegin(t, st, interfaces.AcceptedRecord{Device: device, Seq: seq, At: baseTime})
		mustFinish(t, st, interfaces.FinalRecord{Device: device, Seq: seq, At: baseTime.Add(-time.Hour)})
	}
	mustBegin(t, st, interfaces.AcceptedRecord{Device: device, Seq: 3, At: baseTime})
	mustFinish(t, st, interfaces.FinalRecord{Device: device, Seq: 3, At: baseTime.Add(time.Hour)})
	mustBegin(t, st, interfaces.AcceptedRecord{Device: device, Seq: 4, At: baseTime.Add(-48 * time.Hour)})

	count, err := st.DeleteExpired(ctx(), baseTime)
	mustNoErr(t, err, "DeleteExpired")
	if count != 2 {
		t.Fatalf("DeleteExpired count = %d, want 2", count)
	}
	for _, seq := range []uint64{1, 2} {
		if _, found, err := st.Lookup(ctx(), device, seq); err != nil || found {
			t.Fatalf("Lookup(seq %d) = (found %v, err %v), want (false, nil)", seq, found, err)
		}
	}
	if rec := mustLookup(t, st, device, 3); rec.Phase != interfaces.PhaseFinal {
		t.Fatalf("seq 3 phase = %q, want final", rec.Phase)
	}
	if rec := mustLookup(t, st, device, 4); rec.Phase != interfaces.PhaseAccepted {
		t.Fatalf("seq 4 phase = %q, want accepted", rec.Phase)
	}
}

func testDeleteExpiredEmptyStore(t *testing.T, h Harness) {
	st := h.Store()
	count, err := st.DeleteExpired(ctx(), baseTime)
	mustNoErr(t, err, "DeleteExpired")
	if count != 0 {
		t.Fatalf("DeleteExpired count = %d, want 0", count)
	}
	count, err = st.DeleteExpired(ctx(), baseTime)
	mustNoErr(t, err, "second DeleteExpired")
	if count != 0 {
		t.Fatalf("second DeleteExpired count = %d, want 0", count)
	}
}

func testDeleteExpiredAfterReopen(t *testing.T, h Harness) {
	st := h.Store()
	mustBegin(t, st, interfaces.AcceptedRecord{Device: devA, Seq: 1, At: baseTime})
	mustFinish(t, st, interfaces.FinalRecord{Device: devA, Seq: 1, At: baseTime.Add(-time.Hour)})
	mustBegin(t, st, interfaces.AcceptedRecord{Device: devA, Seq: 2, At: baseTime})
	mustFinish(t, st, interfaces.FinalRecord{Device: devA, Seq: 2, At: baseTime.Add(time.Hour)})

	h.Reopen(t)
	st = h.Store()

	count, err := st.DeleteExpired(ctx(), baseTime)
	mustNoErr(t, err, "DeleteExpired")
	if count != 1 {
		t.Fatalf("DeleteExpired count = %d, want 1", count)
	}
	if _, found, err := st.Lookup(ctx(), devA, 1); err != nil || found {
		t.Fatalf("expired record = (found %v, err %v), want (false, nil)", found, err)
	}
	if rec := mustLookup(t, st, devA, 2); rec.Phase != interfaces.PhaseFinal {
		t.Fatalf("fresh record phase = %q, want final", rec.Phase)
	}
}

func testDeleteExpiredCanceledContextLeavesRecords(t *testing.T, h Harness) {
	st := h.Store()
	mustBegin(t, st, interfaces.AcceptedRecord{Device: devA, Seq: 1, At: baseTime})
	mustFinish(t, st, interfaces.FinalRecord{Device: devA, Seq: 1, At: baseTime.Add(-time.Hour)})

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	count, err := st.DeleteExpired(canceled, baseTime)
	wantIs(t, err, context.Canceled, "DeleteExpired canceled")
	if count != 0 {
		t.Fatalf("DeleteExpired count = %d, want 0", count)
	}
	if _, found, err := st.Lookup(ctx(), devA, 1); err != nil || !found {
		t.Fatalf("record after canceled delete = (found %v, err %v), want (true, nil)", found, err)
	}

	count, err = st.DeleteExpired(ctx(), baseTime)
	mustNoErr(t, err, "DeleteExpired after cancel")
	if count != 1 {
		t.Fatalf("DeleteExpired count = %d, want 1", count)
	}
}

func testReopenPreservesAcceptedPhase(t *testing.T, h Harness) {
	st := h.Store()
	at := baseTime.Add(3 * time.Second)
	mustBegin(t, st, interfaces.AcceptedRecord{Device: devA, Seq: 7, ServiceID: "svc", At: at})

	h.Reopen(t)
	st = h.Store()

	rec := mustLookup(t, st, devA, 7)
	if rec.Phase != interfaces.PhaseAccepted {
		t.Fatalf("Phase after reopen = %q, want accepted", rec.Phase)
	}
	if rec.ServiceID != "svc" {
		t.Fatalf("ServiceID after reopen = %q, want svc", rec.ServiceID)
	}
	if !timeEqual(t, at, rec.AcceptedAt) {
		t.Fatalf("AcceptedAt after reopen = %v, want %v", rec.AcceptedAt, at)
	}
	if got := mustLastSeq(t, st, devA); got != 7 {
		t.Fatalf("LastSeq after reopen = %d, want 7", got)
	}
}

func testReopenPreservesAllData(t *testing.T, h Harness) {
	st := h.Store()
	acceptedAt := baseTime
	finalAt := baseTime.Add(time.Minute)
	mustBegin(t, st, interfaces.AcceptedRecord{Device: devA, Seq: 1, ServiceID: "svc", At: acceptedAt})
	mustFinish(t, st, interfaces.FinalRecord{Device: devA, Seq: 1, Code: 200, Msg: "ok", Data: map[string]any{"n": 1}, At: finalAt})
	states := []interfaces.DeviceState{{Device: devA, Online: true, LastSuccessAt: baseTime, LastSeq: 1, UpdatedAt: baseTime}}
	mustNoErr(t, st.SaveDevices(ctx(), states), "SaveDevices")
	mustNoErr(t, st.SaveSnapshot(ctx(), devA, []interfaces.Property{
		{Key: "p", Value: "v", Unit: "u", Quality: interfaces.QualityGood, TS: baseTime},
	}), "SaveSnapshot")

	h.Reopen(t)
	st = h.Store()

	rec := mustLookup(t, st, devA, 1)
	if rec.Phase != interfaces.PhaseFinal || rec.Code != 200 || rec.Msg != "ok" {
		t.Fatalf("record after reopen = %+v", rec)
	}
	if !jsonEqual(t, map[string]any{"n": 1}, rec.Data) {
		t.Fatalf("Data after reopen = %v, want {n:1}", rec.Data)
	}
	if !timeEqual(t, acceptedAt, rec.AcceptedAt) || !timeEqual(t, finalAt, rec.FinalAt) {
		t.Fatalf("times after reopen = (%v, %v)", rec.AcceptedAt, rec.FinalAt)
	}
	if got := mustLastSeq(t, st, devA); got != 1 {
		t.Fatalf("LastSeq after reopen = %d, want 1", got)
	}
	loaded := mustLoadDevices(t, st)
	if len(loaded) != 1 || loaded[0].Device != devA || !loaded[0].Online {
		t.Fatalf("devices after reopen = %+v", loaded)
	}
	props := mustLoadSnapshot(t, st, devA)
	if len(props) != 1 || props[0].Key != "p" || props[0].Value != "v" {
		t.Fatalf("snapshot after reopen = %+v", props)
	}
}

func testReturnedValuesAreCopies(t *testing.T, h Harness) {
	st := h.Store()

	// Record.Data: neither the input map nor the returned map aliases storage.
	data := map[string]any{
		"nested": map[string]any{"k": "v"},
		"list":   []any{1, 2},
	}
	mustBegin(t, st, interfaces.AcceptedRecord{Device: devA, Seq: 1, At: baseTime})
	mustFinish(t, st, interfaces.FinalRecord{Device: devA, Seq: 1, Data: data, At: baseTime})
	data["nested"].(map[string]any)["k"] = "mutated-input"
	data["extra"] = true

	rec := mustLookup(t, st, devA, 1)
	if rec.Data["extra"] != nil {
		t.Fatalf("returned Data aliases the input map: extra = %v", rec.Data["extra"])
	}
	if got := rec.Data["nested"].(map[string]any)["k"]; got != "v" {
		t.Fatalf("input mutation leaked into storage: k = %v", got)
	}
	rec.Data["nested"].(map[string]any)["k"] = "mutated-output"
	again := mustLookup(t, st, devA, 1)
	if got := again.Data["nested"].(map[string]any)["k"]; got != "v" {
		t.Fatalf("returned Data aliases storage: k = %v", got)
	}

	// DeviceState: input slice and returned slice are both copies.
	states := []interfaces.DeviceState{{Device: devA, Online: true, LastSeq: 1, UpdatedAt: baseTime}}
	mustNoErr(t, st.SaveDevices(ctx(), states), "SaveDevices")
	states[0].Online = false
	loaded := mustLoadDevices(t, st)
	if !findDevice(t, loaded, devA).Online {
		t.Fatal("SaveDevices aliased the input slice")
	}
	loaded[0].Online = false
	if !findDevice(t, mustLoadDevices(t, st), devA).Online {
		t.Fatal("LoadDevices aliased internal storage")
	}

	// Property.Value nested containers are copies too.
	value := map[string]any{"inner": map[string]any{"x": 1}}
	props := []interfaces.Property{{Key: "p", Value: value, Unit: "u", Quality: interfaces.QualityGood, TS: baseTime}}
	mustNoErr(t, st.SaveSnapshot(ctx(), devA, props), "SaveSnapshot")
	value["inner"].(map[string]any)["x"] = 2

	saved := mustLoadSnapshot(t, st, devA)
	if got := saved[0].Value.(map[string]any)["inner"].(map[string]any)["x"]; got != 1.0 {
		t.Fatalf("snapshot input mutation leaked into storage: x = %v", got)
	}
	saved[0].Value.(map[string]any)["inner"].(map[string]any)["x"] = 3.0
	savedAgain := mustLoadSnapshot(t, st, devA)
	if got := savedAgain[0].Value.(map[string]any)["inner"].(map[string]any)["x"]; got != 1.0 {
		t.Fatalf("returned snapshot aliases storage: x = %v", got)
	}
}

func testJSONRoundTripSemantics(t *testing.T, h Harness) {
	st := h.Store()
	recordData := map[string]any{
		"int":    1,
		"nested": map[string]any{"s": "x", "arr": []any{1, 2}},
		"nil":    nil,
	}
	mustBegin(t, st, interfaces.AcceptedRecord{Device: devA, Seq: 1, At: baseTime})
	mustFinish(t, st, interfaces.FinalRecord{Device: devA, Seq: 1, Data: recordData, At: baseTime})

	rec := mustLookup(t, st, devA, 1)
	if got, ok := rec.Data["int"].(float64); !ok || got != 1 {
		t.Fatalf("Data[int] = %#v, want float64(1)", rec.Data["int"])
	}
	if rec.Data["nil"] != nil {
		t.Fatalf("Data[nil] = %#v, want nil", rec.Data["nil"])
	}
	nested, ok := rec.Data["nested"].(map[string]any)
	if !ok {
		t.Fatalf("Data[nested] = %#v, want map[string]any", rec.Data["nested"])
	}
	arr, ok := nested["arr"].([]any)
	if !ok || len(arr) != 2 {
		t.Fatalf("Data[nested].arr = %#v, want []any of length 2", nested["arr"])
	}
	if v, ok := arr[0].(float64); !ok || v != 1 {
		t.Fatalf("Data[nested].arr[0] = %#v, want float64(1)", arr[0])
	}
	if !jsonEqual(t, recordData, rec.Data) {
		t.Fatalf("Data JSON = %v, want %v", rec.Data, recordData)
	}

	propertyValue := map[string]any{"n": 3}
	mustNoErr(t, st.SaveSnapshot(ctx(), devA, []interfaces.Property{
		{Key: "p", Value: propertyValue, Unit: "u", Quality: interfaces.QualityGood, TS: baseTime},
	}), "SaveSnapshot")
	props := mustLoadSnapshot(t, st, devA)
	gotMap, ok := props[0].Value.(map[string]any)
	if !ok {
		t.Fatalf("Value = %#v, want map[string]any", props[0].Value)
	}
	if got, ok := gotMap["n"].(float64); !ok || got != 3 {
		t.Fatalf("Value[n] = %#v, want float64(3)", gotMap["n"])
	}
}

func testTimesCompareByInstant(t *testing.T, h Harness) {
	st := h.Store()
	loc := time.FixedZone("UTC+8", 8*3600)
	at := time.Date(2026, 11, 23, 18, 30, 15, 123456789, loc)
	monotonic := time.Now()
	mustBegin(t, st, interfaces.AcceptedRecord{Device: devA, Seq: 1, At: monotonic})
	mustFinish(t, st, interfaces.FinalRecord{Device: devA, Seq: 1, At: at})
	mustNoErr(t, st.SaveDevices(ctx(), []interfaces.DeviceState{
		{Device: devA, LastSuccessAt: at, UpdatedAt: monotonic},
	}), "SaveDevices")
	mustNoErr(t, st.SaveSnapshot(ctx(), devA, []interfaces.Property{
		{Key: "p", Value: 1.0, Unit: "u", Quality: interfaces.QualityGood, TS: at},
	}), "SaveSnapshot")

	rec := mustLookup(t, st, devA, 1)
	if !timeEqual(t, monotonic, rec.AcceptedAt) {
		t.Fatalf("AcceptedAt = %v, want instant %v", rec.AcceptedAt, monotonic)
	}
	if !timeEqual(t, at, rec.FinalAt) {
		t.Fatalf("FinalAt = %v, want instant %v", rec.FinalAt, at)
	}
	state := findDevice(t, mustLoadDevices(t, st), devA)
	if !timeEqual(t, at, state.LastSuccessAt) || !timeEqual(t, monotonic, state.UpdatedAt) {
		t.Fatalf("device times = (%v, %v)", state.LastSuccessAt, state.UpdatedAt)
	}
	if ts := mustLoadSnapshot(t, st, devA)[0].TS; !timeEqual(t, at, ts) {
		t.Fatalf("Property.TS = %v, want instant %v", ts, at)
	}
}

func testZeroTimesRoundTrip(t *testing.T, h Harness) {
	st := h.Store()
	mustBegin(t, st, interfaces.AcceptedRecord{Device: devA, Seq: 1, At: time.Time{}})
	rec := mustLookup(t, st, devA, 1)
	if !rec.AcceptedAt.IsZero() || !timeEqual(t, time.Time{}, rec.AcceptedAt) {
		t.Fatalf("AcceptedAt = %v, want zero", rec.AcceptedAt)
	}

	mustFinish(t, st, interfaces.FinalRecord{Device: devA, Seq: 1, At: time.Time{}})
	rec = mustLookup(t, st, devA, 1)
	if !rec.FinalAt.IsZero() {
		t.Fatalf("FinalAt = %v, want zero", rec.FinalAt)
	}

	mustNoErr(t, st.SaveDevices(ctx(), []interfaces.DeviceState{
		{Device: devB, LastSuccessAt: time.Time{}, UpdatedAt: time.Time{}},
	}), "SaveDevices")
	state := findDevice(t, mustLoadDevices(t, st), devB)
	if !state.LastSuccessAt.IsZero() || !state.UpdatedAt.IsZero() {
		t.Fatalf("device times = (%v, %v), want zero", state.LastSuccessAt, state.UpdatedAt)
	}

	mustNoErr(t, st.SaveSnapshot(ctx(), devB, []interfaces.Property{
		{Key: "p", Value: 1.0, Unit: "u", Quality: interfaces.QualityGood, TS: time.Time{}},
	}), "SaveSnapshot")
	if ts := mustLoadSnapshot(t, st, devB)[0].TS; !ts.IsZero() {
		t.Fatalf("Property.TS = %v, want zero", ts)
	}
}

func testDataTimeBecomesString(t *testing.T, h Harness) {
	st := h.Store()
	mustBegin(t, st, interfaces.AcceptedRecord{Device: devA, Seq: 1, At: baseTime})
	mustFinish(t, st, interfaces.FinalRecord{
		Device: devA, Seq: 1, Data: map[string]any{"t": baseTime}, At: baseTime,
	})
	rec := mustLookup(t, st, devA, 1)
	want := baseTime.Format(time.RFC3339Nano)
	got, ok := rec.Data["t"].(string)
	if !ok || got != want {
		t.Fatalf("Data[t] = %#v, want string %q", rec.Data["t"], want)
	}

	mustNoErr(t, st.SaveSnapshot(ctx(), devA, []interfaces.Property{
		{Key: "p", Value: baseTime, Unit: "u", Quality: interfaces.QualityGood, TS: baseTime},
	}), "SaveSnapshot")
	value := mustLoadSnapshot(t, st, devA)[0].Value
	got, ok = value.(string)
	if !ok || got != want {
		t.Fatalf("Property.Value = %#v, want string %q", value, want)
	}
}

func testNilVsEmptyMapSlice(t *testing.T, h Harness) {
	st := h.Store()
	mustBegin(t, st, interfaces.AcceptedRecord{Device: devA, Seq: 1, At: baseTime})
	mustFinish(t, st, interfaces.FinalRecord{Device: devA, Seq: 1, Data: nil, At: baseTime})
	mustBegin(t, st, interfaces.AcceptedRecord{Device: devA, Seq: 2, At: baseTime})
	mustFinish(t, st, interfaces.FinalRecord{Device: devA, Seq: 2, Data: map[string]any{}, At: baseTime})

	if data := mustLookup(t, st, devA, 1).Data; data != nil {
		t.Fatalf("nil Data round-trip = %#v, want nil", data)
	}
	if data := mustLookup(t, st, devA, 2).Data; data == nil || len(data) != 0 {
		t.Fatalf("empty Data round-trip = %#v, want non-nil empty map", data)
	}

	mustNoErr(t, st.SaveSnapshot(ctx(), "dev-nil", []interfaces.Property{
		{Key: "p", Value: nil, Unit: "u", Quality: interfaces.QualityGood, TS: baseTime},
	}), "SaveSnapshot nil value")
	mustNoErr(t, st.SaveSnapshot(ctx(), "dev-empty-value", []interfaces.Property{
		{Key: "p", Value: map[string]any{}, Unit: "u", Quality: interfaces.QualityGood, TS: baseTime},
	}), "SaveSnapshot empty value")

	if v := mustLoadSnapshot(t, st, "dev-nil")[0].Value; v != nil {
		t.Fatalf("nil Value round-trip = %#v, want nil", v)
	}
	v := mustLoadSnapshot(t, st, "dev-empty-value")[0].Value
	m, ok := v.(map[string]any)
	if !ok || m == nil || len(m) != 0 {
		t.Fatalf("empty Value round-trip = %#v, want non-nil empty map", v)
	}
}

func testCanceledContextReturnsPromptly(t *testing.T, h Harness) {
	st := h.Store()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	beginErr := st.BeginAccepted(canceled, interfaces.AcceptedRecord{Device: devA, Seq: 1, At: baseTime})
	finishErr := st.Finish(canceled, interfaces.FinalRecord{Device: devA, Seq: 1, At: baseTime})
	_, _, lookupErr := st.Lookup(canceled, devA, 1)
	_, lastSeqErr := st.LastSeq(canceled, devA)
	_, devicesErr := st.LoadDevices(canceled)
	saveDevicesErr := st.SaveDevices(canceled, []interfaces.DeviceState{{Device: devA}})
	_, _, snapshotErr := st.LoadSnapshot(canceled, devA)
	saveSnapshotErr := st.SaveSnapshot(canceled, devA, []interfaces.Property{{Key: "p"}})
	count, deleteErr := st.DeleteExpired(canceled, baseTime)

	for name, err := range map[string]error{
		"BeginAccepted": beginErr,
		"Finish":        finishErr,
		"Lookup":        lookupErr,
		"LastSeq":       lastSeqErr,
		"LoadDevices":   devicesErr,
		"SaveDevices":   saveDevicesErr,
		"LoadSnapshot":  snapshotErr,
		"SaveSnapshot":  saveSnapshotErr,
		"DeleteExpired": deleteErr,
	} {
		wantIs(t, err, context.Canceled, name)
	}
	if count != 0 {
		t.Fatalf("DeleteExpired count = %d, want 0", count)
	}

	if _, found, err := st.Lookup(ctx(), devA, 1); err != nil || found {
		t.Fatalf("canceled BeginAccepted wrote a record: found %v, err %v", found, err)
	}
	if got := mustLastSeq(t, st, devA); got != 0 {
		t.Fatalf("canceled BeginAccepted moved LastSeq to %d", got)
	}
	if states := mustLoadDevices(t, st); len(states) != 0 {
		t.Fatalf("canceled SaveDevices wrote %d states", len(states))
	}
	if _, found, err := st.LoadSnapshot(ctx(), devA); err != nil || found {
		t.Fatalf("canceled SaveSnapshot wrote a snapshot: found %v, err %v", found, err)
	}
}

func testCloseIsIdempotent(t *testing.T, h Harness) {
	st := h.Store()
	if err := st.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func testOperationsAfterCloseReturnErrClosed(t *testing.T, h Harness) {
	st := h.Store()
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	beginErr := st.BeginAccepted(ctx(), interfaces.AcceptedRecord{Device: devA, Seq: 1})
	finishErr := st.Finish(ctx(), interfaces.FinalRecord{Device: devA, Seq: 1})
	_, _, lookupErr := st.Lookup(ctx(), devA, 1)
	_, lastSeqErr := st.LastSeq(ctx(), devA)
	_, devicesErr := st.LoadDevices(ctx())
	saveDevicesErr := st.SaveDevices(ctx(), []interfaces.DeviceState{{Device: devA}})
	_, _, snapshotErr := st.LoadSnapshot(ctx(), devA)
	saveSnapshotErr := st.SaveSnapshot(ctx(), devA, []interfaces.Property{{Key: "p"}})
	_, deleteErr := st.DeleteExpired(ctx(), baseTime)

	for name, err := range map[string]error{
		"BeginAccepted": beginErr,
		"Finish":        finishErr,
		"Lookup":        lookupErr,
		"LastSeq":       lastSeqErr,
		"LoadDevices":   devicesErr,
		"SaveDevices":   saveDevicesErr,
		"LoadSnapshot":  snapshotErr,
		"SaveSnapshot":  saveSnapshotErr,
		"DeleteExpired": deleteErr,
	} {
		wantIs(t, err, interfaces.ErrClosed, name+" after Close")
	}
}

func testCloseWhileWritesInFlight(t *testing.T, h Harness) {
	st := h.Store()
	const workers = 4
	var wg sync.WaitGroup
	errCh := make(chan error, workers)
	stop := make(chan struct{})

	for w := range workers {
		device := fmt.Sprintf("close-dev-%d", w)
		wg.Go(func() {
			for seq := uint64(1); ; seq++ {
				select {
				case <-stop:
					return
				default:
				}
				if err := st.BeginAccepted(ctx(), interfaces.AcceptedRecord{Device: device, Seq: seq, At: baseTime}); err != nil {
					if errors.Is(err, interfaces.ErrClosed) {
						return
					}
					errCh <- fmt.Errorf("beginAccepted: %w", err)
					return
				}
				if err := st.Finish(ctx(), interfaces.FinalRecord{Device: device, Seq: seq, Code: 200, At: baseTime}); err != nil {
					if errors.Is(err, interfaces.ErrClosed) {
						return
					}
					errCh <- fmt.Errorf("finish: %w", err)
					return
				}
				if _, _, err := st.Lookup(ctx(), device, seq); err != nil {
					if errors.Is(err, interfaces.ErrClosed) {
						return
					}
					errCh <- fmt.Errorf("lookup: %w", err)
					return
				}
			}
		})
	}

	time.Sleep(20 * time.Millisecond)
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	close(stop)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("in-flight operation: %v", err)
	}

	if err := st.BeginAccepted(ctx(), interfaces.AcceptedRecord{Device: "after-close", Seq: 1}); !errors.Is(err, interfaces.ErrClosed) {
		t.Fatalf("BeginAccepted after Close: err = %v, want ErrClosed", err)
	}
	if _, _, err := st.Lookup(ctx(), "after-close", 1); !errors.Is(err, interfaces.ErrClosed) {
		t.Fatalf("Lookup after Close: err = %v, want ErrClosed", err)
	}
}

func testConcurrentWritesAndReadsDoNotPanic(t *testing.T, h Harness) {
	st := h.Store()
	const (
		workers   = 8
		perWorker = 20
	)
	var wg sync.WaitGroup
	errCh := make(chan error, workers*3)

	for w := range workers {
		device := fmt.Sprintf("dev-%d", w)

		wg.Go(func() {
			for seq := uint64(1); seq <= perWorker; seq++ {
				if err := st.BeginAccepted(ctx(), interfaces.AcceptedRecord{Device: device, Seq: seq, ServiceID: "svc", At: baseTime}); err != nil {
					errCh <- fmt.Errorf("%s BeginAccepted(%d): %w", device, seq, err)
					return
				}
				if err := st.Finish(ctx(), interfaces.FinalRecord{Device: device, Seq: seq, Code: 200, At: baseTime}); err != nil {
					errCh <- fmt.Errorf("%s Finish(%d): %w", device, seq, err)
					return
				}
			}
		})

		wg.Go(func() {
			for i := range perWorker {
				seq := uint64(i%perWorker) + 1
				if _, _, err := st.Lookup(ctx(), device, seq); err != nil {
					errCh <- fmt.Errorf("%s Lookup(%d): %w", device, seq, err)
					return
				}
				if _, err := st.LastSeq(ctx(), device); err != nil {
					errCh <- fmt.Errorf("%s LastSeq: %w", device, err)
					return
				}
				if _, err := st.LoadDevices(ctx()); err != nil {
					errCh <- fmt.Errorf("%s LoadDevices: %w", device, err)
					return
				}
				if _, _, err := st.LoadSnapshot(ctx(), device); err != nil {
					errCh <- fmt.Errorf("%s LoadSnapshot: %w", device, err)
					return
				}
			}
		})

		wg.Go(func() {
			for seq := uint64(1); seq <= perWorker; seq++ {
				if err := st.SaveDevices(ctx(), []interfaces.DeviceState{{Device: device, LastSeq: seq, UpdatedAt: baseTime}}); err != nil {
					errCh <- fmt.Errorf("%s SaveDevices(%d): %w", device, seq, err)
					return
				}
				if err := st.SaveSnapshot(ctx(), device, []interfaces.Property{
					{Key: "p", Value: float64(seq), Unit: "u", Quality: interfaces.QualityGood, TS: baseTime},
				}); err != nil {
					errCh <- fmt.Errorf("%s SaveSnapshot(%d): %w", device, seq, err)
					return
				}
			}
		})
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent operation: %v", err)
	}

	states := mustLoadDevices(t, st)
	sortDevices(states)
	if len(states) != workers {
		t.Fatalf("LoadDevices length = %d, want %d", len(states), workers)
	}
	for w := range workers {
		device := fmt.Sprintf("dev-%d", w)
		if got := mustLastSeq(t, st, device); got != perWorker {
			t.Fatalf("%s LastSeq = %d, want %d", device, got, perWorker)
		}
		for seq := uint64(1); seq <= perWorker; seq++ {
			if rec := mustLookup(t, st, device, seq); rec.Phase != interfaces.PhaseFinal {
				t.Fatalf("%s seq %d phase = %q, want final", device, seq, rec.Phase)
			}
		}
		if got := findDevice(t, states, device).LastSeq; got != perWorker {
			t.Fatalf("%s DeviceState.LastSeq = %d, want %d", device, got, perWorker)
		}
		props := mustLoadSnapshot(t, st, device)
		if len(props) != 1 || props[0].Value != float64(perWorker) {
			t.Fatalf("%s snapshot = %+v, want one property with value %d", device, props, perWorker)
		}
	}
}
