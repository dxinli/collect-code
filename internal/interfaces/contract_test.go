package interfaces_test

import (
	"context"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/dxinli/collect-code/internal/interfaces"
)

// Compile-time proof that each seam can be implemented with the frozen
// method sets.
var (
	_ interfaces.Clock           = (*stubClock)(nil)
	_ interfaces.Driver          = (*stubDriver)(nil)
	_ interfaces.Channel         = (*stubChannel)(nil)
	_ interfaces.UplinkAdapter   = (*stubUplink)(nil)
	_ interfaces.DownlinkAdapter = (*stubDownlink)(nil)
	_ interfaces.Store           = (*stubStore)(nil)
)

type stubClock struct{}

func (stubClock) Now() time.Time                            { return time.Time{} }
func (stubClock) NewTicker(time.Duration) interfaces.Ticker { return stubTicker{} }
func (stubClock) NewTimer(time.Duration) interfaces.Timer   { return stubTimer{} }
func (stubClock) Sleep(time.Duration)                       {}

type stubTicker struct{}

func (stubTicker) C() <-chan time.Time { return nil }
func (stubTicker) Stop()               {}

type stubTimer struct{}

func (stubTimer) C() <-chan time.Time { return nil }
func (stubTimer) Stop()               {}

type stubDriver struct{}

func (stubDriver) Name() string                          { return "stub" }
func (stubDriver) Open(context.Context) error            { return nil }
func (stubDriver) Close() error                          { return nil }
func (stubDriver) Capabilities() interfaces.Capabilities { return interfaces.Capabilities{} }
func (stubDriver) Read(context.Context, interfaces.ReadRequest) ([]interfaces.Sample, interfaces.Outcome) {
	return nil, interfaces.OutcomeOK
}
func (stubDriver) Write(context.Context, interfaces.WriteRequest) (interfaces.CommandHandle, error) {
	return interfaces.CommandHandle{}, nil
}
func (stubDriver) Events() <-chan interfaces.DriverEvent { return nil }
func (stubDriver) Status() interfaces.LinkStatus         { return interfaces.LinkStatus{} }

type stubChannel struct{}

func (stubChannel) Publish(context.Context, interfaces.Message) error { return nil }
func (stubChannel) Subscribe(context.Context, string) (interfaces.Subscription, error) {
	return stubSubscription{}, nil
}
func (stubChannel) Status() interfaces.ChannelStatus { return interfaces.ChannelStatus{} }
func (stubChannel) Close() error                     { return nil }

type stubSubscription struct{}

func (stubSubscription) C() <-chan interfaces.Message { return nil }
func (stubSubscription) Close() error                 { return nil }

type stubUplink struct{}

func (stubUplink) PropertyReport(interfaces.PropertyReport) (interfaces.Message, error) {
	return interfaces.Message{}, nil
}
func (stubUplink) EventReport(interfaces.EventReport) (interfaces.Message, error) {
	return interfaces.Message{}, nil
}
func (stubUplink) DecodePayload(string, []byte, int, time.Time) ([]interfaces.Sample, error) {
	return nil, nil
}

type stubDownlink struct{}

func (stubDownlink) ParseInvocation(interfaces.Message) (interfaces.Invocation, error) {
	return interfaces.Invocation{}, nil
}
func (stubDownlink) ServiceReply(interfaces.ServiceReply) (interfaces.Message, error) {
	return interfaces.Message{}, nil
}

type stubStore struct{}

func (stubStore) BeginAccepted(context.Context, interfaces.AcceptedRecord) error { return nil }
func (stubStore) Finish(context.Context, interfaces.FinalRecord) error           { return nil }
func (stubStore) Lookup(context.Context, string, uint64) (interfaces.Record, bool, error) {
	return interfaces.Record{}, false, nil
}
func (stubStore) LastSeq(context.Context, string) (uint64, error) { return 0, nil }
func (stubStore) LoadDevices(context.Context) ([]interfaces.DeviceState, error) {
	return nil, nil
}
func (stubStore) SaveDevices(context.Context, []interfaces.DeviceState) error { return nil }
func (stubStore) LoadSnapshot(context.Context, string) ([]interfaces.Property, bool, error) {
	return nil, false, nil
}
func (stubStore) SaveSnapshot(context.Context, string, []interfaces.Property) error { return nil }
func (stubStore) DeleteExpired(context.Context, time.Time) (int, error)             { return 0, nil }
func (stubStore) Close() error                                                      { return nil }

// TestInterfaceMethodSets pins each interface's method set. A compile-time
// assertion alone cannot notice a removed method, so the names are checked
// explicitly.
func TestInterfaceMethodSets(t *testing.T) {
	cases := []struct {
		name  string
		iface any
		want  []string
	}{
		{"Clock", (*interfaces.Clock)(nil), []string{"NewTicker", "NewTimer", "Now", "Sleep"}},
		{"Ticker", (*interfaces.Ticker)(nil), []string{"C", "Stop"}},
		{"Timer", (*interfaces.Timer)(nil), []string{"C", "Stop"}},
		{"Driver", (*interfaces.Driver)(nil), []string{
			"Capabilities", "Close", "Events", "Name", "Open", "Read", "Status", "Write",
		}},
		{"Channel", (*interfaces.Channel)(nil), []string{"Close", "Publish", "Status", "Subscribe"}},
		{"Subscription", (*interfaces.Subscription)(nil), []string{"C", "Close"}},
		{"UplinkAdapter", (*interfaces.UplinkAdapter)(nil), []string{"DecodePayload", "EventReport", "PropertyReport"}},
		{"DownlinkAdapter", (*interfaces.DownlinkAdapter)(nil), []string{"ParseInvocation", "ServiceReply"}},
		{"Store", (*interfaces.Store)(nil), []string{
			"BeginAccepted", "Close", "DeleteExpired", "Finish", "LastSeq",
			"LoadDevices", "LoadSnapshot", "Lookup", "SaveDevices", "SaveSnapshot",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			typ := reflect.TypeOf(tc.iface).Elem()
			if typ.Kind() != reflect.Interface {
				t.Fatalf("%s is %s, want interface", tc.name, typ.Kind())
			}
			var got []string
			for m := range typ.Methods() {
				got = append(got, m.Name)
			}
			sort.Strings(got)
			sort.Strings(tc.want)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("method set = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestWireVisibleConstantValues(t *testing.T) {
	cases := map[string]string{
		"quality.good":     string(interfaces.QualityGood),
		"quality.stale":    string(interfaces.QualityStale),
		"quality.bad":      string(interfaces.QualityBad),
		"mode.snapshot":    string(interfaces.ReportSnapshot),
		"mode.delta":       string(interfaces.ReportDelta),
		"link.up":          string(interfaces.LinkUp),
		"link.degraded":    string(interfaces.LinkDegraded),
		"link.down":        string(interfaces.LinkDown),
		"stage.gateway_tx": string(interfaces.StageGatewayTx),
		"stage.device_mac": string(interfaces.StageDeviceMAC),
		"stage.business":   string(interfaces.StageBusiness),
		"event.uplink":     string(interfaces.EventUplink),
		"event.txack":      string(interfaces.EventTxAck),
		"event.ack":        string(interfaces.EventAck),
		"event.status":     string(interfaces.EventDeviceStatus),
		"event.driver_err": string(interfaces.EventDriverError),
	}
	want := map[string]string{
		"quality.good":     "good",
		"quality.stale":    "stale",
		"quality.bad":      "bad",
		"mode.snapshot":    "snapshot",
		"mode.delta":       "delta",
		"link.up":          "up",
		"link.degraded":    "degraded",
		"link.down":        "down",
		"stage.gateway_tx": "gateway_tx",
		"stage.device_mac": "device_mac",
		"stage.business":   "business",
		"event.uplink":     "uplink",
		"event.txack":      "txack",
		"event.ack":        "ack",
		"event.status":     "device_status",
		"event.driver_err": "driver_error",
	}
	for name, got := range cases {
		if got != want[name] {
			t.Errorf("%s = %q, want %q", name, got, want[name])
		}
	}
}
