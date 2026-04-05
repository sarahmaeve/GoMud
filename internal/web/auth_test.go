package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/GoMudEngine/GoMud/internal/mudlog"
	"github.com/GoMudEngine/GoMud/internal/ratelimit"
)

// TestMain initialises the global slog logger so that mudlog calls inside
// production code do not panic with a nil slogInstance during tests.
func TestMain(m *testing.M) {
	mudlog.SetupLogger(nil, "", "", false)
	os.Exit(m.Run())
}

// TestAuthCache_ConcurrentAccess verifies that concurrent reads and writes to
// authCache do not produce a data race. Run with -race.
func TestAuthCache_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	const goroutines = 100

	// Pre-populate a few entries so readers have something to find.
	authCacheMu.Lock()
	for i := 0; i < 5; i++ {
		authCache[string(rune('a'+i))] = time.Now().Add(time.Minute)
	}
	authCacheMu.Unlock()

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			key := string(rune('a' + i%5))
			switch i % 3 {
			case 0:
				// Read
				authCacheMu.RLock()
				_ = authCache[key]
				authCacheMu.RUnlock()
			case 1:
				// Write / add
				authCacheMu.Lock()
				authCache[key] = time.Now().Add(time.Minute)
				authCacheMu.Unlock()
			case 2:
				// Prune
				authCacheMu.Lock()
				pruneAuthCacheLocked()
				authCacheMu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	// Clean up global state so other tests start fresh.
	authCacheMu.Lock()
	authCache = map[string]time.Time{}
	authCacheMu.Unlock()
}

// TestPruneAuthCacheLocked_RemovesExpiredEntries verifies that only expired
// entries are pruned and live entries are retained.
func TestPruneAuthCacheLocked_RemovesExpiredEntries(t *testing.T) {
	t.Parallel()

	// Operate on a local snapshot to avoid touching the package-level map from
	// other concurrent tests. We use white-box access to call the helper
	// directly, so we must temporarily swap the map.
	authCacheMu.Lock()
	original := authCache
	authCache = map[string]time.Time{
		"expired": time.Now().Add(-1 * time.Second),
		"live":    time.Now().Add(1 * time.Hour),
	}
	pruneAuthCacheLocked()
	snapshot := authCache
	authCache = original
	authCacheMu.Unlock()

	if _, ok := snapshot["expired"]; ok {
		t.Error("pruneAuthCacheLocked should have removed the expired entry")
	}
	if _, ok := snapshot["live"]; !ok {
		t.Error("pruneAuthCacheLocked should have retained the live entry")
	}
}

// TestDoBasicAuth_EmptyHeaderDoesNotRecordFailure verifies that credential-less
// requests (the RFC 7617 initial challenge-response probe) do not increment the
// rate-limiter's failure counter, preventing legitimate admins from locking
// themselves out on initial page load.
func TestDoBasicAuth_EmptyHeaderDoesNotRecordFailure(t *testing.T) {
	// Reset the limiter for a clean baseline.
	webAuthLimiter = ratelimit.New()
	t.Cleanup(func() { webAuthLimiter = ratelimit.New() })

	handler := doBasicAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Send 10 requests with no Authorization header from the same non-localhost IP.
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
		req.RemoteAddr = "203.0.113.5:12345"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		// Each should get 401, not 429.
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("request %d: expected 401, got %d", i, rr.Code)
		}
	}

	// After 10 credential-less requests the limiter must NOT have blocked this IP.
	if webAuthLimiter.IsBlocked("203.0.113.5") {
		t.Error("expected IP to NOT be rate-limited after 10 credential-less requests")
	}
}

// TestDoBasicAuth_WrongCredentialsRecordFailure verifies that requests carrying
// an Authorization header with incorrect credentials do increment the failure
// counter and eventually trigger the rate limiter.
func TestDoBasicAuth_WrongCredentialsRecordFailure(t *testing.T) {
	webAuthLimiter = ratelimit.New()
	t.Cleanup(func() { webAuthLimiter = ratelimit.New() })

	handler := doBasicAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Send 5 requests with wrong credentials from the same IP.
	// Progressive backoff: 5 failures → blocked (10 s).
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
		req.RemoteAddr = "203.0.113.6:12345"
		req.SetBasicAuth("nosuchuser", "nosuchpass")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}

	// After 5 wrong-credential attempts the limiter SHOULD have blocked this IP.
	if !webAuthLimiter.IsBlocked("203.0.113.6") {
		t.Error("expected IP to be rate-limited after 5 wrong-credential requests")
	}
}
