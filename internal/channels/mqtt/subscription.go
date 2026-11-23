package mqtt

import (
	"github.com/dxinli/collect-code/internal/interfaces"
)

// subscription is one registered inbound filter. Its delivery and close
// semantics are implemented with the channel semantics.
type subscription struct {
	ch   chan interfaces.Message
	done chan struct{}
}

func (s *subscription) C() <-chan interfaces.Message { return s.ch }

func (s *subscription) Close() error { return nil }
