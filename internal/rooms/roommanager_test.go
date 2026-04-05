package rooms

import (
	"testing"

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

	// Room 500 is NOT in roomManager.rooms
	// The nil deref happens at the range over roomBeingReplaced.Containers
	roomTpl := Room{
		RoomId: 500,
		Zone:   "testzone",
		Containers: map[string]Container{
			"chest": {Gold: 10},
		},
	}

	// This test exercises the nil path at save_and_load.go:185-188
	// SaveRoomTemplate does filesystem I/O before reaching the nil deref,
	// so this test will fail for a different reason (missing config/paths)
	// unless the fix is applied first. The key assertion is: no panic from
	// nil pointer dereference on roomBeingReplaced.
	require.NotPanics(t, func() {
		_ = SaveRoomTemplate(roomTpl)
	})
}
