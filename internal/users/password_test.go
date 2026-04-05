package users

import (
	"strings"
	"sync"
	"testing"

	"github.com/GoMudEngine/GoMud/internal/configs"
	"github.com/GoMudEngine/GoMud/internal/util"
	"github.com/stretchr/testify/require"
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
	// Not parallel: mutates global config via setupPasswordConfig.
	setupPasswordConfig(t, 1, 72)

	u := newTestUser()
	require.NoError(t, u.SetPassword("hunter2"))

	if !strings.HasPrefix(u.Password, "$2a$") && !strings.HasPrefix(u.Password, "$2b$") {
		t.Errorf("expected bcrypt hash (prefix $2a$ or $2b$), got %q", u.Password)
	}

	// Sanity check: the stored hash should verify against the original password.
	if err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte("hunter2")); err != nil {
		t.Errorf("SetPassword did not store a verifiable bcrypt hash: %v", err)
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
// PasswordMatches — SHA256 legacy path (no in-place upgrade)
// ---------------------------------------------------------------------------

// TestUserRecord_PasswordMatches_LegacySHA256_StillWorks verifies that a user
// whose password is stored as an unsalted SHA256 hash can still log in.
func TestUserRecord_PasswordMatches_LegacySHA256_StillWorks(t *testing.T) {
	t.Parallel()

	const pw = "legacy-password"
	u := newTestUser()
	u.Password = util.Hash(pw)

	if !u.PasswordMatches(pw) {
		t.Fatal("PasswordMatches returned false for a valid legacy SHA256 password")
	}
}

// TestUserRecord_PasswordMatches_LegacySHA256_NotUpgraded verifies that
// PasswordMatches does NOT upgrade the stored SHA256 hash to bcrypt in place.
// This is the key behavioral change from the old code: no in-place upgrade
// means no unsynchronized write to u.Password on the read path (Bug A fix).
func TestUserRecord_PasswordMatches_LegacySHA256_NotUpgraded(t *testing.T) {
	t.Parallel()

	const pw = "legacy-password"
	u := newTestUser()
	originalHash := util.Hash(pw)
	u.Password = originalHash

	u.PasswordMatches(pw)

	// The stored password must still be the SHA256 hash, not a bcrypt hash.
	if u.Password != originalHash {
		t.Errorf("PasswordMatches upgraded the stored hash in place; got %q, want original SHA256 %q", u.Password, originalHash)
	}
	if strings.HasPrefix(u.Password, "$2a$") || strings.HasPrefix(u.Password, "$2b$") {
		t.Errorf("PasswordMatches re-hashed to bcrypt in place; stored value is now %q", u.Password)
	}
}

// TestUserRecord_PasswordMatches_LegacySHA256_NoDataRace verifies that
// concurrent PasswordMatches calls on a SHA256-format UserRecord do not cause
// a data race. Run with -race. The old code wrote u.Password = string(hash)
// without synchronization; the new code has no write on this path at all.
func TestUserRecord_PasswordMatches_LegacySHA256_NoDataRace(t *testing.T) {
	t.Parallel()

	const pw = "legacy-password"
	u := newTestUser()
	u.Password = util.Hash(pw)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			if !u.PasswordMatches(pw) {
				t.Errorf("PasswordMatches returned false for correct legacy password")
			}
		}()
	}
	wg.Wait()
}

// TestUserRecord_PasswordMatches_LegacySHA256_WrongPassword verifies that an
// incorrect password is rejected even when the stored hash is a SHA256 hash.
func TestUserRecord_PasswordMatches_LegacySHA256_WrongPassword(t *testing.T) {
	t.Parallel()

	u := newTestUser()
	u.Password = util.Hash("correct-password")

	if u.PasswordMatches("wrong-password") {
		t.Error("PasswordMatches returned true for incorrect password against SHA256 stored hash")
	}
}

// ---------------------------------------------------------------------------
// Different users, same password → different hashes (bcrypt salts)
// ---------------------------------------------------------------------------

func TestUserRecord_SetPassword_DifferentHashesForSamePassword(t *testing.T) {
	// Not parallel: mutates global config via setupPasswordConfig.
	setupPasswordConfig(t, 1, 72)

	const pw = "shared-password"

	u1, u2 := newTestUser(), newTestUser()
	require.NoError(t, u1.SetPassword(pw))
	require.NoError(t, u2.SetPassword(pw))

	if u1.Password == u2.Password {
		t.Error("SetPassword produced identical hashes for the same password — salt is not being applied")
	}

	// Both records must still authenticate with the shared password.
	if !u1.PasswordMatches(pw) {
		t.Error("u1.PasswordMatches returned false after SetPassword")
	}
	if !u2.PasswordMatches(pw) {
		t.Error("u2.PasswordMatches returned false after SetPassword")
	}
}

// ---------------------------------------------------------------------------
// Password length validation
// ---------------------------------------------------------------------------

func setupPasswordConfig(t *testing.T, min, max int) {
	t.Helper()
	configs.SetTestValidation(configs.Validation{
		PasswordSizeMin: configs.ConfigInt(min),
		PasswordSizeMax: configs.ConfigInt(max),
		NameSizeMin:     1,
		NameSizeMax:     32,
	})
}

func TestValidatePassword_TooShort(t *testing.T) {
	setupPasswordConfig(t, 8, 24)

	err := ValidatePassword("short")
	if err == nil {
		t.Error("expected error for password shorter than minimum, got nil")
	}
}

func TestValidatePassword_TooLong(t *testing.T) {
	setupPasswordConfig(t, 8, 24)

	err := ValidatePassword("this-password-is-way-too-long-for-the-max")
	if err == nil {
		t.Error("expected error for password longer than maximum, got nil")
	}
}

func TestValidatePassword_ValidLength(t *testing.T) {
	setupPasswordConfig(t, 8, 24)

	err := ValidatePassword("good-password")
	if err != nil {
		t.Errorf("expected nil error for valid password, got: %v", err)
	}
}

func TestValidatePassword_ExactMinimum(t *testing.T) {
	setupPasswordConfig(t, 8, 24)

	err := ValidatePassword("12345678") // exactly 8
	if err != nil {
		t.Errorf("expected nil error for password at exact minimum, got: %v", err)
	}
}

func TestValidatePassword_ExactMaximum(t *testing.T) {
	setupPasswordConfig(t, 8, 24)

	err := ValidatePassword("123456789012345678901234") // exactly 24
	if err != nil {
		t.Errorf("expected nil error for password at exact maximum, got: %v", err)
	}
}

func TestValidatePassword_SpecialCharacters(t *testing.T) {
	setupPasswordConfig(t, 8, 24)

	passwords := []string{
		"pass_word!",
		"p@$$w0rd#",
		"my^secret%",
		"test&pass=1",
		"spaces work",
	}
	for _, pw := range passwords {
		if err := ValidatePassword(pw); err != nil {
			t.Errorf("expected special characters to be allowed in %q, got: %v", pw, err)
		}
	}
}
