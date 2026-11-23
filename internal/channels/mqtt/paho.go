package mqtt

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"

	"github.com/dxinli/collect-code/internal/interfaces"
)

var (
	errPublishTimeout   = errors.New("mqtt: publish timed out waiting for PUBACK")
	errSubscribeTimeout = errors.New("mqtt: subscribe timed out waiting for SUBACK")
	errSubscribeReject  = errors.New("mqtt: broker rejected the subscription")
)

// pahoDialer is the production connection seam.
type pahoDialer struct {
	clock           interfaces.Clock
	log             *slog.Logger
	registerCleanup func(func())
}

func (d *pahoDialer) Dial(ctx context.Context, cfg dialConfig) (transport, error) {
	lost := make(chan error, 1)
	opts := paho.NewClientOptions()
	opts.AddBroker(cfg.BrokerURL)
	opts.SetClientID(cfg.ClientID)
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(false)
	opts.SetConnectRetry(false)
	opts.SetConnectTimeout(connectTimeout)
	opts.SetWriteTimeout(writeTimeout)
	opts.SetKeepAlive(cfg.KeepAlive)
	opts.SetProtocolVersion(4) // MQTT 3.1.1
	opts.SetOrderMatters(true)
	opts.SetBinaryWill(cfg.WillTopic, cfg.WillPayload, cfg.WillQoS, cfg.WillRetain)
	if cfg.TLSConfig != nil {
		opts.SetTLSConfig(cfg.TLSConfig)
	}
	if cfg.Username != "" {
		opts.SetUsername(cfg.Username)
	}
	if cfg.Password != "" {
		opts.SetPassword(cfg.Password)
	}
	opts.SetConnectionLostHandler(func(_ paho.Client, err error) {
		select {
		case lost <- err:
		default:
		}
	})

	client := paho.NewClient(opts)
	token := client.Connect()
	select {
	case <-token.Done():
		if err := token.Error(); err != nil {
			return nil, fmt.Errorf("mqtt: connect: %w", err)
		}
		return &pahoTransport{client: client, lost: lost, clock: d.clock, log: d.log}, nil
	case <-ctx.Done():
		// paho's Connect runs in the background and may still succeed after
		// the caller gave up; a late success must be disconnected or it leaks
		// a live client until process exit.
		if d.registerCleanup != nil {
			d.registerCleanup(func() { d.cleanupPendingConnect(client, token) })
		}
		return nil, ctx.Err()
	}
}

func (d *pahoDialer) cleanupPendingConnect(client paho.Client, token paho.Token) {
	timer := d.clock.NewTimer(connectTimeout)
	defer timer.Stop()
	select {
	case <-token.Done():
		if token.Error() == nil {
			client.Disconnect(uint(disconnectQuiesce / time.Millisecond))
		}
	case <-timer.C():
		// paho should have honored SetConnectTimeout. Do not touch a client
		// that is still connecting; log it so the leak is observable.
		if d.log != nil {
			d.log.Warn("abandoned MQTT connect attempt after timeout")
		}
	}
}

// pahoTransport is one live paho connection.
type pahoTransport struct {
	client paho.Client
	lost   chan error
	clock  interfaces.Clock
	log    *slog.Logger
}

func (t *pahoTransport) Publish(ctx context.Context, msg interfaces.Message) error {
	if !t.client.IsConnected() {
		return fmt.Errorf("mqtt: publish %q: %w", msg.Topic, interfaces.ErrNotConnected)
	}
	token := t.client.Publish(msg.Topic, byte(msg.QoS), msg.Retain, msg.Payload)
	timer := t.clock.NewTimer(publishWait)
	defer timer.Stop()
	select {
	case <-token.Done():
		if err := token.Error(); err != nil {
			return t.wrapPublishError(msg.Topic, err)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C():
		return fmt.Errorf("mqtt: publish %q: %w", msg.Topic, errPublishTimeout)
	}
}

func (t *pahoTransport) wrapPublishError(topic string, err error) error {
	if errors.Is(err, paho.ErrNotConnected) || !t.client.IsConnected() {
		return fmt.Errorf("mqtt: publish %q: %w", topic, interfaces.ErrNotConnected)
	}
	return fmt.Errorf("mqtt: publish %q: %w", topic, err)
}

func (t *pahoTransport) Subscribe(ctx context.Context, filter string, qos interfaces.QoS, deliver func(interfaces.Message)) error {
	if !t.client.IsConnected() {
		return fmt.Errorf("mqtt: subscribe %q: %w", filter, interfaces.ErrNotConnected)
	}
	token := t.client.Subscribe(filter, byte(qos), func(_ paho.Client, m paho.Message) {
		if m.Qos() > 1 {
			if t.log != nil {
				t.log.Warn("dropping inbound qos2 message", "topic", m.Topic())
			}
			return
		}
		deliver(interfaces.Message{
			Topic:   m.Topic(),
			Payload: append([]byte(nil), m.Payload()...),
			QoS:     interfaces.QoS(m.Qos()),
			Retain:  m.Retained(),
		})
	})
	timer := t.clock.NewTimer(publishWait)
	defer timer.Stop()
	select {
	case <-token.Done():
		if err := token.Error(); err != nil {
			if errors.Is(err, paho.ErrNotConnected) || !t.client.IsConnected() {
				return fmt.Errorf("mqtt: subscribe %q: %w", filter, interfaces.ErrNotConnected)
			}
			return fmt.Errorf("mqtt: subscribe %q: %w", filter, err)
		}
		if result, ok := token.(*paho.SubscribeToken).Result()[filter]; !ok || result == 0x80 {
			return fmt.Errorf("mqtt: subscribe %q: %w", filter, errSubscribeReject)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C():
		return fmt.Errorf("mqtt: subscribe %q: %w", filter, errSubscribeTimeout)
	}
}

func (t *pahoTransport) Lost() <-chan error { return t.lost }

func (t *pahoTransport) IsConnected() bool { return t.client.IsConnected() }

func (t *pahoTransport) Close() error {
	t.client.Disconnect(uint(disconnectQuiesce / time.Millisecond))
	return nil
}
