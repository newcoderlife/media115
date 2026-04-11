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
func (r *RateLimiter) Acquire() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.waitForSlot(); err != nil {
		return err
	}
	r.count++
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

// waitForSlot is the inner loop called with the mutex already held.
func (r *RateLimiter) waitForSlot() error {
	if r.cache == nil {
		return nil
	}

	minInterval := 1.0 / r.qps

	for {
		now := float64(time.Now().UnixNano()) / 1e9

		if r.cache.TryAcquireSlot(r.name, now, minInterval, r.qpm) {
			return nil
		}

		// Failed — inspect why.
		state := r.cache.GetRateLimit(r.name)

		// Cooldown active?
		if now < state.CooldownUntil {
			remaining := int(state.CooldownUntil - now)
			mins := remaining / 60
			secs := remaining % 60
			return fmt.Errorf("rate limit cooldown: %dm%02ds remaining", mins, secs)
		}

		// QPM exceeded?
		minuteStart := state.LastRequest // fallback; cache stores it internally
		_ = minuteStart
		// Re-read full state from DB to decide: the cache row tracks minute_start
		// but GetRateLimit only returns CooldownUntil, LastRequest, MinuteCount.
		// Use LastRequest as minute_start proxy; the real minute_start is in TryAcquireSlot logic.
		// We check MinuteCount against qpm.
		if state.MinuteCount >= r.qpm {
			// Wait until the minute window resets. We estimate based on LastRequest.
			wait := 60.0 - (now - state.LastRequest)
			if wait < 0 {
				wait = 0.1
			}
			slog.Debug("THROTTLE QPM",
				"limiter", r.name,
				"qpm", r.qpm,
				"count", state.MinuteCount,
				"wait_s", fmt.Sprintf("%.1f", wait),
			)
			sleep(wait)
			continue
		}

		// QPS too fast?
		wait := minInterval - (now - state.LastRequest)
		if wait > 0 {
			slog.Debug("THROTTLE QPS",
				"limiter", r.name,
				"qps", r.qps,
				"wait_s", fmt.Sprintf("%.1f", wait),
			)
			sleep(wait)
		} else {
			// Edge case: slot not obtained yet but no obvious reason — brief pause.
			sleep(0.05)
		}
	}
}

// sleep sleeps for the given number of seconds (float).
func sleep(seconds float64) {
	time.Sleep(time.Duration(seconds * float64(time.Second)))
}
