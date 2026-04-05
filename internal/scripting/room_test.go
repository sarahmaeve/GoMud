package scripting

import (
	"testing"

	"github.com/GoMudEngine/GoMud/internal/characters"
	"github.com/GoMudEngine/GoMud/internal/users"
	"github.com/stretchr/testify/require"
)

func TestTryRoomCommand_NilRoom_DoesNotPanic(t *testing.T) {
	users.ResetActiveUsers()
	defer users.ResetActiveUsers()

	// Create a user whose RoomId points to a non-existent room
	u := &users.UserRecord{
		UserId:   77777,
		Username: "testplayer",
		Character: &characters.Character{
			RoomId: 999999, // This room does not exist — LoadRoom returns nil
		},
	}
	users.SetTestUser(u)

	// TryRoomCommand should handle nil room gracefully, not panic
	require.NotPanics(t, func() {
		_, err := TryRoomCommand("north", "", 77777)
		// We expect an error or false return, not a panic
		_ = err
	})
}

func TestTryRoomCommand_UserNotFound_ReturnsError(t *testing.T) {
	users.ResetActiveUsers()
	defer users.ResetActiveUsers()

	// Call with a userId that has no user registered
	handled, err := TryRoomCommand("look", "", 99999)

	require.Error(t, err)
	require.False(t, handled)
}
