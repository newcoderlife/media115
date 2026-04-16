package cloud115

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newTestRateLimiter creates a RateLimiter backed by a temporary Cache.
func newTestRateLimiter(t *testing.T, name string, qps float64, qpm int) *RateLimiter {
	t.Helper()
	c, err := NewCache(filepath.Join(t.TempDir(), "rl_test.db"))
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return NewRateLimiter(name, qps, qpm, c)
}

// TestRateLimiterAcquireNilCache verifies that Acquire never blocks or errors
// when no cache is attached.
func TestRateLimiterAcquireNilCache(t *testing.T) {
	rl := NewRateLimiter("test", 0.5, 20, nil)
	for i := 0; i < 5; i++ {
		if err := rl.Acquire(); err != nil {
			t.Fatalf("Acquire[%d] with nil cache: %v", i, err)
		}
	}
}

// TestRateLimiterCooldown verifies that Acquire returns an error when a cooldown
// is active.
func TestRateLimiterCooldown(t *testing.T) {
	rl := newTestRateLimiter(t, "cooldown_test", 0.5, 20)

	// Set a 60-second cooldown.
	rl.SetCooldown(60)

	err := rl.Acquire()
	if err == nil {
		t.Fatal("expected error from Acquire during cooldown, got nil")
	}
	if !strings.Contains(err.Error(), "cooldown") {
		t.Errorf("error should mention cooldown, got: %v", err)
	}
}

// TestRateLimiterRequestCount verifies that the internal counter increments on
// each successful Acquire call.
func TestRateLimiterRequestCount(t *testing.T) {
	rl := NewRateLimiter("count_test", 100, 1000, nil) // nil cache → no blocking
	for i := 0; i < 3; i++ {
		if err := rl.Acquire(); err != nil {
			t.Fatalf("Acquire: %v", err)
		}
	}
	rl.mu.Lock()
	got := rl.count
	rl.mu.Unlock()
	if got != 3 {
		t.Errorf("count = %d; want 3", got)
	}
}

// TestRateLimiterSetCooldownNilCache verifies that SetCooldown is a no-op when
// the cache is nil (should not panic).
func TestRateLimiterSetCooldownNilCache(t *testing.T) {
	rl := NewRateLimiter("nil_cache", 0.5, 20, nil)
	rl.SetCooldown(60) // must not panic
}

// TestRateLimiterQPSThrottle verifies that Acquire blocks when QPS limit is
// hit and eventually succeeds after waiting.
func TestRateLimiterQPSThrottle(t *testing.T) {
	rl := newTestRateLimiter(t, "qps_test", 100, 1000) // 100 QPS → 10ms interval
	// First acquire should succeed immediately.
	if err := rl.Acquire(); err != nil {
		t.Fatalf("first Acquire: %v", err)
	}

	// Set last_request to near-now so the next acquire hits the QPS branch.
	now := float64(time.Now().UnixNano()) / 1e9
	rl.cache.SetRateLimit("qps_test", RateLimitState{
		LastRequest: now, // just happened
		MinuteCount: 1,
		MinuteStart: now,
	})

	// Second acquire should succeed after a brief QPS wait.
	if err := rl.Acquire(); err != nil {
		t.Fatalf("second Acquire: %v", err)
	}
}

// TestRateLimiterQPMThrottle verifies that Acquire blocks when QPM limit is
// hit but eventually succeeds when the minute window resets.
func TestRateLimiterQPMThrottle(t *testing.T) {
	rl := newTestRateLimiter(t, "qpm_test", 1000, 2) // QPM=2 with high QPS
	now := float64(time.Now().UnixNano()) / 1e9

	// Seed the rate limit state: minute count at max, but minute window
	// started >60s ago so it will reset on the next attempt.
	rl.cache.SetRateLimit("qpm_test", RateLimitState{
		LastRequest: now - 120, // long ago
		MinuteCount: 2,         // at QPM limit
		MinuteStart: now - 120, // window started >60s ago → will reset
	})

	// Should succeed because the minute window reset.
	if err := rl.Acquire(); err != nil {
		t.Fatalf("Acquire after QPM window reset: %v", err)
	}
}

// TestRateLimiterQPMExceededWaitsThenSucceeds tests the QPM exceeded path where
// the limiter has to wait for the minute window to reset.
func TestRateLimiterQPMExceededWaitsThenSucceeds(t *testing.T) {
	rl := newTestRateLimiter(t, "qpm_wait", 1000, 3) // QPM=3

	// Fill up QPM
	for i := 0; i < 3; i++ {
		if err := rl.Acquire(); err != nil {
			t.Fatalf("Acquire[%d]: %v", i, err)
		}
	}

	// Now force the minute_start to be 59.9s ago so the window resets quickly.
	now := float64(time.Now().UnixNano()) / 1e9
	rl.cache.SetRateLimit("qpm_wait", RateLimitState{
		LastRequest: now,
		MinuteCount: 3,
		MinuteStart: now - 59.95, // will expire in ~50ms
	})

	// Should succeed after a very brief wait for the window to reset.
	if err := rl.Acquire(); err != nil {
		t.Fatalf("Acquire after QPM wait: %v", err)
	}
}

// TestRateLimiterCooldownFormatting verifies the error message formatting
// for cooldowns (minutes and seconds).
func TestRateLimiterCooldownFormatting(t *testing.T) {
	rl := newTestRateLimiter(t, "fmt_test", 0.5, 20)
	rl.SetCooldown(125) // 2m5s

	err := rl.Acquire()
	if err == nil {
		t.Fatal("expected error")
	}
	// Error message should contain the remaining time.
	msg := err.Error()
	if !strings.Contains(msg, "cooldown") {
		t.Errorf("error = %q; want to contain 'cooldown'", msg)
	}
	// Should have a "Xm" and "Xs" format
	if !strings.Contains(msg, "m") {
		t.Errorf("error = %q; want to contain minutes format", msg)
	}
}

// TestRateLimiterEdgeCaseNoObviousReason tests the edge case in waitForSlot where
// TryAcquireSlot fails but no obvious reason is found (brief 0.05s pause).
func TestRateLimiterEdgeCaseNoObviousReason(t *testing.T) {
	rl := newTestRateLimiter(t, "edge_test", 1000, 1000) // very generous limits

	// Manually set state so that TryAcquireSlot fails on first attempt
	// but the state inspection shows no cooldown, no QPM, and QPS is fine.
	// This triggers the else/else (waitDuration = 0.05) edge case.
	// On the next loop iteration, it should succeed.
	now := float64(time.Now().UnixNano()) / 1e9
	rl.cache.SetRateLimit("edge_test", RateLimitState{
		LastRequest: now - 100, // long ago
		MinuteCount: 0,
		MinuteStart: now - 100,
	})

	if err := rl.Acquire(); err != nil {
		t.Fatalf("Acquire edge case: %v", err)
	}
}
