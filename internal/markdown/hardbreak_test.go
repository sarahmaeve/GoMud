package markdown

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHardBreak_PreservesColumnarLayout verifies that the markdown parser
// preserves line breaks in columnar content when lines end with two trailing
// spaces (the standard markdown hard break syntax).
//
// This is a regression test for issue #82 where help command output ran
// together on one line because parseParagraphNodes() joined lines with spaces.
func TestHardBreak_PreservesColumnarLayout(t *testing.T) {
	SetFormatter(ANSITags{})
	defer SetFormatter(ReMarkdown{})

	tests := []struct {
		name           string
		input          string
		mustContainAll []string // substrings that must appear in output
		mustNotContain []string // substrings that must NOT appear
		minNewlines    int      // minimum number of \n in rendered output
	}{
		{
			name:  "hard break between header and content",
			input: "**Header**  \n    item1 item2 item3",
			// Header and items must be on separate lines, not joined
			mustNotContain: []string{"Header     item1"},
			minNewlines:    1,
		},
		{
			name:  "hard break between rows of items",
			input: "    ~cmd1~ ~cmd2~ ~cmd3~ ~cmd4~  \n    ~cmd5~ ~cmd6~",
			// Row 1 and row 2 must be on separate lines
			mustNotContain: []string{"cmd4~ ~cmd5"},
			minNewlines:    1,
		},
		{
			name: "header then two rows with hard breaks",
			input: "**Combat**  \n" +
				"    ~attack~ ~cast~ ~flee~ ~shoot~  \n" +
				"    ~consider~ ~break~",
			// Each section on its own line
			mustNotContain: []string{
				"Combat     ~",     // header merged with first row
				"shoot~ ~consider", // rows merged together
			},
			minNewlines: 2,
		},
		{
			name:  "no hard break — lines joined (baseline)",
			input: "line one\nline two",
			// Without trailing spaces, lines SHOULD be joined (standard markdown)
			mustContainAll: []string{"line one line two"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(tt.input)
			ast := parser.Parse()
			output := ast.String(0)

			for _, s := range tt.mustContainAll {
				assert.Contains(t, output, s, "output must contain %q", s)
			}
			for _, s := range tt.mustNotContain {
				assert.NotContains(t, output, s, "output must NOT contain %q", s)
			}
			if tt.minNewlines > 0 {
				// Count newlines in the rendered body (strip the outer wrapper newlines)
				body := strings.TrimSpace(output)
				nlCount := strings.Count(body, "\n")
				require.GreaterOrEqual(t, nlCount, tt.minNewlines,
					"expected at least %d newline(s) in output, got %d.\nOutput: %q",
					tt.minNewlines, nlCount, body)
			}
		})
	}
}

// TestHardBreak_MultipleConsecutiveRows verifies that a sequence of
// hard-break-terminated lines produces the correct number of line breaks,
// simulating the help command's 4-commands-per-row layout.
func TestHardBreak_MultipleConsecutiveRows(t *testing.T) {
	SetFormatter(ANSITags{})
	defer SetFormatter(ReMarkdown{})

	// Simulate 3 rows of 4 items each, separated by hard breaks
	input := "    row1a row1b row1c row1d  \n" +
		"    row2a row2b row2c row2d  \n" +
		"    row3a row3b row3c row3d"

	parser := NewParser(input)
	ast := parser.Parse()
	output := ast.String(0)

	// Should have at least 2 newlines (between 3 rows)
	body := strings.TrimSpace(output)
	nlCount := strings.Count(body, "\n")
	assert.GreaterOrEqual(t, nlCount, 2,
		"expected at least 2 newlines for 3 rows, got %d.\nOutput: %q", nlCount, body)

	// Each row's content should be intact (not merged with adjacent rows)
	assert.Contains(t, output, "row1d")
	assert.Contains(t, output, "row2a")
	assert.NotContains(t, output, "row1d row2a",
		"rows should be separated by newline, not joined with space")
}
