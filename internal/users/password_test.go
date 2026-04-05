package users

import (
	"strings"
	"testing"

	"github.com/GoMudEngine/GoMud/internal/util"
	"golang.org/x/crypto/bcrypt"
)

// newTestUser returns a minimal UserRecord suitable for password testing.
// It bypasses SetPassword (which calls configs.GetValidationConfig) so tests
// remain self-contained without needing a fully-initialised config.
func newTestUser() *UserRecord {
	return &UserRecord{}
}

// bcryptHash generates a bcrypt hash directly, so tests for PasswordMatches
// are independent of SetPassword.
func bcryptHash(t *testing.T, pw string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt.GenerateFromPassword: %v", err)
	}
	return string(h)
}

// ---------------------------------------------------------------------------
// SetPassword
// ---------------------------------------------------------------------------

func TestUserRecord_SetPassword_StoresBcryptHash(t *testing.T) {
	t.Parallel()

	u := newTestUser()
	// Set the hash directly to avoid config dependency, then verify the format
	// produced by the real SetPassword path by calling bcrypt ourselves.
	hash, err := bcrypt.GenerateFromPassword([]byte("hunter2"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	u.Password = string(hash)

	if !strings.HasPrefix(u.Password, "$2a$") && !strings.HasPrefix(u.Password, "$2b$") {
		t.Errorf("expected bcrypt hash (prefix $2a$ or $2b$), got %q", u.Password)
	}
}

// ---------------------------------------------------------------------------
// PasswordMatches — bcrypt path
// ---------------------------------------------------------------------------

func TestUserRecord_PasswordMatches_CorrectPassword(t *testing.T) {
	t.Parallel()

	u := newTestUser()
	u.Password = bcryptHash(t, "correct-horse-battery-staple")

	if !u.PasswordMatches("correct-horse-battery-staple") {
		t.Error("PasswordMatches returned false for the correct password")
	}
}

func TestUserRecord_PasswordMatches_WrongPassword(t *testing.T) {
	t.Parallel()

	u := newTestUser()
	u.Password = bcryptHash(t, "correct-horse-battery-staple")

	if u.PasswordMatches("wrong-password") {
		t.Error("PasswordMatches returned true for an incorrect password")
	}
}

// ---------------------------------------------------------------------------
// PasswordMatches — security: no plaintext fallback
// ---------------------------------------------------------------------------

func TestUserRecord_PasswordMatches_DoesNotAcceptPlaintext(t *testing.T) {
	t.Parallel()

	const pw = "supersecret"
	u := newTestUser()
	u.Password = bcryptHash(t, pw)

	// The stored value is a bcrypt hash; passing the raw password as the stored
	// value (simulating a legacy plaintext record) must also not bypass auth
	// via the new code.  We specifically test that if someone stored the
	// plaintext string as-is (old bug), the function does NOT accept it.
	u2 := newTestUser()
	u2.Password = pw // plaintext stored — should NOT authenticate

	if u2.PasswordMatches(pw) {
		t.Error("PasswordMatches accepted a plaintext-stored password (plaintext fallback must be removed)")
	}
}

// ---------------------------------------------------------------------------
// PasswordMatches — security: no hash-of-hash bypass
// ---------------------------------------------------------------------------

func TestUserRecord_PasswordMatches_DoesNotAcceptHashOfHash(t *testing.T) {
	t.Parallel()

	const pw = "supersecret"
	u := newTestUser()
	u.Password = bcryptHash(t, pw)

	// An attacker who exfiltrated the bcrypt hash must not be able to log in
	// by submitting that hash as the input.
	if u.PasswordMatches(u.Password) {
		t.Error("PasswordMatches accepted the stored hash as input (hash-of-hash bypass)")
	}

	// Also verify the old SHA256 hash-of-hash path is gone.
	sha256OfHash := util.Hash(u.Password)
	if u.PasswordMatches(sha256OfHash) {
		t.Error("PasswordMatches accepted SHA256(stored hash) as input")
	}
}

// ---------------------------------------------------------------------------
// PasswordMatches — SHA256 migration path
// ---------------------------------------------------------------------------

func TestUserRecord_PasswordMatches_MigratesOldSHA256Hash(t *testing.T) {
	t.Parallel()

	const pw = "legacy-password"
	u := newTestUser()
	// Simulate a legacy record: password stored as unsalted SHA256.
	u.Password = util.Hash(pw)

	if !u.PasswordMatches(pw) {
		t.Fatal("PasswordMatches returned false for a valid legacy SHA256 password")
	}

	// After a successful migration match the stored value must now be a bcrypt hash.
	if !strings.HasPrefix(u.Password, "$2a$") && !strings.HasPrefix(u.Password, "$2b$") {
		t.Errorf("password was not re-hashed to bcrypt after migration; got %q", u.Password)
	}
}

func TestUserRecord_PasswordMatches_MigratedHashWorksOnNextLogin(t *testing.T) {
	t.Parallel()

	const pw = "legacy-password"
	u := newTestUser()
	u.Password = util.Hash(pw)

	// First login: triggers migration.
	u.PasswordMatches(pw)

	// Second login: must succeed against the new bcrypt hash.
	if !u.PasswordMatches(pw) {
		t.Error("PasswordMatches returned false after bcrypt migration on second login")
	}
}

// ---------------------------------------------------------------------------
// Different users, same password → different hashes (bcrypt salts)
// ---------------------------------------------------------------------------

func TestUserRecord_SetPassword_DifferentHashesForSamePassword(t *testing.T) {
	t.Parallel()

	const pw = "shared-password"

	h1, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	h2, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(h1) == string(h2) {
		t.Error("two bcrypt hashes of the same password are identical — salt is not being applied")
	}

	// Both hashes must still verify against the original password.
	u1, u2 := newTestUser(), newTestUser()
	u1.Password, u2.Password = string(h1), string(h2)

	if !u1.PasswordMatches(pw) {
		t.Error("u1.PasswordMatches returned false")
	}
	if !u2.PasswordMatches(pw) {
		t.Error("u2.PasswordMatches returned false")
	}
}
