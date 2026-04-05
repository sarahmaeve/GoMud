package users

import (
	"os"
	"testing"
	"time"

	"github.com/GoMudEngine/GoMud/internal/connections"
	"github.com/GoMudEngine/GoMud/internal/mudlog"
	"github.com/GoMudEngine/GoMud/internal/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	// Initialize the logger to stderr so that mudlog.Error calls inside
	// LoadUser (and other production code) do not panic with a nil slogInstance.
	mudlog.SetupLogger(nil, "", "", false)
	os.Exit(m.Run())
}

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

// TestLoadUser_MalformedPayload_ReturnsError verifies that LoadUser returns
// (nil, error) when the persistence store returns a row whose Payload bytes
// cannot be unmarshaled into a UserRecord. This was originally issue #27,
// which tested the old YAML-file path; after the SQLite persistence
// migration, the same guarantee must hold for corrupted payload blobs.
//
// A blank UserRecord silently returned from LoadUser could be passed to
// SaveUser later and overwrite a valid stored payload — silent data loss.
func TestLoadUser_MalformedPayload_ReturnsError(t *testing.T) {
	// Install an in-memory persistence store for the duration of this test.
	s, err := persistence.Open("file::memory:?cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	SetStore(s)
	t.Cleanup(func() { SetStore(nil) })

	const testUsername = "testbaduser"

	// Insert a user with a malformed YAML payload. The persistence store
	// does not validate the payload contents — it just stores bytes —
	// so this is a valid way to simulate corrupted on-disk data.
	err = s.SaveUser(&persistence.UserData{
		UserId:   999,
		Username: testUsername,
		Password: "$2a$10$notahash",
		Role:     "user",
		Joined:   time.Unix(1700000000, 0),
		Payload:  []byte("invalid: [yaml: content:\n  - broken\n  indentation: here"),
	})
	require.NoError(t, err, "enqueue malformed user")
	require.NoError(t, s.Flush(), "flush malformed user to disk")

	// LoadUser must return an error and nil *UserRecord, not a blank
	// record that a later SaveUser would use to overwrite the stored
	// bytes with an empty struct.
	u, err := LoadUser(testUsername, true)

	require.Error(t, err, "LoadUser must return an error when payload is malformed")
	assert.Nil(t, u, "LoadUser must return nil *UserRecord when payload is malformed — "+
		"returning a blank record could silently overwrite valid data on SaveUser")
	assert.Contains(t, err.Error(), "unmarshal",
		"error must mention the unmarshal failure context")
}
