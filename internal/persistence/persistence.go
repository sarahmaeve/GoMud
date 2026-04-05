// Package persistence provides a SQLite-backed store for GoMud runtime state.
//
// The store separates mutable runtime state (users, room instances) from
// content templates (rooms, mobs, items, quests) which remain in YAML.
// All writes are enqueued through a write-through buffer and committed in
// batches by a background goroutine, so game logic never blocks on disk I/O.
//
// Coalescing: rapid successive writes to the same entity (kind, id) are
// coalesced in the batch so only the final state is persisted, reducing
// write amplification for high-churn gameplay events.
//
// Durability: SQLite is configured with WAL mode and synchronous=NORMAL,
// which survives process crashes and loses at most the current in-memory
// batch on a power cut or kernel panic.
package persistence

import (
	"errors"
	"time"
)

// UserData is the persistence-layer representation of a user.
// The users package marshals its in-memory UserRecord into a UserData
// before handing it to the store, and unmarshals back on load.
//
// Password must be a bcrypt hash — the store does not re-hash.
// JSONBlob carries the variable-shape payload (character, inventory,
// buffs, stats, quest progress, etc.) as serialized JSON.
type UserData struct {
	UserId    int
	Username  string
	Password  string
	Role      string
	Joined    time.Time
	LastLogin time.Time
	Email     string
	JSONBlob  []byte
}

// RoomInstanceData is the persistence-layer representation of a room
// instance overlay. The rooms package extracts the instance-only fields
// from its in-memory Room struct and serializes them as JSON.
//
// The template fields (title, description, exits, etc.) live in YAML and
// are not persisted here.
type RoomInstanceData struct {
	RoomId    int
	Zone      string
	JSONBlob  []byte
	UpdatedAt time.Time
}

// Store is the interface for persisting runtime game state.
//
// Save operations are asynchronous: they enqueue a write request into a
// buffered channel and return immediately. A background worker commits
// writes in batches. Call Flush before Close (or rely on Close's
// implicit flush) to ensure all pending writes reach disk.
//
// Load operations read directly from the database and may block on
// in-flight commits briefly.
type Store interface {
	// User operations

	// LoadUser returns the user with the given UserId, or ErrNotFound.
	LoadUser(userId int) (*UserData, error)

	// LoadUserByUsername returns the user with the given username
	// (case-insensitive), or ErrNotFound.
	LoadUserByUsername(username string) (*UserData, error)

	// SaveUser enqueues a write for the given user. The write completes
	// asynchronously; call Flush to wait for all enqueued writes.
	// Returns an error if the write queue is saturated beyond a timeout.
	SaveUser(*UserData) error

	// DeleteUser enqueues a delete for the given user.
	DeleteUser(userId int) error

	// AllUsernames returns every username currently stored.
	AllUsernames() ([]string, error)

	// AllUserIds returns every user id currently stored.
	AllUserIds() ([]int, error)

	// UserExists returns true if a user with the given username
	// (case-insensitive) is currently stored.
	UserExists(username string) (bool, error)

	// Room instance operations

	// LoadRoomInstance returns the instance overlay for the given room id,
	// or ErrNotFound if no overlay exists (in which case the room has
	// only its template state and should be constructed from YAML alone).
	LoadRoomInstance(roomId int) (*RoomInstanceData, error)

	// SaveRoomInstance enqueues a write for the given room instance.
	SaveRoomInstance(*RoomInstanceData) error

	// DeleteRoomInstance enqueues a delete for the given room instance.
	DeleteRoomInstance(roomId int) error

	// AllRoomInstanceIds returns every room id that has a stored instance overlay.
	AllRoomInstanceIds() ([]int, error)

	// Lifecycle

	// Flush blocks until every write enqueued before the call has been
	// committed to disk.
	Flush() error

	// Close flushes pending writes, stops the background worker, and
	// closes the underlying database connection.
	Close() error
}

// ErrNotFound is returned by Load* methods when the requested entity
// does not exist.
var ErrNotFound = errors.New("persistence: not found")
