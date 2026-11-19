package config

import (
	"fmt"
	"math"
	"net"
	"sort"
	"strconv"
	"strings"
)

var (
	knownDrivers = map[string]bool{
		"modbus-rtu":     true,
		"modbus-tcp":     true,
		"modbus-rtu-tcp": true,
		"lorawan":        true,
	}
	knownParity = map[string]bool{"none": true, "even": true, "odd": true}
	knownAccess = map[string]bool{"r": true, "rw": true}
)

type validator struct {
	b         *Bundle
	loaded    []loadedProfile
	positions map[string]*positionIndex
	ds        Diagnostics
}

func validate(b *Bundle, loaded []loadedProfile, positions map[string]*positionIndex) Diagnostics {
	v := &validator{b: b, loaded: loaded, positions: positions}
	v.required()
	v.unique()
	v.references()
	v.numeric()
	v.enums()
	v.ports()
	v.logOptions()
	return v.ds
}

func (v *validator) add(file string, p nodePath, rule, format string, args ...any) {
	line, col := 1, 1
	if pi := v.positions[file]; pi != nil {
		line, col = pi.at(p)
	}
	v.ds = append(v.ds, Diagnostic{
		File: file, Line: line, Col: col, RuleID: rule,
		Message: fmt.Sprintf(format, args...),
	})
}

func linkPath(i int) nodePath    { return indexPath(fieldPath(nil, "links"), i) }
func devicePath(i int) nodePath  { return indexPath(fieldPath(nil, "devices"), i) }
func pointPath(i int) nodePath   { return indexPath(fieldPath(nil, "points"), i) }
func servicePath(i int) nodePath { return indexPath(fieldPath(nil, "services"), i) }
func eventPath(i int) nodePath   { return indexPath(fieldPath(nil, "events"), i) }

func (v *validator) required() {
	cfg := v.b.Config
	if cfg.Node.ID == "" {
		v.add("config.yaml", fieldPath(fieldPath(nil, "node"), "id"), RuleRequiredConfig, "node.id: required")
	}
	if cfg.Node.ProductKey == "" {
		v.add("config.yaml", fieldPath(fieldPath(nil, "node"), "product_key"), RuleRequiredConfig, "node.product_key: required")
	}
	if cfg.Northbound.MQTT.Broker == "" {
		v.add("config.yaml", fieldPath(fieldPath(fieldPath(nil, "northbound"), "mqtt"), "broker"),
			RuleRequiredConfig, "northbound.mqtt.broker: required")
	}

	for i, l := range v.b.Links {
		p := fmt.Sprintf("links[%d]", i)
		if l.ID == "" {
			v.add("links.yaml", fieldPath(linkPath(i), "id"), RuleRequiredLink, "%s.id: required", p)
		}
		if l.Driver == "" {
			v.add("links.yaml", fieldPath(linkPath(i), "driver"), RuleRequiredLink, "%s.driver: required", p)
		}
		if l.Serial == nil && l.TCP == nil && l.Chirpstack == nil {
			v.add("links.yaml", linkPath(i), RuleRequiredLink, "%s: one of serial/tcp/chirpstack", p)
		}
	}

	for i, d := range v.b.Devices {
		p := fmt.Sprintf("devices[%d]", i)
		if d.SN == "" {
			v.add("devices.yaml", fieldPath(devicePath(i), "sn"), RuleRequiredDevice, "%s.sn: required", p)
		}
		if d.DeviceType == "" {
			v.add("devices.yaml", fieldPath(devicePath(i), "device_type"), RuleRequiredDevice, "%s.device_type: required", p)
		}
		if d.Profile == "" {
			v.add("devices.yaml", fieldPath(devicePath(i), "profile"), RuleRequiredDevice, "%s.profile: required", p)
		}
		if d.Link == "" {
			v.add("devices.yaml", fieldPath(devicePath(i), "link"), RuleRequiredDevice, "%s.link: required", p)
		}
		if d.Address == nil {
			v.add("devices.yaml", fieldPath(devicePath(i), "address"), RuleRequiredDevice, "%s.address: required", p)
		}
	}

	for _, lp := range v.loaded {
		p := lp.profile
		if p.Profile == "" {
			v.add(lp.file, fieldPath(nil, "profile"), RuleRequiredProfile, "profile: required")
		}
		for i, point := range p.Points {
			if point.Key == "" {
				v.add(lp.file, fieldPath(pointPath(i), "key"), RuleRequiredPointKey,
					"points[%d].key: required", i)
			}
		}
		for i, svc := range p.Services {
			if svc.ServiceID == "" {
				v.add(lp.file, fieldPath(servicePath(i), "service_id"), RuleRequiredServiceID,
					"services[%d].service_id: required", i)
			}
		}
		for i, ev := range p.Events {
			if ev.EventID == "" {
				v.add(lp.file, fieldPath(eventPath(i), "event_id"), RuleRequiredEventID,
					"events[%d].event_id: required", i)
			}
		}
	}
}

type firstPos struct {
	file      string
	line, col int
}

func (v *validator) pos(file string, p nodePath) (line, col int) {
	if pi := v.positions[file]; pi != nil {
		return pi.at(p)
	}
	return 1, 1
}

type uniqueEntry struct {
	key     string
	file    string
	path    nodePath
	rule    string
	label   string
	context string
}

func (v *validator) checkUnique(entries []uniqueEntry) {
	seen := map[string]firstPos{}
	for _, e := range entries {
		if e.key == "" {
			continue
		}
		line, col := v.pos(e.file, e.path)
		if first, ok := seen[e.key]; ok {
			v.add(e.file, e.path, e.rule,
				"duplicate %s %q%s (first defined at %s:%d:%d)",
				e.label, e.key, e.context, first.file, first.line, first.col)
			continue
		}
		seen[e.key] = firstPos{e.file, line, col}
	}
}

func (v *validator) unique() {
	var links []uniqueEntry
	for i, l := range v.b.Links {
		links = append(links, uniqueEntry{
			key: l.ID, file: "links.yaml", path: fieldPath(linkPath(i), "id"),
			rule: RuleUniqueLinkID, label: "link id",
		})
	}
	v.checkUnique(links)

	var devices []uniqueEntry
	for i, d := range v.b.Devices {
		devices = append(devices, uniqueEntry{
			key: d.SN, file: "devices.yaml", path: fieldPath(devicePath(i), "sn"),
			rule: RuleUniqueDeviceSN, label: "device sn",
		})
	}
	v.checkUnique(devices)

	for _, lp := range v.loaded {
		p := lp.profile
		context := fmt.Sprintf(" in profile %q", p.Profile)
		var points []uniqueEntry
		for i, point := range p.Points {
			points = append(points, uniqueEntry{
				key: point.Key, file: lp.file, path: fieldPath(pointPath(i), "key"),
				rule: RuleUniquePointKey, label: "point key", context: context,
			})
		}
		v.checkUnique(points)

		var services []uniqueEntry
		for i, svc := range p.Services {
			services = append(services, uniqueEntry{
				key: svc.ServiceID, file: lp.file, path: fieldPath(servicePath(i), "service_id"),
				rule: RuleUniqueServiceID, label: "service_id", context: context,
			})
		}
		v.checkUnique(services)

		var events []uniqueEntry
		for i, ev := range p.Events {
			events = append(events, uniqueEntry{
				key: ev.EventID, file: lp.file, path: fieldPath(eventPath(i), "event_id"),
				rule: RuleUniqueEventID, label: "event_id", context: context,
			})
		}
		v.checkUnique(events)
	}
}

type pointCap struct {
	exists   bool
	readable bool
	writable bool
	invalid  bool
	access   string
}

func pointCaps(p *Profile) map[string]pointCap {
	caps := map[string]pointCap{}
	put := func(key string, mutate func(*pointCap)) {
		c := caps[key]
		c.exists = true
		mutate(&c)
		caps[key] = c
	}
	for _, point := range p.Points {
		if point.Key == "" {
			continue
		}
		access := point.Access
		put(point.Key, func(c *pointCap) {
			if c.access == "" {
				c.access = access
			}
			switch access {
			case "r":
				c.readable = true
			case "rw":
				c.readable = true
				c.writable = true
			case "":
			default:
				c.invalid = true
			}
		})
	}
	if p.Payload != nil {
		for _, f := range p.Payload.Decode {
			if f.Key == "" {
				continue
			}
			put(f.Key, func(c *pointCap) {
				c.readable = true
				if c.access == "" {
					c.access = "payload.decode"
				}
			})
		}
		for _, f := range p.Payload.Encode {
			if f.Key == "" {
				continue
			}
			put(f.Key, func(c *pointCap) {
				c.writable = true
				if c.access == "" {
					c.access = "payload.encode"
				}
			})
		}
	}
	return caps
}

func (v *validator) references() {
	links := map[string]bool{}
	for _, l := range v.b.Links {
		if l.ID != "" {
			links[l.ID] = true
		}
	}

	for i, d := range v.b.Devices {
		if d.Profile != "" {
			if _, ok := v.b.Profiles[d.Profile]; !ok {
				v.add("devices.yaml", fieldPath(devicePath(i), "profile"), RuleProfileRef,
					"device profile %q not found (no profiles/*.yaml defines it)", d.Profile)
			}
		}
		if d.Link != "" && !links[d.Link] {
			v.add("devices.yaml", fieldPath(devicePath(i), "link"), RuleLinkRef,
				"device link %q not found in links.yaml", d.Link)
		}
	}

	for _, lp := range v.loaded {
		p := lp.profile
		caps := pointCaps(p)
		for i, svc := range p.Services {
			for j, w := range svc.Writes {
				writePath := fieldPath(indexPath(fieldPath(servicePath(i), "writes"), j), "point")
				v.checkCapability(lp.file, writePath, p.Profile, caps, w.Point, true)
				if w.Verify != nil {
					verifyPath := fieldPath(fieldPath(indexPath(fieldPath(servicePath(i), "writes"), j), "verify"), "point")
					v.checkCapability(lp.file, verifyPath, p.Profile, caps, w.Verify.Point, false)
				}
			}
			if svc.Verify != nil {
				verifyPath := fieldPath(fieldPath(servicePath(i), "verify"), "point")
				v.checkCapability(lp.file, verifyPath, p.Profile, caps, svc.Verify.Point, false)
			}
		}
	}
}

func (v *validator) checkCapability(file string, path nodePath, profile string, caps map[string]pointCap, point string, write bool) {
	cap, ok := caps[point]
	if !ok {
		if write {
			v.add(file, path, RuleWritePointMissing,
				"write point %q not found in profile %q", point, profile)
		} else {
			v.add(file, path, RuleVerifyPointMissing,
				"verify point %q not found in profile %q", point, profile)
		}
		return
	}
	if cap.invalid || (write && cap.writable) || (!write && cap.readable) {
		return
	}

	noun, verb, rule := "verify point", "not readable", RuleVerifyPointUnreadable
	if write {
		noun, verb, rule = "write point", "read-only", RuleWritePointReadOnly
	}
	switch {
	case cap.access == "":
		v.add(file, path, rule, "%s %q is %s (access is unset)", noun, point, verb)
	case write && cap.access == "payload.decode":
		v.add(file, path, rule, "%s %q is not writable (only present in payload.decode)", noun, point)
	case !write && cap.access == "payload.encode":
		v.add(file, path, rule, "%s %q is not readable (only present in payload.encode)", noun, point)
	default:
		v.add(file, path, rule, "%s %q is %s (access=%q)", noun, point, verb, cap.access)
	}
}

func (v *validator) numeric() {
	for _, lp := range v.loaded {
		p := lp.profile
		if p.PollIntervalS != nil {
			v.checkPositiveInt(lp.file, fieldPath(nil, "poll_interval_s"), RulePollInterval,
				"poll_interval_s", *p.PollIntervalS)
		}
		for i, point := range p.Points {
			if point.Modbus == nil {
				continue
			}
			v.checkMinMax(lp.file, fieldPath(pointPath(i), "modbus"), "min", "max", point.Modbus.Min, point.Modbus.Max)
			v.checkScale(lp.file, fieldPath(fieldPath(pointPath(i), "modbus"), "scale"), point.Modbus.Scale)
		}
		for i, svc := range p.Services {
			keys := make([]string, 0, len(svc.Params))
			for k := range svc.Params {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				spec := svc.Params[k]
				v.checkMinMax(lp.file, fieldPath(fieldPath(servicePath(i), "params"), k), "min", "max", spec.Min, spec.Max)
			}
		}
		if p.Payload != nil {
			decodeBase := fieldPath(fieldPath(nil, "payload"), "decode")
			for i, f := range p.Payload.Decode {
				v.checkScale(lp.file, fieldPath(indexPath(decodeBase, i), "scale"), f.Scale)
			}
			encodeBase := fieldPath(fieldPath(nil, "payload"), "encode")
			for i, f := range p.Payload.Encode {
				v.checkScale(lp.file, fieldPath(indexPath(encodeBase, i), "scale"), f.Scale)
			}
		}
	}

	for i, d := range v.b.Devices {
		if d.PollIntervalS != nil {
			v.checkPositiveInt("devices.yaml", fieldPath(devicePath(i), "poll_interval_s"), RulePollInterval,
				"poll_interval_s", *d.PollIntervalS)
		}
	}

	t := v.b.Config.Timing
	if t.FreshnessWindowS <= 0 {
		v.add("config.yaml", fieldPath(fieldPath(nil, "timing"), "freshness_window_s"), RuleFreshnessWindow,
			"freshness_window_s must be > 0 (got %d)", t.FreshnessWindowS)
	}
	if t.OfflineAfterIntervals < 1 {
		v.add("config.yaml", fieldPath(fieldPath(nil, "timing"), "offline_after_intervals"), RuleOfflineIntervals,
			"offline_after_intervals must be >= 1 (got %d)", t.OfflineAfterIntervals)
	}
	if q := v.b.Config.Northbound.MQTT.QoS; q != 0 && q != 1 {
		v.add("config.yaml", fieldPath(fieldPath(fieldPath(nil, "northbound"), "mqtt"), "qos"), RuleQoS,
			"qos must be 0 or 1 (got %d)", q)
	}
}

func (v *validator) checkPositiveInt(file string, path nodePath, rule, label string, value int) {
	if value < 1 {
		v.add(file, path, rule, "%s must be >= 1 (got %d)", label, value)
	}
}

func (v *validator) checkMinMax(file string, base nodePath, minKey, maxKey string, min, max *float64) {
	minPath := fieldPath(base, minKey)
	maxPath := fieldPath(base, maxKey)
	if min != nil && badFloat(*min) {
		v.add(file, minPath, RuleMinMax, "%s must be a finite number (got %s)", minKey, formatFloat(*min))
		return
	}
	if max != nil && badFloat(*max) {
		v.add(file, maxPath, RuleMinMax, "%s must be a finite number (got %s)", maxKey, formatFloat(*max))
		return
	}
	if min == nil || max == nil {
		return
	}
	if *min > *max {
		v.add(file, minPath, RuleMinMax, "min must be <= max (min=%s, max=%s)",
			formatFloat(*min), formatFloat(*max))
	}
}

func (v *validator) checkScale(file string, path nodePath, scale *float64) {
	if scale == nil {
		return
	}
	if badFloat(*scale) {
		v.add(file, path, RuleScale, "scale must be a finite number (got %s)", formatFloat(*scale))
		return
	}
	if *scale <= 0 {
		v.add(file, path, RuleScale, "scale must be > 0 (got %s)", formatFloat(*scale))
	}
}

func badFloat(x float64) bool { return math.IsNaN(x) || math.IsInf(x, 0) }

func formatFloat(x float64) string { return strconv.FormatFloat(x, 'g', -1, 64) }

func (v *validator) enums() {
	for i, l := range v.b.Links {
		driverPath := fieldPath(linkPath(i), "driver")
		if l.Driver != "" && !knownDrivers[l.Driver] {
			v.add("links.yaml", driverPath, RuleDriver,
				"unknown driver %q (want modbus-rtu/modbus-tcp/modbus-rtu-tcp/lorawan)", l.Driver)
		}
		if knownDrivers[l.Driver] && (l.Serial != nil || l.TCP != nil || l.Chirpstack != nil) {
			missing := ""
			switch l.Driver {
			case "modbus-rtu":
				if l.Serial == nil {
					missing = "serial"
				}
			case "modbus-tcp", "modbus-rtu-tcp":
				if l.TCP == nil {
					missing = "tcp"
				}
			case "lorawan":
				if l.Chirpstack == nil {
					missing = "chirpstack"
				}
			}
			if missing != "" {
				v.add("links.yaml", driverPath, RuleDriverTransport,
					"driver %q requires %s block", l.Driver, missing)
			}
		}
		if l.Serial != nil && l.Serial.Parity != "" && !knownParity[l.Serial.Parity] {
			v.add("links.yaml", fieldPath(fieldPath(linkPath(i), "serial"), "parity"), RuleParity,
				"unknown parity %q (want none/even/odd)", l.Serial.Parity)
		}
	}

	for _, lp := range v.loaded {
		for i, point := range lp.profile.Points {
			if point.Access != "" && !knownAccess[point.Access] {
				v.add(lp.file, fieldPath(pointPath(i), "access"), RuleAccess,
					"unknown access %q (want r/rw)", point.Access)
			}
		}
	}
}

func (v *validator) ports() {
	health := v.b.Config.Health
	addr := health.Addr
	if addr == "" && !health.Enabled {
		return
	}
	healthPath := fieldPath(fieldPath(nil, "health"), "addr")
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		v.add("config.yaml", healthPath, RuleHealthPort, "health.addr is not a valid host:port")
		return
	}
	portNum, err := strconv.Atoi(port)
	if err != nil {
		v.add("config.yaml", healthPath, RuleHealthPort, "health.addr is not a valid host:port")
		return
	}
	if portNum == 8080 || portNum == 1700 {
		v.add("config.yaml", healthPath, RuleHealthPort, "health.addr uses ChirpStack port %d", portNum)
	}
	for _, l := range v.b.Links {
		if l.Driver != "lorawan" || l.Chirpstack == nil || l.Chirpstack.GRPCAddr == "" {
			continue
		}
		chHost, chPort, err := net.SplitHostPort(l.Chirpstack.GRPCAddr)
		if err != nil || chPort != port {
			continue
		}
		if strings.EqualFold(host, chHost) {
			v.add("config.yaml", healthPath, RuleHealthPort,
				"health.addr conflicts with link %s chirpstack.grpc_addr=%q", l.ID, l.Chirpstack.GRPCAddr)
		}
	}
}

func (v *validator) logOptions() {
	logCfg := v.b.Config.Log
	level := strings.ToLower(logCfg.Level)
	switch level {
	case "debug", "info", "warn", "error":
	default:
		v.add("config.yaml", fieldPath(fieldPath(nil, "log"), "level"), RuleLogLevel,
			"unsupported log.level %q (want debug|info|warn|error)", logCfg.Level)
	}
	if logCfg.Format != "json" {
		v.add("config.yaml", fieldPath(fieldPath(nil, "log"), "format"), RuleLogFormat,
			"unsupported log.format %q (only json in v0.1)", logCfg.Format)
	}
}
