package users

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultUserLookup_ReturnsWorkingSingleton(t *testing.T) {
	ResetActiveUsers()

	ul := DefaultUserLookup()
	require.NotNil(t, ul, "DefaultUserLookup must return a non-nil *ActiveUsers")

	// Unknown user returns nil.
	assert.Nil(t, ul.GetByUserId(99999), "unknown userId must return nil")

	// Add a user and verify lookup resolves it.
	testUser := &UserRecord{UserId: 42}
	SetTestUser(testUser)
	t.Cleanup(func() { RemoveTestUser(42) })

	found := ul.GetByUserId(42)
	require.NotNil(t, found, "known userId must return a *UserRecord")
	assert.Equal(t, 42, found.UserId)
}

func TestPackageLevelGetByUserId_DelegatesToSingleton(t *testing.T) {
	ResetActiveUsers()

	testUser := &UserRecord{UserId: 7}
	SetTestUser(testUser)
	t.Cleanup(func() { RemoveTestUser(7) })

	// Both paths must return the same result.
	fromPkgLevel := GetByUserId(7)
	fromLookup := DefaultUserLookup().GetByUserId(7)

	require.NotNil(t, fromPkgLevel)
	require.NotNil(t, fromLookup)
	assert.Same(t, fromPkgLevel, fromLookup,
		"package-level GetByUserId and DefaultUserLookup().GetByUserId must return the same pointer")

	// Both return nil for unknown users.
	assert.Nil(t, GetByUserId(99999))
	assert.Nil(t, DefaultUserLookup().GetByUserId(99999))
}
