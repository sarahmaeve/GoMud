package users

// DefaultUserLookup returns the global active-user manager.
// The returned *ActiveUsers satisfies any single-method interface
// requiring GetByUserId.
func DefaultUserLookup() *ActiveUsers {
	return userManager
}
