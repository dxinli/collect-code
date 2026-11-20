// Package fake provides in-memory implementations of the northbound seams
// for tests. They record what the kernel publishes and return deterministic
// values; they implement no wire encoding.
package fake

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/dxinli/collect-code/internal/interfaces"
)

// Channel is an in-memory interfaces.Channel.
type Channel struct {
	mu        sync.Mutex
	published []interfaces.Message
	subs      []*subscription
	err       error
	status    interfaces.ChannelStatus
	closed    bool
	dropped   int
}

// NewChannel returns an open fake channel.
func NewChannel() *Channel { return &Channel{} }

// Publish records msg and delivers it to matching subscribers.
func (c *Channel) Publish(_ context.Context, msg interfaces.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case c.closed:
		return fmt.Errorf("fake channel: %w", interfaces.ErrClosed)
	case c.err != nil:
		return fmt.Errorf("fake channel: %w", c.err)
	}
	c.published = append(c.published, msg)
	c.deliverLocked(msg)
	return nil
}

// Subscribe registers a subscription for filter. Cancelling ctx closes it.
func (c *Channel) Subscribe(ctx context.Context, filter string) (interfaces.Subscription, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, fmt.Errorf("fake channel: %w", interfaces.ErrClosed)
	}
	sub := &subscription{
		parent: c,
		filter: filter,
		ch:     make(chan interfaces.Message, 16),
		done:   make(chan struct{}),
	}
	c.subs = append(c.subs, sub)
	c.mu.Unlock()

	if ctx != nil {
		go func() {
			select {
			case <-ctx.Done():
				_ = sub.Close()
			case <-sub.done:
			}
		}()
	}
	return sub, nil
}

// Status returns the last configured status.
func (c *Channel) Status() interfaces.ChannelStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

// Close closes every subscription.
func (c *Channel) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	for _, sub := range c.subs {
		sub.closeLocked()
	}
	c.subs = nil
	return nil
}

// Published returns a copy of everything successfully published.
func (c *Channel) Published() []interfaces.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]interfaces.Message(nil), c.published...)
}

// Fail makes every later Publish return an error wrapping err. A nil err
// clears the failure.
func (c *Channel) Fail(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.err = err
}

// SetStatus replaces the reported status.
func (c *Channel) SetStatus(st interfaces.ChannelStatus) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status = st
}

// Inject delivers msg to matching subscribers without recording it.
func (c *Channel) Inject(msg interfaces.Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.deliverLocked(msg)
}

// Dropped reports deliveries discarded because a subscriber was not reading.
func (c *Channel) Dropped() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dropped
}

func (c *Channel) deliverLocked(msg interfaces.Message) {
	for _, sub := range c.subs {
		if !topicMatch(sub.filter, msg.Topic) {
			continue
		}
		select {
		case sub.ch <- msg:
		default:
			c.dropped++
		}
	}
}

func (c *Channel) removeLocked(target *subscription) {
	for i, sub := range c.subs {
		if sub == target {
			c.subs = append(c.subs[:i], c.subs[i+1:]...)
			return
		}
	}
}

type subscription struct {
	parent *Channel
	filter string
	ch     chan interfaces.Message
	done   chan struct{}
	once   sync.Once
}

func (s *subscription) C() <-chan interfaces.Message { return s.ch }

func (s *subscription) Close() error {
	s.parent.mu.Lock()
	defer s.parent.mu.Unlock()
	s.parent.removeLocked(s)
	s.closeLocked()
	return nil
}

func (s *subscription) closeLocked() {
	s.once.Do(func() {
		close(s.done)
		close(s.ch)
	})
}

// topicMatch implements the MQTT wildcard subset used by the fake:
// exact segments, "+" for one segment and "#" for the remainder.
func topicMatch(filter, topic string) bool {
	f := strings.Split(filter, "/")
	t := strings.Split(topic, "/")
	for i, part := range f {
		if part == "#" {
			return true
		}
		if i >= len(t) {
			return false
		}
		if part != "+" && part != t[i] {
			return false
		}
	}
	return len(f) == len(t)
}
