package main

import (
	"sync"
	"testing"
	"time"
)

// newTestWorld builds a World with only the fields needed by GetAutoComplete
// and the autoCompleteReq channel. The caller is responsible for servicing
// (or not servicing) the channel.
func newTestWorld() *World {
	return &World{
		autoCompleteReq: make(chan autoCompleteRequest),
	}
}

// serviceOnce reads exactly one request from w.autoCompleteReq and replies
// with fixedResult. It blocks until a request arrives or ctx is cancelled.
func serviceOnce(w *World, fixedResult []string, done <-chan struct{}) {
	select {
	case req := <-w.autoCompleteReq:
		select {
		case req.reply <- fixedResult:
		default:
		}
	case <-done:
	}
}

// TestGetAutoComplete_ReplyReceived verifies that when MainWorker services the
// request, GetAutoComplete returns the reply.
func TestGetAutoComplete_ReplyReceived(t *testing.T) {
	t.Parallel()

	w := newTestWorld()
	want := []string{"attack", "appraise"}
	done := make(chan struct{})
	defer close(done)

	go serviceOnce(w, want, done)

	got := w.GetAutoComplete(1, "a")
	if len(got) != len(want) {
		t.Fatalf("GetAutoComplete: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("result[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestGetAutoComplete_TimeoutNoWorker verifies that when nobody reads from the
// channel GetAutoComplete returns nil within its timeout window.
// We shorten the window via the 500 ms guard already in the code; the test
// just confirms the return value is nil and that the call does not hang
// indefinitely.
func TestGetAutoComplete_TimeoutNoWorker(t *testing.T) {
	// Not parallel — this test intentionally waits up to ~500 ms for the
	// timeout to fire. Marking it parallel would only slow the suite down.

	w := newTestWorld()

	start := time.Now()
	got := w.GetAutoComplete(99, "z")
	elapsed := time.Since(start)

	if got != nil {
		t.Errorf("expected nil on timeout, got %v", got)
	}
	// Sanity: should not have returned instantly (< 1 ms) nor blocked forever.
	if elapsed < 10*time.Millisecond {
		t.Errorf("returned too quickly (%v); timeout guard may not be firing", elapsed)
	}
}

// TestGetAutoComplete_WorkerDropsReply verifies the no-reply-after-send path:
// a worker reads the request but does NOT send a reply. GetAutoComplete must
// still return nil rather than hanging.
func TestGetAutoComplete_WorkerDropsReply(t *testing.T) {
	w := newTestWorld()
	done := make(chan struct{})
	defer close(done)

	// Drain the request but never reply.
	go func() {
		select {
		case <-w.autoCompleteReq:
			// intentionally ignore req.reply
		case <-done:
		}
	}()

	got := w.GetAutoComplete(1, "drop")
	if got != nil {
		t.Errorf("expected nil when worker drops reply, got %v", got)
	}
}

// TestGetAutoComplete_Concurrent verifies race-freedom when multiple goroutines
// call GetAutoComplete simultaneously. Run with -race.
func TestGetAutoComplete_Concurrent(t *testing.T) {
	t.Parallel()

	const callers = 10
	w := newTestWorld()
	done := make(chan struct{})
	defer close(done)

	// Start a worker that services all requests sequentially.
	go func() {
		for {
			select {
			case req := <-w.autoCompleteReq:
				select {
				case req.reply <- []string{"look"}:
				default:
				}
			case <-done:
				return
			}
		}
	}()

	var wg sync.WaitGroup
	results := make([][]string, callers)

	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = w.GetAutoComplete(idx+1, "l")
		}(i)
	}

	wg.Wait()

	for i, r := range results {
		if r == nil {
			t.Errorf("caller %d: expected a result, got nil", i)
		}
	}
}
