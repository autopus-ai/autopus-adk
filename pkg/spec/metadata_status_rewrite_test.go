package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Frontmatter is the authoritative carrier when present: the status line is
// rewritten in place, keeping its indentation and every sibling key.
func TestRewriteSpecStatus_ReplacesFrontmatterStatusInPlace(t *testing.T) {
	t.Parallel()

	content := "---\nid: SPEC-X-001\nstatus: draft\nowner: me\n---\n\n# Title\n"

	updated, err := rewriteSpecStatus(content, "approved")

	require.NoError(t, err)
	assert.Contains(t, updated, "status: approved")
	assert.NotContains(t, updated, "status: draft")
	assert.Contains(t, updated, "id: SPEC-X-001")
	assert.Contains(t, updated, "owner: me")
}

// A frontmatter block without a status key gains one inside the block; writing
// it after the closing delimiter would leave the field invisible to parsers.
func TestRewriteSpecStatus_InjectsStatusInsideFrontmatter(t *testing.T) {
	t.Parallel()

	content := "---\nid: SPEC-X-002\n---\n\n# Title\n"

	updated, err := rewriteSpecStatus(content, "implemented")

	require.NoError(t, err)
	lines := strings.Split(updated, "\n")
	closing := -1
	for index, line := range lines {
		if index > 0 && strings.TrimSpace(line) == "---" {
			closing = index
			break
		}
	}
	require.Positive(t, closing, "closing frontmatter delimiter missing: %q", updated)
	assert.Contains(t, lines[:closing], "status: implemented")
}

// Legacy documents carry `**Status**:` prose instead of frontmatter. That line
// is rewritten rather than duplicated, and its indentation survives.
func TestRewriteSpecStatus_RewritesLegacyStatusLine(t *testing.T) {
	t.Parallel()

	content := "# Title\n\n  **Status**: draft\n\n## Body\n"

	updated, err := rewriteSpecStatus(content, "completed")

	require.NoError(t, err)
	assert.Contains(t, updated, "  **Status**: completed")
	assert.Equal(t, 1, strings.Count(updated, "**Status**"))
}

// With neither carrier, the status is inserted after the title and separated by
// blank lines, so the heading and the following prose stay distinct blocks.
func TestRewriteSpecStatus_InsertsAfterHeadingWithBlankSeparation(t *testing.T) {
	t.Parallel()

	content := "# Title\nSome prose immediately after the heading.\n"

	updated, err := rewriteSpecStatus(content, "draft")

	require.NoError(t, err)
	assert.Equal(t, []string{
		"# Title",
		"",
		"**Status**: draft",
		"",
		"Some prose immediately after the heading.",
		"",
	}, strings.Split(updated, "\n"))
}

// A document with no heading has nowhere to anchor the field; guessing a
// location would silently bury the status where no reader looks for it.
func TestRewriteSpecStatus_RejectsDocumentWithoutAnchor(t *testing.T) {
	t.Parallel()

	_, err := rewriteSpecStatus("plain prose with no heading\n", "draft")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "status")
}

func TestUpdateStatus_NormalizesInputAndRejectsBlank(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "spec.md")
	require.NoError(t, os.WriteFile(path, []byte("---\nstatus: draft\n---\n"), 0o644))

	require.NoError(t, UpdateStatus(dir, "  APPROVED  "))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "status: approved")

	require.Error(t, UpdateStatus(dir, "   "))
}

// A missing spec.md must name the read failure instead of creating the file:
// writing a fresh document would fabricate a SPEC that was never authored.
func TestUpdateStatus_ReportsMissingSpecDocument(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	err := UpdateStatus(dir, "draft")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "read spec.md")
	_, statErr := os.Stat(filepath.Join(dir, "spec.md"))
	assert.True(t, os.IsNotExist(statErr))
}
