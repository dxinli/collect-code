package config

import (
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func baseParts(t *testing.T) (*Bundle, []loadedProfile, map[string]*positionIndex) {
	t.Helper()
	const dir = "testdata/valid"
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
	for f, p := range ppos {
		positions[f] = p
	}
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
		{"V41 scale", RuleScale, "scale must be > 0", func(_ *Bundle, loaded []loadedProfile) {
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
	wantRule(t, ds, RuleScale, "scale must be > 0")
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
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := copyTree(t, "testdata/valid")
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
