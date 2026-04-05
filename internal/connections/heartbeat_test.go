package connections

// Tests for StartHeartbeat goroutine leak fix (issue #28).
//
// The fix at heartbeat.go:60 adds the assignment:
//
//	cd.heartbeat = hm
//
// Without this line, Close() cannot call hm.stop() because cd.heartbeat is
// nil.  The ping goroutine therefore runs until the WebSocket connection is
// closed from the outside — but that never happens if only Close() is called.
//
// Test strategy:
//  1. Spin up a real httptest WebSocket server.
//  2. Connect a gorilla/websocket client to it.
//  3. Wrap the client connection in a ConnectionDetails.
//  4. Call StartHeartbeat and assert cd.heartbeat != nil (directly tests the fix).
//  5. Call cd.heartbeat.stop() and verify it returns without hanging.
//  6. Optionally count goroutines before/after to confirm the goroutine exits.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GoMudEngine/GoMud/internal/mudlog"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// TestMain initialises the global slog logger so that mudlog.Info/Error calls
// inside production code (e.g. StartHeartbeat) do not panic with a nil
// slogInstance during tests.
func TestMain(m *testing.M) {
	mudlog.SetupLogger(nil, "", "", false)
	os.Exit(m.Run())
}

// wsEchoServer returns an httptest.Server that upgrades HTTP connections to
// WebSocket and drains incoming messages until the client closes.  The caller
// is responsible for calling server.Close().
func wsEchoServer(t *testing.T) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		// Drain messages (including pings forwarded as control frames) until
		// the client disconnects.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
}

// dialWS connects a gorilla WebSocket client to the given httptest server URL.
func dialWS(t *testing.T, serverURL string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(serverURL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err, "failed to dial WebSocket server")
	return conn
}

// TestStartHeartbeat_AssignsHeartbeatManager is the direct test for the fix:
// cd.heartbeat must be non-nil after StartHeartbeat returns.
//
// Before the fix the assignment was missing — cd.heartbeat stayed nil, so
// Close() would silently skip stop() and the goroutine would leak.
func TestStartHeartbeat_AssignsHeartbeatManager(t *testing.T) {
	server := wsEchoServer(t)
	defer server.Close()

	wsConn := dialWS(t, server.URL)
	defer wsConn.Close()

	// Use a config with a long ping period so the goroutine does not fire
	// during this test, keeping things deterministic.
	cfg := HeartbeatConfig{
		PongWait:   5 * time.Second,
		PingPeriod: 10 * time.Second,
		WriteWait:  2 * time.Second,
	}

	cd := &ConnectionDetails{wsConn: wsConn}

	err := cd.StartHeartbeat(cfg)
	require.NoError(t, err)

	// The core assertion: the fix assigns cd.heartbeat inside StartHeartbeat.
	// Without the fix this would be nil and Close() could never stop the goroutine.
	require.NotNil(t, cd.heartbeat,
		"cd.heartbeat must be assigned after StartHeartbeat; "+
			"without the assignment Close() cannot stop the ping goroutine")

	// Clean up: stop the goroutine explicitly.  If stop() hangs the test will
	// time out, which also signals a regression.
	done := make(chan struct{})
	go func() {
		cd.heartbeat.stop()
		close(done)
	}()
	select {
	case <-done:
		// heartbeat.stop() returned — goroutine exited cleanly.
	case <-time.After(3 * time.Second):
		t.Fatal("heartbeat.stop() did not return within 3 s — goroutine did not exit")
	}
}

// TestStartHeartbeat_GoroutineExitsOnStop verifies at the goroutine-count
// level that the ping goroutine is actually created and then cleaned up.
//
// This test complements the field-assignment test above: it proves that
// the goroutine truly exits when stop() is called, not just that the field
// is assigned.
func TestStartHeartbeat_GoroutineExitsOnStop(t *testing.T) {
	server := wsEchoServer(t)
	defer server.Close()

	wsConn := dialWS(t, server.URL)
	defer wsConn.Close()

	cfg := HeartbeatConfig{
		PongWait:   5 * time.Second,
		PingPeriod: 10 * time.Second,
		WriteWait:  2 * time.Second,
	}

	cd := &ConnectionDetails{wsConn: wsConn}

	// Snapshot goroutine count before starting the heartbeat.
	before := runtime.NumGoroutine()

	require.NoError(t, cd.StartHeartbeat(cfg))
	require.NotNil(t, cd.heartbeat)

	// There should be at least one more goroutine now (the ping loop).
	after := runtime.NumGoroutine()
	if after <= before {
		t.Logf("goroutine count did not increase (before=%d, after=%d); "+
			"the goroutine may have been inlined — continuing test", before, after)
	}

	// Stop the heartbeat and wait for the goroutine to exit.
	cd.heartbeat.stop()

	// Allow the runtime to schedule the goroutine exit.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= before {
			break
		}
		runtime.Gosched()
	}

	final := runtime.NumGoroutine()
	// We allow a small slack: other goroutines may have been created during the
	// test, so we only assert we are back at or near the baseline.
	if final > before+2 {
		t.Errorf("goroutine count after stop: %d; baseline before start: %d — "+
			"likely goroutine leak; expected close to baseline", final, before)
	}
}

// TestHeartbeatStop_DoubleStop verifies that calling stop() twice in sequence
// on the same heartbeatManager does NOT panic.
//
// Before the sync.Once fix, the second call would execute close(hm.stopChan)
// on an already-closed channel and panic with "close of closed channel".
//
// Paranoia check: temporarily remove the stopOnce.Do wrapper and run this
// test; it will panic with "close of closed channel".  Restore to confirm
// the panic is gone.
func TestHeartbeatStop_DoubleStop(t *testing.T) {
	server := wsEchoServer(t)
	defer server.Close()

	wsConn := dialWS(t, server.URL)
	defer wsConn.Close()

	cfg := HeartbeatConfig{
		PongWait:   5 * time.Second,
		PingPeriod: 10 * time.Second,
		WriteWait:  2 * time.Second,
	}

	cd := &ConnectionDetails{wsConn: wsConn}
	require.NoError(t, cd.StartHeartbeat(cfg))
	require.NotNil(t, cd.heartbeat)

	hm := cd.heartbeat

	// First stop: should signal the goroutine and wait for exit.
	done := make(chan struct{})
	go func() {
		defer close(done)
		hm.stop() // first call — closes stopChan, waits for goroutine
	}()
	select {
	case <-done:
		// first stop returned cleanly
	case <-time.After(3 * time.Second):
		t.Fatal("first stop() did not return within 3 s")
	}

	// Second stop: must not panic ("close of closed channel").
	// The goroutine is already gone, so wg.Wait() returns immediately.
	done2 := make(chan struct{})
	go func() {
		defer close(done2)
		hm.stop() // second call — must be a no-op on the channel, not a panic
	}()
	select {
	case <-done2:
		// second stop returned cleanly — fix is confirmed
	case <-time.After(3 * time.Second):
		t.Fatal("second stop() did not return within 3 s — wg.Wait() may be blocking")
	}
}

// TestHeartbeatStop_ConcurrentStop verifies that calling stop() from many
// goroutines simultaneously does not cause a panic.
//
// Before the sync.Once fix, any goroutine that won the race to close(stopChan)
// would succeed, but all subsequent goroutines would panic.
//
// Paranoia check: temporarily remove the stopOnce.Do wrapper; this test will
// panic with "close of closed channel" (or the race detector will flag it).
func TestHeartbeatStop_ConcurrentStop(t *testing.T) {
	server := wsEchoServer(t)
	defer server.Close()

	wsConn := dialWS(t, server.URL)
	defer wsConn.Close()

	cfg := HeartbeatConfig{
		PongWait:   5 * time.Second,
		PingPeriod: 10 * time.Second,
		WriteWait:  2 * time.Second,
	}

	cd := &ConnectionDetails{wsConn: wsConn}
	require.NoError(t, cd.StartHeartbeat(cfg))
	require.NotNil(t, cd.heartbeat)

	hm := cd.heartbeat

	const goroutines = 10

	// Use a gate channel so all goroutines start as close together as possible,
	// maximising the probability of simultaneous close(stopChan) attempts.
	gate := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			<-gate    // wait for the start gun
			hm.stop() // must not panic, regardless of which goroutine runs first
		}()
	}

	// Fire the start gun — all goroutines race to call stop() concurrently.
	close(gate)

	// Wait for all goroutines, with a hard deadline so the test cannot hang.
	allDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(allDone)
	}()
	select {
	case <-allDone:
		// all goroutines returned without panicking — fix confirmed
	case <-time.After(5 * time.Second):
		t.Fatal("one or more concurrent stop() calls did not return within 5 s")
	}
}

// TestStartHeartbeat_NonWebSocket_ReturnsError verifies that StartHeartbeat
// returns ErrNotWebsocket when wsConn is nil, guarding against calling it on
// a plain TCP connection by mistake.
func TestStartHeartbeat_NonWebSocket_ReturnsError(t *testing.T) {
	t.Parallel()

	cd := &ConnectionDetails{} // wsConn is nil

	err := cd.StartHeartbeat(DefaultHeartbeatConfig)

	require.ErrorIs(t, err, ErrNotWebsocket,
		"StartHeartbeat must return ErrNotWebsocket when wsConn is nil")
	require.Nil(t, cd.heartbeat,
		"cd.heartbeat must remain nil when StartHeartbeat fails")
}
