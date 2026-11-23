package mqtt

import (
	"sync"

	"github.com/dxinli/collect-code/internal/interfaces"
)

// subscription is one registered inbound filter. It survives reconnects and
// enforces a bounded delivery buffer; a consumer that stalls past
// deliveryBlockTimeout loses the subscription (and sees its channel closed).
type subscription struct {
	parent *Channel
	filter string
	ch     chan interfaces.Message
	done   chan struct{}
	once   sync.Once
	sendMu sync.RWMutex
}

func newSubscription(parent *Channel, filter string) *subscription {
	return &subscription{
		parent: parent,
		filter: filter,
		ch:     make(chan interfaces.Message, subscribeBuffer),
		done:   make(chan struct{}),
	}
}

func (s *subscription) C() <-chan interfaces.Message { return s.ch }

// Close unblocks any blocked delivery, removes the registration, and closes
// the delivery channel. It is idempotent.
func (s *subscription) Close() error {
	s.once.Do(func() {
		close(s.done)
		s.parent.removeSubscription(s)
		s.sendMu.Lock()
		close(s.ch)
		s.sendMu.Unlock()
	})
	return nil
}

// deliver copies nothing (the transport already copied the payload) and
// applies bounded backpressure: a normal send, then at most
// deliveryBlockTimeout of blocking, then overflow close.
func (s *subscription) deliver(msg interfaces.Message) {
	overflow := false
	func() {
		s.sendMu.RLock()
		defer s.sendMu.RUnlock()

		select {
		case <-s.done:
			return
		default:
		}
		select {
		case s.ch <- msg:
			return
		default:
		}

		timer := s.parent.clock.NewTimer(deliveryBlockTimeout)
		defer timer.Stop()
		select {
		case s.ch <- msg:
		case <-s.done:
		case <-timer.C():
			overflow = true
		}
	}()
	if overflow {
		s.parent.log.Warn("subscription overflow; closing", "filter", s.filter)
		_ = s.Close()
	}
}
