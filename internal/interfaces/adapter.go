package interfaces

import "time"

// UplinkAdapter translates kernel state into northbound messages.
type UplinkAdapter interface {
	PropertyReport(report PropertyReport) (Message, error)
	EventReport(report EventReport) (Message, error)
	// DecodePayload must stamp every returned Sample.At with at, which comes
	// from the driver event and therefore from the injected Clock.
	DecodePayload(device string, payload []byte, fPort int, at time.Time) ([]Sample, error)
}

// PropertyReport is one property snapshot or delta.
type PropertyReport struct {
	MsgID      string
	Mode       ReportMode
	ProductKey string
	DeviceSN   string
	DeviceType string
	TS         time.Time
	Online     bool
	Properties []Property
}

// EventReport is one event instance.
type EventReport struct {
	MsgID    string
	DedupKey string
	DeviceSN string
	EventID  string
	Episode  uint64
	Level    string
	TS       time.Time
	Data     map[string]any
}

// DownlinkAdapter translates inbound invocations and outgoing replies.
type DownlinkAdapter interface {
	ParseInvocation(msg Message) (Invocation, error)
	ServiceReply(reply ServiceReply) (Message, error)
}

// Invocation is a parsed service.invoke message.
type Invocation struct {
	MsgID     string
	DeviceSN  string
	ServiceID string
	Seq       uint64
	Params    map[string]any
	Timeout   time.Duration
	IssuedAt  time.Time
}

// ServiceReply is the result returned to the cloud.
//
// Stage is the typed source of data.stage: a caller filling Data["stage"]
// must derive it from Stage rather than formatting the string again.
type ServiceReply struct {
	MsgID     string
	DeviceSN  string
	ServiceID string
	Seq       uint64
	Code      int
	Msg       string
	Stage     ConfirmationStage // business when code==0; gateway_tx/device_mac for 5041
	Data      map[string]any
	Replay    bool
	Elapsed   time.Duration
}
