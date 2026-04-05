package scripting

// Tests for GetAllActors nil filtering (issue #15).
//
// The fix at room_func.go:135-148 adds nil-check guards before appending
// GetActor results.  GetActor returns nil when users.GetByUserId returns nil
// (unknown userId) or mobs.GetInstance returns nil (unknown mobInstanceId).
//
// Strategy: build a ScriptRoom whose underlying rooms.Room has player IDs
// that are NOT registered in the users package.  rooms.Room.AddPlayer does
// not look up the users registry, so we can inject arbitrary IDs.  When
// GetAllActors iterates those IDs, every GetActor call returns nil.  The
// fixed code skips nils; the pre-fix code would have appended them.

import (
	"testing"

	"github.com/GoMudEngine/GoMud/internal/characters"
	"github.com/GoMudEngine/GoMud/internal/rooms"
	"github.com/GoMudEngine/GoMud/internal/users"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetActor_UnknownIds_ReturnsNil confirms the precondition of issue #15:
// GetActor returns nil when neither the userId nor the mobInstanceId is found
// in their respective registries.  This is the value that the old code would
// have blindly appended into the actor list.
//
// Not marked t.Parallel() because it reads from the shared userManager global
// (via users.GetByUserId) and races with other tests that call ResetActiveUsers.
func TestGetActor_UnknownIds_ReturnsNil(t *testing.T) {
	// userId 88881 and mobInstanceId 88882 use values well outside the range of
	// any test-registered IDs, so no ResetActiveUsers is needed.

	// userId 88881 is not registered — GetByUserId returns nil.
	actor := GetActor(88881, 0)
	assert.Nil(t, actor, "GetActor should return nil for an unknown userId")

	// mobInstanceId 88882 is not in mobInstances — GetInstance returns nil.
	actor = GetActor(0, 88882)
	assert.Nil(t, actor, "GetActor should return nil for an unknown mob instance id")
}

// TestGetAllActors_SkipsNilActors verifies that GetAllActors never appends nil
// entries when the underlying player/mob IDs are not in their registries.
//
// Before the fix the nil guard was absent: every GetActor result was appended
// unconditionally, so the returned slice would contain nil pointers that would
// panic on first dereference.  After the fix the slice must be empty (or
// contain only non-nil entries from other registered actors).
func TestGetAllActors_SkipsNilActors(t *testing.T) {
	// Not t.Parallel() — mutates the shared userManager global via ResetActiveUsers.
	users.ResetActiveUsers()
	defer users.ResetActiveUsers()

	// Build a Room and inject player IDs that are NOT in the users registry.
	// rooms.Room.AddPlayer just appends to the internal slice; no registry
	// lookup occurs there, so this succeeds even for unknown IDs.
	r := rooms.NewRoom("test-zone")
	r.AddPlayer(77771) // not registered in users package
	r.AddPlayer(77772) // not registered in users package

	// Verify the room reports the players so that GetAllActors will actually
	// iterate them (otherwise the test would trivially pass for the wrong reason).
	require.ElementsMatch(t, []int{77771, 77772}, r.GetPlayers(),
		"setup: room must report the two injected player IDs")

	sr := ScriptRoom{
		roomId:     r.RoomId,
		roomRecord: r,
	}

	actors := sr.GetAllActors()

	// Every GetActor call returned nil (users not registered), so after the
	// fix the slice must be empty — zero nil entries.
	for i, a := range actors {
		assert.NotNil(t, a, "GetAllActors returned a nil *ScriptActor at index %d", i)
	}
	assert.Empty(t, actors,
		"GetAllActors must return an empty slice when all GetActor calls return nil; "+
			"a non-empty slice means nil entries slipped through")
}

// TestGetAllActors_IncludesRegisteredActors is the positive counterpart: when
// a player IS registered, GetAllActors must include them.
func TestGetAllActors_IncludesRegisteredActors(t *testing.T) {
	users.ResetActiveUsers()
	defer users.ResetActiveUsers()

	const knownUserId = 77780

	u := &users.UserRecord{
		UserId:    knownUserId,
		Username:  "known-actor-user",
		Character: &characters.Character{},
	}
	users.SetTestUser(u)

	r := rooms.NewRoom("test-zone")
	r.AddPlayer(knownUserId)
	r.AddPlayer(77781) // unknown — should be skipped

	sr := ScriptRoom{roomId: r.RoomId, roomRecord: r}

	actors := sr.GetAllActors()

	require.Len(t, actors, 1,
		"GetAllActors should contain exactly the one registered player; unknown id must be skipped")
	assert.Equal(t, knownUserId, actors[0].UserId(),
		"the returned actor must be the registered player")
}
