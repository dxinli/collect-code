package bbolt_test

import (
	"context"
	"testing"
	"time"

	"github.com/dxinli/collect-code/internal/interfaces"
)

func TestBboltDeviceAndSnapshotState(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	at := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)

	if err := st.SaveDevices(ctx, []interfaces.DeviceState{
		{Device: "dev-a", Online: true, LastSeq: 3, UpdatedAt: at},
		{Device: "dev-b", LastSeq: 4, UpdatedAt: at},
	}); err != nil {
		t.Fatalf("SaveDevices: %v", err)
	}
	if err := st.SaveDevices(ctx, []interfaces.DeviceState{{Device: "dev-a", LastSeq: 9, UpdatedAt: at}}); err != nil {
		t.Fatalf("SaveDevices update: %v", err)
	}
	states, err := st.LoadDevices(ctx)
	if err != nil {
		t.Fatalf("LoadDevices: %v", err)
	}
	if len(states) != 2 {
		t.Fatalf("LoadDevices length = %d, want 2", len(states))
	}

	if err := st.SaveSnapshot(ctx, "dev-a", []interfaces.Property{
		{Key: "p1", Value: 1.0, Unit: "u", Quality: interfaces.QualityGood, TS: at},
		{Key: "p2", Value: 2.0, Unit: "u", Quality: interfaces.QualityGood, TS: at},
	}); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}
	if err := st.SaveSnapshot(ctx, "dev-a", []interfaces.Property{
		{Key: "p3", Value: 3.0, Unit: "u", Quality: interfaces.QualityGood, TS: at},
	}); err != nil {
		t.Fatalf("SaveSnapshot replace: %v", err)
	}
	props, found, err := st.LoadSnapshot(ctx, "dev-a")
	if err != nil || !found {
		t.Fatalf("LoadSnapshot = (found %v, err %v)", found, err)
	}
	if len(props) != 1 || props[0].Key != "p3" {
		t.Fatalf("snapshot = %+v, want [p3]", props)
	}

	if err := st.SaveSnapshot(ctx, "dev-empty", nil); err != nil {
		t.Fatalf("SaveSnapshot(nil): %v", err)
	}
	props, found, err = st.LoadSnapshot(ctx, "dev-empty")
	if err != nil || !found || props == nil || len(props) != 0 {
		t.Fatalf("nil snapshot = (props %v, found %v, err %v), want non-nil empty and found", props, found, err)
	}
}

func TestBboltDeleteExpired(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	cutoff := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)

	for seq := uint64(1); seq <= 2; seq++ {
		if err := st.BeginAccepted(ctx, interfaces.AcceptedRecord{Device: "dev-a", Seq: seq, At: cutoff}); err != nil {
			t.Fatalf("BeginAccepted(%d): %v", seq, err)
		}
		if err := st.Finish(ctx, interfaces.FinalRecord{Device: "dev-a", Seq: seq, At: cutoff.Add(-time.Hour)}); err != nil {
			t.Fatalf("Finish(%d): %v", seq, err)
		}
	}
	if err := st.BeginAccepted(ctx, interfaces.AcceptedRecord{Device: "dev-a", Seq: 3, At: cutoff}); err != nil {
		t.Fatalf("BeginAccepted(3): %v", err)
	}
	if err := st.Finish(ctx, interfaces.FinalRecord{Device: "dev-a", Seq: 3, At: cutoff}); err != nil {
		t.Fatalf("Finish(3): %v", err)
	}
	if err := st.BeginAccepted(ctx, interfaces.AcceptedRecord{Device: "dev-b", Seq: 1, At: cutoff.Add(-time.Hour)}); err != nil {
		t.Fatalf("BeginAccepted(dev-b): %v", err)
	}

	count, err := st.DeleteExpired(ctx, cutoff)
	if err != nil {
		t.Fatalf("DeleteExpired: %v", err)
	}
	if count != 2 {
		t.Fatalf("DeleteExpired count = %d, want 2", count)
	}
	if _, found, err := st.Lookup(ctx, "dev-a", 1); err != nil || found {
		t.Fatalf("expired record = (found %v, err %v), want (false, nil)", found, err)
	}
	if _, found, err := st.Lookup(ctx, "dev-a", 3); err != nil || !found {
		t.Fatalf("cutoff record = (found %v, err %v), want (true, nil)", found, err)
	}
	if _, found, err := st.Lookup(ctx, "dev-b", 1); err != nil || !found {
		t.Fatalf("accepted record = (found %v, err %v), want (true, nil)", found, err)
	}
}
