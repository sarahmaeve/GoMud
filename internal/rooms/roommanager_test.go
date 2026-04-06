package rooms

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GoMudEngine/GoMud/internal/configs"
	"github.com/GoMudEngine/GoMud/internal/mudlog"
	"github.com/GoMudEngine/GoMud/internal/persistence"
	"github.com/GoMudEngine/GoMud/internal/users"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	// Tests in this package call production code paths that log via
	// mudlog. Initialize the logger to stderr so those calls don't
	// panic on a nil slog instance.
	mudlog.SetupLogger(nil, "", "", false)
	os.Exit(m.Run())
}

func resetRoomManager() {
	roomManager.rooms = make(map[int]*Room)
	roomManager.zones = make(map[string]*ZoneConfig)
	roomManager.roomsWithUsers = make(map[int]int)
	roomManager.roomsWithMobs = make(map[int]int)
	roomManager.roomIdToFileCache = make(map[int]string)
	SetUserLookup(users.DefaultUserLookup())
}

func TestMoveToRoom_NilUser_DoesNotPanic(t *testing.T) {
	resetRoomManager()

	// Add a target room so LoadRoom won't return nil for toRoomId
	targetRoom := &Room{RoomTemplate: RoomTemplate{RoomId: 100, Zone: "testzone"}}
	roomManager.rooms[100] = targetRoom

	// Call with a userId that does not exist in userManager
	// users.GetByUserId(99999) returns nil — this should not panic
	require.NotPanics(t, func() {
		err := MoveToRoom(99999, 100)
		require.Error(t, err)
	})
}

func TestSaveRoomTemplate_RoomNotInMemory_DoesNotPanic(t *testing.T) {
	resetRoomManager()

	// Set up a temp DataFiles path so the YAML write inside SaveRoomTemplate
	// actually succeeds, allowing execution to reach the nil-deref path at
	// save_and_load.go:185 (roomBeingReplaced := roomManager.rooms[...]).
	tmp := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "rooms", "testzone"), 0755))
	configs.SetTestDataFilesPath(tmp)

	// Register the zone so GetZoneConfig returns a non-nil *ZoneConfig —
	// otherwise the line `events.AddToQueue(events.RebuildMap{MapRootRoomId: cfg.RoomId})`
	// would nil-deref before we reach the actual bug path.
	roomManager.zones["testzone"] = &ZoneConfig{RoomId: 1}

	// Room 500 is NOT in roomManager.rooms — triggers the nil lookup at line 185.
	roomTpl := Room{
		RoomTemplate: RoomTemplate{
			RoomId: 500,
			Zone:   "testzone",
		},
		RoomState: RoomState{
			Containers: map[string]Container{
				"chest": {Gold: 10},
			},
		},
	}

	require.NotPanics(t, func() {
		err := SaveRoomTemplate(roomTpl)
		require.NoError(t, err, "SaveRoomTemplate should succeed for a room not yet in memory")
	})

	// The fix inserts the room into memory as a side effect — verify it's there.
	require.NotNil(t, roomManager.rooms[500], "after SaveRoomTemplate the new room should be in memory")
}

// TestLoadRoomInstance_CorruptOverlay_NotDeletedOnNextSave is the
// regression test for H1. Before the fix, LoadRoomInstance would log
// and return the template-only room on unmarshal failure — without any
// marker on the returned Room. The next call to SaveRoomInstance would
// find that the room equals its template (because the corrupt overlay
// never applied), hit the "no overlay needed" branch, and DELETE the
// corrupt row — destroying the last copy of whatever state was there.
//
// The fix: LoadRoomInstance sets room.instanceOverlayCorrupt=true, and
// SaveRoomInstance preserves the row (with a warning) when that flag
// is set and no overlay would otherwise be written.
func TestLoadRoomInstance_CorruptOverlay_NotDeletedOnNextSave(t *testing.T) {
	resetRoomManager()

	// Stage a template YAML so LoadRoomTemplate succeeds.
	tmp := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "rooms", "testzone"), 0755))
	configs.SetTestDataFilesPath(tmp)
	roomManager.zones["testzone"] = &ZoneConfig{RoomId: 700}

	tpl := Room{
		RoomTemplate: RoomTemplate{
			RoomId:      700,
			Zone:        "testzone",
			Title:       "Regression Room",
			Description: "For H1.",
		},
	}
	require.NoError(t, SaveRoomTemplate(tpl))

	// Install an in-memory persistence store and seed it with a
	// deliberately corrupt overlay row for this room id.
	s, err := persistence.Open("file::memory:?cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	SetStore(s)
	t.Cleanup(func() { SetStore(nil) })

	require.NoError(t, s.SaveRoomInstance(&persistence.RoomInstanceData{
		RoomId:    700,
		Zone:      "testzone",
		Payload:   []byte("this: is: not: valid: yaml: [\n  - broken"),
		UpdatedAt: time.Now(),
	}))
	require.NoError(t, s.Flush())

	// Sanity: the corrupt row is present before we trigger load/save.
	before, err := s.LoadRoomInstance(700)
	require.NoError(t, err, "corrupt overlay must exist before load/save cycle")
	require.NotNil(t, before)

	// Load — must mark the room corrupt and return template-only state.
	loaded := LoadRoomInstance(700)
	require.NotNil(t, loaded)
	assert.True(t, loaded.instanceOverlayCorrupt,
		"LoadRoomInstance must mark the room when the overlay failed to unmarshal")

	// Save — must NOT delete the corrupt row. Before the fix this would
	// compare equal to the template and hit DeleteRoomInstance.
	require.NoError(t, SaveRoomInstance(*loaded))
	require.NoError(t, s.Flush())

	// Verify the corrupt row is still present.
	after, err := s.LoadRoomInstance(700)
	require.NoError(t, err, "corrupt overlay must survive a save cycle (H1)")
	require.NotNil(t, after)
	assert.Equal(t, before.Payload, after.Payload,
		"corrupt overlay payload must be preserved verbatim, not overwritten or deleted")
}
