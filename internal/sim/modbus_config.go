package sim

import (
	"fmt"

	"github.com/dxinli/collect-code/internal/config"
)

// loadModbusConfig derives the served units and register access from the same
// four-layer configuration the engine uses. Only modbus-tcp and
// modbus-rtu-tcp links are simulated; serial modbus-rtu devices are skipped
// because the simulator only listens on TCP.
func loadModbusConfig(dir string) (map[uint8]*unitState, error) {
	bundle, err := config.Load(dir)
	if err != nil {
		return nil, fmt.Errorf("sim: load config %s: %w", dir, err)
	}

	linkDriver := make(map[string]string, len(bundle.Links))
	for _, link := range bundle.Links {
		linkDriver[link.ID] = link.Driver
	}

	registers := make(map[uint8]map[modbusKey]modbusAccess)
	widths := make(map[uint8]map[modbusKey]int)
	declared := make(map[uint8]map[modbusKey]modbusAccess)

	declare := func(unitID uint8, key modbusKey, access modbusAccess) error {
		byKey := declared[unitID]
		if byKey == nil {
			byKey = make(map[modbusKey]modbusAccess)
			declared[unitID] = byKey
		}
		if previous, ok := byKey[key]; ok && previous != access {
			return fmt.Errorf("sim: modbus unit %d register 0x%04X has conflicting access 0x%02X and 0x%02X",
				unitID, key.address, previous, access)
		}
		byKey[key] = access
		return nil
	}

	add := func(unitID uint8, key modbusKey, access modbusAccess, width int) error {
		byKey := registers[unitID]
		if byKey == nil {
			byKey = make(map[modbusKey]modbusAccess)
			registers[unitID] = byKey
		}
		byWidth := widths[unitID]
		if byWidth == nil {
			byWidth = make(map[modbusKey]int)
			widths[unitID] = byWidth
		}
		if existing, ok := byWidth[key]; ok && existing != width {
			return fmt.Errorf("sim: modbus unit %d register 0x%04X has conflicting widths %d and %d",
				unitID, key.address, existing, width)
		}
		byWidth[key] = width
		byKey[key] |= access
		return nil
	}

	for _, device := range bundle.Devices {
		driver := linkDriver[device.Link]
		if driver != "modbus-tcp" && driver != "modbus-rtu-tcp" {
			continue
		}
		if device.Address == nil || device.Address.UnitID == nil {
			return nil, fmt.Errorf("sim: device %s on %s link %s has no unit_id",
				device.SN, driver, device.Link)
		}
		unitID, err := validateUnitID(*device.Address.UnitID)
		if err != nil {
			return nil, fmt.Errorf("sim: device %s: %w", device.SN, err)
		}
		profile := bundle.Profiles[device.Profile]
		if profile == nil {
			return nil, fmt.Errorf("sim: device %s references unknown profile %q", device.SN, device.Profile)
		}
		for _, point := range profile.Points {
			if point.Modbus == nil {
				continue
			}
			if point.Modbus.Func == nil || point.Modbus.Address == nil {
				return nil, fmt.Errorf("sim: device %s point %s has an incomplete modbus mapping",
					device.SN, point.Key)
			}
			function := *point.Modbus.Func
			address := *point.Modbus.Address
			if address < 0 || address > 0xFFFF {
				return nil, fmt.Errorf("sim: device %s point %s address %d outside 0..65535",
					device.SN, point.Key, address)
			}
			input := false
			switch function {
			case 3:
			case 4:
				input = true
			default:
				return nil, fmt.Errorf("sim: device %s point %s has unsupported read function %d",
					device.SN, point.Key, function)
			}
			width := modbusPointWidth(point.Modbus.Type)
			type declaredRegister struct {
				key    modbusKey
				access modbusAccess
				width  int
			}
			var declarations []declaredRegister
			for i := range width {
				declarations = append(declarations, declaredRegister{
					key:    modbusKey{input: input, address: uint16(address) + uint16(i)},
					access: modbusCanRead,
					width:  width,
				})
			}
			if point.Write != nil {
				if point.Write.Func == nil || point.Write.Address == nil {
					return nil, fmt.Errorf("sim: device %s point %s has an incomplete write mapping",
						device.SN, point.Key)
				}
				writeFunc := *point.Write.Func
				if writeFunc != modbusFuncWriteSingle && writeFunc != modbusFuncWriteMultiple {
					return nil, fmt.Errorf("sim: device %s point %s has unsupported write function %d",
						device.SN, point.Key, writeFunc)
				}
				writeAddress := *point.Write.Address
				if writeAddress < 0 || writeAddress > 0xFFFF {
					return nil, fmt.Errorf("sim: device %s point %s write address %d outside 0..65535",
						device.SN, point.Key, writeAddress)
				}
				writeWidth := width
				if writeFunc == modbusFuncWriteSingle {
					writeWidth = 1
				}
				for i := range writeWidth {
					key := modbusKey{address: uint16(writeAddress) + uint16(i)}
					merged := false
					for j := range declarations {
						if declarations[j].key == key {
							declarations[j].access |= modbusCanWrite
							merged = true
							break
						}
					}
					if !merged {
						declarations = append(declarations, declaredRegister{
							key:    key,
							access: modbusCanWrite,
							width:  writeWidth,
						})
					}
				}
			}
			for _, declaration := range declarations {
				if err := declare(unitID, declaration.key, declaration.access); err != nil {
					return nil, err
				}
				if err := add(unitID, declaration.key, declaration.access, declaration.width); err != nil {
					return nil, err
				}
			}
		}
	}

	units := make(map[uint8]*unitState, len(registers))
	for unitID, extra := range registers {
		units[unitID] = newUnitState(unitID, extra)
	}
	return units, nil
}

func modbusPointWidth(typ string) int {
	switch typ {
	case "uint32", "int32":
		return 2
	default:
		return 1
	}
}
