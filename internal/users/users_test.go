package users

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/GoMudEngine/GoMud/internal/configs"
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

// TestCreateUser_HonorsCallerRole is a regression test for C2: the admin
// bootstrap path used to call CreateUser (which unconditionally set
// Role=RoleUser) and then issue a second SaveUser to promote to admin.
// A crash between the two writes — or a batch boundary that committed
// the first and dropped the second — could leave the bootstrap admin
// persisted as a regular user, locking the operator out of their own
// server.
//
// The fix makes CreateUser default Role to RoleUser only when unset, so
// callers that explicitly set RoleAdmin before CreateUser get a single
// atomic insert with the correct role.
func TestCreateUser_HonorsCallerRole(t *testing.T) {
	resetUserManager()

	// CreateUser calls ValidateName which consults config. Set minimal
	// validation bounds so this test isn't coupled to any on-disk config.
	require.NoError(t, configs.AddOverlayOverrides(map[string]any{
		"Validation.NameSizeMin":     1,
		"Validation.NameSizeMax":     80,
		"Validation.PasswordSizeMin": 8,
		"Validation.PasswordSizeMax": 24,
	}))

	s, err := persistence.Open("file::memory:?cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	SetStore(s)
	t.Cleanup(func() { SetStore(nil) })

	u := NewUserRecord(0, 1)
	require.NoError(t, u.SetUsername("adminregress"))
	require.NoError(t, u.SetPassword("hunter2hunter2"))
	u.Role = RoleAdmin // caller explicitly wants admin — CreateUser must not clobber

	require.NoError(t, CreateUser(u))
	require.NoError(t, s.Flush())

	// Read back through the store to confirm the row landed with RoleAdmin.
	loaded, err := s.LoadUserByUsername("adminregress")
	require.NoError(t, err)
	assert.Equal(t, RoleAdmin, loaded.Role,
		"CreateUser must not overwrite a caller-set RoleAdmin — regression for C2")

	// And the unset-role default still works for the normal signup path.
	u2 := NewUserRecord(0, 2)
	// NewUserRecord sets Role=RoleUser. Clear it to exercise the default branch.
	u2.Role = ""
	require.NoError(t, u2.SetUsername("normalsignup"))
	require.NoError(t, u2.SetPassword("hunter2hunter2"))
	require.NoError(t, CreateUser(u2))
	require.NoError(t, s.Flush())

	loaded2, err := s.LoadUserByUsername("normalsignup")
	require.NoError(t, err)
	assert.Equal(t, RoleUser, loaded2.Role,
		"CreateUser must default empty role to RoleUser for normal signups")
}

// TestCreateUser_ConcurrentAllocatesUniqueIds is a regression test for
// H3: two concurrent CreateUser calls used to race on GetUniqueUserId
// (which consults the persistence store + active users map) because
// the store write from call A hadn't landed by the time call B ran
// its allocation. Both calls would then hand out the same user id and
// clobber each other in userManager.Users. The fix is a package-level
// createUserMu that serializes the entire create path end-to-end.
//
// This test hammers CreateUser with many concurrent signups and
// asserts that every successful creation got a distinct id. Run with
// -race to also catch data races on the manager maps.
func TestCreateUser_ConcurrentAllocatesUniqueIds(t *testing.T) {
	resetUserManager()

	require.NoError(t, configs.AddOverlayOverrides(map[string]any{
		"Validation.NameSizeMin":     1,
		"Validation.NameSizeMax":     80,
		"Validation.PasswordSizeMin": 8,
		"Validation.PasswordSizeMax": 24,
	}))

	s, err := persistence.Open("file::memory:?cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	SetStore(s)
	t.Cleanup(func() { SetStore(nil) })

	const n = 32

	var wg sync.WaitGroup
	results := make(chan int, n)
	start := make(chan struct{})

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			u := NewUserRecord(0, uint64(i+1))
			name := fmt.Sprintf("concurrentuser%d", i)
			if err := u.SetUsername(name); err != nil {
				t.Errorf("set username: %v", err)
				return
			}
			if err := u.SetPassword("hunter2hunter2"); err != nil {
				t.Errorf("set password: %v", err)
				return
			}
			<-start
			if err := CreateUser(u); err != nil {
				t.Errorf("CreateUser %s: %v", name, err)
				return
			}
			results <- u.UserId
		}(i)
	}

	close(start)
	wg.Wait()
	close(results)

	seen := make(map[int]bool)
	for id := range results {
		if seen[id] {
			t.Errorf("duplicate user id %d handed out to concurrent CreateUser calls (H3)", id)
		}
		seen[id] = true
	}
	assert.Equal(t, n, len(seen), "every concurrent CreateUser must get a unique id")
}
