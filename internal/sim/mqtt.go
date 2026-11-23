package sim

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/mochi-mqtt/server/v2/packets"
)

const (
	defaultMQTTAddr      = "127.0.0.1:0"
	mqttListenerID       = "mqtt"
	maxDisconnectHoldMS  = int64(86400000)
	inlineSubscriptionID = 1
	defaultTopicFilter   = "#"
)

// MQTTBrokerOptions configures the embedded broker.
type MQTTBrokerOptions struct {
	Addr        string
	TopicFilter string
	Runtime     *Runtime
	Now         func() time.Time
	// Logger is passed to mochi-mqtt. Callers normally supply a warn-level
	// JSON logger so broker internals do not pollute stdout.
	Logger *slog.Logger
}

// MQTTBroker is the embedded pure-Go broker plus the cloud-stub capture and
// downlink injection endpoints.
type MQTTBroker struct {
	mu          sync.Mutex
	server      *mqtt.Server
	listener    *listeners.TCP
	runtime     *Runtime
	auth        *authHook
	topicFilter string
	host        string
	port        int
	started     bool
	closed      bool
}

// authHook is the single mochi hook. mochi defaults to denying both
// authentication and ACL checks, so this hook must provide both.
type authHook struct {
	mqtt.HookBase

	mu        sync.Mutex
	holdUntil time.Time
	now       func() time.Time
}

func (h *authHook) ID() string { return "sim-auth" }

func (h *authHook) Provides(b byte) bool {
	return b == mqtt.OnConnectAuthenticate || b == mqtt.OnACLCheck
}

func (h *authHook) OnConnectAuthenticate(_ *mqtt.Client, _ packets.Packet) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return !h.now().Before(h.holdUntil)
}

func (h *authHook) OnACLCheck(_ *mqtt.Client, _ string, _ bool) bool { return true }

func (h *authHook) setHold(until time.Time) {
	h.mu.Lock()
	h.holdUntil = until
	h.mu.Unlock()
}

// NewMQTTBroker builds the broker and binds its TCP listener. Call Register
// before Start so the capture subscription is installed before clients can
// publish.
func NewMQTTBroker(opts MQTTBrokerOptions) (*MQTTBroker, error) {
	if opts.Runtime == nil {
		return nil, errors.New("sim: runtime is required")
	}
	addr := strings.TrimSpace(opts.Addr)
	if addr == "" {
		addr = defaultMQTTAddr
	}
	filter := strings.TrimSpace(opts.TopicFilter)
	if filter == "" {
		filter = defaultTopicFilter
	}
	if !mqtt.IsValidFilter(filter, false) {
		return nil, fmt.Errorf("sim: invalid topic filter %q", filter)
	}
	now := opts.Now
	if now == nil {
		now = opts.Runtime.Now
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}

	server := mqtt.New(&mqtt.Options{
		InlineClient: true,
		Logger:       logger,
	})
	auth := &authHook{now: now}
	if err := server.AddHook(auth, nil); err != nil {
		return nil, fmt.Errorf("sim: add auth hook: %w", err)
	}

	listener := listeners.NewTCP(listeners.Config{ID: mqttListenerID, Address: addr})
	if err := server.AddListener(listener); err != nil {
		return nil, fmt.Errorf("sim: listen mqtt %s: %w", addr, err)
	}
	host, portText, err := net.SplitHostPort(listener.Address())
	if err != nil {
		return nil, fmt.Errorf("sim: mqtt address %q: %w", listener.Address(), err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return nil, fmt.Errorf("sim: mqtt port %q: %w", portText, err)
	}

	return &MQTTBroker{
		server:      server,
		listener:    listener,
		runtime:     opts.Runtime,
		auth:        auth,
		topicFilter: filter,
		host:        host,
		port:        port,
	}, nil
}

// Address returns the actual broker host:port.
func (b *MQTTBroker) Address() string {
	return net.JoinHostPort(b.host, strconv.Itoa(b.port))
}

// URL returns a mqtt:// URL usable as northbound.mqtt.broker.
func (b *MQTTBroker) URL() string {
	return "mqtt://" + b.Address()
}

// Register contributes the mqtt section, endpoints, routes, control action
// and inline capture subscription to the runtime. It must run before Start.
func (b *MQTTBroker) Register(r *Runtime) error {
	if r == nil {
		return errors.New("sim: runtime is required")
	}
	info := map[string]any{
		"url":              b.URL(),
		"host":             b.host,
		"port":             b.port,
		"tls":              false,
		"protocols":        []string{"3.1.1", "5.0"},
		"topic_filters":    []string{b.topicFilter},
		"capture_capacity": r.CaptureCapacity(),
	}
	if err := r.SetSection("mqtt", info); err != nil {
		return err
	}
	for name, path := range map[string]string{
		"captured": pathCaptured,
		"invoke":   pathInvoke,
		"control":  pathControl,
	} {
		if err := r.SetEndpoint(name, path); err != nil {
			return err
		}
	}
	r.Handle("GET "+pathCaptured, r.handleCaptured)
	r.Handle("POST "+pathInvoke, b.handleInvoke)
	if err := r.RegisterControl("", "broker_disconnect", b.handleBrokerDisconnect); err != nil {
		return err
	}
	if err := b.server.Subscribe(b.topicFilter, inlineSubscriptionID, b.captureInline); err != nil {
		return fmt.Errorf("sim: subscribe inline capture: %w", err)
	}
	return nil
}

// Start starts the broker event loop and listeners.
func (b *MQTTBroker) Start() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return errors.New("sim: broker is closed")
	}
	if b.started {
		b.mu.Unlock()
		return errors.New("sim: broker is already started")
	}
	b.started = true
	b.mu.Unlock()

	if err := b.server.Serve(); err != nil {
		return fmt.Errorf("sim: start mqtt broker: %w", err)
	}
	return nil
}

// Close stops the broker and releases the listener port.
func (b *MQTTBroker) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	b.mu.Unlock()

	// mochi-mqtt v2.7.9 deadlocks in Clients.GetByListener when a client is
	// deleted concurrently with Server.Close: GetByListener holds RLock and
	// calls Len(), which takes RLock again while the Delete writer waits.
	// Drain real clients (the permanent inline client is excluded) before
	// closing, then force-disconnect anything still attached.
	b.drainRealClients(2 * time.Second)
	if b.realClients() > 0 {
		for _, cl := range b.server.Clients.GetAll() {
			if !cl.Net.Inline {
				_ = b.server.DisconnectClient(cl, packets.ErrServerShuttingDown)
			}
		}
		b.drainRealClients(2 * time.Second)
	}
	return b.server.Close()
}

// realClients counts clients other than the permanent inline client.
func (b *MQTTBroker) realClients() int {
	n := 0
	for _, cl := range b.server.Clients.GetAll() {
		if !cl.Net.Inline {
			n++
		}
	}
	return n
}

// drainRealClients waits until no real clients remain or the timeout expires.
func (b *MQTTBroker) drainRealClients(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) && b.realClients() > 0 {
		time.Sleep(10 * time.Millisecond)
	}
}

// Publish publishes through the inline client. qos must be 0 or 1.
func (b *MQTTBroker) Publish(topic string, payload []byte, qos byte, retain bool) error {
	if err := validatePublishTopic(topic); err != nil {
		return err
	}
	if qos > 1 {
		return errors.New("qos must be 0 or 1")
	}
	// mochi's parameter order is (topic, payload, retain, qos).
	return b.server.Publish(topic, payload, retain, qos)
}

func (b *MQTTBroker) captureInline(_ *mqtt.Client, _ packets.Subscription, pk packets.Packet) {
	if pk.Origin == mqtt.InlineClientId {
		return
	}
	if isSysTopic(pk.TopicName) {
		return
	}
	b.runtime.Capture().Add(pk.TopicName, pk.Payload, pk.FixedHeader.Qos, pk.FixedHeader.Retain)
}

func isSysTopic(topic string) bool {
	return topic == "$SYS" || strings.HasPrefix(topic, "$SYS/")
}

func validatePublishTopic(topic string) error {
	if topic == "" {
		return errors.New("topic is required")
	}
	// IsValidFilter(topic, true) rejects wildcards and $SYS topics, but it
	// accepts an empty string; the explicit check above is required.
	if !mqtt.IsValidFilter(topic, true) {
		return fmt.Errorf("invalid publish topic %q", topic)
	}
	return nil
}

type invokeRequest struct {
	Topic   string          `json:"topic"`
	Payload json.RawMessage `json:"payload"`
	QoS     *byte           `json:"qos"`
	Retain  *bool           `json:"retain"`
}

func (b *MQTTBroker) handleInvoke(w http.ResponseWriter, req *http.Request) {
	body, err := readJSONBody(w, req)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	var in invokeRequest
	if err := decodeStrict(body, &in); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid invoke request: "+err.Error())
		return
	}
	if err := validatePublishTopic(in.Topic); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(in.Payload) == 0 {
		writeAPIError(w, http.StatusBadRequest, "payload is required")
		return
	}
	qos := byte(1)
	if in.QoS != nil {
		qos = *in.QoS
	}
	if qos > 1 {
		writeAPIError(w, http.StatusBadRequest, "qos must be 0 or 1")
		return
	}
	retain := false
	if in.Retain != nil {
		retain = *in.Retain
	}
	if err := b.Publish(in.Topic, []byte(in.Payload), qos, retain); err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"topic":  in.Topic,
		"qos":    qos,
		"retain": retain,
		"bytes":  len(in.Payload),
	})
}

type disconnectRequest struct {
	Action     string `json:"action"`
	Module     string `json:"module"`
	DurationMS *int64 `json:"duration_ms"`
}

func (b *MQTTBroker) handleBrokerDisconnect(req ControlRequest) (any, error) {
	var body disconnectRequest
	if err := decodeStrict(req.Body, &body); err != nil {
		return nil, BadRequestf("invalid broker_disconnect request: %v", err)
	}
	duration := int64(0)
	if body.DurationMS != nil {
		duration = *body.DurationMS
	}
	if duration < 0 || duration > maxDisconnectHoldMS {
		return nil, BadRequestf("duration_ms must be between 0 and %d", maxDisconnectHoldMS)
	}

	var holdUntil time.Time
	var holdText any
	if duration > 0 {
		holdUntil = b.auth.now().Add(time.Duration(duration) * time.Millisecond)
		holdText = holdUntil.UTC().Format(time.RFC3339Nano)
	}
	b.auth.setHold(holdUntil)

	clients := b.server.Clients.GetByListener(mqttListenerID)
	disconnected := 0
	for _, cl := range clients {
		if cl.Net.Inline {
			continue
		}
		cl.Stop(errors.New("simulated broker disconnect"))
		disconnected++
	}

	return map[string]any{
		"ok":           true,
		"action":       "broker_disconnect",
		"disconnected": disconnected,
		"hold_until":   holdText,
	}, nil
}
