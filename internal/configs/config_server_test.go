package configs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestServerValidate_EmptyFieldsGetDefaults(t *testing.T) {
	t.Parallel()
	s := Server{}
	s.Validate()

	assert.Equal(t, ConfigString("an open source MUD Library written in Go"), s.Tagline)
	assert.Equal(t, ConfigString("GoMud is an open source MUD (Multi-user Dungeon) game world and library."), s.Description)
	assert.Equal(t, ConfigString("github.com/GoMudEngine/GoMud"), s.URL)
	assert.Equal(t, ConfigString("discord.gg/cjukKvQWyy"), s.DiscordURL)
	assert.Equal(t, ConfigSecret("Mud"), s.Seed)
	assert.Equal(t, ConfigString("0.9.0"), s.CurrentVersion)
}

func TestServerValidate_CustomValuesPreserved(t *testing.T) {
	t.Parallel()
	s := Server{
		MudName:     "MyMUD",
		Tagline:     "a custom tagline",
		Description: "My custom MUD description.",
		URL:         "example.com/mymud",
		DiscordURL:  "discord.gg/custom",
		AdminName:   "Admin",
		AdminEmail:  "admin@example.com",
		Seed:        "CustomSeed",
	}
	s.Validate()

	assert.Equal(t, ConfigString("MyMUD"), s.MudName)
	assert.Equal(t, ConfigString("a custom tagline"), s.Tagline)
	assert.Equal(t, ConfigString("My custom MUD description."), s.Description)
	assert.Equal(t, ConfigString("example.com/mymud"), s.URL)
	assert.Equal(t, ConfigString("discord.gg/custom"), s.DiscordURL)
	assert.Equal(t, ConfigString("Admin"), s.AdminName)
	assert.Equal(t, ConfigString("admin@example.com"), s.AdminEmail)
	assert.Equal(t, ConfigSecret("CustomSeed"), s.Seed)
}

func TestServerValidate_OptionalFieldsStayEmpty(t *testing.T) {
	t.Parallel()
	s := Server{}
	s.Validate()

	assert.Equal(t, ConfigString(""), s.MudName, "MudName should not get a default")
	assert.Equal(t, ConfigString(""), s.AdminName, "AdminName should stay empty when unset")
	assert.Equal(t, ConfigString(""), s.AdminEmail, "AdminEmail should stay empty when unset")
}
