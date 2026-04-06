package rooms

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRequireUserLookup_NilReturnsError(t *testing.T) {
	saved := userLookup
	userLookup = nil
	t.Cleanup(func() { userLookup = saved })

	err := requireUserLookup()
	assert.Error(t, err, "requireUserLookup must return an error when userLookup is nil")
	assert.Contains(t, err.Error(), "user lookup not initialized")
}

func TestRequireUserLookup_SetReturnsNil(t *testing.T) {
	resetRoomManager() // sets userLookup via users.DefaultUserLookup()

	err := requireUserLookup()
	assert.NoError(t, err, "requireUserLookup must succeed when userLookup is set")
}
