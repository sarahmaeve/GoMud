package web

import (
	"sync"
	"testing"
	"time"
)

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
