package configs

// Test helpers for cross-package testing.
// These functions allow other internal packages to configure
// settings for testing without filesystem dependencies.
// Since this is under internal/, it cannot be used outside the module.

// SetTestValidation sets password and name validation config directly.
// For testing only.
func SetTestValidation(v Validation) {
	configDataLock.Lock()
	defer configDataLock.Unlock()
	configData.Validation = v
	configData.Validation.Validate()
	configData.validated = true
}

// SetTestDataFilesPath sets the data files path for testing.
// For testing only.
func SetTestDataFilesPath(path string) {
	configDataLock.Lock()
	defer configDataLock.Unlock()
	configData.FilePaths.DataFiles = ConfigString(path)
}
