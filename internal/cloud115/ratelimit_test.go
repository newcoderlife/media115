package cloud115

import (
	"path/filepath"
	"strings"
	"testing"
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
