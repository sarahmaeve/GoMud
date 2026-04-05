package connections

import (
	"net"
	"testing"
	"time"
)

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
