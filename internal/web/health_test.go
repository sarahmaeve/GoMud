package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthHandler(t *testing.T) {
	t.Parallel()

	t.Run("returns 200 with JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()

		healthHandler(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	})

	t.Run("response body has expected structure", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()

		healthHandler(rec, req)

		var body map[string]any
		err := json.Unmarshal(rec.Body.Bytes(), &body)
		require.NoError(t, err, "response body must be valid JSON")

		assert.Equal(t, "ok", body["status"], "status field must be 'ok'")

		players, ok := body["players"]
		require.True(t, ok, "response must contain 'players' key")
		assert.IsType(t, float64(0), players, "players must be a number")
	})

	t.Run("player count reflects stats", func(t *testing.T) {
		// With no connections registered, GetStats() returns empty OnlineUsers
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()

		healthHandler(rec, req)

		var body map[string]any
		err := json.Unmarshal(rec.Body.Bytes(), &body)
		require.NoError(t, err)

		assert.Equal(t, float64(0), body["players"])
	})
}
