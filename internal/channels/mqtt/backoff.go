package mqtt

import (
	"hash/fnv"
	"strconv"
	"time"
)

// jitterFunc maps (clientID, attempt) to [0, 1). Tests inject fixed values;
// production uses fnvJitter so the same node and attempt always produce the
// same delay without global random state.
type jitterFunc func(clientID string, attempt int) float64

func fnvJitter(clientID string, attempt int) float64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(clientID))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(strconv.Itoa(attempt)))
	// Keep 53 bits so the conversion to float64 is exact.
	return float64(h.Sum64()>>11) / float64(uint64(1)<<53)
}

// backoff returns the delay before the next attempt, using equal jitter in
// [base/2, base] clamped to [min, max]. The first retry is min.
func (c *Channel) backoff(attempt int) time.Duration {
	min, max := c.opts.Reconnect.Min, c.opts.Reconnect.Max
	base := min
	for i := 0; i < attempt; i++ {
		if base >= max || base > max/2 {
			base = max
			break
		}
		base *= 2
	}
	if base > max {
		base = max
	}
	half := base / 2
	delay := half + time.Duration(c.jitter(c.deriv.clientID, attempt)*float64(half))
	if delay < min {
		delay = min
	}
	if delay > max {
		delay = max
	}
	return delay
}
