package plugins

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestPlugin creates a Plugin and resets the global registry afterwards so
// tests do not pollute one another.
func newTestPlugin(t *testing.T, name, version string) *Plugin {
	t.Helper()

	// Ensure registration is open for the duration of plugin creation.
	origOpen := registrationOpen
	registrationOpen = true

	origRegistry := registry
	registry = pluginRegistry{}

	p := New(name, version)
	require.NotNil(t, p, "New() returned nil — registrationOpen was false")

	t.Cleanup(func() {
		registry = origRegistry
		registrationOpen = origOpen
	})

	return p
}

// TestPlugin_ReadIntoStruct_ValidYAML verifies that ReadIntoStruct returns nil
// when the stored bytes contain well-formed YAML that maps onto the target type.
func TestPlugin_ReadIntoStruct_ValidYAML(t *testing.T) {
	// Cannot run in parallel: both tests mutate package-level writeFolderPath.
	tmp := t.TempDir()

	origPath := writeFolderPath
	writeFolderPath = tmp
	t.Cleanup(func() { writeFolderPath = origPath })

	p := newTestPlugin(t, "testplugin", "1.0.0")

	type Config struct {
		Name  string `yaml:"name"`
		Value int    `yaml:"value"`
	}

	validYAML := []byte("name: hello\nvalue: 42\n")
	require.NoError(t, p.WriteBytes("config", validYAML))

	var out Config
	err := p.ReadIntoStruct("config", &out)
	assert.NoError(t, err, "ReadIntoStruct should return nil for valid YAML")
	assert.Equal(t, "hello", out.Name)
	assert.Equal(t, 42, out.Value)
}

// TestPlugin_ReadIntoStruct_InvalidYAML verifies that ReadIntoStruct returns a
// non-nil error when the stored bytes are malformed YAML.
func TestPlugin_ReadIntoStruct_InvalidYAML(t *testing.T) {
	// Cannot run in parallel: both tests mutate package-level writeFolderPath.
	tmp := t.TempDir()

	origPath := writeFolderPath
	writeFolderPath = tmp
	t.Cleanup(func() { writeFolderPath = origPath })

	p := newTestPlugin(t, "testplugin2", "1.0.0")

	// Tabs are illegal in YAML block scalars; this forces a parse error.
	malformedYAML := []byte("key:\n\t- bad indentation with tab\n")
	require.NoError(t, p.WriteBytes("config", malformedYAML))

	var out map[string]any
	err := p.ReadIntoStruct("config", &out)
	assert.Error(t, err, "ReadIntoStruct should return a non-nil error for malformed YAML")
}
