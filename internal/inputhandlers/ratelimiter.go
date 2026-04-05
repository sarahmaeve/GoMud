package inputhandlers

import (
	"net"
	"sync"
	"time"
)

type attemptInfo struct {
	failures     int
	lastFailure  time.Time
	blockedUntil time.Time
}

// LoginRateLimiter tracks failed login attempts per IP and enforces progressive
// backoff to defend against brute-force credential stuffing.
type LoginRateLimiter struct {
	mu       sync.Mutex
	attempts map[string]*attemptInfo
	disabled bool // for testing
}

var defaultRateLimiter = NewLoginRateLimiter()

// NewLoginRateLimiter creates a ready-to-use LoginRateLimiter.
func NewLoginRateLimiter() *LoginRateLimiter {
	return &LoginRateLimiter{
		attempts: make(map[string]*attemptInfo),
	}
}

// IsBlocked returns true if the IP is currently rate-limited.
// Localhost (127.0.0.1 and ::1) is never blocked — admin backdoor.
func (l *LoginRateLimiter) IsBlocked(ip string) bool {
	if l.disabled {
		return false
	}
	if isLocalhost(ip) {
		return false
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	info, ok := l.attempts[ip]
	if !ok {
		return false
	}
	return time.Now().Before(info.blockedUntil)
}

// RecordFailure records a failed login attempt for the given IP and sets
// progressive backoff: 2 s after 3 failures, 10 s after 5, 30 s after 10+.
func (l *LoginRateLimiter) RecordFailure(ip string) {
	if isLocalhost(ip) {
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
}

// RecordSuccess clears the failure record for the given IP.
func (l *LoginRateLimiter) RecordSuccess(ip string) {
	if isLocalhost(ip) {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.attempts, ip)
}

// Reset clears all state. Intended for testing.
func (l *LoginRateLimiter) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.attempts = make(map[string]*attemptInfo)
}

// SetDisabled disables or enables rate limiting. Intended for testing.
func (l *LoginRateLimiter) SetDisabled(v bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.disabled = v
}

// isLocalhost returns true for loopback addresses (127.0.0.1 and ::1).
// These are never subject to rate limiting as an admin backdoor.
func isLocalhost(ip string) bool {
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
