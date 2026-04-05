package users

import (
	"errors"
	"fmt"

	"github.com/GoMudEngine/GoMud/internal/persistence"
	"gopkg.in/yaml.v2"
)

// store is the persistence backend for user records. It must be set
// via SetStore before any user operations run. The server main package
// initializes this during startup.
var store persistence.Store

// SetStore installs the persistence backend. Must be called exactly
// once during server startup, before any user operations.
func SetStore(s persistence.Store) {
	store = s
}

// GetStore returns the currently installed persistence store. Exported
// so that command-line tooling (e.g. --create-admin) and graceful
// shutdown code can call Flush/Close on it.
func GetStore() persistence.Store {
	return store
}

// requireStore is a sanity check used at the start of every persistence
// operation. It returns a descriptive error if the store has not been
// installed, rather than producing a nil pointer panic.
func requireStore() error {
	if store == nil {
		return errors.New("users: persistence store not initialized (call users.SetStore first)")
	}
	return nil
}

// userRecordToData serializes a UserRecord into a persistence.UserData.
// The variable-shape portion of the UserRecord is marshaled via YAML
// into the Payload field, preserving the existing on-disk format.
func userRecordToData(u *UserRecord) (*persistence.UserData, error) {
	if u == nil {
		return nil, errors.New("userRecordToData: nil UserRecord")
	}
	payload, err := yaml.Marshal(u)
	if err != nil {
		return nil, fmt.Errorf("marshal user payload: %w", err)
	}
	return &persistence.UserData{
		UserId:   u.UserId,
		Username: u.Username,
		Password: u.Password,
		Role:     u.Role,
		Joined:   u.Joined,
		Email:    u.EmailAddress,
		Payload:  payload,
	}, nil
}

// dataToUserRecord reconstructs a UserRecord from a persistence.UserData
// by unmarshaling the Payload. Also initializes any zero-value runtime
// fields (unsent state, etc.).
func dataToUserRecord(d *persistence.UserData) (*UserRecord, error) {
	if d == nil {
		return nil, errors.New("dataToUserRecord: nil UserData")
	}
	u := &UserRecord{}
	if err := yaml.Unmarshal(d.Payload, u); err != nil {
		return nil, fmt.Errorf("unmarshal user payload: %w", err)
	}
	// The Payload is authoritative for all struct fields, but the
	// store's indexed columns (UserId, Username, Password, Role,
	// Joined, Email) are what SQL queries rely on. If they have
	// somehow drifted from the payload, prefer the payload — that's
	// what the in-memory struct will be compared against.
	//
	// In practice the two are kept in lockstep via userRecordToData.
	// unsent is a pointer field skipped by yaml serialization —
	// initialize it post-unmarshal so SetUnsentText doesn't nil-deref.
	u.unsent = &unsentState{}
	return u, nil
}
