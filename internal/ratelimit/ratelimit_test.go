package ratelimit

import (
	"sync"
	"testing"
	"time"
)

// ---- constructor / defaults ----

func TestNew_Defaults(t *testing.T) {
	t.Parallel()
	l := New()
	if l.attempts == nil {
		t.Error("New() should initialise the attempts map")
	}
	if l.pruneInterval != defaultPruneInterval {
		t.Errorf("pruneInterval = %v, want %v", l.pruneInterval, defaultPruneInterval)
	}
	if l.entryTTL != defaultEntryTTL {
		t.Errorf("entryTTL = %v, want %v", l.entryTTL, defaultEntryTTL)
	}
	if l.disabled {
		t.Error("new limiter should not be disabled")
	}
}

// ---- IsBlocked on fresh limiter ----

func TestIsBlocked_FreshLimiter(t *testing.T) {
	t.Parallel()
	l := New()
	if l.IsBlocked("1.2.3.4") {
		t.Error("fresh limiter should not block any IP")
	}
}

// ---- progressive backoff thresholds ----

func TestRecordFailure_OneFailure_NoBlock(t *testing.T) {
	t.Parallel()
	l := New()
	l.RecordFailure("1.2.3.4")
	if l.IsBlocked("1.2.3.4") {
		t.Error("single failure should not block")
	}
}

func TestRecordFailure_TwoFailures_NoBlock(t *testing.T) {
	t.Parallel()
	l := New()
	l.RecordFailure("1.2.3.4")
	l.RecordFailure("1.2.3.4")
	if l.IsBlocked("1.2.3.4") {
		t.Error("two failures should not block")
	}
}

func TestRecordFailure_ThreeFailures_Blocks2s(t *testing.T) {
	t.Parallel()
	l := New()

	for i := 0; i < 3; i++ {
		l.RecordFailure("1.2.3.4")
	}

	if !l.IsBlocked("1.2.3.4") {
		t.Fatal("3 failures should block")
	}

	// Inspect internal state directly — no time.Sleep needed.
	l.mu.Lock()
	info := l.attempts["1.2.3.4"]
	remaining := time.Until(info.blockedUntil)
	l.mu.Unlock()

	if remaining <= 0 || remaining > 2*time.Second+100*time.Millisecond {
		t.Errorf("expected ~2 s block, got remaining=%v", remaining)
	}

	// Simulate time passing by back-dating blockedUntil.
	l.mu.Lock()
	l.attempts["1.2.3.4"].blockedUntil = time.Now().Add(-1 * time.Second)
	l.mu.Unlock()

	if l.IsBlocked("1.2.3.4") {
		t.Error("block should have expired after advancing time")
	}
}

func TestRecordFailure_FiveFailures_Blocks10s(t *testing.T) {
	t.Parallel()
	l := New()

	for i := 0; i < 5; i++ {
		l.RecordFailure("10.0.0.1")
	}

	if !l.IsBlocked("10.0.0.1") {
		t.Fatal("5 failures should block")
	}

	l.mu.Lock()
	info := l.attempts["10.0.0.1"]
	remaining := time.Until(info.blockedUntil)
	l.mu.Unlock()

	if remaining <= 0 || remaining > 10*time.Second+100*time.Millisecond {
		t.Errorf("expected ~10 s block, got remaining=%v", remaining)
	}

	l.mu.Lock()
	l.attempts["10.0.0.1"].blockedUntil = time.Now().Add(-1 * time.Second)
	l.mu.Unlock()

	if l.IsBlocked("10.0.0.1") {
		t.Error("block should have expired after advancing time")
	}
}

func TestRecordFailure_TenFailures_Blocks30s(t *testing.T) {
	t.Parallel()
	l := New()

	for i := 0; i < 10; i++ {
		l.RecordFailure("192.168.1.1")
	}

	if !l.IsBlocked("192.168.1.1") {
		t.Fatal("10 failures should block")
	}

	l.mu.Lock()
	info := l.attempts["192.168.1.1"]
	remaining := time.Until(info.blockedUntil)
	l.mu.Unlock()

	if remaining <= 0 || remaining > 30*time.Second+100*time.Millisecond {
		t.Errorf("expected ~30 s block, got remaining=%v", remaining)
	}

	l.mu.Lock()
	l.attempts["192.168.1.1"].blockedUntil = time.Now().Add(-1 * time.Second)
	l.mu.Unlock()

	if l.IsBlocked("192.168.1.1") {
		t.Error("block should have expired after advancing time")
	}
}

// ---- RecordSuccess clears state ----

func TestRecordSuccess_ClearsState(t *testing.T) {
	t.Parallel()
	l := New()

	for i := 0; i < 5; i++ {
		l.RecordFailure("2.3.4.5")
	}
	if !l.IsBlocked("2.3.4.5") {
		t.Fatal("should be blocked before success")
	}

	l.RecordSuccess("2.3.4.5")

	if l.IsBlocked("2.3.4.5") {
		t.Error("RecordSuccess should unblock the IP")
	}

	// Entry must be fully removed from the map.
	l.mu.Lock()
	_, exists := l.attempts["2.3.4.5"]
	l.mu.Unlock()
	if exists {
		t.Error("RecordSuccess should delete the entry from the attempts map")
	}
}

// ---- localhost bypass ----

func TestIsBlocked_LocalhostIPv4_NeverBlocked(t *testing.T) {
	t.Parallel()
	l := New()
	for i := 0; i < 20; i++ {
		l.RecordFailure("127.0.0.1")
	}
	if l.IsBlocked("127.0.0.1") {
		t.Error("127.0.0.1 must never be blocked")
	}
}

func TestIsBlocked_LocalhostIPv6_NeverBlocked(t *testing.T) {
	t.Parallel()
	l := New()
	for i := 0; i < 20; i++ {
		l.RecordFailure("::1")
	}
	if l.IsBlocked("::1") {
		t.Error("::1 must never be blocked")
	}
}

// ---- Reset ----

func TestReset_ClearsAllState(t *testing.T) {
	t.Parallel()
	l := New()

	for i := 0; i < 10; i++ {
		l.RecordFailure("3.4.5.6")
	}
	if !l.IsBlocked("3.4.5.6") {
		t.Fatal("should be blocked before Reset")
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

// ---- SetDisabled ----

func TestSetDisabled_PreventsBlocking(t *testing.T) {
	t.Parallel()
	l := New()
	l.SetDisabled(true)

	for i := 0; i < 10; i++ {
		l.RecordFailure("4.5.6.7")
	}
	if l.IsBlocked("4.5.6.7") {
		t.Error("disabled limiter should never block")
	}

	// Re-enable — underlying state should still cause a block.
	l.SetDisabled(false)
	if !l.IsBlocked("4.5.6.7") {
		t.Error("after re-enabling, recorded failures should still block")
	}
}

// TestSetDisabled_RaceFree hammers SetDisabled and IsBlocked concurrently.
// Run with -race to verify no data race.
func TestSetDisabled_RaceFree(t *testing.T) {
	t.Parallel()
	l := New()

	var wg sync.WaitGroup
	const goroutines = 50

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				l.SetDisabled(i%4 == 0)
			} else {
				_ = l.IsBlocked("5.5.5.5")
			}
		}(i)
	}
	wg.Wait()
}

// ---- Pruning ----

func TestPruneLocked_RemovesStaleEntries(t *testing.T) {
	t.Parallel()
	l := New()
	// Shorten TTL so the test doesn't have to wait.
	l.entryTTL = 100 * time.Millisecond

	l.RecordFailure("6.7.8.9")

	// Manually back-date lastFailure beyond TTL.
	l.mu.Lock()
	l.attempts["6.7.8.9"].lastFailure = time.Now().Add(-200 * time.Millisecond)
	// Force pruneInterval to expire so the next RecordFailure triggers pruning.
	l.lastPrune = time.Now().Add(-l.pruneInterval - time.Second)
	l.mu.Unlock()

	// Trigger opportunistic pruning via a RecordFailure on a different IP.
	l.RecordFailure("9.9.9.9")

	l.mu.Lock()
	_, staleExists := l.attempts["6.7.8.9"]
	l.mu.Unlock()

	if staleExists {
		t.Error("pruneLocked should have removed the stale entry for 6.7.8.9")
	}
}

// ---- Independent IP tracking ----

func TestIndependentIPs_TrackedSeparately(t *testing.T) {
	t.Parallel()
	l := New()

	for i := 0; i < 5; i++ {
		l.RecordFailure("5.6.7.8")
	}
	l.RecordFailure("9.8.7.6")

	if !l.IsBlocked("5.6.7.8") {
		t.Error("5.6.7.8 should be blocked after 5 failures")
	}
	if l.IsBlocked("9.8.7.6") {
		t.Error("9.8.7.6 should not be blocked after 1 failure")
	}

	l.RecordSuccess("5.6.7.8")
	if l.IsBlocked("5.6.7.8") {
		t.Error("5.6.7.8 should be unblocked after RecordSuccess")
	}
	if l.IsBlocked("9.8.7.6") {
		t.Error("clearing 5.6.7.8 must not affect 9.8.7.6")
	}
}

// ---- IsLocalhost ----

func TestIsLocalhost(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{"IPv4 loopback canonical", "127.0.0.1", true},
		{"IPv6 loopback canonical", "::1", true},
		{"IPv4 loopback non-standard", "127.0.0.2", true},
		{"public IPv4", "8.8.8.8", false},
		{"private RFC1918", "192.168.1.1", false},
		{"empty string", "", false},
		{"garbage", "not-an-ip", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := IsLocalhost(tt.ip)
			if got != tt.want {
				t.Errorf("IsLocalhost(%q) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}
