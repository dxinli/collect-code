package sim_test

// This file implements a deliberately small MQTT 3.1.1 client for tests.
// It does not import mochi-mqtt: the broker is the system under test, so the
// assertions must come from an independent implementation.

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

type mqttTestClient struct {
	t        *testing.T
	conn     net.Conn
	r        *bufio.Reader
	packetID uint16
	pending  []mqttTestMessage
}

type mqttTestMessage struct {
	Topic   string
	Payload []byte
	QoS     byte
	Retain  bool
	Dup     bool
}

func dialMQTT(t *testing.T, addr, clientID string) *mqttTestClient {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	c := &mqttTestClient{t: t, conn: conn, r: bufio.NewReader(conn)}
	t.Cleanup(func() { _ = conn.Close() })
	code, err := c.connect(clientID)
	if err != nil {
		t.Fatalf("CONNECT: %v", err)
	}
	if code != 0 {
		t.Fatalf("CONNACK code = %d, want 0", code)
	}
	return c
}

// tryConnect sends CONNECT and returns the CONNACK return code. A connection
// error (for example the broker closing after a rejected CONNECT) is returned
// as err.
func tryConnect(t *testing.T, addr, clientID string, timeout time.Duration) (byte, error) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return 0, err
	}
	defer func() { _ = conn.Close() }()
	c := &mqttTestClient{t: t, conn: conn, r: bufio.NewReader(conn)}
	if err := c.writePacket(0x10, connectBody(clientID)); err != nil {
		return 0, err
	}
	hdr, body, err := c.readPacket(timeout)
	if err != nil {
		return 0, err
	}
	if hdr>>4 != 2 || len(body) < 2 {
		return 0, fmt.Errorf("unexpected CONNACK header %x body %x", hdr, body)
	}
	return body[1], nil
}

func (c *mqttTestClient) connect(clientID string) (byte, error) {
	if err := c.writePacket(0x10, connectBody(clientID)); err != nil {
		return 0, err
	}
	hdr, body, err := c.readPacket(2 * time.Second)
	if err != nil {
		return 0, err
	}
	if hdr>>4 != 2 || len(body) < 2 {
		return 0, fmt.Errorf("unexpected CONNACK header %x body %x", hdr, body)
	}
	return body[1], nil
}

func connectBody(clientID string) []byte {
	var body []byte
	body = append(body, encodeString("MQTT")...)
	body = append(body, 4)    // protocol level 3.1.1
	body = append(body, 0x02) // clean session
	body = append(body, 0x00, 0x3c)
	body = append(body, encodeString(clientID)...)
	return body
}

func (c *mqttTestClient) subscribe(filter string, qos byte) byte {
	c.t.Helper()
	c.packetID++
	pid := c.packetID
	var body []byte
	body = append(body, byte(pid>>8), byte(pid))
	body = append(body, encodeString(filter)...)
	body = append(body, qos)
	if err := c.writePacket(0x82, body); err != nil {
		c.t.Fatalf("SUBSCRIBE: %v", err)
	}
	for {
		hdr, resp, err := c.readPacket(2 * time.Second)
		if err != nil {
			c.t.Fatalf("SUBACK: %v", err)
		}
		if hdr>>4 == 3 {
			// A retained PUBLISH may arrive before SUBACK; keep it for
			// readPublish instead of failing the SUBACK assertion.
			msg, msgPID, err := parsePublish(hdr, resp)
			if err != nil {
				c.t.Fatalf("retained PUBLISH: %v", err)
			}
			if msg.QoS == 1 {
				if err := c.sendPuback(msgPID); err != nil {
					c.t.Fatalf("PUBACK: %v", err)
				}
			}
			c.pending = append(c.pending, msg)
			continue
		}
		if hdr>>4 != 9 || len(resp) < 3 {
			c.t.Fatalf("unexpected SUBACK header %x body %x", hdr, resp)
		}
		if got := binary.BigEndian.Uint16(resp[:2]); got != pid {
			c.t.Fatalf("SUBACK packet id = %d, want %d", got, pid)
		}
		return resp[2]
	}
}

func (c *mqttTestClient) publish(topic string, payload []byte, qos byte, retain bool) {
	c.t.Helper()
	var body []byte
	body = append(body, encodeString(topic)...)
	var pid uint16
	if qos > 0 {
		c.packetID++
		pid = c.packetID
		body = append(body, byte(pid>>8), byte(pid))
	}
	body = append(body, payload...)
	flags := qos << 1
	if retain {
		flags |= 1
	}
	if err := c.writePacket(0x30|flags, body); err != nil {
		c.t.Fatalf("PUBLISH: %v", err)
	}
	if qos == 1 {
		hdr, resp, err := c.readPacket(2 * time.Second)
		if err != nil {
			c.t.Fatalf("PUBACK: %v", err)
		}
		if hdr>>4 != 4 || len(resp) < 2 {
			c.t.Fatalf("unexpected PUBACK header %x body %x", hdr, resp)
		}
		if got := binary.BigEndian.Uint16(resp[:2]); got != pid {
			c.t.Fatalf("PUBACK packet id = %d, want %d", got, pid)
		}
	}
}

func (c *mqttTestClient) readPublish(timeout time.Duration) (mqttTestMessage, error) {
	if len(c.pending) > 0 {
		msg := c.pending[0]
		c.pending = c.pending[1:]
		return msg, nil
	}
	for {
		hdr, body, err := c.readPacket(timeout)
		if err != nil {
			return mqttTestMessage{}, err
		}
		if hdr>>4 != 3 {
			continue // ignore broker housekeeping packets
		}
		msg, pid, err := parsePublish(hdr, body)
		if err != nil {
			return mqttTestMessage{}, err
		}
		if msg.QoS == 1 {
			if err := c.sendPuback(pid); err != nil {
				return mqttTestMessage{}, err
			}
		}
		return msg, nil
	}
}

func parsePublish(hdr byte, body []byte) (mqttTestMessage, uint16, error) {
	qos := (hdr >> 1) & 0x3
	retain := hdr&1 != 0
	dup := hdr&0x8 != 0
	topic, rest, err := takeString(body)
	if err != nil {
		return mqttTestMessage{}, 0, err
	}
	var pid uint16
	if qos > 0 {
		if len(rest) < 2 {
			return mqttTestMessage{}, 0, errors.New("short publish packet id")
		}
		pid = binary.BigEndian.Uint16(rest[:2])
		rest = rest[2:]
	}
	return mqttTestMessage{
		Topic:   topic,
		Payload: append([]byte(nil), rest...),
		QoS:     qos,
		Retain:  retain,
		Dup:     dup,
	}, pid, nil
}

func (c *mqttTestClient) expectClosed(timeout time.Duration) {
	c.t.Helper()
	_ = c.conn.SetReadDeadline(time.Now().Add(timeout))
	var b [1]byte
	n, err := c.r.Read(b[:])
	if err == nil && n > 0 {
		c.t.Fatalf("connection still open; read %x", b[:n])
	}
	if err == nil {
		return
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		c.t.Fatalf("connection not closed within %s", timeout)
	}
}

func (c *mqttTestClient) sendPuback(pid uint16) error {
	return c.writePacket(0x40, []byte{byte(pid >> 8), byte(pid)})
}

func (c *mqttTestClient) writePacket(hdr byte, body []byte) error {
	buf := make([]byte, 0, 1+len(body)+4)
	buf = append(buf, hdr)
	buf = append(buf, encodeLength(len(body))...)
	buf = append(buf, body...)
	_ = c.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	for len(buf) > 0 {
		n, err := c.conn.Write(buf)
		if err != nil {
			return err
		}
		buf = buf[n:]
	}
	return nil
}

func (c *mqttTestClient) readPacket(timeout time.Duration) (byte, []byte, error) {
	_ = c.conn.SetReadDeadline(time.Now().Add(timeout))
	var hdr [1]byte
	if _, err := io.ReadFull(c.r, hdr[:]); err != nil {
		return 0, nil, err
	}
	n, err := readLength(c.r)
	if err != nil {
		return 0, nil, err
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(c.r, body); err != nil {
		return 0, nil, err
	}
	return hdr[0], body, nil
}

func encodeString(s string) []byte {
	out := make([]byte, 2+len(s))
	binary.BigEndian.PutUint16(out, uint16(len(s)))
	copy(out[2:], s)
	return out
}

func takeString(body []byte) (string, []byte, error) {
	if len(body) < 2 {
		return "", nil, errors.New("short string length")
	}
	n := int(binary.BigEndian.Uint16(body[:2]))
	if len(body) < 2+n {
		return "", nil, errors.New("short string body")
	}
	return string(body[2 : 2+n]), body[2+n:], nil
}

func encodeLength(n int) []byte {
	var out []byte
	for {
		b := byte(n % 128)
		n /= 128
		if n > 0 {
			b |= 0x80
		}
		out = append(out, b)
		if n == 0 {
			return out
		}
	}
}

func readLength(r *bufio.Reader) (int, error) {
	mult := 1
	value := 0
	for range 4 {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		value += int(b&0x7f) * mult
		if b&0x80 == 0 {
			return value, nil
		}
		mult *= 128
	}
	return 0, errors.New("malformed remaining length")
}
