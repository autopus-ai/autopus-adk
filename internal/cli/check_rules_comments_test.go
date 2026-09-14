package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func commentAwareGoFixture(codeLines, commentLines int) string {
	return strings.Repeat("// documentation\n", commentLines) + "package example\n" +
		strings.Repeat("var _ = 1 // inline comment remains a code line\n", codeLines-1)
}

func TestCheckArchExcludesCommentOnlyLinesAtBoundary(t *testing.T) {
	root := t.TempDir()
	writeFileSizePolicy(t, root, 300)
	writeTestFile(t, root, "boundary.go", commentAwareGoFixture(300, 400))
	var out bytes.Buffer
	assert.True(t, checkArch(root, &out, true, false), out.String())
	writeTestFile(t, root, "boundary.go", commentAwareGoFixture(301, 400))
	out.Reset()
	assert.False(t, checkArch(root, &out, true, false))
	assert.Contains(t, out.String(), "301")
}

func TestCheckArchCommentPolicyUsesIndexRatherThanWorktree(t *testing.T) {
	root := t.TempDir()
	writeFileSizePolicy(t, root, 300)
	initTestGitRepo(t, root)
	writeTestFile(t, root, "index.go", commentAwareGoFixture(300, 400))
	runGitCommand(t, root, "add", "index.go")
	writeTestFile(t, root, "index.go", commentAwareGoFixture(301, 0))
	var staged, working bytes.Buffer
	assert.True(t, checkArch(root, &staged, true, true), staged.String())
	assert.False(t, checkArch(root, &working, true, false))

	runGitCommand(t, root, "add", "index.go")
	writeTestFile(t, root, "index.go", commentAwareGoFixture(300, 400))
	staged.Reset()
	working.Reset()
	assert.False(t, checkArch(root, &staged, true, true))
	assert.True(t, checkArch(root, &working, true, false), working.String())
}

func TestCountLinesKeepsBlankAndImportLinesAboveWarnThreshold(t *testing.T) {
	root := t.TempDir()
	body := "// package docs\npackage p\n\nimport \"fmt\" // import\n" +
		strings.Repeat("// filler\n", warnLineLimit) + "var _ = fmt.Sprint()\n"
	path := writeTestFile(t, root, "count.go", body)
	count, err := countLines(path)
	require.NoError(t, err)
	assert.Equal(t, 4, count)
}

func TestCountLinesDoesNotDiscardCommentMarkersInStrings(t *testing.T) {
	root := t.TempDir()
	body := "package p\nvar text = `\n" +
		strings.Repeat("// not a comment\n/* not a comment */\n", warnLineLimit) + "`\n"
	path := writeTestFile(t, root, "strings.go", body)
	count, err := countLines(path)
	require.NoError(t, err)
	assert.Equal(t, 3+2*warnLineLimit, count)
}

func TestCountLinesUnderWarnThresholdSkipsLexing(t *testing.T) {
	root := t.TempDir()
	path := writeTestFile(t, root, "small.unknownext", strings.Repeat("x\n", 10))
	count, err := countLines(path)
	require.NoError(t, err)
	assert.Equal(t, 10, count)
}
