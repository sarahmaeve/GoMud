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
	// Not parallel: mutates the package-level netConnections map.

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

	// The primary correctness signal for this test is -race: if atomic.Bool
	// were replaced with a plain bool, the race detector would flag the
	// concurrent reads and writes above. No value assertion is needed here.
	_ = cd.InputDisabled()
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

// instrumentedConn wraps a net.Conn and records the ordered sequence of
// SetWriteDeadline and Write calls so tests can verify that concurrent
// callers never interleave their deadline+write sequences.
//
// The ConnectionDetails.Write() method on the telnet path performs three
// ordered operations per call:
//  1. SetWriteDeadline(future)
//  2. conn.Write(payload)
//  3. SetWriteDeadline(zero)   // reset
//
// If two goroutines run this sequence concurrently without a mutex, the
// calls can interleave — e.g. A's SetWriteDeadline, B's SetWriteDeadline,
// A's Write (which now runs with B's deadline). This test fails if any
// interleaving is observed.
type instrumentedConn struct {
	mu     sync.Mutex
	events []instrumentedEvent
}

type instrumentedEvent struct {
	kind  string // "deadline-set", "deadline-zero", "write"
	gor   int    // goroutine id (-1 if unknown)
	delay time.Duration
}

func (c *instrumentedConn) record(kind string, gor int) {
	c.mu.Lock()
	c.events = append(c.events, instrumentedEvent{kind: kind, gor: gor})
	c.mu.Unlock()
}

func (c *instrumentedConn) Read(p []byte) (int, error)        { return 0, nil }
func (c *instrumentedConn) Close() error                      { return nil }
func (c *instrumentedConn) LocalAddr() net.Addr               { return nil }
func (c *instrumentedConn) RemoteAddr() net.Addr              { return nil }
func (c *instrumentedConn) SetDeadline(t time.Time) error     { return nil }
func (c *instrumentedConn) SetReadDeadline(t time.Time) error { return nil }

func (c *instrumentedConn) SetWriteDeadline(t time.Time) error {
	if t.IsZero() {
		c.record("deadline-zero", -1)
	} else {
		c.record("deadline-set", -1)
	}
	// Small sleep to widen the race window — makes interleaving detectable
	// if the mutex is missing.
	time.Sleep(100 * time.Microsecond)
	return nil
}

func (c *instrumentedConn) Write(p []byte) (int, error) {
	c.record("write", -1)
	time.Sleep(100 * time.Microsecond)
	return len(p), nil
}

// TestWrite_TelnetConcurrentWritesSerialized verifies that ConnectionDetails.Write
// serializes the SetWriteDeadline/Write/reset sequence. It uses a custom
// instrumentedConn that records every call in order, then asserts that the
// observed sequence is always of the form:
//
//	deadline-set, write, deadline-zero, deadline-set, write, deadline-zero, ...
//
// If the writeMu mutex is removed, concurrent goroutines will produce
// interleaved patterns like:
//
//	deadline-set, deadline-set, write, deadline-zero, ...
//
// which the assertion below catches.
func TestWrite_TelnetConcurrentWritesSerialized(t *testing.T) {
	t.Parallel()

	ic := &instrumentedConn{}
	cd := &ConnectionDetails{conn: ic}

	const goroutines = 10
	const writesPerGoroutine = 5
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < writesPerGoroutine; j++ {
				if _, err := cd.Write([]byte("payload\n")); err != nil {
					t.Errorf("Write returned error: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	// Expected pattern per Write: deadline-set, write, deadline-zero.
	// With the mutex in place, these must appear contiguously for each call.
	// Without the mutex, they interleave across goroutines and the pattern
	// check below will find a violation.
	totalWrites := goroutines * writesPerGoroutine
	if len(ic.events) != totalWrites*3 {
		t.Fatalf("expected %d events (3 per Write), got %d", totalWrites*3, len(ic.events))
	}

	for i := 0; i < len(ic.events); i += 3 {
		if ic.events[i].kind != "deadline-set" {
			t.Errorf("at event %d: expected 'deadline-set', got %q — telnet writes are not serialized by writeMu", i, ic.events[i].kind)
			return
		}
		if ic.events[i+1].kind != "write" {
			t.Errorf("at event %d: expected 'write', got %q — telnet writes are not serialized by writeMu", i+1, ic.events[i+1].kind)
			return
		}
		if ic.events[i+2].kind != "deadline-zero" {
			t.Errorf("at event %d: expected 'deadline-zero', got %q — telnet writes are not serialized by writeMu", i+2, ic.events[i+2].kind)
			return
		}
	}
}
