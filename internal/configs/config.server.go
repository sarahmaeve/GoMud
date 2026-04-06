package configs

type Server struct {
	MudName         ConfigString      `yaml:"MudName"`         // Name of the MUD
	Tagline         ConfigString      `yaml:"Tagline"`         // Short tagline shown on login splash
	Description     ConfigString      `yaml:"Description"`     // Longer description for about page
	URL             ConfigString      `yaml:"URL"`             // Project or server URL
	DiscordURL      ConfigString      `yaml:"DiscordURL"`      // Community Discord invite link
	AdminName       ConfigString      `yaml:"AdminName"`       // Server operator name (optional)
	AdminEmail      ConfigString      `yaml:"AdminEmail"`      // Contact email (optional)
	CurrentVersion  ConfigString      `yaml:"CurrentVersion"`  // Current version this mud has been updated to
	Seed            ConfigSecret      `yaml:"Seed"`            // Seed that may be used for generating content
	MaxCPUCores     ConfigInt         `yaml:"MaxCPUCores"`     // How many cores to allow for multi-core operations
	OnLoginCommands ConfigSliceString `yaml:"OnLoginCommands"` // Commands to run when a user logs in
	Motd            ConfigString      `yaml:"Motd"`            // Message of the day to display when a user logs in
	NextRoomId      ConfigInt         `yaml:"NextRoomId"`      // The next room id to use when creating a new room
	Locked          ConfigSliceString `yaml:"Locked"`          // List of locked config properties that cannot be changed without editing the file directly.
}

func (s *Server) Validate() {

	// Ignore MudName
	// Ignore OnLoginCommands
	// Ignore Motd
	// Ignore NextRoomId
	// Ignore Locked

	if s.Tagline == `` {
		s.Tagline = `an open source MUD Library written in Go`
	}

	if s.Description == `` {
		s.Description = `GoMud is an open source MUD (Multi-user Dungeon) game world and library.`
	}

	if s.URL == `` {
		s.URL = `github.com/GoMudEngine/GoMud`
	}

	if s.DiscordURL == `` {
		s.DiscordURL = `discord.gg/cjukKvQWyy`
	}

	// Ignore AdminName
	// Ignore AdminEmail

	if s.Seed == `` {
		s.Seed = `Mud` // default
	}

	if s.MaxCPUCores < 0 {
		s.MaxCPUCores = 0 // default
	}

	if s.CurrentVersion == `` {
		s.CurrentVersion = `0.9.0` // If no version found, failover to a known version
	}

}

func GetServerConfig() Server {
	configDataLock.RLock()
	defer configDataLock.RUnlock()

	if !configData.validated {
		configData.Validate()
	}
	return configData.Server
}
