package rooms

import (
	"github.com/GoMudEngine/GoMud/internal/exit"
	"github.com/GoMudEngine/GoMud/internal/mutators"
)

// RoomTemplate holds the static, YAML-defined properties of a room.
// These fields are loaded from template files and rarely change during
// gameplay. RoomTemplate is embedded in Room so that existing callers
// can continue to access promoted fields directly (e.g. room.Title).
type RoomTemplate struct {
	RoomId          int                      `yaml:"roomid"`                    // a unique numeric index of the room. Also the filename.
	Zone            string                   `yaml:"zone"`                      // zone is a way to partition rooms into groups. Also into folders.
	MusicFile       string                   `yaml:"musicfile,omitempty"`       // background music to play when in this room
	IsBank          bool                     `yaml:"isbank,omitempty"`          // Is this a bank room? If so, players can deposit/withdraw gold here.
	IsStorage       bool                     `yaml:"isstorage,omitempty"`       // Is this a storage room? If so, players can add/remove objects here.
	IsCharacterRoom bool                     `yaml:"ischaracterroom,omitempty"` // Is this a room where characters can create new characters to swap between them?
	Title           string                   `yaml:"title"`                     // Title shown to the user
	Description     string                   `yaml:"description"`               // Description shown to the user
	MapSymbol       string                   `yaml:"mapsymbol,omitempty"`       // The symbol to use when generating a map of the zone
	MapLegend       string                   `yaml:"maplegend,omitempty"`       // The text to display in the legend for this room. Should be one word.
	Biome           string                   `yaml:"biome,omitempty"`           // The biome of the room. Used for weather generation.
	Exits           map[string]exit.RoomExit `yaml:"exits"`                     // Exits to other rooms
	Nouns           map[string]string        `yaml:"nouns,omitempty"`           // Interesting nouns to highlight in the room or reveal on succesful searches.
	SkillTraining   map[string]TrainingRange `yaml:"skilltraining,omitempty"`   // list of skills that can be trained in this room
	IdleMessages    []string                 `yaml:"idlemessages,omitempty"`    // list of messages that can be displayed to players in the room
	Mutators        mutators.MutatorList     `yaml:"mutators,omitempty"`        // mutators this room spawns with.
	Pvp             bool                     `yaml:"pvp,omitempty"`             // if config pvp is set to `limited`, uses this value
}
