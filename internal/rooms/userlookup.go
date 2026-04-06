package rooms

import (
	"github.com/GoMudEngine/GoMud/internal/users"
)

// UserLookup is the minimal read-only interface for resolving active
// users by their user ID. Defined in rooms (the consumer) per Go
// convention: interfaces belong to the package that uses them.
type UserLookup interface {
	GetByUserId(userId int) *users.UserRecord
}

// Compile-time check: *users.ActiveUsers must satisfy UserLookup.
var _ UserLookup = (*users.ActiveUsers)(nil)

// userLookup is the active-user lookup backend. Set once during
// startup via SetUserLookup before any concurrent room operations.
// Not guarded by a mutex because the write happens-before any reads
// (single-threaded initialization in main.go).
var userLookup UserLookup

// SetUserLookup installs the user lookup backend. Must be called
// exactly once during server startup, before any room operations that
// resolve users (e.g., MoveToRoom, SendText to players).
// Panics if ul is nil — failing to provide a required dependency is a
// programmer error and should be caught at startup.
func SetUserLookup(ul UserLookup) {
	if ul == nil {
		panic("rooms: SetUserLookup called with nil")
	}
	userLookup = ul
}
