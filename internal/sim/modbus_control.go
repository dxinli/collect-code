package sim

import (
	"errors"
	"time"
)

const modbusMaxControlDurationMS = int64(86400000)

type modbusFaultKind uint8

const (
	modbusFaultNone modbusFaultKind = iota
	modbusFaultOffline
	modbusFaultSlow
	modbusFaultBadCRC
)

type modbusFault struct {
	kind  modbusFaultKind
	delay time.Duration
	until time.Time
}

type modbusControlBase struct {
	Module     string `json:"module"`
	Action     string `json:"action"`
	UnitID     *uint8 `json:"unit_id"`
	DurationMS *int64 `json:"duration_ms"`
}

type modbusSlowControl struct {
	modbusControlBase
	DelayMS *int64 `json:"delay_ms"`
}

func (b modbusControlBase) controlEnvelope() (string, string) { return b.Module, b.Action }

func (m *Modbus) handleUnitOffline(req ControlRequest) (any, error) {
	return m.handleSimpleFault(req, "unit_offline", modbusFaultOffline)
}

func (m *Modbus) handleUnitBadCRC(req ControlRequest) (any, error) {
	return m.handleSimpleFault(req, "unit_bad_crc", modbusFaultBadCRC)
}

func (m *Modbus) handleSimpleFault(req ControlRequest, action string, kind modbusFaultKind) (any, error) {
	var body modbusControlBase
	if err := decodeModbusControl(req, &body, action); err != nil {
		return nil, err
	}
	return m.applyFault(body.UnitID, body.DurationMS, kind, 0, action)
}

func (m *Modbus) handleUnitSlow(req ControlRequest) (any, error) {
	var body modbusSlowControl
	if err := decodeModbusControl(req, &body, "unit_slow"); err != nil {
		return nil, err
	}
	if body.DelayMS == nil {
		return nil, BadRequestf("unit_slow: delay_ms is required")
	}
	delay, ok := modbusControlDuration(*body.DelayMS)
	if !ok || delay <= 0 {
		return nil, BadRequestf("unit_slow: delay_ms must be in 1..%d", modbusMaxControlDurationMS)
	}
	return m.applyFault(body.UnitID, body.DurationMS, modbusFaultSlow, delay, "unit_slow")
}

func decodeModbusControl(req ControlRequest, dst any, action string) error {
	if req.Body == nil {
		return BadRequestf("%s: missing request body", action)
	}
	if err := decodeStrict(req.Body, dst); err != nil {
		return BadRequestf("%s: invalid request: %v", action, err)
	}
	envelope, ok := dst.(interface{ controlEnvelope() (string, string) })
	if !ok {
		return errors.New("sim: unsupported modbus control body")
	}
	module, gotAction := envelope.controlEnvelope()
	if module != "modbus" {
		return BadRequestf("%s: module must be \"modbus\"", action)
	}
	if gotAction != action {
		return BadRequestf("%s: action must be %q", action, action)
	}
	return nil
}

func (m *Modbus) applyFault(unitID *uint8, durationMS *int64, kind modbusFaultKind,
	delay time.Duration, action string,
) (any, error) {
	if unitID == nil {
		return nil, BadRequestf("%s: unit_id is required", action)
	}
	if *unitID < 1 || *unitID > 247 {
		return nil, BadRequestf("%s: unit_id %d outside 1..247", action, *unitID)
	}
	var duration time.Duration
	if durationMS != nil {
		var ok bool
		duration, ok = modbusControlDuration(*durationMS)
		if !ok {
			return nil, BadRequestf("%s: duration_ms must be in 0..%d", action, modbusMaxControlDurationMS)
		}
	}

	m.mu.Lock()
	unit, ok := m.units[*unitID]
	if !ok {
		m.mu.Unlock()
		return nil, BadRequestf("%s: unknown unit_id %d", action, *unitID)
	}
	var until time.Time
	if duration > 0 {
		until = m.now().Add(duration)
	}
	unit.fault = modbusFault{kind: kind, delay: delay, until: until}
	m.mu.Unlock()

	result := map[string]any{
		"ok":      true,
		"action":  action,
		"unit_id": *unitID,
		"until":   modbusUntilValue(until),
	}
	if kind == modbusFaultSlow {
		result["delay_ms"] = delay.Milliseconds()
	}
	return result, nil
}

func modbusControlDuration(ms int64) (time.Duration, bool) {
	if ms < 0 || ms > modbusMaxControlDurationMS {
		return 0, false
	}
	return time.Duration(ms) * time.Millisecond, true
}

func modbusUntilValue(until time.Time) any {
	if until.IsZero() {
		return nil
	}
	return until.UTC().Format(time.RFC3339Nano)
}

// faultFor returns the active fault for a unit, lazily clearing an expired
// time-bounded mode. The caller must hold m.mu.
func (m *Modbus) faultFor(unit *unitState) modbusFault {
	fault := unit.fault
	if fault.kind == modbusFaultNone || fault.until.IsZero() {
		return fault
	}
	if !m.now().Before(fault.until) {
		unit.fault = modbusFault{}
		return modbusFault{}
	}
	return fault
}
