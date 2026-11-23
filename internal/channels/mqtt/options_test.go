package mqtt

// Covers: EXT-TOPIC-01, EXT-LWT-01, REQ-TIME-01, REQ-OPS-03.

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dxinli/collect-code/internal/interfaces"
	"github.com/dxinli/collect-code/internal/testutil/fakeclock"
)

func baseOptions(t *testing.T) Options {
	t.Helper()
	clk := fakeclock.New(time.Unix(1_700_000_000, 0).UTC())
	t.Cleanup(clk.Close)
	return Options{
		BrokerURL:         "mqtt://broker.example:1883",
		ClientIDTemplate:  "collector-{node.id}",
		Username:          "user",
		Password:          "s3cr3t",
		QoS:               interfaces.QoS1,
		KeepAlive:         60 * time.Second,
		Reconnect:         ReconnectOptions{Min: time.Second, Max: 30 * time.Second},
		NodeID:            "gateway-001",
		ProductKey:        "irrigation",
		WillTopicTemplate: "v1/{product_key}/$nodes/{node_id}/state",
		Clock:             clk,
	}
}

func TestOptionsValid(t *testing.T) {
	opts := baseOptions(t)
	c, err := newChannel(opts, nil)
	if err != nil {
		t.Fatalf("newChannel: %v", err)
	}
	if c.deriv.clientID != "collector-gateway-001" {
		t.Fatalf("client id = %q", c.deriv.clientID)
	}
	if c.deriv.willTopic != "v1/irrigation/$nodes/gateway-001/state" {
		t.Fatalf("will topic = %q", c.deriv.willTopic)
	}
	wantPayload := `{"online":false,"node_id":"gateway-001"}`
	if got := string(c.deriv.willPayload); got != wantPayload {
		t.Fatalf("will payload = %q, want %q", got, wantPayload)
	}
	if c.deriv.brokerURL != "mqtt://broker.example:1883" {
		t.Fatalf("broker URL = %q", c.deriv.brokerURL)
	}
	if got := c.deriv.brokerLabel; got != "mqtt://broker.example:1883" {
		t.Fatalf("broker label = %q", got)
	}
	// The will payload is valid JSON with the contract's semantics.
	if !strings.Contains(string(c.deriv.willPayload), `"online":false`) {
		t.Fatalf("will payload lacks online=false: %s", c.deriv.willPayload)
	}
}

func TestOptionsNonDefaultTemplates(t *testing.T) {
	opts := baseOptions(t)
	opts.ClientIDTemplate = "{product_key}-{node_id}"
	opts.ProductKey = "farm-a"
	opts.WillTopicTemplate = "custom/v2/{product_key}/nodes/{node.id}/offline"
	c, err := newChannel(opts, nil)
	if err != nil {
		t.Fatalf("newChannel: %v", err)
	}
	if c.deriv.clientID != "farm-a-gateway-001" {
		t.Fatalf("client id = %q", c.deriv.clientID)
	}
	if c.deriv.willTopic != "custom/v2/farm-a/nodes/gateway-001/offline" {
		t.Fatalf("will topic = %q", c.deriv.willTopic)
	}
}

func TestOptionsInvalid(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Options)
	}{
		{name: "nil clock", mutate: func(o *Options) { o.Clock = nil }},
		{name: "empty broker", mutate: func(o *Options) { o.BrokerURL = "" }},
		{name: "bad broker URL", mutate: func(o *Options) { o.BrokerURL = "://bad" }},
		{name: "no host", mutate: func(o *Options) { o.BrokerURL = "mqtt://" }},
		{name: "websocket scheme", mutate: func(o *Options) { o.BrokerURL = "ws://broker.example:1883" }},
		{name: "unix scheme", mutate: func(o *Options) { o.BrokerURL = "unix:///tmp/mqtt.sock" }},
		{name: "empty client id", mutate: func(o *Options) { o.ClientIDTemplate = "" }},
		{name: "bad client id template", mutate: func(o *Options) { o.ClientIDTemplate = "collector-{missing}" }},
		{name: "empty node id", mutate: func(o *Options) { o.NodeID = "" }},
		{name: "empty product key", mutate: func(o *Options) { o.ProductKey = "" }},
		{name: "qos 2", mutate: func(o *Options) { o.QoS = 2 }},
		{name: "negative keepalive", mutate: func(o *Options) { o.KeepAlive = -time.Second }},
		{name: "keepalive overflow", mutate: func(o *Options) { o.KeepAlive = 65536 * time.Second }},
		{name: "zero reconnect min", mutate: func(o *Options) { o.Reconnect.Min = 0 }},
		{name: "max below min", mutate: func(o *Options) { o.Reconnect.Max = 500 * time.Millisecond }},
		{name: "password without username", mutate: func(o *Options) { o.Username = "" }},
		{name: "will topic wildcard", mutate: func(o *Options) { o.WillTopicTemplate = "v1/{product_key}/#" }},
		{name: "tls off with ca file", mutate: func(o *Options) { o.TLS.CAFile = "/tmp/ca.pem" }},
		{name: "tls off with insecure", mutate: func(o *Options) { o.TLS.InsecureSkipVerify = true }},
		{name: "tls off with tls scheme", mutate: func(o *Options) { o.BrokerURL = "ssl://broker.example:8883" }},
		{name: "tls on with missing ca", mutate: func(o *Options) {
			o.TLS.Enabled = true
			o.TLS.CAFile = "/definitely/missing/ca.pem"
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts := baseOptions(t)
			tc.mutate(&opts)
			if _, err := newChannel(opts, nil); err == nil {
				t.Fatalf("newChannel accepted invalid options")
			}
		})
	}
}

func TestOptionsTLSDisabledZeroValueIsValid(t *testing.T) {
	opts := baseOptions(t)
	opts.TLS = TLSFiles{}
	if _, err := newChannel(opts, nil); err != nil {
		t.Fatalf("zero TLS with tls.enabled=false must be valid: %v", err)
	}
}

func TestOptionsTLSSchemeNormalization(t *testing.T) {
	tests := []struct {
		name       string
		broker     string
		tlsEnabled bool
		want       string
	}{
		{name: "mqtt to ssl", broker: "mqtt://broker.example:1883", tlsEnabled: true, want: "ssl://broker.example:1883"},
		{name: "tcp to ssl", broker: "tcp://broker.example:1883", tlsEnabled: true, want: "ssl://broker.example:1883"},
		{name: "keep ssl", broker: "ssl://broker.example:8883", tlsEnabled: true, want: "ssl://broker.example:8883"},
		{name: "keep mqtts", broker: "mqtts://broker.example:8883", tlsEnabled: true, want: "mqtts://broker.example:8883"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts := baseOptions(t)
			opts.BrokerURL = tc.broker
			opts.TLS.Enabled = tc.tlsEnabled
			c, err := newChannel(opts, nil)
			if err != nil {
				t.Fatalf("newChannel: %v", err)
			}
			if c.deriv.brokerURL != tc.want {
				t.Fatalf("broker URL = %q, want %q", c.deriv.brokerURL, tc.want)
			}
		})
	}
}

func TestOptionsBrokerUserinfoIsRedacted(t *testing.T) {
	opts := baseOptions(t)
	opts.BrokerURL = "mqtt://user:userinfo-secret@broker.example:1883"
	opts.Username = ""
	opts.Password = ""
	c, err := newChannel(opts, nil)
	if err != nil {
		t.Fatalf("newChannel: %v", err)
	}
	if strings.Contains(c.deriv.brokerLabel, "userinfo-secret") {
		t.Fatalf("broker label leaks userinfo: %q", c.deriv.brokerLabel)
	}
	if got := c.redact("failed: userinfo-secret"); strings.Contains(got, "userinfo-secret") {
		t.Fatalf("redact left the secret: %q", got)
	}
}

func TestSecretFormatting(t *testing.T) {
	opts := baseOptions(t)
	if formatted := fmt.Sprintf("%#v", opts); strings.Contains(formatted, "s3cr3t") {
		t.Fatalf("Options formatting leaks the password: %s", formatted)
	}
	if got := fmt.Sprintf("%v", Secret("s3cr3t")); got != "[REDACTED]" {
		t.Fatalf("Secret formatting = %q", got)
	}
	if got := Secret("s3cr3t").Reveal(); got != "s3cr3t" {
		t.Fatalf("Reveal = %q", got)
	}
}
