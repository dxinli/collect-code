// Package config defines the four-layer YAML configuration model and its
// strict loader. It deliberately performs only presence checks for ticket
// 01a; reference integrity, uniqueness and range rules belong to 01b.
package config

// Config is config.yaml: node-level settings.
type Config struct {
	Node       Node       `yaml:"node"`
	Northbound Northbound `yaml:"northbound"`
	Adapter    Adapter    `yaml:"adapter"`
	Timing     Timing     `yaml:"timing"`
	Cache      Cache      `yaml:"cache"`
	Queue      Queue      `yaml:"queue"`
	State      State      `yaml:"state"`
	Log        LogConfig  `yaml:"log"`
	Health     Health     `yaml:"health"`
}

type Node struct {
	ID         string `yaml:"id"`
	ProductKey string `yaml:"product_key"`
}

type Northbound struct {
	MQTT MQTTConfig `yaml:"mqtt"`
}

type MQTTConfig struct {
	Broker     string          `yaml:"broker"`
	ClientID   string          `yaml:"client_id"`
	Username   string          `yaml:"username"`
	Password   Secret          `yaml:"password"`
	QoS        int             `yaml:"qos"`
	KeepaliveS int             `yaml:"keepalive_s"`
	TLS        TLSConfig       `yaml:"tls"`
	Reconnect  ReconnectConfig `yaml:"reconnect"`
}

type TLSConfig struct {
	Enabled            bool   `yaml:"enabled"`
	CAFile             string `yaml:"ca_file"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify"`
}

type ReconnectConfig struct {
	MinBackoffS int `yaml:"min_backoff_s"`
	MaxBackoffS int `yaml:"max_backoff_s"`
}

type Adapter struct {
	Name   string `yaml:"name"`
	Topics Topics `yaml:"topics"`
}

// Topics is a fixed-field struct on purpose: a map would silently accept
// misspelled topic names and defeat strict unknown-key checking.
type Topics struct {
	PropertyPost  string `yaml:"property_post"`
	ServiceReply  string `yaml:"service_reply"`
	Event         string `yaml:"event"`
	ServiceInvoke string `yaml:"service_invoke"`
	NodeState     string `yaml:"node_state"`
}

type Timing struct {
	SnapshotIntervalS     int `yaml:"snapshot_interval_s"`
	DeltaDebounceMS       int `yaml:"delta_debounce_ms"`
	FreshnessWindowS      int `yaml:"freshness_window_s"`
	OfflineAfterIntervals int `yaml:"offline_after_intervals"`
	ReadDeadlineMS        int `yaml:"read_deadline_ms"`
	WriteVerifyDeadlineMS int `yaml:"write_verify_deadline_ms"`
	CmdTimeoutMS          int `yaml:"cmd_timeout_ms"`
	CmdRetryMax           int `yaml:"cmd_retry_max"`
	LoraAckWindows        int `yaml:"lora_ack_windows"`
	LoraWindowMarginS     int `yaml:"lora_window_margin_s"`
}

type Cache struct {
	EventQueueMax       int `yaml:"event_queue_max"`
	EventMaxBytes       int `yaml:"event_max_bytes"`
	EventDroppedReserve int `yaml:"event_dropped_reserve"`
}

type Queue struct {
	PendingMax int `yaml:"pending_max"`
}

type State struct {
	DBPath              string `yaml:"db_path"`
	IdempotencyTTLHours int    `yaml:"idempotency_ttl_hours"`
	CompactionIntervalS int    `yaml:"compaction_interval_s"`
}

type LogConfig struct {
	Level        string   `yaml:"level"`
	Format       string   `yaml:"format"`
	RedactFields []string `yaml:"redact_fields"`
}

type Health struct {
	Enabled bool   `yaml:"enabled"`
	Addr    string `yaml:"addr"`
}

// LinksFile is links.yaml.
type LinksFile struct {
	Links []Link `yaml:"links"`
}

type Link struct {
	ID             string            `yaml:"id"`
	Driver         string            `yaml:"driver"`
	Serial         *SerialConfig     `yaml:"serial"`
	TCP            *TCPConfig        `yaml:"tcp"`
	Chirpstack     *ChirpstackConfig `yaml:"chirpstack"`
	ReadDeadlineMS *int              `yaml:"read_deadline_ms"`
	Retry          *RetryConfig      `yaml:"retry"`
}

type SerialConfig struct {
	Device   string `yaml:"device"`
	Baud     int    `yaml:"baud"`
	DataBits int    `yaml:"data_bits"`
	Parity   string `yaml:"parity"`
	StopBits int    `yaml:"stop_bits"`
}

type TCPConfig struct {
	Host string `yaml:"host"`
	Port *int   `yaml:"port"`
}

type RetryConfig struct {
	Max       int `yaml:"max"`
	BackoffMS int `yaml:"backoff_ms"`
}

type ChirpstackConfig struct {
	GRPCAddr        string         `yaml:"grpc_addr"`
	APITokenEnv     string         `yaml:"api_token_env"`
	ApplicationID   string         `yaml:"application_id"`
	MQTT            ChirpstackMQTT `yaml:"mqtt"`
	ExpectedUplinkS int            `yaml:"expected_uplink_interval_s"`
	EnqueueTimeoutS int            `yaml:"enqueue_timeout_s"`

	// apiToken is resolved from APITokenEnv after decoding; it is never
	// populated directly from YAML so the token value cannot leak through
	// generic marshaling.
	apiToken Secret
}

type ChirpstackMQTT struct {
	Broker      string `yaml:"broker"`
	TopicFilter string `yaml:"topic_filter"`
	JSON        bool   `yaml:"json"`
}

// DevicesFile is devices.yaml.
type DevicesFile struct {
	Devices []Device `yaml:"devices"`
}

type Device struct {
	SN            string   `yaml:"sn"`
	DeviceType    string   `yaml:"device_type"`
	Profile       string   `yaml:"profile"`
	Link          string   `yaml:"link"`
	Address       *Address `yaml:"address"`
	PollIntervalS *int     `yaml:"poll_interval_s"`
}

// Address uses pointers so that unit_id: 0 (the Modbus broadcast address)
// is distinguishable from an absent address.
type Address struct {
	UnitID *uint16 `yaml:"unit_id"`
	DevEUI string  `yaml:"dev_eui"`
	FPort  *int    `yaml:"f_port"`
}

// Profile is one file under profiles/.
type Profile struct {
	Profile       string    `yaml:"profile"`
	DeviceType    string    `yaml:"device_type"`
	PollIntervalS *int      `yaml:"poll_interval_s"`
	Points        []Point   `yaml:"points"`
	Services      []Service `yaml:"services"`
	Events        []Event   `yaml:"events"`
	Payload       *Payload  `yaml:"payload"`
}

type Point struct {
	Key         string       `yaml:"key"`
	Access      string       `yaml:"access"`
	Modbus      *ModbusPoint `yaml:"modbus"`
	Write       *ModbusWrite `yaml:"write"`
	StaleAfterS *int         `yaml:"stale_after_s"`
}

// ModbusWrite is the write-side function code and address for a point.
type ModbusWrite struct {
	Func    *int `yaml:"func"`
	Address *int `yaml:"address"`
}

type ModbusPoint struct {
	Func    *int     `yaml:"func"`
	Address *int     `yaml:"address"`
	Type    string   `yaml:"type"`
	Scale   *float64 `yaml:"scale"`
	Unit    string   `yaml:"unit"`
	Min     *float64 `yaml:"min"`
	Max     *float64 `yaml:"max"`
}

type Service struct {
	ServiceID string               `yaml:"service_id"`
	Confirmed *bool                `yaml:"confirmed"`
	Params    map[string]ParamSpec `yaml:"params"`
	Writes    []Write              `yaml:"writes"`
	Verify    *Verify              `yaml:"verify"`
	TimeoutMS *int                 `yaml:"timeout_ms"`
	RetryMax  *int                 `yaml:"retry_max"`
}

type ParamSpec struct {
	Type string   `yaml:"type"`
	Min  *float64 `yaml:"min"`
	Max  *float64 `yaml:"max"`
	Unit string   `yaml:"unit"`
}

type Write struct {
	Point  string  `yaml:"point"`
	From   string  `yaml:"from"`
	Verify *Verify `yaml:"verify"`
}

type Verify struct {
	Type      string   `yaml:"type"`
	Point     string   `yaml:"point"`
	Tolerance *float64 `yaml:"tolerance"`
}

type Event struct {
	EventID string  `yaml:"event_id"`
	Trigger Trigger `yaml:"trigger"`
	Level   string  `yaml:"level"`
}

type Trigger struct {
	Type  string   `yaml:"type"`
	Point string   `yaml:"point"`
	Op    string   `yaml:"op"`
	Value *float64 `yaml:"value"`
}

type Payload struct {
	FPort  *int       `yaml:"f_port"`
	Decode []BitField `yaml:"decode"`
	Encode []BitField `yaml:"encode"`
}

type BitField struct {
	Key    string   `yaml:"key"`
	Offset *int     `yaml:"offset"`
	Length *int     `yaml:"length"`
	Type   string   `yaml:"type"`
	Scale  *float64 `yaml:"scale"`
	Unit   string   `yaml:"unit"`
}

// Bundle is the loaded four-layer configuration.
type Bundle struct {
	Config         *Config
	Links          []Link
	Devices        []Device
	Profiles       map[string]*Profile
	ProfileSources map[string]string
}
