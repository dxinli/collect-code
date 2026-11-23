package fake_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dxinli/collect-code/internal/interfaces"
	"github.com/dxinli/collect-code/internal/testutil/fake"
)

var _ interfaces.Driver = (*fake.Driver)(nil)

func TestFakeDriverSatisfiesInterface(t *testing.T) {
	d := fake.NewDriver("modbus")
	if got := d.Name(); got != "modbus" {
		t.Fatalf("Name() = %q, want modbus", got)
	}
	if !d.Capabilities().Pollable {
		t.Fatal("new fake driver must be pollable by default")
	}
	if _, err := d.Write(t.Context(), interfaces.WriteRequest{}); !errors.Is(err, interfaces.ErrUnsupported) {
		t.Fatalf("Write error = %v, want ErrUnsupported", err)
	}
	if d.Events() != nil {
		t.Fatal("Events() must be nil for a pull-only fake")
	}
}

func TestFakeDriverRecordsReadRequests(t *testing.T) {
	d := fake.NewDriver("modbus")
	deadline := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	d.SetDefaultReadHandler(func(context.Context, fake.ReadCall) ([]interfaces.Sample, interfaces.Outcome) {
		return []interfaces.Sample{{PointKey: "p", Quality: interfaces.QualityGood}}, interfaces.OutcomeOK
	})
	samples, outcome := d.Read(t.Context(), interfaces.ReadRequest{
		Device:   "d1",
		Points:   []string{"p"},
		Deadline: deadline,
	})
	if outcome != interfaces.OutcomeOK || len(samples) != 1 {
		t.Fatalf("Read = (%v, %v), want one OK sample", samples, outcome)
	}
	calls := d.Calls()
	if len(calls) != 1 || calls[0].Device != "d1" || calls[0].Deadline != deadline {
		t.Fatalf("Calls() = %+v", calls)
	}
	select {
	case call := <-d.Started():
		if call.Device != "d1" {
			t.Fatalf("Started device = %q, want d1", call.Device)
		}
	default:
		t.Fatal("Started() did not report the read")
	}
}

func TestFakeDriverHonorsContextCancellation(t *testing.T) {
	d := fake.NewDriver("modbus")
	canceled := make(chan struct{})
	d.SetDefaultReadHandler(func(ctx context.Context, _ fake.ReadCall) ([]interfaces.Sample, interfaces.Outcome) {
		<-ctx.Done()
		close(canceled)
		return nil, interfaces.OutcomeFailed
	})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		_, _ = d.Read(ctx, interfaces.ReadRequest{Device: "d1"})
		close(done)
	}()
	<-d.Started()
	cancel()
	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not observe cancellation")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Read did not return after cancellation")
	}
}

func TestFakeDriverSetCapabilitiesAndStatus(t *testing.T) {
	d := fake.NewDriver("lorawan")
	d.SetCapabilities(interfaces.Capabilities{Pollable: false})
	if d.Capabilities().Pollable {
		t.Fatal("SetCapabilities did not update Pollable")
	}
	want := interfaces.LinkStatus{State: interfaces.LinkDown, Failures: 3, LastError: "timeout"}
	d.SetStatus(want)
	if got := d.Status(); got != want {
		t.Fatalf("Status() = %+v, want %+v", got, want)
	}
}
