package rooms

import (
	"testing"

	"github.com/GoMudEngine/GoMud/internal/users"
	"github.com/stretchr/testify/assert"
)

// Tests in this file mutate package-level globals (userLookup).
// Not safe for t.Parallel().

func TestSetUserLookup_NilPanics(t *testing.T) {
	assert.Panics(t, func() { SetUserLookup(nil) },
		"SetUserLookup(nil) must panic — missing dependency is a programmer error")
}

// fakeUserLookup is a test stub proving the interface enables
// dependency injection without the real users package.
type fakeUserLookup struct {
	users map[int]*users.UserRecord
}

func (f *fakeUserLookup) GetByUserId(userId int) *users.UserRecord {
	return f.users[userId]
}

func TestSetUserLookup_WithStub(t *testing.T) {
	saved := userLookup
	t.Cleanup(func() { userLookup = saved })

	fake := &fakeUserLookup{
		users: map[int]*users.UserRecord{
			1: {UserId: 1},
		},
	}
	SetUserLookup(fake)

	assert.NotNil(t, userLookup.GetByUserId(1))
	assert.Nil(t, userLookup.GetByUserId(999))
}
