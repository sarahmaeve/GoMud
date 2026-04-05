package users

// Test helpers for cross-package testing.
// These functions allow other internal packages to set up user state
// for testing without going through the full LoginUser flow.
// Since this is under internal/, it cannot be used outside the module.

// SetTestUser adds a user directly to the active users map.
// For testing only.
func SetTestUser(u *UserRecord) {
	userManager.Users[u.UserId] = u
}

// RemoveTestUser removes a user from the active users map.
// For testing only.
func RemoveTestUser(userId int) {
	delete(userManager.Users, userId)
}

// ResetActiveUsers resets the user manager to a clean state.
// For testing only.
func ResetActiveUsers() {
	userManager = newUserManager()
}
