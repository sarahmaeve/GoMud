package main

import (
	"sync"
	"testing"
	"time"

	"github.com/GoMudEngine/GoMud/internal/connections"
	"github.com/stretchr/testify/require"
)

// TestRecoverConnection_StopsProcessPanic verifies that recoverConnection
// catches a panic in a goroutine and allows the goroutine to exit cleanly
// without crashing the process.
//
// LIFO defer order used inside the goroutine:
//
//	1. defer wg.Done()       — registered first, runs last
//	2. defer observation()   — registered second, runs second
//	3. defer recoverConnection() — registered last, runs first
//
// recoverConnection consumes the panic. The observation defer then sees
// recover() == nil (no active panic), confirming recovery succeeded.
func TestRecoverConnection_StopsProcessPanic(t *testing.T) {
	t.Parallel()

	const testConnId connections.ConnectionId = 0 // no real connection needed

	var wg sync.WaitGroup
	wg.Add(1)

	recovered := make(chan bool, 1)

	go func() {
		defer wg.Done()
		defer func() {
			// If recoverConnection already consumed the panic, recover() here
			// returns nil — that is the success case.
			if r := recover(); r == nil {
				recovered <- true
			} else {
				recovered <- false
				panic(r) // re-panic so the test runner sees it
			}
		}()
		defer recoverConnection(testConnId) // runs first (LIFO)

		panic("test panic — should be caught by recoverConnection")
	}()

	wg.Wait()

	select {
	case ok := <-recovered:
		require.True(t, ok, "recoverConnection should have consumed the panic")
	default:
		t.Fatal("recovered channel was not written — goroutine did not complete as expected")
	}
}

// TestRecoverConnection_WaitGroupFires verifies that wg.Done() is called even
// when a panic occurs inside the goroutine, so callers of wg.Wait() are never
// permanently blocked.
func TestRecoverConnection_WaitGroupFires(t *testing.T) {
	t.Parallel()

	const testConnId connections.ConnectionId = 0

	var wg sync.WaitGroup
	wg.Add(1)

	done := make(chan struct{})

	go func() {
		defer wg.Done() // must fire even after panic
		defer recoverConnection(testConnId)
		panic("wg.Done must still fire after this panic")
	}()

	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// wg.Wait() returned — Done() fired correctly.
	case <-time.After(5 * time.Second):
		t.Fatal("wg.Wait() did not return within 5s — wg.Done() was not called after panic recovery")
	}
}
