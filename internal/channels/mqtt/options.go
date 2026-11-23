package mqtt

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dxinli/collect-code/internal/interfaces"
	"github.com/dxinli/collect-code/internal/topics"
)

const (
	maxMQTTStringBytes = 65535
	maxKeepAlive       = 65535 * time.Second
)

// Secret hides a credential from formatting. Call Reveal only when handing
// the value to the MQTT client.
type Secret string

func (s Secret) String() string   { return "[REDACTED]" }
func (s Secret) GoString() string { return "[REDACTED]" }

// Reveal returns the plaintext secret.
func (s Secret) Reveal() string { return string(s) }

// TLSFiles describes the file-based TLS settings from configuration.
type TLSFiles struct {
	Enabled            bool
	CAFile             string
	InsecureSkipVerify bool
}

// ReconnectOptions is the configured reconnect backoff range.
type ReconnectOptions struct {
	Min time.Duration
	Max time.Duration
}

// Options configures a northbound MQTT channel. The assembly layer maps
// configuration into this struct; this package does not import config.
type Options struct {
	BrokerURL         string
	ClientIDTemplate  string
	Username          string
	Password          Secret
	QoS               interfaces.QoS
	KeepAlive         time.Duration
	TLS               TLSFiles
	Reconnect         ReconnectOptions
	NodeID            string
	ProductKey        string
	WillTopicTemplate string
	Clock             interfaces.Clock
	Logger            *slog.Logger
}

// derivedOptions is the validated, normalized view of Options.
type derivedOptions struct {
	brokerURL    string
	brokerLabel  string
	clientID     string
	username     string
	password     string
	willTopic    string
	willPayload  []byte
	tlsConfig    *tls.Config
	secrets      []string
	warnUserinfo bool
}

func validateOptions(opts Options) (derivedOptions, error) {
	var d derivedOptions

	if opts.Clock == nil {
		return d, fmt.Errorf("mqtt: clock is required")
	}
	if strings.TrimSpace(opts.BrokerURL) == "" {
		return d, fmt.Errorf("mqtt: broker URL is required")
	}
	u, err := url.Parse(opts.BrokerURL)
	if err != nil {
		return d, fmt.Errorf("mqtt: broker URL: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	plainScheme := scheme == "mqtt" || scheme == "tcp"
	tlsScheme := scheme == "ssl" || scheme == "tls" || scheme == "mqtts" ||
		scheme == "mqtt+ssl" || scheme == "tcps"
	if !plainScheme && !tlsScheme {
		return d, fmt.Errorf("mqtt: broker scheme %q is not supported", u.Scheme)
	}
	if u.Hostname() == "" {
		return d, fmt.Errorf("mqtt: broker URL %q has no host", opts.BrokerURL)
	}

	switch {
	case opts.TLS.Enabled && plainScheme:
		u.Scheme = "ssl"
	case opts.TLS.Enabled && tlsScheme:
		// keep the operator's TLS scheme
	case !opts.TLS.Enabled && tlsScheme:
		return d, fmt.Errorf("mqtt: broker scheme %q requires tls.enabled", u.Scheme)
	case !opts.TLS.Enabled && (opts.TLS.CAFile != "" || opts.TLS.InsecureSkipVerify):
		return d, fmt.Errorf("mqtt: tls.ca_file/insecure_skip_verify require tls.enabled")
	}
	d.brokerURL = u.String()
	d.brokerLabel = brokerLabel(u)
	if u.User != nil {
		if pwd, ok := u.User.Password(); ok && pwd != "" {
			d.secrets = append(d.secrets, pwd)
		}
	}

	if opts.QoS > interfaces.QoS1 {
		return d, fmt.Errorf("mqtt: qos %d is not supported", opts.QoS)
	}
	if opts.KeepAlive < 0 {
		return d, fmt.Errorf("mqtt: keepalive must not be negative")
	}
	if opts.KeepAlive > maxKeepAlive {
		return d, fmt.Errorf("mqtt: keepalive %s exceeds the MQTT uint16 limit", opts.KeepAlive)
	}
	if opts.Reconnect.Min <= 0 {
		return d, fmt.Errorf("mqtt: reconnect.min_backoff must be positive")
	}
	if opts.Reconnect.Max < opts.Reconnect.Min {
		return d, fmt.Errorf("mqtt: reconnect.max_backoff must be >= min_backoff")
	}
	if opts.NodeID == "" {
		return d, fmt.Errorf("mqtt: node id is required")
	}
	if opts.ProductKey == "" {
		return d, fmt.Errorf("mqtt: product key is required")
	}

	vars := map[string]string{
		"node.id":     opts.NodeID,
		"node_id":     opts.NodeID,
		"product_key": opts.ProductKey,
	}
	d.clientID, err = topics.Render(opts.ClientIDTemplate, vars)
	if err != nil {
		return d, fmt.Errorf("mqtt: client id: %w", err)
	}
	if err := validateClientID(d.clientID); err != nil {
		return d, err
	}
	d.willTopic, err = topics.RenderTopic(opts.WillTopicTemplate, vars)
	if err != nil {
		return d, fmt.Errorf("mqtt: will topic: %w", err)
	}
	d.willPayload = willPayload(opts.NodeID)

	if opts.Password != "" && opts.Username == "" {
		return d, fmt.Errorf("mqtt: password requires a username")
	}
	d.username = opts.Username
	d.password = opts.Password.Reveal()
	if d.password != "" {
		d.secrets = append(d.secrets, d.password)
	}
	if u.User != nil && (opts.Username != "" || opts.Password != "") {
		d.warnUserinfo = true
	}

	if opts.TLS.Enabled {
		d.tlsConfig, err = buildTLSConfig(opts.TLS, u.Hostname())
		if err != nil {
			return d, err
		}
	}
	return d, nil
}

func validateClientID(id string) error {
	if id == "" {
		return fmt.Errorf("mqtt: client id is empty")
	}
	if len(id) > maxMQTTStringBytes {
		return fmt.Errorf("mqtt: client id exceeds %d bytes", maxMQTTStringBytes)
	}
	if !utf8.ValidString(id) {
		return fmt.Errorf("mqtt: client id is not valid UTF-8")
	}
	for _, r := range id {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("mqtt: client id contains a control character")
		}
	}
	return nil
}

func buildTLSConfig(files TLSFiles, serverName string) (*tls.Config, error) {
	cfg := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		ServerName:         serverName,
		InsecureSkipVerify: files.InsecureSkipVerify, //nolint:gosec // operator opt-in
	}
	if files.CAFile == "" {
		return cfg, nil
	}
	pem, err := os.ReadFile(files.CAFile)
	if err != nil {
		return nil, fmt.Errorf("mqtt: read tls ca_file: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("mqtt: tls ca_file %q contains no certificates", files.CAFile)
	}
	cfg.RootCAs = pool
	return cfg, nil
}

func brokerLabel(u *url.URL) string {
	host := u.Host
	if u.User != nil {
		return u.Scheme + "://[REDACTED]@" + host
	}
	return u.Scheme + "://" + host
}
