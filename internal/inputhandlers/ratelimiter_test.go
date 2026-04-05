package inputhandlers

import (
	"testing"
	"time"
)

// newTestLimiter returns a fresh limiter for each test to ensure isolation.
func newTestLimiter() *LoginRateLimiter {
	return NewLoginRateLimiter()
}

func TestRateLimiter_NewLimiterHasNoBlocks(t *testing.T) {
	t.Parallel()
	l := newTestLimiter()
	if l.IsBlocked("1.2.3.4") {
		t.Error("new limiter should not block any IP")
	}
}

func TestRateLimiter_SingleFailureDoesNotBlock(t *testing.T) {
	t.Parallel()
	l := newTestLimiter()
	l.RecordFailure("1.2.3.4")
	if l.IsBlocked("1.2.3.4") {
		t.Error("single failure should not block")
	}
}

func TestRateLimiter_TwoFailuresDoNotBlock(t *testing.T) {
	t.Parallel()
	l := newTestLimiter()
	l.RecordFailure("1.2.3.4")
	l.RecordFailure("1.2.3.4")
	if l.IsBlocked("1.2.3.4") {
		t.Error("two failures should not block")
	}
}

func TestRateLimiter_ThreeFailuresBlockFor2s(t *testing.T) {
	t.Parallel()
	l := newTestLimiter()

	for i := 0; i < 3; i++ {
		l.RecordFailure("1.2.3.4")
	}

	if !l.IsBlocked("1.2.3.4") {
		t.Error("3 failures should block")
	}

	// Verify the block duration is ~2 seconds by inspecting internal state.
	l.mu.Lock()
	info := l.attempts["1.2.3.4"]
	remaining := time.Until(info.blockedUntil)
	l.mu.Unlock()

	if remaining <= 0 || remaining > 2*time.Second+100*time.Millisecond {
		t.Errorf("expected ~2s block, got remaining=%v", remaining)
	}

	// Simulate time passing: set blockedUntil to the past.
	l.mu.Lock()
	l.attempts["1.2.3.4"].blockedUntil = time.Now().Add(-1 * time.Second)
	l.mu.Unlock()

	if l.IsBlocked("1.2.3.4") {
		t.Error("block should have expired after advancing time")
	}
}

func TestRateLimiter_FiveFailuresBlockFor10s(t *testing.T) {
	t.Parallel()
	l := newTestLimiter()

	for i := 0; i < 5; i++ {
		l.RecordFailure("10.0.0.1")
	}

	if !l.IsBlocked("10.0.0.1") {
		t.Error("5 failures should block")
	}

	l.mu.Lock()
	info := l.attempts["10.0.0.1"]
	remaining := time.Until(info.blockedUntil)
	l.mu.Unlock()

	if remaining <= 0 || remaining > 10*time.Second+100*time.Millisecond {
		t.Errorf("expected ~10s block, got remaining=%v", remaining)
	}

	// Simulate time passing.
	l.mu.Lock()
	l.attempts["10.0.0.1"].blockedUntil = time.Now().Add(-1 * time.Second)
	l.mu.Unlock()

	if l.IsBlocked("10.0.0.1") {
		t.Error("block should have expired after advancing time")
	}
}

func TestRateLimiter_TenFailuresBlockFor30s(t *testing.T) {
	t.Parallel()
	l := newTestLimiter()

	for i := 0; i < 10; i++ {
		l.RecordFailure("192.168.1.1")
	}

	if !l.IsBlocked("192.168.1.1") {
		t.Error("10 failures should block")
	}

	l.mu.Lock()
	info := l.attempts["192.168.1.1"]
	remaining := time.Until(info.blockedUntil)
	l.mu.Unlock()

	if remaining <= 0 || remaining > 30*time.Second+100*time.Millisecond {
		t.Errorf("expected ~30s block, got remaining=%v", remaining)
	}

	// Simulate time passing.
	l.mu.Lock()
	l.attempts["192.168.1.1"].blockedUntil = time.Now().Add(-1 * time.Second)
	l.mu.Unlock()

	if l.IsBlocked("192.168.1.1") {
		t.Error("block should have expired after advancing time")
	}
}

func TestRateLimiter_SuccessfulLoginClearsFailures(t *testing.T) {
	t.Parallel()
	l := newTestLimiter()

	for i := 0; i < 5; i++ {
		l.RecordFailure("2.3.4.5")
	}
	if !l.IsBlocked("2.3.4.5") {
		t.Fatal("should be blocked before success")
	}

	l.RecordSuccess("2.3.4.5")

	if l.IsBlocked("2.3.4.5") {
		t.Error("successful login should clear block")
	}

	// Entry should be fully removed, not just unblocked.
	l.mu.Lock()
	_, exists := l.attempts["2.3.4.5"]
	l.mu.Unlock()
	if exists {
		t.Error("RecordSuccess should delete the attempts entry")
	}
}

func TestRateLimiter_LocalhostIPv4NeverBlocked(t *testing.T) {
	t.Parallel()
	l := newTestLimiter()

	for i := 0; i < 20; i++ {
		l.RecordFailure("127.0.0.1")
	}
	if l.IsBlocked("127.0.0.1") {
		t.Error("127.0.0.1 should never be blocked")
	}
}

func TestRateLimiter_LocalhostIPv6NeverBlocked(t *testing.T) {
	t.Parallel()
	l := newTestLimiter()

	for i := 0; i < 20; i++ {
		l.RecordFailure("::1")
	}
	if l.IsBlocked("::1") {
		t.Error("::1 should never be blocked")
	}
}

func TestRateLimiter_Reset(t *testing.T) {
	t.Parallel()
	l := newTestLimiter()

	for i := 0; i < 10; i++ {
		l.RecordFailure("3.4.5.6")
	}
	if !l.IsBlocked("3.4.5.6") {
		t.Fatal("should be blocked before reset")
	}

	l.Reset()

	if l.IsBlocked("3.4.5.6") {
		t.Error("Reset should clear all blocks")
	}

	l.mu.Lock()
	count := len(l.attempts)
	l.mu.Unlock()
	if count != 0 {
		t.Errorf("Reset should empty attempts map, got %d entries", count)
	}
}

func TestRateLimiter_SetDisabledPreventsBlocking(t *testing.T) {
	t.Parallel()
	l := newTestLimiter()
	l.SetDisabled(true)

	for i := 0; i < 10; i++ {
		l.RecordFailure("4.5.6.7")
	}
	if l.IsBlocked("4.5.6.7") {
		t.Error("disabled limiter should never block")
	}

	// Re-enable and verify the underlying state is present.
	l.SetDisabled(false)
	if !l.IsBlocked("4.5.6.7") {
		t.Error("after re-enabling, recorded failures should still block")
	}
}

func TestRateLimiter_DifferentIPsTrackedIndependently(t *testing.T) {
	t.Parallel()
	l := newTestLimiter()

	// Block ip1 with 5 failures.
	for i := 0; i < 5; i++ {
		l.RecordFailure("5.6.7.8")
	}

	// ip2 has only 1 failure — should not be blocked.
	l.RecordFailure("9.8.7.6")

	if !l.IsBlocked("5.6.7.8") {
		t.Error("5.6.7.8 should be blocked")
	}
	if l.IsBlocked("9.8.7.6") {
		t.Error("9.8.7.6 should not be blocked")
	}

	// Clearing ip1 should not affect ip2.
	l.RecordSuccess("5.6.7.8")
	if l.IsBlocked("5.6.7.8") {
		t.Error("5.6.7.8 should be unblocked after success")
	}
	if l.IsBlocked("9.8.7.6") {
		t.Error("9.8.7.6 should still not be blocked")
	}
}

func TestIsLocalhost(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{"IPv4 loopback", "127.0.0.1", true},
		{"IPv6 loopback", "::1", true},
		{"IPv4 loopback non-standard", "127.0.0.2", true},
		{"public IPv4", "8.8.8.8", false},
		{"private RFC1918", "192.168.1.1", false},
		{"empty string", "", false},
		{"garbage", "not-an-ip", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isLocalhost(tt.ip)
			if got != tt.want {
				t.Errorf("isLocalhost(%q) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}
