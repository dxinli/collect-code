package sim_test

// The independent MQTT 3.1.1 test client lives in internal/testutil so the
// channel integration tests can share it. The broker is the system under
// test, so assertions must come from an implementation that does not import
// mochi-mqtt or paho.

import (
	"testing"
	"time"

	"github.com/dxinli/collect-code/internal/testutil/mqttclient"
)

type mqttTestClient = mqttclient.Client
type mqttTestMessage = mqttclient.Message

func dialMQTT(t *testing.T, addr, clientID string) *mqttTestClient {
	t.Helper()
	return mqttclient.Dial(t, addr, clientID)
}

// tryConnect sends CONNECT and returns the CONNACK return code.
func tryConnect(t *testing.T, addr, clientID string, timeout time.Duration) (byte, error) {
	t.Helper()
	return mqttclient.TryConnect(addr, clientID, timeout)
}
