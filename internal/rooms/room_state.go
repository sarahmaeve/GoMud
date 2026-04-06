package rooms

import (
	"github.com/GoMudEngine/GoMud/internal/items"
)

// RoomState holds the mutable instance data for a room — fields that
// change during gameplay (floor items, gold, container contents, etc.).
// This state is persisted as an overlay on top of the room template
// via the persistence store (SQLite).
//
// RoomState is embedded in Room so that existing callers can continue
// to access promoted fields directly (e.g. room.Gold).
type RoomState struct {
	Items             []items.Item         `yaml:"items,omitempty"`             // Items on the floor
	Stash             []items.Item         `yaml:"stash,omitempty"`             // list of items in the room that are not visible to players
	Gold              int                  `yaml:"gold,omitempty"`              // How much gold is on the ground?
	Containers        map[string]Container `yaml:"containers,omitempty"`        // If this room has a chest, what is in it?
	Signs             []Sign               `yaml:"sign,omitempty"`              // list of scribbles in the room
	LongTermDataStore map[string]any       `yaml:"longtermdatastore,omitempty"` // Long term data store for the room
}
