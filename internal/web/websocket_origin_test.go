package web

// White-box tests: same package so checkWebSocketOrigin (unexported) is accessible.

import (
	"net/http/httptest"
	"testing"

	"github.com/GoMudEngine/GoMud/internal/configs"
)

func TestCheckWebSocketOrigin(t *testing.T) {
	// Not parallel: test cases share global config state via SetTestNetwork.

	tests := []struct {
		name           string
		origin         string   // value of the Origin request header ("" means omit)
		requestHost    string   // value of the Host request header
		allowedOrigins string   // AllowedWebOrigins config value ("" means unset)
		wantAllowed    bool
	}{
		{
			name:        "empty origin allows non-browser clients",
			origin:      "",
			requestHost: "mymud.example.com",
			wantAllowed: true,
		},
		{
			name:        "localhost origin always allowed",
			origin:      "http://localhost:8080",
			requestHost: "mymud.example.com",
			wantAllowed: true,
		},
		{
			name:        "127.0.0.1 origin always allowed",
			origin:      "http://127.0.0.1:3000",
			requestHost: "mymud.example.com",
			wantAllowed: true,
		},
		{
			name:        "IPv6 loopback ::1 always allowed",
			origin:      "http://[::1]:9000",
			requestHost: "mymud.example.com",
			wantAllowed: true,
		},
		{
			name:        "same origin as request host allowed",
			origin:      "https://mymud.example.com",
			requestHost: "mymud.example.com",
			wantAllowed: true,
		},
		{
			name:        "same origin with port as request host allowed",
			origin:      "http://mymud.example.com:8080",
			requestHost: "mymud.example.com:8080",
			wantAllowed: true,
		},
		{
			name:        "attacker origin with no allowlist rejected",
			origin:      "https://attacker.example.com",
			requestHost: "mymud.example.com",
			wantAllowed: false,
		},
		{
			name:           "origin in configured allowlist allowed",
			origin:         "https://trusted.example.com",
			requestHost:    "mymud.example.com",
			allowedOrigins: "trusted.example.com,other.example.com",
			wantAllowed:    true,
		},
		{
			name:           "origin with port in configured allowlist allowed",
			origin:         "https://trusted.example.com:8443",
			requestHost:    "mymud.example.com",
			allowedOrigins: "trusted.example.com:8443",
			wantAllowed:    true,
		},
		{
			name:           "attacker origin not in allowlist rejected",
			origin:         "https://attacker.example.com",
			requestHost:    "mymud.example.com",
			allowedOrigins: "trusted.example.com",
			wantAllowed:    false,
		},
		{
			name:        "malformed origin rejected",
			origin:      "://not a valid url",
			requestHost: "mymud.example.com",
			wantAllowed: false,
		},
		{
			name:           "whitespace in allowlist entry is trimmed",
			origin:         "https://trusted.example.com",
			requestHost:    "mymud.example.com",
			allowedOrigins: " trusted.example.com , other.example.com ",
			wantAllowed:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Not parallel: subtests share global config state via SetTestNetwork.

			// Set up network config for this test case.
			configs.SetTestNetwork(configs.Network{
				AllowedWebOrigins: configs.ConfigString(tt.allowedOrigins),
			})

			req := httptest.NewRequest("GET", "/ws", nil)
			req.Host = tt.requestHost
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}

			got := checkWebSocketOrigin(req)
			if got != tt.wantAllowed {
				t.Errorf("checkWebSocketOrigin() = %v, want %v (origin=%q, host=%q, allowlist=%q)",
					got, tt.wantAllowed, tt.origin, tt.requestHost, tt.allowedOrigins)
			}
		})
	}
}
