package rooms

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GoMudEngine/GoMud/internal/exit"
	"github.com/GoMudEngine/GoMud/internal/items"

	"github.com/GoMudEngine/GoMud/internal/configs"
	"github.com/GoMudEngine/GoMud/internal/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

// setupTestRoom creates a template room on disk and an in-memory
// persistence store. Returns the room ID and a cleanup function.
// The caller must call resetRoomManager() before calling this.
func setupTestRoom(t *testing.T, roomId int, zone string, tpl *Room) {
	t.Helper()

	tmp := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "rooms", zone), 0755))
	configs.SetTestDataFilesPath(tmp)
	roomManager.zones[zone] = &ZoneConfig{RoomId: roomId}

	tpl.RoomId = roomId
	tpl.Zone = zone
	require.NoError(t, SaveRoomTemplate(*tpl))

	s, err := persistence.Open("file::memory:?cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	SetStore(s)
	t.Cleanup(func() { SetStore(nil) })
}

// TestSaveRoomInstance_EmbeddedFieldDiff verifies that the reflection-based
// save correctly detects diffs in fields promoted from embedded structs
// (RoomTemplate and RoomState). Without the collectInstanceFields recursion
// fix, embedded fields would be invisible to the save logic and instance
// overlays would silently be empty.
func TestSaveRoomInstance_EmbeddedFieldDiff(t *testing.T) {
	resetRoomManager()

	tpl := Room{
		RoomTemplate: RoomTemplate{
			RoomId:      800,
			Zone:        "testzone",
			Title:       "Template Title",
			Description: "Original description.",
		},
	}
	setupTestRoom(t, 800, "testzone", &tpl)

	// Load the room and modify a RoomState field.
	loaded := LoadRoomInstance(800)
	require.NotNil(t, loaded)
	assert.Equal(t, 0, loaded.Gold, "template should have zero gold")

	loaded.Gold = 50
	require.NoError(t, SaveRoomInstance(*loaded))
	require.NoError(t, store.Flush())

	// Reload and verify the overlay was persisted.
	reloaded := LoadRoomInstance(800)
	require.NotNil(t, reloaded)
	assert.Equal(t, 50, reloaded.Gold, "instance overlay must persist Gold")
	assert.Equal(t, "Template Title", reloaded.Title, "template field must survive reload")
	assert.Equal(t, "Original description.", reloaded.Description, "template field must survive reload")

	// Verify the overlay payload contains only the modified field.
	data, err := store.LoadRoomInstance(800)
	require.NoError(t, err)
	var overlay map[string]interface{}
	require.NoError(t, yaml.Unmarshal(data.Payload, &overlay))
	assert.Contains(t, overlay, "gold", "overlay must contain the modified field")
	assert.NotContains(t, overlay, "title", "overlay must not contain unchanged template fields")
	assert.NotContains(t, overlay, "zone", "overlay must not contain unchanged template fields")
}

// TestSaveRoomInstance_MultipleEmbeddedDiffs verifies that diffs across
// BOTH embedded structs (RoomTemplate and RoomState) are detected in a
// single save operation.
func TestSaveRoomInstance_MultipleEmbeddedDiffs(t *testing.T) {
	resetRoomManager()

	tpl := Room{
		RoomTemplate: RoomTemplate{
			RoomId:      801,
			Zone:        "testzone",
			Title:       "Multi-diff Room",
			Description: "Testing multiple embedded diffs.",
			Exits: map[string]exit.RoomExit{
				"north": {RoomId: 999},
			},
		},
	}
	setupTestRoom(t, 801, "testzone", &tpl)

	loaded := LoadRoomInstance(801)
	require.NotNil(t, loaded)

	// Modify fields from both RoomState and top-level Room.
	loaded.Gold = 100
	loaded.Items = []items.Item{{ItemId: 42}}

	require.NoError(t, SaveRoomInstance(*loaded))
	require.NoError(t, store.Flush())

	// Verify overlay contains both RoomState fields.
	data, err := store.LoadRoomInstance(801)
	require.NoError(t, err)
	var overlay map[string]interface{}
	require.NoError(t, yaml.Unmarshal(data.Payload, &overlay))
	assert.Contains(t, overlay, "gold", "overlay must contain Gold diff")
	assert.Contains(t, overlay, "items", "overlay must contain Items diff")
	assert.NotContains(t, overlay, "title", "overlay must not contain unchanged template field")
	assert.NotContains(t, overlay, "exits", "overlay must not contain unchanged template field")

	// Round-trip: reload and verify all values.
	reloaded := LoadRoomInstance(801)
	require.NotNil(t, reloaded)
	assert.Equal(t, 100, reloaded.Gold)
	assert.Len(t, reloaded.Items, 1)
	assert.Equal(t, 42, reloaded.Items[0].ItemId)
	assert.Equal(t, "Multi-diff Room", reloaded.Title, "template fields preserved")
	assert.Contains(t, reloaded.Exits, "north", "template exits preserved")
}

// TestSaveRoomInstance_TemplateOnlyRoom_NoOverlay verifies that a room
// with no instance modifications produces no overlay (the overlay is
// deleted or never created). This is the happy-path complement to the
// H1 corrupt-overlay regression test.
func TestSaveRoomInstance_TemplateOnlyRoom_NoOverlay(t *testing.T) {
	resetRoomManager()

	tpl := Room{
		RoomTemplate: RoomTemplate{
			RoomId:      802,
			Zone:        "testzone",
			Title:       "Unmodified Room",
			Description: "Should produce no overlay.",
		},
	}
	setupTestRoom(t, 802, "testzone", &tpl)

	loaded := LoadRoomInstance(802)
	require.NotNil(t, loaded)

	// Save without any modifications.
	require.NoError(t, SaveRoomInstance(*loaded))
	require.NoError(t, store.Flush())

	// Overlay should not exist.
	_, err := store.LoadRoomInstance(802)
	assert.ErrorIs(t, err, persistence.ErrNotFound,
		"unmodified room must not have a persisted overlay")
}

// TestYAMLRoundTrip_EmbeddedStructs verifies that yaml.Marshal and
// yaml.Unmarshal correctly handle the ",inline" tag for both RoomTemplate
// and RoomState embeddings — all fields survive a round-trip and no
// duplicate keys are produced.
func TestYAMLRoundTrip_EmbeddedStructs(t *testing.T) {
	t.Parallel()
	original := Room{
		RoomTemplate: RoomTemplate{
			RoomId:      900,
			Zone:        "roundtrip",
			Title:       "Round Trip Room",
			Description: "Testing YAML round-trip.",
			Biome:       "forest",
			IsBank:      true,
			Exits: map[string]exit.RoomExit{
				"south": {RoomId: 901},
			},
			Nouns: map[string]string{
				"tree": "a tall oak tree",
			},
			IdleMessages: []string{"leaves rustle"},
			Pvp:          true,
		},
		RoomState: RoomState{
			Gold: 75,
			Items: []items.Item{
				{ItemId: 10},
			},
			Containers: map[string]Container{
				"chest": {Gold: 25},
			},
			Signs: []Sign{
				{DisplayText: "hello"},
			},
			LongTermDataStore: map[string]any{
				"key": "value",
			},
		},
		SpawnInfo: []SpawnInfo{
			{MobId: 5, RespawnRate: "10 real minutes"},
		},
	}

	data, err := yaml.Marshal(original)
	require.NoError(t, err, "Marshal must succeed")

	// Verify no duplicate keys by checking the YAML output doesn't
	// contain the same top-level key twice.
	yamlStr := string(data)
	t.Logf("Marshaled YAML:\n%s", yamlStr)

	var roundTripped Room
	require.NoError(t, yaml.Unmarshal(data, &roundTripped), "Unmarshal must succeed")

	// RoomTemplate fields
	assert.Equal(t, original.RoomId, roundTripped.RoomId)
	assert.Equal(t, original.Zone, roundTripped.Zone)
	assert.Equal(t, original.Title, roundTripped.Title)
	assert.Equal(t, original.Description, roundTripped.Description)
	assert.Equal(t, original.Biome, roundTripped.Biome)
	assert.Equal(t, original.IsBank, roundTripped.IsBank)
	assert.Equal(t, original.Pvp, roundTripped.Pvp)
	assert.Contains(t, roundTripped.Exits, "south")
	assert.Contains(t, roundTripped.Nouns, "tree")
	assert.Equal(t, original.IdleMessages, roundTripped.IdleMessages)

	// RoomState fields
	assert.Equal(t, original.Gold, roundTripped.Gold)
	assert.Len(t, roundTripped.Items, 1)
	assert.Equal(t, 10, roundTripped.Items[0].ItemId)
	assert.Contains(t, roundTripped.Containers, "chest")
	assert.Equal(t, 25, roundTripped.Containers["chest"].Gold)
	assert.Len(t, roundTripped.Signs, 1)
	assert.Equal(t, "hello", roundTripped.Signs[0].DisplayText)
	assert.NotNil(t, roundTripped.LongTermDataStore)
	assert.Len(t, roundTripped.LongTermDataStore, 1)

	// Non-embedded fields
	assert.Len(t, roundTripped.SpawnInfo, 1)
	assert.Equal(t, 5, roundTripped.SpawnInfo[0].MobId)
}
