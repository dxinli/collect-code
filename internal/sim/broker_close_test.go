package sim_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dxinli/collect-code/internal/testutil/mqttclient"
)

// TestBrokerCloseWhileClientsDisconnect closes several client connections and
// the broker at the same time. A client Delete racing with Server.Close used
// to deadlock inside mochi-mqtt; the test bounds Close so a regression fails
// in seconds rather than at the 10-minute test timeout.
func TestBrokerCloseWhileClientsDisconnect(t *testing.T) {
	for i := range 20 {
		t.Run(fmt.Sprintf("%02d", i), func(t *testing.T) {
			s := newTestSim(t, 4, nil)
			const clients = 8
			conns := make([]*mqttclient.Client, clients)
			for j := range conns {
				conns[j] = mqttclient.Dial(t, s.broker.Address(), fmt.Sprintf("close-%02d-%d", i, j))
			}

			var wg sync.WaitGroup
			for _, c := range conns {
				wg.Go(func() { _ = c.Close() })
			}

			done := make(chan error, 1)
			go func() { done <- s.broker.Close() }()

			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("Close: %v", err)
				}
			case <-time.After(6 * time.Second):
				t.Fatal("broker Close blocked (deadlock)")
			}
			wg.Wait()
		})
	}
}
