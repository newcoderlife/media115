package cloud115

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// RateLimiter enforces QPS and QPM limits, persisting state to SQLite via Cache.
// When cache is nil the limiter still tracks request counts but does not block.
type RateLimiter struct {
	name  string
	qps   float64
	qpm   int
	cache *Cache
	mu    sync.Mutex
	count int // total successful acquires (for stats / tests)
}

// NewRateLimiter creates a RateLimiter.
// name   – key used in the rate_limit table (e.g. "api", "download").
// qps    – target queries per second (e.g. 0.5 means one request every 2 s).
// qpm    – maximum requests per rolling 60-second window.
// cache  – pass nil to disable persistence (no blocking beyond mutex).
func NewRateLimiter(name string, qps float64, qpm int, cache *Cache) *RateLimiter {
	return &RateLimiter{
		name:  name,
		qps:   qps,
		qpm:   qpm,
		cache: cache,
	}
}

// Acquire blocks until a slot is available, then increments the request count.
// Returns an error only when a cooldown is active.
// The mutex is released during any sleep so other goroutines are not blocked.
func (r *RateLimiter) Acquire() error {
	if err := r.waitForSlot(); err != nil {
		return err
	}
	r.mu.Lock()
	r.count++
	r.mu.Unlock()
	return nil
}

// SetCooldown persists a cooldown_until timestamp for this limiter.
// After a 429 response the caller sets a cooldown so the next Acquire will fail
// immediately instead of retrying.
func (r *RateLimiter) SetCooldown(seconds float64) {
	if r.cache == nil {
		return
	}
	until := float64(time.Now().Unix()) + seconds
	r.cache.SetRateLimit(r.name, RateLimitState{CooldownUntil: until})
	slog.Warn("COOLDOWN", "limiter", r.name, "seconds", int(seconds), "until", until)
}

// waitForSlot loops until a slot is available or a cooldown error is returned.
// The mutex is acquired only for the check and TryAcquireSlot call, then
// released before any sleep so other goroutines are not blocked.
func (r *RateLimiter) waitForSlot() error {
	if r.cache == nil {
		return nil
	}

	minInterval := 1.0 / r.qps

	for {
		r.mu.Lock()
		now := float64(time.Now().UnixNano()) / 1e9

		if r.cache.TryAcquireSlot(r.name, now, minInterval, r.qpm) {
			r.mu.Unlock()
			return nil
		}

		// Failed — inspect why.
		state := r.cache.GetRateLimit(r.name)

		// Cooldown active?
		if now < state.CooldownUntil {
			remaining := int(state.CooldownUntil - now)
			mins := remaining / 60
			secs := remaining % 60
			r.mu.Unlock()
			return fmt.Errorf("rate limit cooldown: %dm%02ds remaining", mins, secs)
		}

		var waitDuration float64

		// QPM exceeded?
		if state.MinuteCount >= r.qpm {
			// Wait until the minute window resets using the actual minute_start timestamp.
			minuteStart := state.MinuteStart
			if minuteStart == 0 {
				minuteStart = state.LastRequest
			}
			waitDuration = 60.0 - (now - minuteStart)
			if waitDuration < 0 {
				waitDuration = 0.1
			}
			slog.Debug("THROTTLE QPM",
				"limiter", r.name,
				"qpm", r.qpm,
				"count", state.MinuteCount,
				"wait_s", fmt.Sprintf("%.1f", waitDuration),
			)
		} else {
			// QPS too fast?
			waitDuration = minInterval - (now - state.LastRequest)
			if waitDuration > 0 {
				slog.Debug("THROTTLE QPS",
					"limiter", r.name,
					"qps", r.qps,
					"wait_s", fmt.Sprintf("%.1f", waitDuration),
				)
			} else {
				// Edge case: slot not obtained yet but no obvious reason — brief pause.
				waitDuration = 0.05
			}
		}

		r.mu.Unlock() // release before sleeping
		sleep(waitDuration)
	}
}

// sleep sleeps for the given number of seconds (float).
func sleep(seconds float64) {
	time.Sleep(time.Duration(seconds * float64(time.Second)))
}
