package bbolt_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/dxinli/collect-code/internal/interfaces"
	storebbolt "github.com/dxinli/collect-code/internal/store/bbolt"
)

func newStore(t *testing.T) *storebbolt.Store {
	t.Helper()
	st, err := storebbolt.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return st
}

func TestBboltRecordLifecycle(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	acceptedAt := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	finalAt := acceptedAt.Add(time.Minute)
	device := "valve-1"

	if err := st.BeginAccepted(ctx, interfaces.AcceptedRecord{Device: device, Seq: 5, ServiceID: "svc-open", At: acceptedAt}); err != nil {
		t.Fatalf("BeginAccepted: %v", err)
	}
	rec, found, err := st.Lookup(ctx, device, 5)
	if err != nil || !found {
		t.Fatalf("Lookup = (found %v, err %v), want (true, nil)", found, err)
	}
	if rec.Phase != interfaces.PhaseAccepted || rec.ServiceID != "svc-open" || !rec.AcceptedAt.Equal(acceptedAt) {
		t.Fatalf("accepted record = %+v", rec)
	}
	if got, err := st.LastSeq(ctx, device); err != nil || got != 5 {
		t.Fatalf("LastSeq = (%d, %v), want (5, nil)", got, err)
	}

	if err := st.BeginAccepted(ctx, interfaces.AcceptedRecord{Device: device, Seq: 5, ServiceID: "other", At: finalAt}); !errors.Is(err, interfaces.ErrAlreadyAccepted) {
		t.Fatalf("duplicate BeginAccepted error = %v, want ErrAlreadyAccepted", err)
	}

	if err := st.Finish(ctx, interfaces.FinalRecord{Device: device, Seq: 5, Code: 200, Msg: "ok", Data: map[string]any{"target": 45}, At: finalAt}); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	rec, found, err = st.Lookup(ctx, device, 5)
	if err != nil || !found {
		t.Fatalf("Lookup after Finish = (found %v, err %v)", found, err)
	}
	if rec.Phase != interfaces.PhaseFinal || rec.Code != 200 || rec.Msg != "ok" || !rec.FinalAt.Equal(finalAt) {
		t.Fatalf("final record = %+v", rec)
	}
	if rec.ServiceID != "svc-open" || !rec.AcceptedAt.Equal(acceptedAt) {
		t.Fatalf("Finish lost accepted fields: %+v", rec)
	}
	if got, err := st.LastSeq(ctx, device); err != nil || got != 5 {
		t.Fatalf("LastSeq after Finish = (%d, %v), want (5, nil)", got, err)
	}

	if err := st.Finish(ctx, interfaces.FinalRecord{Device: device, Seq: 5, Code: 500, Msg: "again", At: finalAt}); !errors.Is(err, interfaces.ErrAlreadyFinal) {
		t.Fatalf("second Finish error = %v, want ErrAlreadyFinal", err)
	}
	if err := st.Finish(ctx, interfaces.FinalRecord{Device: device, Seq: 99, At: finalAt}); !errors.Is(err, interfaces.ErrRecordNotFound) {
		t.Fatalf("Finish unknown seq error = %v, want ErrRecordNotFound", err)
	}

	if _, found, err := st.Lookup(ctx, "missing", 1); err != nil || found {
		t.Fatalf("Lookup missing = (found %v, err %v), want (false, nil)", found, err)
	}
	if got, err := st.LastSeq(ctx, "missing"); err != nil || got != 0 {
		t.Fatalf("LastSeq missing = (%d, %v), want (0, nil)", got, err)
	}
}
