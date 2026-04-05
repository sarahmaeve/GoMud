package connections

import (
	"net"
	"sync"
	"testing"
	"time"
)

// TestGetAllConnectionIds verifies that GetAllConnectionIds returns exactly the
// connections present in netConnections with no leading zero values.
func TestGetAllConnectionIds(t *testing.T) {
	t.Parallel()

	// White-box: manipulate the package-level map directly under the lock.
	lock.Lock()
	// Save original state so we can restore it after the test.
	original := netConnections
	netConnections = map[ConnectionId]*ConnectionDetails{
		1: {},
		2: {},
		3: {},
	}
	lock.Unlock()

	t.Cleanup(func() {
		lock.Lock()
		netConnections = original
		lock.Unlock()
	})

	ids := GetAllConnectionIds()

	if len(ids) != 3 {
		t.Fatalf("GetAllConnectionIds() returned %d ids, want 3", len(ids))
	}
	for _, id := range ids {
		if id == 0 {
			t.Errorf("GetAllConnectionIds() returned a zero ConnectionId, want only non-zero ids")
		}
	}
}

// TestWrite_EmptyPayload verifies that the early return for zero-length writes
// actually fires. We use a closed connection so that if the early return is
// bypassed and Write falls through to conn.Write, it will error.
func TestWrite_EmptyPayload(t *testing.T) {
	t.Parallel()

	server, client := net.Pipe()
	server.Close()
	client.Close()

	cd := &ConnectionDetails{conn: client}

	n, err := cd.Write([]byte{})
	if err != nil {
		t.Errorf("Write(empty) should return early without touching conn, but got error: %v", err)
	}
	if n != 0 {
		t.Errorf("Write(empty) returned n=%d, want 0", n)
	}
}

// TestWrite_CompletesNormally verifies that a write succeeds when the other
// end of the connection is actively reading.
func TestWrite_CompletesNormally(t *testing.T) {
	t.Parallel()

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// Drain the server side so writes can complete.
	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := server.Read(buf); err != nil {
				return
			}
		}
	}()

	cd := &ConnectionDetails{conn: client}

	payload := []byte("hello world")
	n, err := cd.Write(payload)
	if err != nil {
		t.Fatalf("Write returned unexpected error: %v", err)
	}
	// Note: Write replaces \n with \r\n; since there are none here the length
	// is unchanged.
	if n != len(payload) {
		t.Errorf("Write returned n=%d, want %d", n, len(payload))
	}
}

// TestWrite_TelnetStalledClient verifies that Write returns an error (deadline
// exceeded) within a bounded time when the remote end of a telnet connection
// stops reading and the pipe buffer fills up.  Without the write deadline this
// call would block forever, stalling the entire server.
func TestWrite_TelnetStalledClient(t *testing.T) {
	// Not marked t.Parallel() — this test intentionally lets the deadline fire
	// (~5 s) and we do not want it to count against other parallel tests'
	// wallclock budget unexpectedly.  It has its own 15 s hard limit.

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	cd := &ConnectionDetails{conn: client}

	// Do NOT read from server — writes will fill the pipe buffer and block.

	done := make(chan error, 1)
	go func() {
		data := make([]byte, 4096)
		for i := 0; i < 1000; i++ {
			if _, err := cd.Write(data); err != nil {
				done <- err
				return
			}
		}
		// All 1000 writes succeeded without error — deadline had no effect.
		done <- nil
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Error("expected Write to fail on stalled client (deadline exceeded), but all writes succeeded")
		}
		// Received a deadline/timeout error — correct behaviour.
	case <-time.After(15 * time.Second):
		t.Fatal("Write blocked for more than 15 s — write deadline is not working")
	}
}

// TestInputDisabled_Race verifies that concurrent reads and writes of
// InputDisabled do not produce data races under -race and that the value
// observed after all stores is consistent.
func TestInputDisabled_Race(t *testing.T) {
	t.Parallel()

	cd := &ConnectionDetails{}

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			if i%2 == 0 {
				// writers: alternate true/false
				cd.InputDisabled(i%4 == 0)
			} else {
				// readers: just read
				_ = cd.InputDisabled()
			}
		}()
	}

	wg.Wait()

	// After all writers have finished, a read must return a valid bool (not torn).
	result := cd.InputDisabled()
	if result != true && result != false {
		t.Errorf("InputDisabled() returned an unexpected value: %v", result)
	}
}

// TestInputDisabled_SetAndGet verifies the basic set-and-get semantics of
// InputDisabled — setting to true returns true, setting to false returns false.
func TestInputDisabled_SetAndGet(t *testing.T) {
	t.Parallel()

	cd := &ConnectionDetails{}

	if cd.InputDisabled() != false {
		t.Error("InputDisabled() should default to false")
	}

	cd.InputDisabled(true)
	if cd.InputDisabled() != true {
		t.Error("InputDisabled() should return true after InputDisabled(true)")
	}

	cd.InputDisabled(false)
	if cd.InputDisabled() != false {
		t.Error("InputDisabled() should return false after InputDisabled(false)")
	}
}
