package config

import (
	"io/fs"
	"maps"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func baseParts(t *testing.T) (*Bundle, []loadedProfile, map[string]*positionIndex) {
	t.Helper()
	const dir = "testdata/config/base"
	cfg := Default()
	b := &Bundle{
		Config:         &cfg,
		Profiles:       map[string]*Profile{},
		ProfileSources: map[string]string{},
	}
	positions := map[string]*positionIndex{}
	var ds Diagnostics

	pi, d := parseFile(dir, "config.yaml", b.Config)
	positions["config.yaml"] = pi
	ds = append(ds, d...)
	var links LinksFile
	pi, d = parseFile(dir, "links.yaml", &links)
	positions["links.yaml"] = pi
	ds = append(ds, d...)
	var devices DevicesFile
	pi, d = parseFile(dir, "devices.yaml", &devices)
	positions["devices.yaml"] = pi
	ds = append(ds, d...)
	loaded, sources, ppos, d := parseProfiles(dir)
	ds = append(ds, d...)
	maps.Copy(positions, ppos)
	if len(ds) > 0 {
		t.Fatalf("base fixture is invalid: %v", ds)
	}
	b.Links = links.Links
	b.Devices = devices.Devices
	b.ProfileSources = sources
	for _, lp := range loaded {
		if lp.profile.Profile != "" {
			b.Profiles[lp.profile.Profile] = lp.profile
		}
	}
	return b, loaded, positions
}

func findLoaded(t *testing.T, loaded []loadedProfile, name string) *Profile {
	t.Helper()
	for _, lp := range loaded {
		if lp.profile.Profile == name {
			return lp.profile
		}
	}
	t.Fatalf("profile %q not found", name)
	return nil
}

func validateWith(t *testing.T, mutate func(*Bundle, []loadedProfile)) Diagnostics {
	t.Helper()
	b, loaded, positions := baseParts(t)
	if mutate != nil {
		mutate(b, loaded)
	}
	return validate(b, loaded, positions)
}

func wantRule(t *testing.T, ds Diagnostics, rule, substr string) {
	t.Helper()
	for _, d := range ds {
		if d.RuleID == rule && strings.Contains(d.Message, substr) {
			return
		}
	}
	t.Fatalf("missing %s containing %q in:\n%v", rule, substr, ds)
}

func wantNoRule(t *testing.T, ds Diagnostics, rule string) {
	t.Helper()
	for _, d := range ds {
		if d.RuleID == rule {
			t.Fatalf("unexpected %s: %v", rule, ds)
		}
	}
}

func TestValidateRules(t *testing.T) {
	cases := []struct {
		name   string
		rule   string
		substr string
		mutate func(*Bundle, []loadedProfile)
	}{
		{"V10 required config", RuleRequiredConfig, "node.id: required", func(b *Bundle, _ []loadedProfile) {
			b.Config.Node.ID = ""
		}},
		{"V11 required transport", RuleRequiredLink, "one of serial/tcp/chirpstack", func(b *Bundle, _ []loadedProfile) {
			b.Links[0].Serial = nil
			b.Links[0].TCP = nil
			b.Links[0].Chirpstack = nil
		}},
		{"V12 required address", RuleRequiredDevice, "devices[0].address: required", func(b *Bundle, _ []loadedProfile) {
			b.Devices[0].Address = nil
		}},
		{"V13 required profile name", RuleRequiredProfile, "profile: required", func(_ *Bundle, loaded []loadedProfile) {
			for _, lp := range loaded {
				if lp.profile.Profile == "sensor-soil" {
					lp.profile.Profile = ""
				}
			}
		}},
		{"V14 point key", RuleRequiredPointKey, "points[0].key: required", func(_ *Bundle, loaded []loadedProfile) {
			findLoaded(t, loaded, "sensor-soil").Points[0].Key = ""
		}},
		{"V15 service id", RuleRequiredServiceID, "services[0].service_id: required", func(_ *Bundle, loaded []loadedProfile) {
			findLoaded(t, loaded, "valve-modbus").Services[0].ServiceID = ""
		}},
		{"V16 event id", RuleRequiredEventID, "events[0].event_id: required", func(_ *Bundle, loaded []loadedProfile) {
			findLoaded(t, loaded, "valve-modbus").Events[0].EventID = ""
		}},
		{"V20 duplicate link", RuleUniqueLinkID, "duplicate link id", func(b *Bundle, _ []loadedProfile) {
			dup := b.Links[0]
			b.Links = append(b.Links, dup)
		}},
		{"V21 duplicate device", RuleUniqueDeviceSN, "duplicate device sn", func(b *Bundle, _ []loadedProfile) {
			dup := b.Devices[0]
			b.Devices = append(b.Devices, dup)
		}},
		{"V22 duplicate point", RuleUniquePointKey, "duplicate point key", func(_ *Bundle, loaded []loadedProfile) {
			p := findLoaded(t, loaded, "sensor-soil")
			p.Points = append(p.Points, p.Points[0])
		}},
		{"V23 duplicate service", RuleUniqueServiceID, "duplicate service_id", func(_ *Bundle, loaded []loadedProfile) {
			p := findLoaded(t, loaded, "valve-modbus")
			p.Services = append(p.Services, p.Services[0])
		}},
		{"V24 duplicate event", RuleUniqueEventID, "duplicate event_id", func(_ *Bundle, loaded []loadedProfile) {
			p := findLoaded(t, loaded, "valve-modbus")
			p.Events = append(p.Events, p.Events[0])
		}},
		{"V30 profile ref", RuleProfileRef, `device profile "missing"`, func(b *Bundle, _ []loadedProfile) {
			b.Devices[0].Profile = "missing"
		}},
		{"V31 link ref", RuleLinkRef, `device link "missing"`, func(b *Bundle, _ []loadedProfile) {
			b.Devices[0].Link = "missing"
		}},
		{"V32 write point", RuleWritePointMissing, `write point "missing"`, func(_ *Bundle, loaded []loadedProfile) {
			findLoaded(t, loaded, "valve-modbus").Services[0].Writes[0].Point = "missing"
		}},
		{"V33 read-only write", RuleWritePointReadOnly, "is read-only", func(_ *Bundle, loaded []loadedProfile) {
			findLoaded(t, loaded, "valve-modbus").Services[0].Writes[0].Point = "opening"
		}},
		{"V34 verify point", RuleVerifyPointMissing, `verify point "missing"`, func(_ *Bundle, loaded []loadedProfile) {
			findLoaded(t, loaded, "valve-modbus").Services[0].Writes[0].Verify.Point = "missing"
		}},
		{"V35 verify unreadable", RuleVerifyPointUnreadable, "is not readable", func(_ *Bundle, loaded []loadedProfile) {
			findLoaded(t, loaded, "valve-lora").Services[0].Verify.Point = "target_opening"
		}},
		{"V40 min max", RuleMinMax, "min must be <= max", func(_ *Bundle, loaded []loadedProfile) {
			p := findLoaded(t, loaded, "valve-modbus")
			min, max := 101.0, 100.0
			p.Points[1].Modbus.Min, p.Points[1].Modbus.Max = &min, &max
		}},
		{"V41 scale", RuleScale, "scale must be finite and > 0", func(_ *Bundle, loaded []loadedProfile) {
			p := findLoaded(t, loaded, "valve-modbus")
			zero := 0.0
			p.Points[1].Modbus.Scale = &zero
		}},
		{"V42 poll", RulePollInterval, "poll_interval_s must be >= 1", func(_ *Bundle, loaded []loadedProfile) {
			zero := 0
			findLoaded(t, loaded, "sensor-soil").PollIntervalS = &zero
		}},
		{"V43 freshness", RuleFreshnessWindow, "freshness_window_s must be > 0", func(b *Bundle, _ []loadedProfile) {
			b.Config.Timing.FreshnessWindowS = 0
		}},
		{"V44 offline", RuleOfflineIntervals, "offline_after_intervals must be >= 1", func(b *Bundle, _ []loadedProfile) {
			b.Config.Timing.OfflineAfterIntervals = 0
		}},
		{"V45 qos", RuleQoS, "qos must be 0 or 1", func(b *Bundle, _ []loadedProfile) {
			b.Config.Northbound.MQTT.QoS = 2
		}},
		{"V50 driver", RuleDriver, "unknown driver", func(b *Bundle, _ []loadedProfile) {
			b.Links[0].Driver = "bogus"
		}},
		{"V51 transport", RuleDriverTransport, "requires chirpstack block", func(b *Bundle, _ []loadedProfile) {
			b.Links[0].Driver = "lorawan"
			b.Links[0].Chirpstack = nil
		}},
		{"V52 parity", RuleParity, "unknown parity", func(b *Bundle, _ []loadedProfile) {
			b.Links[0].Serial.Parity = "mark"
		}},
		{"V53 access", RuleAccess, "unknown access", func(_ *Bundle, loaded []loadedProfile) {
			findLoaded(t, loaded, "valve-modbus").Points[0].Access = "ro"
		}},
		{"V60 port", RuleHealthPort, "ChirpStack port 8080", func(b *Bundle, _ []loadedProfile) {
			b.Config.Health.Addr = "127.0.0.1:8080"
		}},
		{"V62 log level", RuleLogLevel, "unsupported log.level", func(b *Bundle, _ []loadedProfile) {
			b.Config.Log.Level = ""
		}},
		{"V63 log format", RuleLogFormat, "unsupported log.format", func(b *Bundle, _ []loadedProfile) {
			b.Config.Log.Format = "text"
		}},
		{"V70 device type mismatch", RuleDeviceTypeMismatch, `does not match profile "sensor-soil"`, func(b *Bundle, _ []loadedProfile) {
			b.Devices[0].DeviceType = "valve"
		}},
		{"V71 device type enum", RuleDeviceTypeEnum, `unknown device_type "pump"`, func(b *Bundle, _ []loadedProfile) {
			b.Devices[0].DeviceType = "pump"
		}},
		{"V71 profile type enum", RuleDeviceTypeEnum, `unknown device_type "pump"`, func(_ *Bundle, loaded []loadedProfile) {
			findLoaded(t, loaded, "sensor-soil").DeviceType = "pump"
		}},
		{"V72 trigger point missing", RuleEventTriggerPoint, `trigger point "missing" not found`, func(_ *Bundle, loaded []loadedProfile) {
			findLoaded(t, loaded, "valve-modbus").Events[0].Trigger.Point = "missing"
		}},
		{"V72 trigger point unreadable", RuleEventTriggerPoint, "only present in payload.encode", func(_ *Bundle, loaded []loadedProfile) {
			findLoaded(t, loaded, "valve-lora").Events[0].Trigger.Point = "target_opening"
		}},
		{"V73 from undeclared", RuleWriteFromParam, `from param "missing" is not declared`, func(_ *Bundle, loaded []loadedProfile) {
			findLoaded(t, loaded, "valve-modbus").Services[0].Writes[0].From = "params.missing"
		}},
		{"V73 from malformed", RuleWriteFromParam, "from must be params.<name>", func(_ *Bundle, loaded []loadedProfile) {
			findLoaded(t, loaded, "valve-modbus").Services[0].Writes[0].From = "opening"
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wantRule(t, validateWith(t, tc.mutate), tc.rule, tc.substr)
		})
	}
}

func TestNumericCoversParamsAndPayload(t *testing.T) {
	ds := validateWith(t, func(_ *Bundle, loaded []loadedProfile) {
		p := findLoaded(t, loaded, "valve-modbus")
		min, max := 101.0, 100.0
		spec := p.Services[0].Params["opening"]
		spec.Min, spec.Max = &min, &max
		p.Services[0].Params["opening"] = spec
	})
	wantRule(t, ds, RuleMinMax, "min must be <= max")

	ds = validateWith(t, func(_ *Bundle, loaded []loadedProfile) {
		p := findLoaded(t, loaded, "valve-lora")
		zero := 0.0
		p.Payload.Decode[0].Scale = &zero
	})
	wantRule(t, ds, RuleScale, "scale must be finite and > 0")
}

func TestHealthConflictOnlyForLorawan(t *testing.T) {
	ds := validateWith(t, func(b *Bundle, _ []loadedProfile) {
		b.Config.Health.Addr = "127.0.0.1:9999"
		b.Links[0].Chirpstack = &ChirpstackConfig{GRPCAddr: "127.0.0.1:9999"}
	})
	for _, d := range ds {
		if d.RuleID == RuleHealthPort && strings.Contains(d.Message, "conflicts") {
			t.Fatalf("non-lorawan chirpstack block must not conflict: %v", d)
		}
	}

	ds = validateWith(t, func(b *Bundle, _ []loadedProfile) {
		b.Config.Health.Addr = "127.0.0.1:9999"
		b.Links[0].Driver = "lorawan"
		b.Links[0].Chirpstack = &ChirpstackConfig{GRPCAddr: "127.0.0.1:9999"}
	})
	wantRule(t, ds, RuleHealthPort, "conflicts with link")
}

func TestHealthInvalidGRPCSkipped(t *testing.T) {
	ds := validateWith(t, func(b *Bundle, _ []loadedProfile) {
		b.Links[2].Chirpstack.GRPCAddr = "not-a-host-port"
	})
	for _, d := range ds {
		if d.RuleID == RuleHealthPort {
			t.Fatalf("invalid grpc_addr must be skipped by V60: %v", d)
		}
	}
}

func TestReadOnlyWriteDoesNotDoubleReportInvalidAccess(t *testing.T) {
	ds := validateWith(t, func(_ *Bundle, loaded []loadedProfile) {
		p := findLoaded(t, loaded, "valve-modbus")
		p.Points[0].Access = "bogus"
		p.Services[0].Writes[0].Point = "opening"
	})
	if len(ds) != 1 || ds[0].RuleID != RuleAccess {
		t.Fatalf("want only V53, got %v", ds)
	}
}

func TestV40AcceptsEqualBoundsAndRejectsNonFinite(t *testing.T) {
	ds := validateWith(t, func(_ *Bundle, loaded []loadedProfile) {
		p := findLoaded(t, loaded, "valve-modbus")
		eq := 100.0
		p.Points[1].Modbus.Min, p.Points[1].Modbus.Max = &eq, &eq
	})
	if len(ds) != 0 {
		t.Fatalf("min==max should be valid: %v", ds)
	}

	ds = validateWith(t, func(_ *Bundle, loaded []loadedProfile) {
		p := findLoaded(t, loaded, "valve-modbus")
		bad := math.NaN()
		p.Points[1].Modbus.Min = &bad
	})
	wantRule(t, ds, RuleMinMax, "finite")
}

func TestDeviceTypeConsistencyGuards(t *testing.T) {
	t.Run("profile missing skips V70", func(t *testing.T) {
		ds := validateWith(t, func(b *Bundle, _ []loadedProfile) {
			b.Devices[0].Profile = "missing"
		})
		wantRule(t, ds, RuleProfileRef, `device profile "missing"`)
		wantNoRule(t, ds, RuleDeviceTypeMismatch)
	})

	t.Run("empty profile type skips V70 and V71", func(t *testing.T) {
		ds := validateWith(t, func(_ *Bundle, loaded []loadedProfile) {
			findLoaded(t, loaded, "sensor-soil").DeviceType = ""
		})
		wantNoRule(t, ds, RuleDeviceTypeMismatch)
		wantNoRule(t, ds, RuleDeviceTypeEnum)
	})

	t.Run("empty device type skips V70 and V71", func(t *testing.T) {
		ds := validateWith(t, func(b *Bundle, _ []loadedProfile) {
			b.Devices[0].DeviceType = ""
		})
		wantRule(t, ds, RuleRequiredDevice, "devices[0].device_type: required")
		wantNoRule(t, ds, RuleDeviceTypeMismatch)
		wantNoRule(t, ds, RuleDeviceTypeEnum)
	})

	t.Run("invalid profile type only reports V71", func(t *testing.T) {
		ds := validateWith(t, func(_ *Bundle, loaded []loadedProfile) {
			findLoaded(t, loaded, "sensor-soil").DeviceType = "pump"
		})
		wantRule(t, ds, RuleDeviceTypeEnum, `unknown device_type "pump"`)
		wantNoRule(t, ds, RuleDeviceTypeMismatch)
	})

	t.Run("invalid device type only reports V71", func(t *testing.T) {
		ds := validateWith(t, func(b *Bundle, _ []loadedProfile) {
			b.Devices[0].DeviceType = "pump"
		})
		wantRule(t, ds, RuleDeviceTypeEnum, `unknown device_type "pump"`)
		wantNoRule(t, ds, RuleDeviceTypeMismatch)
	})

	t.Run("gateway is accepted", func(t *testing.T) {
		ds := validateWith(t, func(b *Bundle, loaded []loadedProfile) {
			b.Devices[0].DeviceType = "gateway"
			b.Devices[1].DeviceType = "gateway"
			findLoaded(t, loaded, "sensor-soil").DeviceType = "gateway"
		})
		wantNoRule(t, ds, RuleDeviceTypeMismatch)
		wantNoRule(t, ds, RuleDeviceTypeEnum)
	})
}

func TestEventTriggerPoint(t *testing.T) {
	t.Run("non point_changed is ignored", func(t *testing.T) {
		ds := validateWith(t, func(_ *Bundle, loaded []loadedProfile) {
			ev := &findLoaded(t, loaded, "valve-modbus").Events[0]
			ev.Trigger.Type = "device_offline"
			ev.Trigger.Point = "missing"
		})
		wantNoRule(t, ds, RuleEventTriggerPoint)
	})

	t.Run("empty trigger type is ignored", func(t *testing.T) {
		ds := validateWith(t, func(_ *Bundle, loaded []loadedProfile) {
			ev := &findLoaded(t, loaded, "valve-modbus").Events[0]
			ev.Trigger.Type = ""
			ev.Trigger.Point = "missing"
		})
		wantNoRule(t, ds, RuleEventTriggerPoint)
	})

	t.Run("empty point is required", func(t *testing.T) {
		ds := validateWith(t, func(_ *Bundle, loaded []loadedProfile) {
			findLoaded(t, loaded, "valve-modbus").Events[0].Trigger.Point = ""
		})
		wantRule(t, ds, RuleEventTriggerPoint, "trigger point is required")
	})

	t.Run("decode point is readable", func(t *testing.T) {
		wantNoRule(t, validateWith(t, nil), RuleEventTriggerPoint)
	})

	t.Run("encode-only point is not readable", func(t *testing.T) {
		ds := validateWith(t, func(_ *Bundle, loaded []loadedProfile) {
			findLoaded(t, loaded, "valve-lora").Events[0].Trigger.Point = "target_opening"
		})
		wantRule(t, ds, RuleEventTriggerPoint, "only present in payload.encode")
	})

	t.Run("unset access is not readable", func(t *testing.T) {
		ds := validateWith(t, func(_ *Bundle, loaded []loadedProfile) {
			p := findLoaded(t, loaded, "valve-modbus")
			p.Points = append(p.Points, Point{Key: "unset"})
			p.Events[0].Trigger.Point = "unset"
		})
		wantRule(t, ds, RuleEventTriggerPoint, "access is unset")
	})

	t.Run("invalid access is only V53", func(t *testing.T) {
		ds := validateWith(t, func(_ *Bundle, loaded []loadedProfile) {
			p := findLoaded(t, loaded, "valve-modbus")
			p.Points = append(p.Points, Point{Key: "bogus", Access: "bogus"})
			p.Events[0].Trigger.Point = "bogus"
		})
		wantRule(t, ds, RuleAccess, `unknown access "bogus"`)
		wantNoRule(t, ds, RuleEventTriggerPoint)
	})
}

func TestWriteFromParam(t *testing.T) {
	setFrom := func(from string) func(*Bundle, []loadedProfile) {
		return func(_ *Bundle, loaded []loadedProfile) {
			findLoaded(t, loaded, "valve-modbus").Services[0].Writes[0].From = from
		}
	}

	cases := []struct {
		name   string
		from   string
		substr string
	}{
		{"empty", "", "from must be params.<name>"},
		{"missing prefix", "opening", "from must be params.<name>"},
		{"empty name", "params.", "from must be params.<name>"},
		{"wrong prefix case", "Params.opening", "from must be params.<name>"},
		{"leading space", " params.opening", "from must be params.<name>"},
		{"trailing space", "params.opening ", `from param "opening " is not declared`},
		{"undeclared", "params.missing", `from param "missing" is not declared`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wantRule(t, validateWith(t, setFrom(tc.from)), RuleWriteFromParam, tc.substr)
		})
	}

	t.Run("dotted name declared", func(t *testing.T) {
		ds := validateWith(t, func(_ *Bundle, loaded []loadedProfile) {
			p := findLoaded(t, loaded, "valve-modbus")
			p.Services[0].Params["a.b"] = ParamSpec{Type: "number"}
			p.Services[0].Writes[0].From = "params.a.b"
		})
		wantNoRule(t, ds, RuleWriteFromParam)
	})

	t.Run("base is valid", func(t *testing.T) {
		wantNoRule(t, validateWith(t, nil), RuleWriteFromParam)
	})
}

func TestReferenceLocations(t *testing.T) {
	cases := []struct {
		name    string
		file    string
		old     string
		new     string
		wantPos string
	}{
		{"V30 profile", "devices.yaml", "profile: sensor-soil", "profile: missing-profile", "devices.yaml:4:5: V30:"},
		{"V31 link", "devices.yaml", "link: sensors-485", "link: missing-link", "devices.yaml:5:5: V31:"},
		{"V32 write point", "profiles/valve-modbus.yaml", "      - point: target_opening", "      - point: missing", "profiles/valve-modbus.yaml:29:9: V32:"},
		{"V34 verify point", "profiles/valve-modbus.yaml", "verify: {point: opening, tolerance: 1.0}", "verify: {point: missing, tolerance: 1.0}", "profiles/valve-modbus.yaml:31:18: V34:"},
		{"V70 device type mismatch", "devices.yaml", "profile: sensor-soil", "profile: valve-modbus", "devices.yaml:3:5: V70:"},
		{"V71 device type enum", "devices.yaml", "device_type: sensor", "device_type: pump", "devices.yaml:3:5: V71:"},
		{"V72 trigger point", "profiles/valve-lora.yaml", "point: fault_code", "point: target_opening", "profiles/valve-lora.yaml:28:36: V72:"},
		{"V73 write from", "profiles/valve-modbus.yaml", "from: params.opening", "from: params.missing", "profiles/valve-modbus.yaml:30:9: V73:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := copyTree(t, "testdata/config/base")
			writeRel(t, dir, tc.file, strings.Replace(readRel(t, dir, tc.file), tc.old, tc.new, 1))
			_, err := Load(dir)
			if err == nil {
				t.Fatal("expected reference error")
			}
			if !strings.Contains(err.Error(), tc.wantPos) {
				t.Fatalf("want %s, got %v", tc.wantPos, err)
			}
		})
	}
}

func copyTree(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copy tree: %v", err)
	}
	return dst
}

func writeRel(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func readRel(t *testing.T, dir, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return string(data)
}
