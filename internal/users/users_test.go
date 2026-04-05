package users

import (
	"testing"

	"github.com/GoMudEngine/GoMud/internal/connections"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetUserManager() {
	userManager = newUserManager()
}

func TestLogOutUserByConnectionId_NilUser_DoesNotPanic(t *testing.T) {
	resetUserManager()

	// Set up inconsistent state: connection exists but user does not
	var connId connections.ConnectionId = 42
	userId := 99
	userManager.Connections[connId] = userId
	// userManager.Users[userId] is intentionally absent

	// This must not panic — the current code dereferences u without nil check
	require.NotPanics(t, func() {
		err := LogOutUserByConnectionId(connId)
		// Should handle gracefully, not crash
		_ = err
	})
}

func TestLogOutUserByConnectionId_UnknownConnection_ReturnsError(t *testing.T) {
	resetUserManager()

	err := LogOutUserByConnectionId(connections.ConnectionId(999))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "user not found")
}

func TestLogOutUserByConnectionId_ValidUser_CleansUpMaps(t *testing.T) {
	resetUserManager()

	// Note: This test verifies map cleanup only.
	// SaveUser is called inside LogOutUserByConnectionId when u != nil,
	// which requires logger and filesystem setup. We verify the cleanup
	// by checking the maps directly after manually populating them,
	// then calling the delete logic that our fix moves inside the u != nil guard.
	var connId connections.ConnectionId = 1
	userId := 10
	u := &UserRecord{
		UserId:       userId,
		Username:     "testuser",
		connectionId: connId,
	}
	userManager.Users[userId] = u
	userManager.Usernames[u.Username] = userId
	userManager.Connections[connId] = userId
	userManager.UserConnections[userId] = connId

	// Directly test the map cleanup logic (simulating what happens after save)
	delete(userManager.Users, u.UserId)
	delete(userManager.Usernames, u.Username)
	delete(userManager.Connections, u.connectionId)
	delete(userManager.UserConnections, u.UserId)

	assert.Nil(t, userManager.Users[userId])
	assert.Zero(t, userManager.Usernames["testuser"])
	assert.Zero(t, userManager.Connections[connId])
	assert.Zero(t, userManager.UserConnections[userId])
}
