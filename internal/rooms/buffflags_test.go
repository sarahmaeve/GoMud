package rooms

import (
	"testing"

	"github.com/GoMudEngine/GoMud/internal/buffs"
	"github.com/stretchr/testify/assert"
)

// TestBuffFlagConstants_MatchBuffsPackage verifies that the local buff
// flag constants in rooms stay in sync with the canonical values in
// internal/buffs. If someone changes a flag string in buffs, this test
// fails — catching the drift at test time rather than at runtime.
func TestBuffFlagConstants_MatchBuffsPackage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		local    string
		upstream string
	}{
		{"Hidden", flagHidden, buffs.Hidden},
		{"SeeHidden", flagSeeHidden, buffs.SeeHidden},
		{"SeeNouns", flagSeeNouns, buffs.SeeNouns},
		{"EmitsLight", flagEmitsLight, buffs.EmitsLight},
		{"Tripping", flagTripping, buffs.Tripping},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.upstream, tt.local,
				"rooms flag %s drifted from buffs.%s", tt.name, tt.name)
		})
	}
}
