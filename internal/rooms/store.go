package rooms

import (
	"errors"

	"github.com/GoMudEngine/GoMud/internal/persistence"
)

// store is the persistence backend for room instance overlays.
// Room TEMPLATES stay in YAML (content creators edit them directly or
// via OLC). Only the mutable instance overlay — floor items, gold,
// dynamic containers, signs — lives in the store.
//
// Must be set via SetStore before any room instance operations run.
// The server main package initializes this during startup.
var store persistence.Store

// SetStore installs the persistence backend. Must be called exactly
// once during server startup, before any room instance operations.
func SetStore(s persistence.Store) {
	store = s
}

// requireStore returns a descriptive error if the store has not been
// installed, rather than producing a nil pointer panic.
func requireStore() error {
	if store == nil {
		return errors.New("rooms: persistence store not initialized (call rooms.SetStore first)")
	}
	return nil
}
