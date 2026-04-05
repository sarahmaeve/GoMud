package web

import (
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/GoMudEngine/GoMud/internal/mudlog"
	"github.com/GoMudEngine/GoMud/internal/ratelimit"
	"github.com/GoMudEngine/GoMud/internal/users"
)

var (
	authCache   = map[string]time.Time{}
	authCacheMu sync.RWMutex
)

// webAuthLimiter is the shared rate limiter for the admin web panel.
var webAuthLimiter = ratelimit.New()

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

// pruneAuthCacheLocked removes expired entries from authCache.
// Must be called with authCacheMu held for writing.
func pruneAuthCacheLocked() {
	now := time.Now()
	for key, expiry := range authCache {
		if !expiry.After(now) {
			delete(authCache, key)
		}
	}
}

func doBasicAuth(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		ip := remoteIP(r)

		// Reject rate-limited IPs before doing any credential work.
		if webAuthLimiter.IsBlocked(ip) {
			mudlog.Warn("ADMIN LOGIN BLOCKED", "ip", ip, "reason", "rate limited")
			w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
			http.Error(w, "Too many failed attempts. Please try again later.", http.StatusTooManyRequests)
			return
		}

		authHeader := r.Header.Get("Authorization")

		// Fast path: check the auth cache under a read lock.
		authCacheMu.RLock()
		cachedExpiry, cached := authCache[authHeader]
		authCacheMu.RUnlock()

		if cached {
			if cachedExpiry.After(time.Now()) {
				next.ServeHTTP(w, r)
				return
			}
			// Entry is expired — promote to write lock to remove it.
			authCacheMu.Lock()
			delete(authCache, authHeader)
			authCacheMu.Unlock()
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

						webAuthLimiter.RecordSuccess(ip)

						// Cache auth for 30 minutes to avoid re-auth every load.
						// Acquire write lock, prune stale entries opportunistically,
						// then insert the new entry.
						authCacheMu.Lock()
						pruneAuthCacheLocked()
						authCache[authHeader] = time.Now().Add(time.Minute * 30)
						authCacheMu.Unlock()

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

		// Only record a failed attempt if credentials were actually provided.
		// An empty Authorization header is part of the RFC 7617 challenge-response
		// flow (browser must request without credentials first to receive 401 +
		// WWW-Authenticate). Counting that as a failure would lock out legitimate
		// admins on their initial page load.
		if authHeader != "" {
			webAuthLimiter.RecordFailure(ip)
		}

		// If the Authentication header is not present, is invalid, or the
		// username or password is wrong, then set a WWW-Authenticate
		// header to inform the client that we expect them to use basic
		// authentication and send a 401 Unauthorized response.
		w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})
}
