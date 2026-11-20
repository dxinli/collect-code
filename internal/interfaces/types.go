package interfaces

import "time"

// Quality is the freshness label carried on the wire.
type Quality string

const (
	QualityGood  Quality = "good"
	QualityStale Quality = "stale"
	QualityBad   Quality = "bad"
)

// Outcome is the overall result of one Driver.Read call.
type Outcome int

const (
	OutcomeOK Outcome = iota
	OutcomePartial
	OutcomeFailed
	OutcomeUnsupported
)

// CommandState tracks a downlink command through the confirmation levels.
type CommandState int

const (
	// CmdNone is the zero value, used for events that carry no command state.
	CmdNone CommandState = iota
	CmdAccepted
	CmdDelivered
	CmdCompleted
	CmdFailed
)

// EventKind classifies a DriverEvent.
type EventKind string

const (
	EventUplink       EventKind = "uplink"
	EventTxAck        EventKind = "txack"
	EventAck          EventKind = "ack"
	EventDeviceStatus EventKind = "device_status"
	EventDriverError  EventKind = "driver_error"
)

// LinkState is the lifecycle state of one southbound link.
type LinkState string

const (
	LinkUnknown  LinkState = ""
	LinkUp       LinkState = "up"
	LinkDegraded LinkState = "degraded"
	LinkDown     LinkState = "down"
)

// LinkStatus is a snapshot of one link's health.
type LinkStatus struct {
	State       LinkState
	Failures    int
	LastSuccess time.Time
	LastError   string
}

// Capabilities describes what a driver supports.
type Capabilities struct {
	Pollable   bool
	SyncVerify bool
	AsyncAck   bool
	Confirmed  bool
}

// Sample is one point observation produced by a driver.
type Sample struct {
	PointKey string
	Value    any
	Quality  Quality
	At       time.Time
	Err      string
}

// ReportMode selects full-snapshot or delta property reporting.
type ReportMode string

const (
	ReportSnapshot ReportMode = "snapshot"
	ReportDelta    ReportMode = "delta"
)

// Property is one wire-ready property value.
type Property struct {
	Key     string
	Value   any
	Unit    string
	Quality Quality
	TS      time.Time
}

// ConfirmationStage names how far a downlink command has been confirmed.
type ConfirmationStage string

const (
	StageGatewayTx ConfirmationStage = "gateway_tx"
	StageDeviceMAC ConfirmationStage = "device_mac"
	StageBusiness  ConfirmationStage = "business"
)
