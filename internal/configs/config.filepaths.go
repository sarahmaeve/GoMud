package configs

import (
	"path/filepath"
)

type FilePaths struct {
	WebDomain        ConfigString `yaml:"WebDomain"`
	WebCDNLocation   ConfigString `yaml:"WebCDNLocation"`
	DataFiles        ConfigString `yaml:"DataFiles"`
	PublicHtml       ConfigString `yaml:"PublicHtml"`
	AdminHtml        ConfigString `yaml:"AdminHtml"`
	HttpsCertFile    ConfigString `yaml:"HttpsCertFile"`
	HttpsKeyFile     ConfigString `yaml:"HttpsKeyFile"`
	CarefulSaveFiles ConfigBool   `yaml:"CarefulSaveFiles"`

	// DatabasePath is the filesystem path to the SQLite persistence
	// database. Relative paths resolve against the data directory
	// base (see SetDataDir). Defaults to db/default_mud.db under
	// the data directory. The file and its parent directory are
	// created by the --init-db flag on first run.
	DatabasePath ConfigString `yaml:"DatabasePath"`

	// SampleScripts is the path to sample script templates used by
	// OLC commands when creating new mobs and spells. Relative paths
	// resolve against the data directory base.
	SampleScripts ConfigString `yaml:"SampleScripts"`

	// LocalizePath is the path to the localize/ directory. Relative
	// paths resolve against the data directory base.
	LocalizePath ConfigString `yaml:"LocalizePath"`
}

func (f *FilePaths) Validate() {

	// Ignore WebDomain
	// Ignore WebCDNLocation
	// Ignore PublicHtml
	// Ignore AdminHtml
	// Ignore CarefulSaveFiles

	if f.DataFiles == `` {
		f.DataFiles = `world/default` // default under data dir
	}

	if f.DatabasePath == `` {
		f.DatabasePath = `db/default_mud.db` // default under data dir
	}

	if f.SampleScripts == `` {
		f.SampleScripts = `sample-scripts` // default under data dir
	}

	if f.LocalizePath == `` {
		f.LocalizePath = `localize` // default under data dir
	}
}

// Resolve returns the given path joined with the configured data
// directory base if it is relative, or the path as-is if absolute.
// Use this helper whenever reading a FilePaths field that may be a
// relative path.
func Resolve(path string) string {
	if path == `` {
		return ``
	}
	if filepath.IsAbs(path) {
		return path
	}
	configDataLock.RLock()
	base := dataDirBase
	configDataLock.RUnlock()
	return filepath.Join(base, path)
}

// GetFilePathsConfig returns the FilePaths config with all relative
// path fields resolved against the data directory base. Absolute
// paths are returned unchanged.
//
// Callers MUST NOT assume the returned FilePaths is identical to
// the raw config — use this getter rather than configData.FilePaths
// directly so that relative paths are consistently resolved.
func GetFilePathsConfig() FilePaths {
	configDataLock.RLock()
	if !configData.validated {
		configDataLock.RUnlock()
		// Validate requires the write lock.
		configDataLock.Lock()
		if !configData.validated {
			configData.Validate()
		}
		configDataLock.Unlock()
		configDataLock.RLock()
	}
	base := dataDirBase
	fp := configData.FilePaths
	configDataLock.RUnlock()

	// Resolve relative paths against the data dir base.
	resolve := func(p ConfigString) ConfigString {
		s := string(p)
		if s == `` || filepath.IsAbs(s) {
			return p
		}
		return ConfigString(filepath.Join(base, s))
	}
	fp.DataFiles = resolve(fp.DataFiles)
	fp.DatabasePath = resolve(fp.DatabasePath)
	fp.SampleScripts = resolve(fp.SampleScripts)
	fp.LocalizePath = resolve(fp.LocalizePath)
	// PublicHtml, AdminHtml, HttpsCertFile, HttpsKeyFile are
	// operator-configured and left as-is. If an operator wants them
	// relative to data dir, they can set them relative in the
	// config and the same resolve logic would apply — but this
	// preserves backward compatibility for operators who set them
	// to absolute paths or paths relative to the CWD.
	return fp
}
