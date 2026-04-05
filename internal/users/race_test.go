package users

import (
	"strings"
	"sync"
	"testing"

	"github.com/GoMudEngine/GoMud/internal/connections"
	"gopkg.in/yaml.v3"
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

// TestUserRecord_UnsentText_ConcurrentAccess exercises concurrent SetUnsentText /
// GetUnsentText calls on a single UserRecord from multiple goroutines.
// Run with -race: the test MUST FAIL (data race) before the unsentMu mutex is
// added to UserRecord, and MUST PASS afterwards.
func TestUserRecord_UnsentText_ConcurrentAccess(t *testing.T) {
	u := &UserRecord{unsent: &unsentState{}}

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Writers: call SetUnsentText concurrently.
	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			u.SetUnsentText("typing something", "suggestion"+string(rune('A'+i%26)))
		}()
	}

	// Readers: call GetUnsentText concurrently.
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_, _ = u.GetUnsentText()
		}()
	}

	wg.Wait()
}

// TestUserRecord_YAMLMarshal_UnsentMuNotSerialized verifies two things:
//  1. A UserRecord can be marshaled to YAML and back without error (the mutex
//     field does not break serialization).
//  2. The mutex field (unsentMu) is NOT present in the YAML output — it must
//     never be persisted to the user file on disk.
func TestUserRecord_YAMLMarshal_UnsentMuNotSerialized(t *testing.T) {
	u := &UserRecord{
		UserId:   42,
		Username: "testplayer",
		Role:     RoleUser,
		unsent:   &unsentState{},
	}
	// Set some unsent text to prove the mutex is in use.
	u.SetUnsentText("partial command", "suggestion")

	out, err := yaml.Marshal(u)
	if err != nil {
		t.Fatalf("yaml.Marshal failed: %v", err)
	}

	yamlStr := string(out)

	// The mutex field must not appear in the YAML output.
	if strings.Contains(yamlStr, "unsentmu") || strings.Contains(yamlStr, "unsentMu") {
		t.Errorf("YAML output contains mutex field, which must not be serialized:\n%s", yamlStr)
	}
	// The unsent text fields are unexported and must also not appear.
	if strings.Contains(yamlStr, "unsenttext") || strings.Contains(yamlStr, "unsentText") {
		t.Errorf("YAML output contains unsentText field, which must not be serialized:\n%s", yamlStr)
	}
	if strings.Contains(yamlStr, "suggesttext") || strings.Contains(yamlStr, "suggestText") {
		t.Errorf("YAML output contains suggestText field, which must not be serialized:\n%s", yamlStr)
	}

	// Round-trip: unmarshal back and verify exported fields survived.
	var u2 UserRecord
	if err := yaml.Unmarshal(out, &u2); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}
	if u2.UserId != u.UserId {
		t.Errorf("UserId mismatch after round-trip: got %d, want %d", u2.UserId, u.UserId)
	}
	if u2.Username != u.Username {
		t.Errorf("Username mismatch after round-trip: got %q, want %q", u2.Username, u.Username)
	}
	if u2.Role != u.Role {
		t.Errorf("Role mismatch after round-trip: got %q, want %q", u2.Role, u.Role)
	}
}

// TestUserRecord_CopyByValue verifies that copying a UserRecord by value is safe
// and does not copy a mutex (regression test for the pointer-to-substruct fix).
// Both the original and the copy share the same *unsentState, so writes via
// either are visible to both — but crucially there is no data race.
func TestUserRecord_CopyByValue(t *testing.T) {
	u := &UserRecord{unsent: &unsentState{}}
	u.SetUnsentText("hello", "world")

	// Copy the struct by value — must not trigger go vet "copies lock value".
	copied := *u

	unsent1, sug1 := u.GetUnsentText()
	unsent2, sug2 := copied.GetUnsentText()

	if unsent1 != unsent2 {
		t.Errorf("unsentText mismatch: original %q, copy %q", unsent1, unsent2)
	}
	if sug1 != sug2 {
		t.Errorf("suggestText mismatch: original %q, copy %q", sug1, sug2)
	}
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
