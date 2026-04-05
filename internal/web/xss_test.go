package web

// TestXSSEscaping verifies that the web package uses html/template, which
// auto-escapes user-controlled data before writing it into HTML output.
// If this file is accidentally changed to import "text/template", the test
// will fail because text/template does NOT escape HTML special characters.

import (
	"bytes"
	"html/template"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestXSSEscaping(t *testing.T) {
	t.Parallel()

	tmpl, err := template.New("test").Parse("<p>{{.}}</p>")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, "<script>alert(1)</script>")
	require.NoError(t, err)

	output := buf.String()
	// html/template escapes < and > to &lt; and &gt;
	require.NotContains(t, output, "<script>", "html/template should escape raw <script> tags")
	require.Contains(t, output, "&lt;script&gt;", "output should contain HTML-escaped content")
}

func TestXSSEscapingAttributes(t *testing.T) {
	t.Parallel()

	tmpl, err := template.New("test").Parse(`<input value="{{.}}">`)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, `"><script>alert(1)</script>`)
	require.NoError(t, err)

	output := buf.String()
	require.NotContains(t, output, "<script>", "html/template should escape attribute injection attempts")
}

func TestXSSEscapingAmpersand(t *testing.T) {
	t.Parallel()

	tmpl, err := template.New("test").Parse("<p>{{.}}</p>")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, "bread & butter")
	require.NoError(t, err)

	output := buf.String()
	require.Contains(t, output, "&amp;", "html/template should escape & to &amp;")
	require.NotContains(t, output, " & ", "unescaped ampersand should not appear in output")
}
