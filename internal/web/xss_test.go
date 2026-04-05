package web

// These tests verify that the web package uses html/template (which
// auto-escapes user-controlled data) rather than text/template (which
// does not). They work by scanning the actual source files in this
// package for the forbidden import — this is a direct regression guard
// that fails if any web package file switches back to text/template.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestWebPackageUsesHtmlTemplate scans every non-test .go file in the web
// package and verifies it does NOT import "text/template". This is the
// actual XSS regression guard.
func TestWebPackageUsesHtmlTemplate(t *testing.T) {
	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	var scanned int
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		if strings.HasSuffix(name, "_test.go") {
			continue
		}

		path := filepath.Join(".", name)
		data, err := os.ReadFile(path)
		require.NoError(t, err, "reading %s", path)

		content := string(data)
		require.NotContains(t, content, `"text/template"`,
			"%s imports text/template — this is an XSS vulnerability. Use html/template instead.", name)

		// We also want at least SOME file in the package to import html/template,
		// otherwise the package isn't using templates at all and the guard is meaningless.
		scanned++
	}

	require.Greater(t, scanned, 0, "no .go source files found in web package")
}

// TestWebPackageImportsHtmlTemplate verifies at least one source file
// actually imports html/template, ensuring the regression guard above
// has something to guard against.
func TestWebPackageImportsHtmlTemplate(t *testing.T) {
	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	var foundHtmlTemplate bool
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(".", name))
		require.NoError(t, err)
		if strings.Contains(string(data), `"html/template"`) {
			foundHtmlTemplate = true
			break
		}
	}

	require.True(t, foundHtmlTemplate, "no web package file imports html/template")
}
