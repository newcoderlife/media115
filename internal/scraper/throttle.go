package scraper

import (
	"sync"
	"time"
)

// Throttle is a simple rate limiter that enforces a minimum interval between calls.
type Throttle struct {
	interval time.Duration
	last     time.Time
	mu       sync.Mutex
}

// NewThrottle creates a Throttle that allows qps queries per second.
func NewThrottle(qps float64) *Throttle {
	return &Throttle{interval: time.Duration(float64(time.Second) / qps)}
}

// Wait blocks until the minimum interval since the last call has elapsed.
func (t *Throttle) Wait() {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	wait := t.interval - now.Sub(t.last)
	if wait > 0 {
		time.Sleep(wait)
	}
	t.last = time.Now()
}
