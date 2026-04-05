package flags

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/GoMudEngine/GoMud/internal/mudlog"
)

// ParsedFlags carries the results of command-line flag parsing that
// the main package needs to act on. Existing flags (-version,
// -port-search) are handled internally and cause the process to exit;
// the fields below are for flags that feed into normal startup.
type ParsedFlags struct {
	// ConfigPath is the path to the main config.yaml file. If empty,
	// the caller should use the default path (data-dir/config.yaml).
	// Absolute or relative; relative paths resolve against the CWD
	// the operator launched the binary from.
	ConfigPath string

	// DataDir is the base data directory that contains world content,
	// localize/, db/, sample-scripts/, etc. If empty, the caller
	// should derive it from ConfigPath's parent directory or fall back
	// to the project-root default "_datafiles". Used to resolve
	// relative paths in the config file.
	DataDir string

	// InitDB is true when --init-db was passed. The server should
	// create the persistence database file if it does not exist.
	InitDB bool

	// CreateAdminUsername and CreateAdminPassword are populated when
	// --create-admin <username>:<password> was passed. Both fields
	// are set together or neither is set. Admin creation requires
	// InitDB to also be true; HandleFlags enforces this.
	CreateAdminUsername string
	CreateAdminPassword string
}

// HasCreateAdmin reports whether --create-admin was supplied with a
// non-empty username and password.
func (p ParsedFlags) HasCreateAdmin() bool {
	return p.CreateAdminUsername != "" && p.CreateAdminPassword != ""
}

func HandleFlags(serverVersion string) ParsedFlags {

	var portsearch string
	var showVersion bool
	var initDB bool
	var createAdmin string
	var configPath string
	var dataDir string

	flag.StringVar(&portsearch, "port-search", "", "Search for the first 10 open ports: -port-search=30000-40000")
	flag.BoolVar(&showVersion, "version", false, "Display the current binary version")
	flag.StringVar(&configPath, "config", "", "Path to the main config file. Default: <data-dir>/config.yaml or ./_datafiles/config.yaml")
	flag.StringVar(&dataDir, "data-dir", "", "Base data directory. Contains world content, localize/, db/, sample-scripts/. Default: ./_datafiles")
	flag.BoolVar(&initDB, "init-db", false, "Initialize a fresh persistence database if one does not exist at the configured path")
	flag.StringVar(&createAdmin, "create-admin", "", "Create an admin account on --init-db. Format: username:password")

	flag.Parse()

	if showVersion {
		fmt.Println(serverVersion)
		os.Exit(0)
	}

	if portsearch != `` {
		doPortSearch(portsearch)
		os.Exit(0)
	}

	parsed := ParsedFlags{
		ConfigPath: configPath,
		DataDir:    dataDir,
		InitDB:     initDB,
	}

	if createAdmin != "" {
		if !initDB {
			fmt.Fprintln(os.Stderr, "--create-admin requires --init-db")
			os.Exit(2)
		}
		username, password, err := parseCreateAdmin(createAdmin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "--create-admin: %v\n", err)
			os.Exit(2)
		}
		parsed.CreateAdminUsername = username
		parsed.CreateAdminPassword = password
	}

	return parsed
}

// parseCreateAdmin splits a "username:password" flag value. The password
// may contain colons — only the first colon is treated as the separator.
// Returns an error if either side is empty.
func parseCreateAdmin(value string) (username, password string, err error) {
	idx := strings.Index(value, ":")
	if idx < 1 {
		return "", "", errors.New("must be in the form username:password (username cannot be empty)")
	}
	if idx == len(value)-1 {
		return "", "", errors.New("must be in the form username:password (password cannot be empty)")
	}
	return value[:idx], value[idx+1:], nil
}

func doPortSearch(portRangeStr string) {
	portRange := strings.Split(portRangeStr, `-`)

	if len(portRange) < 2 {
		mudlog.Error("-port-search", "error", "Invalid port range specified")
		return
	}

	portRangeStart, _ := strconv.Atoi(portRange[0])
	portRangeEnd, _ := strconv.Atoi(portRange[1])

	if portRangeStart == 0 || portRangeEnd == 0 || portRangeStart >= portRangeEnd {
		mudlog.Error("-port-search", "error", "Invalid port range specified")
		return
	}

	mudlog.Info("-port-search", "message", fmt.Sprintf("Searching for first 10 available ports between %d and %d", portRangeStart, portRangeEnd))

	foundPorts := 0
	for i := portRangeStart; i < portRangeEnd; i++ {

		if !isPortInUse(i) {
			mudlog.Info("-port-search", "message", "Found port", "port", i)
			foundPorts++
		}
		if foundPorts >= 10 {
			break
		}
	}

	mudlog.Info("-port-search", "message", fmt.Sprintf("Found %d available ports", foundPorts))

}

func isPortInUse(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return true
	}
	ln.Close()
	return false
}
