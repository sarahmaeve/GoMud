package rooms

import (
	"errors"

	"github.com/GoMudEngine/GoMud/internal/users"
)

// userLookup is the active-user lookup backend for the rooms package.
// Instead of importing users and calling the package-level functions
// directly, rooms resolves users through this interface. This decouples
// the runtime dependency and enables test stubs.
//
// Must be set via SetUserLookup before any room operations that need
// to resolve users. The server main package initializes this during startup.
var userLookup users.UserLookup

// SetUserLookup installs the user lookup backend. Must be called
// exactly once during server startup, before any room operations that
// resolve users (e.g., MoveToRoom, SendText to players).
func SetUserLookup(ul users.UserLookup) {
	userLookup = ul
}

// requireUserLookup returns a descriptive error if the user lookup has
// not been installed, rather than producing a nil pointer panic.
func requireUserLookup() error {
	if userLookup == nil {
		return errors.New("rooms: user lookup not initialized (call rooms.SetUserLookup first)")
	}
	return nil
}
