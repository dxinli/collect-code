package sim

import (
	"fmt"
	"sort"
)

type modbusAccess uint8

const (
	modbusCanRead modbusAccess = 1 << iota
	modbusCanWrite
)

// modbusKey identifies a register in either the holding (input=false) or
// input (input=true) address space.
type modbusKey struct {
	input   bool
	address uint16
}

type modbusRegister struct {
	access modbusAccess
	value  uint16
	name   string
}

// unitState is one simulated slave. All access is guarded by Modbus.mu.
type unitState struct {
	unitID    uint8
	registers map[modbusKey]modbusRegister
	fault     modbusFault
}

// builtinModbusRegisters is the ASSUMED register table from
// docs/缺口台账.md (B5/C5). Values are raw uint16 words; the engine applies
// scaling. The 32-bit values use two big-endian registers.
var builtinModbusRegisters = []struct {
	key    modbusKey
	access modbusAccess
	value  uint16
	name   string
}{
	{modbusKey{address: 0x0000}, modbusCanRead, 0, "state_word"},
	{modbusKey{address: 0x0001}, modbusCanRead, 500, "current_opening"},
	{modbusKey{address: 0x0002}, modbusCanRead | modbusCanWrite, 500, "target_opening"},
	{modbusKey{address: 0x0003}, modbusCanWrite, 0, "valve_command"},
	{modbusKey{address: 0x0009}, modbusCanRead, 0, "fault_code"},
	{modbusKey{address: 0x000A}, modbusCanRead, 0, "comm_timeout_count"},
	{modbusKey{address: 0x000B}, modbusCanRead, 0x0102, "firmware_version"},

	{modbusKey{input: true, address: 0x0000}, modbusCanRead, 185, "soil_temperature"},
	{modbusKey{input: true, address: 0x0001}, modbusCanRead, 352, "soil_moisture"},
	{modbusKey{input: true, address: 0x0002}, modbusCanRead, 780, "soil_ec"},
	{modbusKey{input: true, address: 0x0003}, modbusCanRead, 680, "soil_ph"},
	{modbusKey{input: true, address: 0x0004}, modbusCanRead, 215, "medium_temperature"},
	{modbusKey{input: true, address: 0x0005}, modbusCanRead, 42, "pressure"},
	{modbusKey{input: true, address: 0x0006}, modbusCanRead, 0, "instantaneous_flow_hi"},
	{modbusKey{input: true, address: 0x0007}, modbusCanRead, 123, "instantaneous_flow_lo"},
	{modbusKey{input: true, address: 0x000C}, modbusCanRead, 0, "total_flow_hi"},
	{modbusKey{input: true, address: 0x000D}, modbusCanRead, 1000, "total_flow_lo"},
}

func builtinUnitIDs() []uint8 {
	ids := make([]uint8, 0, 8)
	for id := uint8(1); id <= 8; id++ {
		ids = append(ids, id)
	}
	return ids
}

// newUnitState builds a unit from the builtin ASSUMED table, then unions in
// any access declared by --config. Values stay builtin where an address is
// known; config-only addresses start at zero.
func newUnitState(unitID uint8, extra map[modbusKey]modbusAccess) *unitState {
	registers := make(map[modbusKey]modbusRegister, len(builtinModbusRegisters)+len(extra))
	for _, def := range builtinModbusRegisters {
		registers[def.key] = modbusRegister{access: def.access, value: def.value, name: def.name}
	}
	for key, access := range extra {
		reg, ok := registers[key]
		if ok {
			reg.access |= access
			registers[key] = reg
			continue
		}
		registers[key] = modbusRegister{access: access}
	}
	return &unitState{unitID: unitID, registers: registers}
}

func (u *unitState) read(input bool, address, quantity uint16) ([]uint16, uint8) {
	if quantity < 1 || quantity > modbusMaxReadRegisters {
		return nil, modbusExceptionIllegalDataValue
	}
	if int(address)+int(quantity) > 1<<16 {
		return nil, modbusExceptionIllegalDataAddress
	}
	values := make([]uint16, quantity)
	for i := range values {
		key := modbusKey{input: input, address: address + uint16(i)}
		reg, ok := u.registers[key]
		if !ok || reg.access&modbusCanRead == 0 {
			return nil, modbusExceptionIllegalDataAddress
		}
		values[i] = reg.value
	}
	return values, 0
}

func (u *unitState) writeSingle(address, value uint16) uint8 {
	key := modbusKey{address: address}
	reg, ok := u.registers[key]
	if !ok || reg.access&modbusCanWrite == 0 {
		return modbusExceptionIllegalDataAddress
	}
	u.applyWrite(key, value)
	return 0
}

func (u *unitState) writeMultiple(address uint16, values []uint16) uint8 {
	if len(values) < 1 || len(values) > modbusMaxWriteRegisters {
		return modbusExceptionIllegalDataValue
	}
	if int(address)+len(values) > 1<<16 {
		return modbusExceptionIllegalDataAddress
	}
	for i := range values {
		key := modbusKey{address: address + uint16(i)}
		reg, ok := u.registers[key]
		if !ok || reg.access&modbusCanWrite == 0 {
			return modbusExceptionIllegalDataAddress
		}
	}
	for i, value := range values {
		u.applyWrite(modbusKey{address: address + uint16(i)}, value)
	}
	return 0
}

// applyWrite stores the raw value and applies the ASSUMED valve semantics:
// writing the target opening makes the current opening follow immediately and
// leaves the device non-running; writing the command register drives both to
// the requested end position. The immediate follow is what makes the engine's
// write-then-read verification deterministic.
func (u *unitState) applyWrite(key modbusKey, value uint16) {
	u.set(key, value)
	if key.input {
		return
	}
	switch key.address {
	case 0x0002:
		u.set(modbusKey{address: 0x0001}, value)
		u.set(modbusKey{address: 0x0000}, 0)
	case 0x0003:
		switch value {
		case 1:
			u.set(modbusKey{address: 0x0001}, 1000)
			u.set(modbusKey{address: 0x0002}, 1000)
			u.set(modbusKey{address: 0x0000}, 0)
		case 2:
			u.set(modbusKey{address: 0x0001}, 0)
			u.set(modbusKey{address: 0x0002}, 0)
			u.set(modbusKey{address: 0x0000}, 0)
		}
	}
}

func (u *unitState) set(key modbusKey, value uint16) {
	if reg, ok := u.registers[key]; ok {
		reg.value = value
		u.registers[key] = reg
	}
}

func sortedUnitIDs(units map[uint8]*unitState) []int {
	ids := make([]int, 0, len(units))
	for id := range units {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
	return ids
}

func validateUnitID(id uint16) (uint8, error) {
	if id < 1 || id > 247 {
		return 0, fmt.Errorf("sim: modbus unit_id %d outside 1..247", id)
	}
	return uint8(id), nil
}
