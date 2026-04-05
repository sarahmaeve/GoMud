package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver
)

const (
	defaultQueueSize      = 10000
	defaultBatchSize      = 500
	defaultBatchWindow    = 100 * time.Millisecond
	defaultEnqueueTimeout = 1 * time.Second
)

// opKind identifies the kind of write operation.
type opKind int

const (
	opUserSave opKind = iota
	opUserDelete
	opRoomSave
	opRoomDelete
)

// opKey identifies a specific entity for coalescing purposes.
// Two ops with the same opKey collapse to one database write.
type opKey struct {
	kind opKind
	id   int
}

// writeOp is a single write request carried through the queue.
type writeOp struct {
	key  opKey
	data interface{} // *UserData, *RoomInstanceData, or nil for delete
}

// storeStats captures counters exposed for testing and observability.
type storeStats struct {
	opsEnqueued      uint64
	opsCoalesced     uint64
	writesExecuted   uint64
	batchesCommitted uint64
}

// sqliteStore is the SQLite-backed Store implementation.
type sqliteStore struct {
	db    *sql.DB
	queue chan writeOp

	// Lifecycle
	workerDone chan struct{}
	closed     atomic.Bool
	closeMu    sync.Mutex

	// Test-only: when non-nil, the worker blocks on this channel before
	// reading from the queue, allowing tests to saturate the queue
	// deterministically. Production code never sets this.
	pauseCh chan struct{}

	// Flush coordination
	flushReqCh chan chan struct{}

	// Stats (atomic)
	opsEnqueued      atomic.Uint64
	opsCoalesced     atomic.Uint64
	writesExecuted   atomic.Uint64
	batchesCommitted atomic.Uint64

	// Tunables (set by Open or test constructor)
	queueSize      int
	batchSize      int
	batchWindow    time.Duration
	enqueueTimeout time.Duration
}

// Open opens a SQLite database at path, applies pending migrations, and
// starts the background writer goroutine.
//
// If path is ":memory:" or "file::memory:?cache=shared", an in-memory
// database is used. For disk-backed databases, pass an absolute or
// relative filesystem path.
func Open(path string) (Store, error) {
	return openWithConfig(path, defaultQueueSize, defaultBatchSize, defaultBatchWindow, defaultEnqueueTimeout)
}

// openSQLite opens a SQLite database, applies PRAGMAs, and runs migrations.
// It does NOT start a worker goroutine — callers are expected to construct
// the sqliteStore themselves. Extracted for reuse between openWithConfig
// and the test-only paused constructor.
func openSQLite(path string) (*sql.DB, error) {
	// The modernc driver accepts the same DSN syntax as mattn.
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}

	// Single connection for in-memory databases so all reads/writes
	// see the same data. For disk-backed DBs, a small pool is fine.
	db.SetMaxOpenConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	}
	for _, p := range pragmas {
		if _, err := db.ExecContext(ctx, p); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("exec %s: %w", p, err)
		}
	}

	if err := applyMigrations(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("apply migrations: %w", err)
	}

	return db, nil
}

// openWithConfig is the internal constructor used by Open and by tests
// that want smaller queues or shorter windows.
func openWithConfig(path string, queueSize, batchSize int, batchWindow, enqueueTimeout time.Duration) (*sqliteStore, error) {
	db, err := openSQLite(path)
	if err != nil {
		return nil, err
	}

	s := &sqliteStore{
		db:             db,
		queue:          make(chan writeOp, queueSize),
		workerDone:     make(chan struct{}),
		flushReqCh:     make(chan chan struct{}, 16),
		queueSize:      queueSize,
		batchSize:      batchSize,
		batchWindow:    batchWindow,
		enqueueTimeout: enqueueTimeout,
	}

	go s.worker()
	return s, nil
}

// worker drains the write queue, coalesces ops to the same entity, and
// commits batches on one of three triggers:
//   - batch size reached
//   - batch window elapsed since the first op of the current batch
//   - flush request received
//
// On commit error, it logs and continues so the game keeps running.
// On queue close, it commits the final batch and exits.
func (s *sqliteStore) worker() {
	defer close(s.workerDone)

	// Test-only pause gate.
	if s.pauseCh != nil {
		<-s.pauseCh
	}

	batch := make(map[opKey]writeOp)
	var batchStart time.Time

	// We use a timer rather than a ticker so we can reset it on each
	// flush and control the exact window.
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	timerActive := false

	armTimer := func() {
		if !timerActive {
			timer.Reset(s.batchWindow)
			timerActive = true
		}
	}

	stopTimer := func() {
		if timerActive {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timerActive = false
		}
	}

	commit := func() {
		if len(batch) == 0 {
			return
		}
		s.commitBatch(batch)
		batch = make(map[opKey]writeOp)
		batchStart = time.Time{}
		stopTimer()
	}

	for {
		select {
		case op, ok := <-s.queue:
			if !ok {
				// Queue closed: commit whatever is in the batch and exit.
				commit()
				return
			}
			if _, exists := batch[op.key]; exists {
				s.opsCoalesced.Add(1)
			}
			batch[op.key] = op
			if batchStart.IsZero() {
				batchStart = time.Now()
				armTimer()
			}
			if len(batch) >= s.batchSize {
				commit()
			}

		case <-timer.C:
			timerActive = false
			commit()

		case ack := <-s.flushReqCh:
			// Drain any currently-queued ops into the batch so they're
			// included in this flush, then commit.
		drainLoop:
			for {
				select {
				case op, ok := <-s.queue:
					if !ok {
						commit()
						close(ack)
						return
					}
					if _, exists := batch[op.key]; exists {
						s.opsCoalesced.Add(1)
					}
					batch[op.key] = op
				default:
					break drainLoop
				}
			}
			commit()
			close(ack)
		}
	}
}

// commitBatch applies all ops in the batch inside a single transaction.
// On any error, it logs and skips the batch (game continues; admin
// investigates).
func (s *sqliteStore) commitBatch(batch map[opKey]writeOp) {
	if len(batch) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		log.Printf("persistence: begin tx failed: %v", err)
		return
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var writesExecuted uint64
	for _, op := range batch {
		switch op.key.kind {
		case opUserSave:
			u := op.data.(*UserData)
			_, err = tx.ExecContext(ctx, `
				INSERT INTO users (user_id, username, password, role, joined, last_login, email, data)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(user_id) DO UPDATE SET
					username   = excluded.username,
					password   = excluded.password,
					role       = excluded.role,
					joined     = excluded.joined,
					last_login = excluded.last_login,
					email      = excluded.email,
					data       = excluded.data
			`, u.UserId, u.Username, u.Password, u.Role, u.Joined.Unix(), u.LastLogin.Unix(), u.Email, u.Payload)
		case opUserDelete:
			_, err = tx.ExecContext(ctx, `DELETE FROM users WHERE user_id = ?`, op.key.id)
		case opRoomSave:
			r := op.data.(*RoomInstanceData)
			_, err = tx.ExecContext(ctx, `
				INSERT INTO room_instances (room_id, zone, data, updated_at)
				VALUES (?, ?, ?, ?)
				ON CONFLICT(room_id) DO UPDATE SET
					zone       = excluded.zone,
					data       = excluded.data,
					updated_at = excluded.updated_at
			`, r.RoomId, r.Zone, r.Payload, r.UpdatedAt.UnixNano())
		case opRoomDelete:
			_, err = tx.ExecContext(ctx, `DELETE FROM room_instances WHERE room_id = ?`, op.key.id)
		}
		if err != nil {
			log.Printf("persistence: exec failed for op kind=%d id=%d: %v", op.key.kind, op.key.id, err)
			return
		}
		writesExecuted++
	}

	if err := tx.Commit(); err != nil {
		log.Printf("persistence: commit failed: %v", err)
		return
	}
	committed = true

	s.writesExecuted.Add(writesExecuted)
	s.batchesCommitted.Add(1)
}

// enqueue adds an op to the write queue with a timeout.
func (s *sqliteStore) enqueue(op writeOp) error {
	if s.closed.Load() {
		return errors.New("persistence: store is closed")
	}
	s.opsEnqueued.Add(1)
	timer := time.NewTimer(s.enqueueTimeout)
	defer timer.Stop()
	select {
	case s.queue <- op:
		return nil
	case <-timer.C:
		return errors.New("persistence: write queue saturated; enqueue timed out")
	}
}

// ---------------------------------------------------------------
// Store interface — user operations
// ---------------------------------------------------------------

func (s *sqliteStore) LoadUser(userId int) (*UserData, error) {
	row := s.db.QueryRow(`
		SELECT user_id, username, password, role, joined, last_login, email, data
		FROM users WHERE user_id = ?
	`, userId)
	return scanUser(row)
}

func (s *sqliteStore) LoadUserByUsername(username string) (*UserData, error) {
	row := s.db.QueryRow(`
		SELECT user_id, username, password, role, joined, last_login, email, data
		FROM users WHERE username = ? COLLATE NOCASE
	`, username)
	return scanUser(row)
}

func scanUser(row *sql.Row) (*UserData, error) {
	var u UserData
	var joinedUnix, lastLoginUnix int64
	err := row.Scan(&u.UserId, &u.Username, &u.Password, &u.Role, &joinedUnix, &lastLoginUnix, &u.Email, &u.Payload)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.Joined = time.Unix(joinedUnix, 0)
	u.LastLogin = time.Unix(lastLoginUnix, 0)
	return &u, nil
}

func (s *sqliteStore) SaveUser(u *UserData) error {
	if u == nil {
		return errors.New("persistence: SaveUser called with nil")
	}
	// Make a shallow copy so the caller can reuse the struct.
	cp := *u
	return s.enqueue(writeOp{
		key:  opKey{kind: opUserSave, id: u.UserId},
		data: &cp,
	})
}

func (s *sqliteStore) DeleteUser(userId int) error {
	return s.enqueue(writeOp{
		key: opKey{kind: opUserDelete, id: userId},
	})
}

func (s *sqliteStore) AllUsernames() ([]string, error) {
	rows, err := s.db.Query(`SELECT username FROM users ORDER BY user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		names = append(names, n)
	}
	return names, rows.Err()
}

func (s *sqliteStore) AllUserIds() ([]int, error) {
	rows, err := s.db.Query(`SELECT user_id FROM users ORDER BY user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *sqliteStore) UserExists(username string) (bool, error) {
	var one int
	err := s.db.QueryRow(`
		SELECT 1 FROM users WHERE username = ? COLLATE NOCASE LIMIT 1
	`, username).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ---------------------------------------------------------------
// Store interface — room instance operations
// ---------------------------------------------------------------

func (s *sqliteStore) LoadRoomInstance(roomId int) (*RoomInstanceData, error) {
	var r RoomInstanceData
	var updatedNano int64
	err := s.db.QueryRow(`
		SELECT room_id, zone, data, updated_at
		FROM room_instances WHERE room_id = ?
	`, roomId).Scan(&r.RoomId, &r.Zone, &r.Payload, &updatedNano)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	r.UpdatedAt = time.Unix(0, updatedNano)
	return &r, nil
}

func (s *sqliteStore) SaveRoomInstance(r *RoomInstanceData) error {
	if r == nil {
		return errors.New("persistence: SaveRoomInstance called with nil")
	}
	cp := *r
	return s.enqueue(writeOp{
		key:  opKey{kind: opRoomSave, id: r.RoomId},
		data: &cp,
	})
}

func (s *sqliteStore) DeleteRoomInstance(roomId int) error {
	return s.enqueue(writeOp{
		key: opKey{kind: opRoomDelete, id: roomId},
	})
}

func (s *sqliteStore) AllRoomInstanceIds() ([]int, error) {
	rows, err := s.db.Query(`SELECT room_id FROM room_instances ORDER BY room_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ---------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------

// Flush blocks until every write enqueued BEFORE the call has been
// committed. Writes enqueued AFTER the call are not guaranteed to be
// included (they may or may not ride along depending on timing).
func (s *sqliteStore) Flush() error {
	if s.closed.Load() {
		return errors.New("persistence: store is closed")
	}
	ack := make(chan struct{})
	select {
	case s.flushReqCh <- ack:
	case <-time.After(30 * time.Second):
		return errors.New("persistence: flush request timed out waiting for worker")
	}
	select {
	case <-ack:
		return nil
	case <-time.After(30 * time.Second):
		return errors.New("persistence: flush ack timed out")
	}
}

// Close flushes all pending writes, stops the worker, and closes the DB.
// Calling Close more than once is safe; subsequent calls are no-ops.
func (s *sqliteStore) Close() error {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()

	if s.closed.Load() {
		return nil
	}

	// Mark closed first so new enqueues see it.
	s.closed.Store(true)

	// Close the queue so the worker drains and exits.
	close(s.queue)

	// Wait for the worker to finish (commits final batch on its way out).
	<-s.workerDone

	return s.db.Close()
}

// stats returns a snapshot of the store's counters. Intended for tests.
func (s *sqliteStore) stats() storeStats {
	return storeStats{
		opsEnqueued:      s.opsEnqueued.Load(),
		opsCoalesced:     s.opsCoalesced.Load(),
		writesExecuted:   s.writesExecuted.Load(),
		batchesCommitted: s.batchesCommitted.Load(),
	}
}
