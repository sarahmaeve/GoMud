package users

import (
	"sync"
	"testing"

	"github.com/GoMudEngine/GoMud/internal/connections"
)

// TestUserManager_ConcurrentAccess exercises concurrent reads and writes on
// userManager's maps.  Run with -race; the test should FAIL (data race) before
// the sync.RWMutex fix is applied and PASS afterwards.
func TestUserManager_ConcurrentAccess(t *testing.T) {
	ResetActiveUsers()

	const goroutines = 20

	var wg sync.WaitGroup
	wg.Add(goroutines * 3)

	// Writers: add users via SetTestUser
	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			connId := connections.ConnectionId(i + 1)
			u := &UserRecord{
				UserId:       i + 1,
				Username:     "user" + string(rune('A'+i)),
				connectionId: connId,
			}
			SetTestUser(u)
			SetTestConnection(connId, u.UserId)
		}()
	}

	// Readers: look up users by id
	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			_ = GetByUserId(i + 1)
		}()
	}

	// Mixed: iterate all active users
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_ = GetAllActiveUsers()
		}()
	}

	wg.Wait()
}

// TestUserManager_ConcurrentLogout exercises concurrent LogOutUserByConnectionId
// calls against the maps.
func TestUserManager_ConcurrentLogout(t *testing.T) {
	ResetActiveUsers()

	const count = 10

	// Pre-populate: only set the Connections map entry (no UserRecord) to
	// exercise the nil-user branch without triggering SaveUser disk I/O.
	for i := 1; i <= count; i++ {
		connId := connections.ConnectionId(i)
		userManager.Connections[connId] = i // orphan connection
	}

	var wg sync.WaitGroup
	wg.Add(count)
	for i := 1; i <= count; i++ {
		i := i
		go func() {
			defer wg.Done()
			connId := connections.ConnectionId(i)
			_ = LogOutUserByConnectionId(connId)
		}()
	}
	wg.Wait()
}
