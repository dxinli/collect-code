package config

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// EffectiveSummary is a deliberately primitive view of the loaded config.
// It never embeds Config, Secret, Profile or any other structure that a
// generic formatter could expand and leak.
type EffectiveSummary struct {
	NodeID, ProductKey       string
	Links, Devices, Profiles int

	MQTTBroker, MQTTClientID, MQTTUsername, MQTTPassword string
	MQTTQoS, MQTTKeepaliveS                              int
	MQTTTLSEnabled, MQTTTLSInsecureSkipVerify            bool
	MQTTTLSCAFile                                        string
	MQTTReconnectMinS, MQTTReconnectMaxS                 int

	AdapterName                                      string
	TopicPropertyPost, TopicServiceReply, TopicEvent string
	TopicServiceInvoke, TopicNodeState               string

	Timing summaryTiming
	Cache  summaryCache

	QueuePendingMax int
	State           summaryState

	LogLevel, LogFormat string
	RedactFields        []string

	HealthEnabled bool
	HealthAddr    string

	LinkItems    []summaryLink
	DeviceItems  []summaryDevice
	ProfileItems []summaryProfile
}

type summaryTiming struct {
	SnapshotIntervalS, DeltaDebounceMS, FreshnessWindowS int
	OfflineAfterIntervals, ReadDeadlineMS                int
	WriteVerifyDeadlineMS, CmdTimeoutMS, CmdRetryMax     int
	LoraAckWindows, LoraWindowMarginS                    int
}

type summaryCache struct {
	EventQueueMax, EventMaxBytes, EventDroppedReserve int
}

type summaryState struct {
	DBPath              string
	IdempotencyTTLHours int
	CompactionIntervalS int
}

type summaryLink struct {
	ID, Driver, Transport    string
	ReadDeadlineMS           int
	RetryMax, RetryBackoffMS *int
	SerialPresent            bool
	TCPPresent               bool
	SerialDevice             string
	SerialBaud               int
	SerialDataBits           int
	SerialParity             string
	SerialStopBits           int
	TCPHost                  string
	TCPPort                  *int
	Chirpstack               *summaryChirpstack
}

type summaryChirpstack struct {
	GRPCAddr, APIToken, ApplicationID string
	MQTTBroker, TopicFilter           string
	JSON                              bool
	ExpectedUplinkS, EnqueueTimeoutS  int
}

type summaryDevice struct {
	SN, DeviceType, Profile, Link, Address string
	PollIntervalS                          *int
}

type summaryProfile struct {
	Name, Source, DeviceType string
	PollIntervalS            *int
	Points, Services, Events int
	Payload                  bool
}

func BuildSummary(b *Bundle) EffectiveSummary {
	cfg := b.Config
	s := EffectiveSummary{
		NodeID:                    cfg.Node.ID,
		ProductKey:                cfg.Node.ProductKey,
		Links:                     len(b.Links),
		Devices:                   len(b.Devices),
		Profiles:                  len(b.Profiles),
		MQTTBroker:                sanitizeBroker(cfg.Northbound.MQTT.Broker),
		MQTTClientID:              cfg.Northbound.MQTT.ClientID,
		MQTTUsername:              cfg.Northbound.MQTT.Username,
		MQTTPassword:              cfg.Northbound.MQTT.Password.Masked(),
		MQTTQoS:                   cfg.Northbound.MQTT.QoS,
		MQTTKeepaliveS:            cfg.Northbound.MQTT.KeepaliveS,
		MQTTTLSEnabled:            cfg.Northbound.MQTT.TLS.Enabled,
		MQTTTLSCAFile:             cfg.Northbound.MQTT.TLS.CAFile,
		MQTTTLSInsecureSkipVerify: cfg.Northbound.MQTT.TLS.InsecureSkipVerify,
		MQTTReconnectMinS:         cfg.Northbound.MQTT.Reconnect.MinBackoffS,
		MQTTReconnectMaxS:         cfg.Northbound.MQTT.Reconnect.MaxBackoffS,
		AdapterName:               cfg.Adapter.Name,
		TopicPropertyPost:         cfg.Adapter.Topics.PropertyPost,
		TopicServiceReply:         cfg.Adapter.Topics.ServiceReply,
		TopicEvent:                cfg.Adapter.Topics.Event,
		TopicServiceInvoke:        cfg.Adapter.Topics.ServiceInvoke,
		TopicNodeState:            cfg.Adapter.Topics.NodeState,
		Timing: summaryTiming{
			SnapshotIntervalS:     cfg.Timing.SnapshotIntervalS,
			DeltaDebounceMS:       cfg.Timing.DeltaDebounceMS,
			FreshnessWindowS:      cfg.Timing.FreshnessWindowS,
			OfflineAfterIntervals: cfg.Timing.OfflineAfterIntervals,
			ReadDeadlineMS:        cfg.Timing.ReadDeadlineMS,
			WriteVerifyDeadlineMS: cfg.Timing.WriteVerifyDeadlineMS,
			CmdTimeoutMS:          cfg.Timing.CmdTimeoutMS,
			CmdRetryMax:           cfg.Timing.CmdRetryMax,
			LoraAckWindows:        cfg.Timing.LoraAckWindows,
			LoraWindowMarginS:     cfg.Timing.LoraWindowMarginS,
		},
		Cache: summaryCache{
			EventQueueMax:       cfg.Cache.EventQueueMax,
			EventMaxBytes:       cfg.Cache.EventMaxBytes,
			EventDroppedReserve: cfg.Cache.EventDroppedReserve,
		},
		QueuePendingMax: cfg.Queue.PendingMax,
		State: summaryState{
			DBPath:              cfg.State.DBPath,
			IdempotencyTTLHours: cfg.State.IdempotencyTTLHours,
			CompactionIntervalS: cfg.State.CompactionIntervalS,
		},
		LogLevel:      cfg.Log.Level,
		LogFormat:     cfg.Log.Format,
		RedactFields:  append([]string(nil), cfg.Log.RedactFields...),
		HealthEnabled: cfg.Health.Enabled,
		HealthAddr:    cfg.Health.Addr,
	}

	for _, l := range b.Links {
		item := summaryLink{
			ID:             l.ID,
			Driver:         l.Driver,
			ReadDeadlineMS: cfg.Timing.ReadDeadlineMS,
		}
		if l.ReadDeadlineMS != nil {
			item.ReadDeadlineMS = *l.ReadDeadlineMS
		}
		if l.Retry != nil {
			max, backoff := l.Retry.Max, l.Retry.BackoffMS
			item.RetryMax, item.RetryBackoffMS = &max, &backoff
		}
		var transport []string
		if l.Serial != nil {
			transport = append(transport, "serial")
			item.SerialPresent = true
			item.SerialDevice = l.Serial.Device
			item.SerialBaud = l.Serial.Baud
			item.SerialDataBits = l.Serial.DataBits
			item.SerialParity = l.Serial.Parity
			item.SerialStopBits = l.Serial.StopBits
		}
		if l.TCP != nil {
			transport = append(transport, "tcp")
			item.TCPPresent = true
			item.TCPHost = l.TCP.Host
			item.TCPPort = l.TCP.Port
		}
		if l.Chirpstack != nil {
			transport = append(transport, "chirpstack")
			item.Chirpstack = &summaryChirpstack{
				GRPCAddr:        l.Chirpstack.GRPCAddr,
				APIToken:        l.Chirpstack.TokenMasked(),
				ApplicationID:   l.Chirpstack.ApplicationID,
				MQTTBroker:      sanitizeBroker(l.Chirpstack.MQTT.Broker),
				TopicFilter:     l.Chirpstack.MQTT.TopicFilter,
				JSON:            l.Chirpstack.MQTT.JSON,
				ExpectedUplinkS: l.Chirpstack.ExpectedUplinkS,
				EnqueueTimeoutS: l.Chirpstack.EnqueueTimeoutS,
			}
		}
		item.Transport = strings.Join(transport, ",")
		s.LinkItems = append(s.LinkItems, item)
	}

	for _, d := range b.Devices {
		item := summaryDevice{
			SN:            d.SN,
			DeviceType:    d.DeviceType,
			Profile:       d.Profile,
			Link:          d.Link,
			Address:       formatAddress(d.Address),
			PollIntervalS: d.PollIntervalS,
		}
		if item.PollIntervalS == nil {
			if p := b.Profiles[d.Profile]; p != nil {
				item.PollIntervalS = p.PollIntervalS
			}
		}
		s.DeviceItems = append(s.DeviceItems, item)
	}

	names := make([]string, 0, len(b.Profiles))
	for name := range b.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		p := b.Profiles[name]
		s.ProfileItems = append(s.ProfileItems, summaryProfile{
			Name:          name,
			Source:        b.ProfileSources[name],
			DeviceType:    p.DeviceType,
			PollIntervalS: p.PollIntervalS,
			Points:        len(p.Points),
			Services:      len(p.Services),
			Events:        len(p.Events),
			Payload:       p.Payload != nil,
		})
	}
	return s
}

func (s EffectiveSummary) Render() string {
	var b strings.Builder
	fmt.Fprintf(&b, "config ok: node=%s product_key=%s links=%d devices=%d profiles=%d\n",
		s.NodeID, s.ProductKey, s.Links, s.Devices, s.Profiles)
	fmt.Fprintf(&b, "northbound.mqtt: broker=%s client_id=%s username=%s password=%s qos=%d keepalive_s=%d tls_enabled=%t tls_ca_file=%s tls_insecure_skip_verify=%t reconnect_min_backoff_s=%d reconnect_max_backoff_s=%d\n",
		quoteOrUnset(s.MQTTBroker), quoteOrUnset(s.MQTTClientID), quoteOrUnset(s.MQTTUsername), quoteOrUnset(s.MQTTPassword),
		s.MQTTQoS, s.MQTTKeepaliveS, s.MQTTTLSEnabled, quoteOrUnset(s.MQTTTLSCAFile), s.MQTTTLSInsecureSkipVerify,
		s.MQTTReconnectMinS, s.MQTTReconnectMaxS)
	fmt.Fprintf(&b, "adapter: name=%s property_post=%s service_reply=%s event=%s service_invoke=%s node_state=%s\n",
		quoteOrUnset(s.AdapterName), quoteOrUnset(s.TopicPropertyPost), quoteOrUnset(s.TopicServiceReply),
		quoteOrUnset(s.TopicEvent), quoteOrUnset(s.TopicServiceInvoke), quoteOrUnset(s.TopicNodeState))
	t := s.Timing
	fmt.Fprintf(&b, "timing: snapshot_interval_s=%d delta_debounce_ms=%d freshness_window_s=%d offline_after_intervals=%d read_deadline_ms=%d write_verify_deadline_ms=%d cmd_timeout_ms=%d cmd_retry_max=%d lora_ack_windows=%d lora_window_margin_s=%d\n",
		t.SnapshotIntervalS, t.DeltaDebounceMS, t.FreshnessWindowS, t.OfflineAfterIntervals,
		t.ReadDeadlineMS, t.WriteVerifyDeadlineMS, t.CmdTimeoutMS, t.CmdRetryMax, t.LoraAckWindows, t.LoraWindowMarginS)
	fmt.Fprintf(&b, "cache: event_queue_max=%d event_max_bytes=%d event_dropped_reserve=%d\n",
		s.Cache.EventQueueMax, s.Cache.EventMaxBytes, s.Cache.EventDroppedReserve)
	fmt.Fprintf(&b, "queue: pending_max=%d\n", s.QueuePendingMax)
	fmt.Fprintf(&b, "state: db_path=%s idempotency_ttl_hours=%d compaction_interval_s=%d\n",
		quoteOrUnset(s.State.DBPath), s.State.IdempotencyTTLHours, s.State.CompactionIntervalS)
	fmt.Fprintf(&b, "log: level=%s format=%s redact_fields=%s\n",
		quoteOrUnset(s.LogLevel), quoteOrUnset(s.LogFormat), renderList(s.RedactFields))
	fmt.Fprintf(&b, "health: enabled=%t addr=%s\n", s.HealthEnabled, quoteOrUnset(s.HealthAddr))

	b.WriteString("links:\n")
	for _, l := range s.LinkItems {
		fmt.Fprintf(&b, "  - id=%s driver=%s transport=%s read_deadline_ms=%d retry_max=%s retry_backoff_ms=%s",
			quoteOrUnset(l.ID), quoteOrUnset(l.Driver), quoteOrUnset(l.Transport), l.ReadDeadlineMS,
			optInt(l.RetryMax), optInt(l.RetryBackoffMS))
		if l.SerialPresent {
			fmt.Fprintf(&b, " serial_device=%s serial_baud=%d serial_data_bits=%d serial_parity=%s serial_stop_bits=%d",
				quoteOrUnset(l.SerialDevice), l.SerialBaud, l.SerialDataBits, quoteOrUnset(l.SerialParity), l.SerialStopBits)
		}
		if l.TCPPresent {
			fmt.Fprintf(&b, " tcp_host=%s tcp_port=%s", quoteOrUnset(l.TCPHost), optInt(l.TCPPort))
		}
		if c := l.Chirpstack; c != nil {
			fmt.Fprintf(&b, " chirpstack_grpc_addr=%s chirpstack_api_token=%s chirpstack_application_id=%s chirpstack_mqtt_broker=%s chirpstack_topic_filter=%s chirpstack_json=%t expected_uplink_interval_s=%d enqueue_timeout_s=%d",
				quoteOrUnset(c.GRPCAddr), quoteOrUnset(c.APIToken), quoteOrUnset(c.ApplicationID),
				quoteOrUnset(c.MQTTBroker), quoteOrUnset(c.TopicFilter), c.JSON, c.ExpectedUplinkS, c.EnqueueTimeoutS)
		}
		b.WriteByte('\n')
	}

	b.WriteString("devices:\n")
	for _, d := range s.DeviceItems {
		fmt.Fprintf(&b, "  - sn=%s device_type=%s profile=%s link=%s address=%s poll_interval_s=%s\n",
			quoteOrUnset(d.SN), quoteOrUnset(d.DeviceType), quoteOrUnset(d.Profile),
			quoteOrUnset(d.Link), quoteOrUnset(d.Address), optInt(d.PollIntervalS))
	}

	b.WriteString("profiles:\n")
	for _, p := range s.ProfileItems {
		fmt.Fprintf(&b, "  - profile=%s source=%s device_type=%s poll_interval_s=%s points=%d services=%d events=%d payload=%t\n",
			quoteOrUnset(p.Name), quoteOrUnset(p.Source), quoteOrUnset(p.DeviceType),
			optInt(p.PollIntervalS), p.Points, p.Services, p.Events, p.Payload)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// Summary renders the full effective configuration summary. The first line
// keeps 01a's stable success marker; later lines are the 01b detail.
func (b *Bundle) Summary() string {
	return BuildSummary(b).Render()
}

func quoteOrUnset(s string) string {
	if s == "" {
		return `"[unset]"`
	}
	return strconv.Quote(s)
}

func optInt(v *int) string {
	if v == nil {
		return "[unset]"
	}
	return strconv.Itoa(*v)
}

func renderList(values []string) string {
	parts := make([]string, 0, len(values))
	for _, v := range values {
		parts = append(parts, strconv.Quote(v))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func formatAddress(a *Address) string {
	if a == nil {
		return ""
	}
	var parts []string
	if a.UnitID != nil {
		parts = append(parts, "unit_id="+strconv.Itoa(int(*a.UnitID)))
	}
	if a.DevEUI != "" {
		parts = append(parts, "dev_eui="+a.DevEUI)
	}
	if a.FPort != nil {
		parts = append(parts, "f_port="+strconv.Itoa(*a.FPort))
	}
	return strings.Join(parts, ",")
}

// sanitizeBroker keeps only scheme, host:port and a placeholder userinfo.
// Query strings, paths and fragments can carry credentials, so they are
// dropped rather than printed.
func sanitizeBroker(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "[unparseable broker]"
	}
	if u.User != nil {
		return u.Scheme + "://[REDACTED]@" + u.Host
	}
	return u.Scheme + "://" + u.Host
}

// SecretValues returns every resolved credential plus broker userinfo
// passwords, for the logging redactor's value-level fallback.
func (b *Bundle) SecretValues() []string {
	var out []string
	add := func(s Secret) {
		if s.value != "" {
			out = append(out, s.value)
		}
	}
	add(b.Config.Northbound.MQTT.Password)
	for i := range b.Links {
		if b.Links[i].Chirpstack != nil {
			add(b.Links[i].Chirpstack.apiToken)
		}
	}

	urls := []string{b.Config.Northbound.MQTT.Broker}
	for i := range b.Links {
		if b.Links[i].Chirpstack != nil {
			urls = append(urls, b.Links[i].Chirpstack.MQTT.Broker)
		}
	}
	for _, raw := range urls {
		u, err := url.Parse(raw)
		if err != nil || u.User == nil {
			continue
		}
		if p, ok := u.User.Password(); ok && p != "" {
			out = append(out, p)
		}
	}
	return out
}
