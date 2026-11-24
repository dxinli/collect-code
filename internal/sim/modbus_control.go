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
	Module     string `json:"module"`
	Action     string `json:"action"`
	UnitID     *uint8 `json:"unit_id"`
	DelayMS    *int64 `json:"delay_ms"`
	DurationMS *int64 `json:"duration_ms"`
}

func (m *Modbus) handleUnitOffline(req ControlRequest) (any, error) {
	var body modbusControlBase
	if err := decodeModbusControl(req, &body, "unit_offline"); err != nil {
		return nil, err
	}
	return m.applyFault(body.UnitID, body.DurationMS, modbusFaultOffline, 0, "unit_offline")
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

func (m *Modbus) handleUnitBadCRC(req ControlRequest) (any, error) {
	var body modbusControlBase
	if err := decodeModbusControl(req, &body, "unit_bad_crc"); err != nil {
		return nil, err
	}
	return m.applyFault(body.UnitID, body.DurationMS, modbusFaultBadCRC, 0, "unit_bad_crc")
}

func decodeModbusControl(req ControlRequest, dst any, action string) error {
	if req.Body == nil {
		return BadRequestf("%s: missing request body", action)
	}
	if err := decodeStrict(req.Body, dst); err != nil {
		return BadRequestf("%s: invalid request: %v", action, err)
	}
	var module, gotAction string
	switch v := dst.(type) {
	case *modbusControlBase:
		module, gotAction = v.Module, v.Action
	case *modbusSlowControl:
		module, gotAction = v.Module, v.Action
	default:
		return errors.New("sim: unsupported modbus control body")
	}
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

func (f modbusFault) String() string {
	switch f.kind {
	case modbusFaultOffline:
		return "offline"
	case modbusFaultSlow:
		return "slow"
	case modbusFaultBadCRC:
		return "bad_crc"
	default:
		return "none"
	}
}
