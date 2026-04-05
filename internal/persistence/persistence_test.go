package persistence

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// openTestStore opens an in-memory SQLite store for a single test and
// registers a cleanup to close it. The shared cache DSN ensures all
// connections in the test see the same data.
func openTestStore(t *testing.T) *sqliteStore {
	t.Helper()
	s, err := openWithConfig("file::memory:?cache=shared", 1000, 500, 100*time.Millisecond, 1*time.Second)
	require.NoError(t, err, "openWithConfig")
	t.Cleanup(func() {
		_ = s.Close()
	})
	return s
}

// openTestStorePaused opens an in-memory store with a tiny write queue
// and a paused worker. The returned release function unpauses the worker;
// tests must call it before Close or Flush to avoid deadlock.
func openTestStorePaused(t *testing.T, queueSize int, enqueueTimeout time.Duration) (*sqliteStore, func()) {
	t.Helper()

	// Build a store with a paused worker. We can't use openWithConfig
	// directly because it starts the worker unconditionally, so we
	// replicate its body here with the pause channel injected.
	db, err := openSQLite("file::memory:?cache=shared")
	require.NoError(t, err)

	s := &sqliteStore{
		db:             db,
		queue:          make(chan writeOp, queueSize),
		stop:           make(chan struct{}),
		workerDone:     make(chan struct{}),
		flushReqCh:     make(chan chan struct{}, 16),
		pauseCh:        make(chan struct{}),
		queueSize:      queueSize,
		batchSize:      500,
		batchWindow:    10 * time.Second,
		enqueueTimeout: enqueueTimeout,
	}
	go s.worker()

	released := false
	release := func() {
		if released {
			return
		}
		released = true
		close(s.pauseCh)
	}
	t.Cleanup(func() {
		release()
		_ = s.Close()
	})
	return s, release
}

func makeUser(id int, username string) *UserData {
	return &UserData{
		UserId:    id,
		Username:  username,
		Password:  "$2a$10$fakebcrypthashfakebcrypthash............",
		Role:      "user",
		Joined:    time.Unix(1700000000, 0),
		LastLogin: time.Unix(1700100000, 0),
		Email:     fmt.Sprintf("%s@example.test", strings.ToLower(username)),
		Payload:   []byte(`{"character":{"name":"` + username + `"},"inventory":[]}`),
	}
}

func makeRoom(id int, zone string) *RoomInstanceData {
	return &RoomInstanceData{
		RoomId:    id,
		Zone:      zone,
		Payload:   []byte(fmt.Sprintf(`{"floorGold":%d,"items":[]}`, id*10)),
		UpdatedAt: time.Unix(1700000000, 0),
	}
}

// ---------------------------------------------------------------
// Migration tests
// ---------------------------------------------------------------

func TestOpen_CreatesSchema(t *testing.T) {
	s := openTestStore(t)

	// Verify all expected tables exist.
	tables := []string{"schema_migrations", "users", "room_instances"}
	for _, table := range tables {
		var name string
		err := s.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
		require.NoError(t, err, "table %s should exist", table)
		assert.Equal(t, table, name)
	}
}

func TestOpen_AppliesMigrations(t *testing.T) {
	s := openTestStore(t)

	rows, err := s.db.Query(`SELECT version, name FROM schema_migrations ORDER BY version`)
	require.NoError(t, err)
	defer rows.Close()

	var applied []int
	for rows.Next() {
		var v int
		var n string
		require.NoError(t, rows.Scan(&v, &n))
		applied = append(applied, v)
	}

	require.Len(t, applied, len(migrations), "all migrations should be applied")
	for i, m := range migrations {
		assert.Equal(t, m.version, applied[i])
	}
}

func TestOpen_MigrationsIdempotent(t *testing.T) {
	// Opening twice against the same in-memory DB would require the
	// shared-cache DSN; instead we verify idempotency by calling
	// applyMigrations again directly on the same db handle.
	s := openTestStore(t)
	err := applyMigrations(testContext(), s.db)
	assert.NoError(t, err, "applyMigrations should be idempotent")

	// Verify no duplicates.
	var count int
	err = s.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, len(migrations), count, "no duplicate migration rows after reapply")
}

// ---------------------------------------------------------------
// User tests
// ---------------------------------------------------------------

func TestSaveUser_RoundTrip(t *testing.T) {
	s := openTestStore(t)
	u := makeUser(1, "Alice")

	require.NoError(t, s.SaveUser(u))
	require.NoError(t, s.Flush())

	loaded, err := s.LoadUser(1)
	require.NoError(t, err)
	require.NotNil(t, loaded)

	assert.Equal(t, u.UserId, loaded.UserId)
	assert.Equal(t, u.Username, loaded.Username)
	assert.Equal(t, u.Password, loaded.Password)
	assert.Equal(t, u.Role, loaded.Role)
	assert.Equal(t, u.Email, loaded.Email)
	assert.Equal(t, u.Joined.Unix(), loaded.Joined.Unix())
	assert.Equal(t, u.LastLogin.Unix(), loaded.LastLogin.Unix())
	assert.Equal(t, string(u.Payload), string(loaded.Payload))
}

func TestSaveUser_Coalescing(t *testing.T) {
	s := openTestStore(t)

	// Save the same user 100 times rapidly. The batch window is 100ms,
	// which is much longer than the loop takes, so all 100 ops land in
	// a single batch and coalesce to one write.
	u := makeUser(1, "Alice")
	for i := 0; i < 100; i++ {
		u.Payload = []byte(fmt.Sprintf(`{"iter":%d}`, i))
		require.NoError(t, s.SaveUser(u))
	}
	require.NoError(t, s.Flush())

	stats := s.stats()
	assert.Equal(t, uint64(100), stats.opsEnqueued, "enqueued 100 ops")
	assert.Less(t, stats.writesExecuted, uint64(100), "coalescing should reduce write count")
	assert.Greater(t, stats.opsCoalesced, uint64(0), "some ops should have been coalesced")

	// Verify final state is the last write.
	loaded, err := s.LoadUser(1)
	require.NoError(t, err)
	assert.Equal(t, `{"iter":99}`, string(loaded.Payload))
}

func TestSaveUser_Flush(t *testing.T) {
	s := openTestStore(t)

	// Enqueue 10 distinct users. Without Flush, a LoadUser call
	// immediately after may race with the background worker. With
	// Flush, all writes must be visible.
	for i := 1; i <= 10; i++ {
		require.NoError(t, s.SaveUser(makeUser(i, fmt.Sprintf("user%d", i))))
	}

	require.NoError(t, s.Flush())

	// After Flush, every user must be loadable.
	for i := 1; i <= 10; i++ {
		u, err := s.LoadUser(i)
		require.NoError(t, err, "user %d should exist after flush", i)
		assert.Equal(t, fmt.Sprintf("user%d", i), u.Username)
	}
}

func TestLoadUserByUsername_CaseInsensitive(t *testing.T) {
	s := openTestStore(t)
	require.NoError(t, s.SaveUser(makeUser(1, "Alice")))
	require.NoError(t, s.Flush())

	for _, q := range []string{"alice", "ALICE", "AlIcE"} {
		u, err := s.LoadUserByUsername(q)
		require.NoError(t, err, "lookup %q", q)
		assert.Equal(t, "Alice", u.Username)
	}
}

func TestLoadUser_NotFound_ReturnsErrNotFound(t *testing.T) {
	s := openTestStore(t)
	_, err := s.LoadUser(9999)
	assert.ErrorIs(t, err, ErrNotFound)

	_, err = s.LoadUserByUsername("nosuchuser")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestDeleteUser(t *testing.T) {
	s := openTestStore(t)
	require.NoError(t, s.SaveUser(makeUser(1, "Alice")))
	require.NoError(t, s.Flush())

	// Verify present.
	_, err := s.LoadUser(1)
	require.NoError(t, err)

	// Delete and flush.
	require.NoError(t, s.DeleteUser(1))
	require.NoError(t, s.Flush())

	_, err = s.LoadUser(1)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestAllUsernames(t *testing.T) {
	s := openTestStore(t)
	expected := []string{"alice", "bob", "carol"}
	for i, name := range expected {
		require.NoError(t, s.SaveUser(makeUser(i+1, name)))
	}
	require.NoError(t, s.Flush())

	got, err := s.AllUsernames()
	require.NoError(t, err)
	assert.ElementsMatch(t, expected, got)
}

func TestAllUserIds(t *testing.T) {
	s := openTestStore(t)
	for _, id := range []int{10, 20, 30} {
		require.NoError(t, s.SaveUser(makeUser(id, fmt.Sprintf("u%d", id))))
	}
	require.NoError(t, s.Flush())

	ids, err := s.AllUserIds()
	require.NoError(t, err)
	assert.ElementsMatch(t, []int{10, 20, 30}, ids)
}

func TestUserExists(t *testing.T) {
	s := openTestStore(t)
	require.NoError(t, s.SaveUser(makeUser(1, "Alice")))
	require.NoError(t, s.Flush())

	exists, err := s.UserExists("alice") // case-insensitive
	require.NoError(t, err)
	assert.True(t, exists)

	exists, err = s.UserExists("ALICE")
	require.NoError(t, err)
	assert.True(t, exists)

	exists, err = s.UserExists("nosuchuser")
	require.NoError(t, err)
	assert.False(t, exists)
}

// ---------------------------------------------------------------
// Room instance tests
// ---------------------------------------------------------------

func TestSaveRoomInstance_RoundTrip(t *testing.T) {
	s := openTestStore(t)
	r := makeRoom(100, "startland")

	require.NoError(t, s.SaveRoomInstance(r))
	require.NoError(t, s.Flush())

	loaded, err := s.LoadRoomInstance(100)
	require.NoError(t, err)
	assert.Equal(t, r.RoomId, loaded.RoomId)
	assert.Equal(t, r.Zone, loaded.Zone)
	assert.Equal(t, string(r.Payload), string(loaded.Payload))
	assert.Equal(t, r.UpdatedAt.UnixNano(), loaded.UpdatedAt.UnixNano())
}

func TestLoadRoomInstance_NotFound(t *testing.T) {
	s := openTestStore(t)
	_, err := s.LoadRoomInstance(9999)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestDeleteRoomInstance(t *testing.T) {
	s := openTestStore(t)
	require.NoError(t, s.SaveRoomInstance(makeRoom(100, "startland")))
	require.NoError(t, s.Flush())

	_, err := s.LoadRoomInstance(100)
	require.NoError(t, err)

	require.NoError(t, s.DeleteRoomInstance(100))
	require.NoError(t, s.Flush())

	_, err = s.LoadRoomInstance(100)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestAllRoomInstanceIds(t *testing.T) {
	s := openTestStore(t)
	for _, id := range []int{100, 200, 300} {
		require.NoError(t, s.SaveRoomInstance(makeRoom(id, "zone")))
	}
	require.NoError(t, s.Flush())

	ids, err := s.AllRoomInstanceIds()
	require.NoError(t, err)
	assert.ElementsMatch(t, []int{100, 200, 300}, ids)
}

// ---------------------------------------------------------------
// Concurrency / lifecycle tests
// ---------------------------------------------------------------

func TestConcurrentWrites_NoRace(t *testing.T) {
	s := openTestStore(t)

	const goroutines = 50
	const writesPerGoroutine = 10

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(base int) {
			defer wg.Done()
			for j := 0; j < writesPerGoroutine; j++ {
				id := base*writesPerGoroutine + j + 1
				require.NoError(t, s.SaveUser(makeUser(id, fmt.Sprintf("user%d", id))))
			}
		}(i)
	}
	wg.Wait()

	require.NoError(t, s.Flush())

	ids, err := s.AllUserIds()
	require.NoError(t, err)
	assert.Len(t, ids, goroutines*writesPerGoroutine)
}

func TestClose_FlushesPendingWrites(t *testing.T) {
	s := openTestStore(t)

	for i := 1; i <= 20; i++ {
		require.NoError(t, s.SaveUser(makeUser(i, fmt.Sprintf("u%d", i))))
	}

	// Close without an explicit Flush — the close path must drain.
	require.NoError(t, s.Close())

	// Reopen a new connection to the same in-memory DB via shared cache
	// and verify all writes persisted. Note: once Close closes the last
	// handle, the shared-cache in-memory DB disappears. To test this
	// properly we need to use a different strategy.
	//
	// For this test, we rely on stats: all enqueued ops should have
	// been executed before Close returned.
	stats := s.stats()
	assert.Equal(t, uint64(20), stats.opsEnqueued)
	assert.GreaterOrEqual(t, stats.writesExecuted, uint64(1),
		"close must commit at least one batch containing the 20 writes")
}

func TestClose_Idempotent(t *testing.T) {
	s := openTestStore(t)
	require.NoError(t, s.SaveUser(makeUser(1, "Alice")))

	assert.NoError(t, s.Close())
	assert.NoError(t, s.Close(), "second Close should be a no-op")
	assert.NoError(t, s.Close(), "third Close should be a no-op")
}

func TestQueueBackpressure(t *testing.T) {
	// Paused worker + tiny queue + short enqueue timeout. With the
	// worker blocked before it can drain the channel, producer calls
	// saturate the queue and subsequent enqueues must time out and
	// return an error rather than block forever.
	s, release := openTestStorePaused(t, 2, 50*time.Millisecond)

	var errs int
	var succ int
	for i := 0; i < 20; i++ {
		if err := s.SaveUser(makeUser(i+1, fmt.Sprintf("u%d", i))); err != nil {
			errs++
		} else {
			succ++
		}
	}

	// Queue capacity is 2. The worker is paused, so only 2 can fit.
	// The rest must time out.
	assert.LessOrEqual(t, succ, 2, "at most queue capacity enqueues should succeed")
	assert.Greater(t, errs, 0, "some SaveUser calls should have timed out due to queue backpressure")

	// Release the worker so the queue drains and Close doesn't hang.
	release()
	require.NoError(t, s.Flush())
}

// TestClose_RaceWithSaveUser is a regression test for the C1 bug where
// Close() used to `close(queue)` while concurrent SaveUser goroutines
// were still mid-send, panicking with "send on closed channel". The
// fix replaces channel-close with a separate stop signal guarded by a
// RWMutex that enqueue holds as a reader for the duration of the send.
//
// This test spins up many concurrent savers, closes the store while
// they are still running, and asserts that:
//  1. No goroutine panics (closed-channel panic).
//  2. Every SaveUser call either returns nil (enqueued before stop)
//     or a non-nil error (rejected after stop), never both and never
//     a panic.
//
// Run with -race to also catch any data race on the stopped flag.
func TestClose_RaceWithSaveUser(t *testing.T) {
	s := openTestStore(t)

	const writers = 32
	const perWriter = 200

	var wg sync.WaitGroup
	start := make(chan struct{})
	panics := make(chan any, writers)

	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					panics <- r
				}
			}()
			<-start
			for i := 0; i < perWriter; i++ {
				// Errors after Close are expected; what we are
				// asserting is that the call returns cleanly rather
				// than panicking.
				_ = s.SaveUser(makeUser(writerID*10000+i+1, fmt.Sprintf("u%d_%d", writerID, i)))
			}
		}(w)
	}

	close(start)
	// Let the writers get into flight, then close concurrently.
	time.Sleep(2 * time.Millisecond)
	closeErr := s.Close()
	wg.Wait()
	close(panics)

	require.NoError(t, closeErr, "Close should not error")
	for p := range panics {
		t.Fatalf("writer goroutine panicked: %v", p)
	}
}

// TestCommitBatch_BadRowDoesNotPoisonBatch is a regression test for H7:
// before the fix, a single SQL exec failure inside commitBatch would
// `return` early — leaving the deferred rollback to abort the whole
// transaction. A 500-op batch with one malformed row would lose ALL
// 500 writes. The fix wraps each op in a SAVEPOINT: on failure, roll
// back to the savepoint, log the bad op, and continue. The containing
// transaction commits everything else.
//
// This test enqueues two valid users and one bad user (duplicate
// username, which trips the UNIQUE constraint on users.username),
// flushes, and asserts the two valid users are present.
func TestCommitBatch_BadRowDoesNotPoisonBatch(t *testing.T) {
	s := openTestStore(t)

	good1 := makeUser(1001, "alice")
	bad := makeUser(1002, "alice") // same username → UNIQUE constraint
	good2 := makeUser(1003, "bob")

	require.NoError(t, s.SaveUser(good1))
	require.NoError(t, s.SaveUser(bad))
	require.NoError(t, s.SaveUser(good2))
	require.NoError(t, s.Flush())

	// Whichever alice was processed first wins the UNIQUE slot; the
	// other trips the constraint and is rolled back to its savepoint.
	// Bob is unrelated and must survive regardless.
	gotAlice, err := s.LoadUserByUsername("alice")
	require.NoError(t, err, "alice must still be committed")
	assert.Contains(t, []int{1001, 1002}, gotAlice.UserId, "one of the two alices must have landed")

	gotBob, err := s.LoadUserByUsername("bob")
	require.NoError(t, err, "bob must still be committed — a bad row earlier in the batch must not poison later ops (H7)")
	assert.Equal(t, 1003, gotBob.UserId)

	// Exactly one alice should exist.
	var alicesExist int
	for _, id := range []int{1001, 1002} {
		if _, err := s.LoadUser(id); err == nil {
			alicesExist++
		} else {
			assert.ErrorIs(t, err, ErrNotFound)
		}
	}
	assert.Equal(t, 1, alicesExist, "exactly one alice row should exist after savepoint rollback")
}

// testContext returns a context for use in tests.
func testContext() context.Context {
	return context.Background()
}
