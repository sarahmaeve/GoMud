// Package ratelimit provides an IP-based rate limiter with progressive backoff
// and bounded memory usage via opportunistic pruning of stale entries.
//
// It is safe for concurrent use from multiple goroutines.
package ratelimit

import (
	"net"
	"sync"
	"time"
)

const (
	defaultPruneInterval = 5 * time.Minute
	defaultEntryTTL      = 1 * time.Hour
)

type attemptInfo struct {
	failures     int
	lastFailure  time.Time
	blockedUntil time.Time
}

// Limiter is an IP-based rate limiter with progressive backoff.
// Progressive backoff thresholds: 3 failures → 2 s, 5 → 10 s, 10+ → 30 s.
// Localhost addresses (127.0.0.1, ::1, and any loopback) are never blocked.
//
// All exported methods are safe for concurrent use.
type Limiter struct {
	mu            sync.Mutex
	attempts      map[string]*attemptInfo
	disabled      bool // guarded by mu
	lastPrune     time.Time
	pruneInterval time.Duration
	entryTTL      time.Duration
}

// New returns a ready-to-use Limiter with default settings.
func New() *Limiter {
	return &Limiter{
		attempts:      make(map[string]*attemptInfo),
		lastPrune:     time.Now(),
		pruneInterval: defaultPruneInterval,
		entryTTL:      defaultEntryTTL,
	}
}

// IsBlocked returns true if the given IP is currently rate-limited.
// Localhost addresses are never blocked.
func (l *Limiter) IsBlocked(ip string) bool {
	if IsLocalhost(ip) {
		return false
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.disabled {
		return false
	}

	info, ok := l.attempts[ip]
	if !ok {
		return false
	}
	return time.Now().Before(info.blockedUntil)
}

// RecordFailure increments the failure counter for the given IP and applies
// progressive backoff. Opportunistically prunes stale entries at intervals.
func (l *Limiter) RecordFailure(ip string) {
	if IsLocalhost(ip) {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	info, ok := l.attempts[ip]
	if !ok {
		info = &attemptInfo{}
		l.attempts[ip] = info
	}

	info.failures++
	info.lastFailure = time.Now()

	var blockDuration time.Duration
	switch {
	case info.failures >= 10:
		blockDuration = 30 * time.Second
	case info.failures >= 5:
		blockDuration = 10 * time.Second
	case info.failures >= 3:
		blockDuration = 2 * time.Second
	}

	if blockDuration > 0 {
		info.blockedUntil = time.Now().Add(blockDuration)
	}

	// Opportunistic pruning — amortises cleanup cost across RecordFailure calls.
	if time.Since(l.lastPrune) > l.pruneInterval {
		l.pruneLocked()
	}
}

// RecordSuccess removes the failure record for the given IP.
func (l *Limiter) RecordSuccess(ip string) {
	if IsLocalhost(ip) {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.attempts, ip)
}

// Reset clears all state. Intended for testing.
func (l *Limiter) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.attempts = make(map[string]*attemptInfo)
}

// SetDisabled enables or disables rate limiting. When disabled, IsBlocked
// always returns false regardless of recorded failures. Intended for testing.
func (l *Limiter) SetDisabled(v bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.disabled = v
}

// pruneLocked removes entries whose last failure is older than entryTTL.
// Must be called with l.mu held.
func (l *Limiter) pruneLocked() {
	cutoff := time.Now().Add(-l.entryTTL)
	for ip, info := range l.attempts {
		if info.lastFailure.Before(cutoff) {
			delete(l.attempts, ip)
		}
	}
	l.lastPrune = time.Now()
}

// IsLocalhost returns true for any loopback address (127.x.x.x, ::1, etc.).
// These IPs are never subject to rate limiting as an admin backdoor.
func IsLocalhost(ip string) bool {
	// Fast path for the two canonical loopback strings.
	if ip == "127.0.0.1" || ip == "::1" {
		return true
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	return parsed.IsLoopback()
}
