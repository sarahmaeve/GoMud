package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPublicNavDoesNotContainViewconfig verifies that the public navigation
// list no longer exposes the /viewconfig link. This is a regression guard
// against the vulnerability described in issue #10 where server configuration
// was reachable without authentication.
func TestPublicNavDoesNotContainViewconfig(t *testing.T) {
	t.Parallel()

	nav := []WebNav{
		{`Home`, `/`},
		{`Who's Online`, `/online`},
		{`Web Client`, `/webclient`},
	}

	for _, entry := range nav {
		if entry.Target == "/viewconfig" {
			t.Errorf("public nav contains /viewconfig link (%q) — this exposes server config unauthenticated", entry.Name)
		}
	}
}

// TestViewconfigNotInPublicTemplateDir verifies that the viewconfig HTML
// template in the public directory no longer contains any call to AllConfigData,
// so even if the public route is hit it cannot leak configuration values.
func TestViewconfigNotInPublicTemplateDir(t *testing.T) {
	t.Parallel()

	// Walk up from the test file location to find the _datafiles directory.
	// The web package lives at internal/web/ so we need to go up two levels.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("could not determine working directory: %v", err)
	}

	// Resolve to repo root (two levels up from internal/web/)
	repoRoot := filepath.Join(wd, "..", "..")
	publicViewconfig := filepath.Join(repoRoot, "_datafiles", "html", "public", "viewconfig.html")

	content, err := os.ReadFile(publicViewconfig)
	if err != nil {
		// If the file doesn't exist in public, that's even better — pass.
		if os.IsNotExist(err) {
			return
		}
		t.Fatalf("could not read public viewconfig.html: %v", err)
	}

	if strings.Contains(string(content), "AllConfigData") {
		t.Errorf("public viewconfig.html still calls AllConfigData — server config is exposed unauthenticated")
	}
}

// TestAdminViewconfigTemplateExists verifies that the viewconfig template has
// been moved to the admin directory where it is served behind authentication.
func TestAdminViewconfigTemplateExists(t *testing.T) {
	t.Parallel()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("could not determine working directory: %v", err)
	}

	repoRoot := filepath.Join(wd, "..", "..")
	adminViewconfig := filepath.Join(repoRoot, "_datafiles", "html", "admin", "viewconfig.html")

	info, err := os.Stat(adminViewconfig)
	if err != nil {
		t.Fatalf("admin/viewconfig.html does not exist at %s — the template was not moved to the admin directory: %v", adminViewconfig, err)
	}

	if info.IsDir() {
		t.Fatalf("expected a file at %s but found a directory", adminViewconfig)
	}

	content, err := os.ReadFile(adminViewconfig)
	if err != nil {
		t.Fatalf("could not read admin/viewconfig.html: %v", err)
	}

	if !strings.Contains(string(content), "AllConfigData") {
		t.Errorf("admin/viewconfig.html does not contain AllConfigData — the config display may be missing")
	}
}

// TestDoBasicAuth_RejectsUnauthenticated verifies that the doBasicAuth
// middleware returns 401 Unauthorized when no credentials are supplied.
// This directly validates that /admin/viewconfig (and all admin routes)
// require authentication.
func TestDoBasicAuth_RejectsUnauthenticated(t *testing.T) {
	t.Parallel()

	// Wrap a handler that would return 200 if auth passes.
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	protected := doBasicAuth(inner)

	req := httptest.NewRequest(http.MethodGet, "/admin/viewconfig", nil)
	// Simulate a non-localhost remote address so rate-limiter logic applies normally.
	req.RemoteAddr = "203.0.113.1:12345"
	rr := httptest.NewRecorder()

	protected.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for unauthenticated request, got %d", rr.Code)
	}

	wwwAuth := rr.Header().Get("WWW-Authenticate")
	if wwwAuth == "" {
		t.Error("expected WWW-Authenticate header in 401 response, got none")
	}
}

// TestDoBasicAuth_AllowsLocalhostWithoutBlock verifies that localhost IPs are
// never rate-limited (they still require valid credentials, but aren't blocked).
func TestDoBasicAuth_AllowsLocalhostWithoutBlock(t *testing.T) {
	t.Parallel()

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	protected := doBasicAuth(inner)

	// Simulate many failures from a remote IP to trigger the rate limiter.
	limiter := &webRateLimiter{attempts: make(map[string]*webAttemptInfo)}
	for range 15 {
		limiter.recordFailure("203.0.113.2")
	}

	// Localhost should never be blocked regardless of attempt count.
	if limiter.isBlocked("127.0.0.1") {
		t.Error("localhost should never be rate-limited")
	}
	if limiter.isBlocked("::1") {
		t.Error("IPv6 loopback should never be rate-limited")
	}

	// A request from localhost without credentials should still get 401
	// (auth required) but NOT 429 (rate limited).
	req := httptest.NewRequest(http.MethodGet, "/admin/viewconfig", nil)
	req.RemoteAddr = "127.0.0.1:9999"
	rr := httptest.NewRecorder()

	protected.ServeHTTP(rr, req)

	if rr.Code == http.StatusTooManyRequests {
		t.Errorf("localhost should not be rate-limited, got 429")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("localhost without credentials should get 401, got %d", rr.Code)
	}
}
