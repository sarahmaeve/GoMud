package users

// UserLookup is the minimal read-only interface for resolving active users
// by their user ID. Most consumers (rooms, mobcommands, scripting) only
// need this single method.
//
// The concrete implementation is *ActiveUsers, which guards the lookup
// with a read lock on userManager.
type UserLookup interface {
	GetByUserId(userId int) *UserRecord
}

// Compile-time check: *ActiveUsers must satisfy UserLookup.
var _ UserLookup = (*ActiveUsers)(nil)

// DefaultUserLookup returns the global active-user manager as a UserLookup.
// Consumers that accept dependency injection should receive this during
// initialization. If no override is provided, this is the default.
func DefaultUserLookup() UserLookup {
	return userManager
}
