package rooms

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GoMudEngine/GoMud/internal/configs"
	"github.com/stretchr/testify/require"
)

func resetRoomManager() {
	roomManager.rooms = make(map[int]*Room)
	roomManager.zones = make(map[string]*ZoneConfig)
	roomManager.roomsWithUsers = make(map[int]int)
	roomManager.roomsWithMobs = make(map[int]int)
	roomManager.roomIdToFileCache = make(map[int]string)
}

func TestMoveToRoom_NilUser_DoesNotPanic(t *testing.T) {
	resetRoomManager()

	// Add a target room so LoadRoom won't return nil for toRoomId
	targetRoom := &Room{RoomId: 100, Zone: "testzone"}
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
		RoomId: 500,
		Zone:   "testzone",
		Containers: map[string]Container{
			"chest": {Gold: 10},
		},
	}

	require.NotPanics(t, func() {
		err := SaveRoomTemplate(roomTpl)
		require.NoError(t, err, "SaveRoomTemplate should succeed for a room not yet in memory")
	})

	// The fix inserts the room into memory as a side effect — verify it's there.
	require.NotNil(t, roomManager.rooms[500], "after SaveRoomTemplate the new room should be in memory")
}
