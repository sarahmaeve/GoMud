package web

import (
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/GoMudEngine/GoMud/internal/mudlog"
	"github.com/GoMudEngine/GoMud/internal/users"
)

var (
	authCache = map[string]time.Time{}
)

// webAuthLimiter is a simple IP-based rate limiter for the admin web panel.
// Progressive backoff: 3 failures = 2 s, 5 = 10 s, 10+ = 30 s.
// Localhost (127.0.0.1, ::1) is never blocked.
var webAuthLimiter = &webRateLimiter{attempts: make(map[string]*webAttemptInfo)}

type webAttemptInfo struct {
	failures     int
	blockedUntil time.Time
}

type webRateLimiter struct {
	mu       sync.Mutex
	attempts map[string]*webAttemptInfo
}

func (wl *webRateLimiter) isBlocked(ip string) bool {
	if isWebLocalhost(ip) {
		return false
	}
	wl.mu.Lock()
	defer wl.mu.Unlock()
	info, ok := wl.attempts[ip]
	if !ok {
		return false
	}
	return time.Now().Before(info.blockedUntil)
}

func (wl *webRateLimiter) recordFailure(ip string) {
	if isWebLocalhost(ip) {
		return
	}
	wl.mu.Lock()
	defer wl.mu.Unlock()
	info, ok := wl.attempts[ip]
	if !ok {
		info = &webAttemptInfo{}
		wl.attempts[ip] = info
	}
	info.failures++
	var d time.Duration
	switch {
	case info.failures >= 10:
		d = 30 * time.Second
	case info.failures >= 5:
		d = 10 * time.Second
	case info.failures >= 3:
		d = 2 * time.Second
	}
	if d > 0 {
		info.blockedUntil = time.Now().Add(d)
	}
}

func (wl *webRateLimiter) recordSuccess(ip string) {
	if isWebLocalhost(ip) {
		return
	}
	wl.mu.Lock()
	defer wl.mu.Unlock()
	delete(wl.attempts, ip)
}

func isWebLocalhost(ip string) bool {
	if ip == "127.0.0.1" || ip == "::1" {
		return true
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	return parsed.IsLoopback()
}

// remoteIP extracts the IP address from an HTTP request's RemoteAddr.
func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func handlerToHandlerFunc(h http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r)
	}
}

func doBasicAuth(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		ip := remoteIP(r)

		// Reject rate-limited IPs before doing any credential work.
		if webAuthLimiter.isBlocked(ip) {
			mudlog.Warn("ADMIN LOGIN BLOCKED", "ip", ip, "reason", "rate limited")
			w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
			http.Error(w, "Too many failed attempts. Please try again later.", http.StatusTooManyRequests)
			return
		}

		authHeader := r.Header.Get("Authorization")

		if t, ok := authCache[authHeader]; ok {

			if t.After(time.Now()) {
				next.ServeHTTP(w, r)
				return
			}

			delete(authCache, authHeader)
		}

		// Extract the username and password from the request
		// Authorization header. If no Authentication header is present
		// or the header value is invalid, then the 'ok' return value
		// will be false.
		username, password, ok := r.BasicAuth()
		if ok {

			// Authorize against actual user record
			uRecord, err := users.LoadUser(username, true)
			if err == nil {

				if uRecord.PasswordMatches(password) {

					if uRecord.Role != users.RoleUser {

						mudlog.Warn("ADMIN LOGIN", "username", username, "success", true)

						webAuthLimiter.recordSuccess(ip)

						// Cache auth for 30 minutes to avoid re-auth every load
						authCache[authHeader] = time.Now().Add(time.Minute * 30)

						next.ServeHTTP(w, r)
						return

					} else {

						mudlog.Error("ADMIN LOGIN", "username", username, "success", false, "error", `Role=`+uRecord.Role)

					}
				}

			} else {
				mudlog.Error("ADMIN LOGIN", "username", username, "success", false, "error", err)
			}
		}

		// Record the failed attempt before responding.
		webAuthLimiter.recordFailure(ip)

		// If the Authentication header is not present, is invalid, or the
		// username or password is wrong, then set a WWW-Authenticate
		// header to inform the client that we expect them to use basic
		// authentication and send a 401 Unauthorized response.
		w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})
}
