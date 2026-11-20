package fake

import (
	"errors"
	"sync"

	"github.com/dxinli/collect-code/internal/interfaces"
)

// ErrNoInvocation is returned by ParseInvocation when the queue is empty.
var ErrNoInvocation = errors.New("fake downlink: no invocation queued")

// DownlinkAdapter serves queued invocations and records replies.
type DownlinkAdapter struct {
	mu       sync.Mutex
	queue    []interfaces.Invocation
	parseErr error
	replies  []interfaces.ServiceReply
	replyErr error
}

// NewDownlinkAdapter returns an empty fake.
func NewDownlinkAdapter() *DownlinkAdapter { return &DownlinkAdapter{} }

// ParseInvocation pops the oldest queued invocation.
func (a *DownlinkAdapter) ParseInvocation(interfaces.Message) (interfaces.Invocation, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.parseErr != nil {
		return interfaces.Invocation{}, a.parseErr
	}
	if len(a.queue) == 0 {
		return interfaces.Invocation{}, ErrNoInvocation
	}
	inv := a.queue[0]
	a.queue = a.queue[1:]
	return inv, nil
}

// ServiceReply records reply and returns a message keyed by its MsgID.
func (a *DownlinkAdapter) ServiceReply(reply interfaces.ServiceReply) (interfaces.Message, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.replyErr != nil {
		return interfaces.Message{}, a.replyErr
	}
	a.replies = append(a.replies, reply)
	return interfaces.Message{Topic: "service/reply/" + reply.DeviceSN, Payload: []byte(reply.MsgID)}, nil
}

// QueueInvocation appends an invocation for ParseInvocation to pop.
func (a *DownlinkAdapter) QueueInvocation(inv interfaces.Invocation) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.queue = append(a.queue, inv)
}

// SetParseError makes ParseInvocation fail with err; nil clears it.
func (a *DownlinkAdapter) SetParseError(err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.parseErr = err
}

// SetReplyError makes ServiceReply fail with err; nil clears it.
func (a *DownlinkAdapter) SetReplyError(err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.replyErr = err
}

// Replies returns a copy of the recorded replies.
func (a *DownlinkAdapter) Replies() []interfaces.ServiceReply {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]interfaces.ServiceReply(nil), a.replies...)
}
