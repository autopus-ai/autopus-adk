package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const reactSampleReport = `# CI Failure Report

- **Run ID**: 909
- **Name**: ci
- **Branch**: main
- **Conclusion**: failure
- **Updated**: 2026-01-01T00:00:00Z
- **Generated**: 2026-01-01T00:00:01Z

## Failure Logs

` + "```" + `
boom
` + "```" + `
`

// seedReactReport chdirs into a scratch dir holding one react report.
func seedReactReport(t *testing.T, runID string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".autopus", "react"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".autopus", "react", runID+".md"), []byte(reactSampleReport), 0o644))
	t.Chdir(dir)
	return dir
}

// stdinScript feeds a canned answer to the confirmation prompt.
func stdinScript(t *testing.T, answer string) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stdin")
	require.NoError(t, err)
	_, err = f.WriteString(answer)
	require.NoError(t, err)
	_, err = f.Seek(0, 0)
	require.NoError(t, err)

	orig := os.Stdin
	os.Stdin = f
	t.Cleanup(func() {
		os.Stdin = orig
		_ = f.Close()
	})
}

// Guards against a non-numeric argument reaching the filesystem as a path fragment.
func TestRunReactApply_RejectsNonNumericRunID(t *testing.T) {
	for _, id := range []string{"abc", "../escape", ""} {
		t.Run("rejects "+id, func(t *testing.T) {
			var out bytes.Buffer
			err := runReactApply(reactCheckCmd(&out), id, true)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "must be a numeric ID")
			assert.Empty(t, out.String())
		})
	}
}

// A missing report must point the user at the command that produces it.
func TestRunReactApply_MissingReportPointsAtCheck(t *testing.T) {
	t.Chdir(t.TempDir())

	var out bytes.Buffer
	err := runReactApply(reactCheckCmd(&out), "777", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), filepath.Join(".autopus/react", "777.md"))
	assert.Contains(t, err.Error(), "auto react check")
}

// --force must skip the confirmation prompt and hand off without consuming stdin.
func TestRunReactApply_ForcePrintsSummaryAndSkipsPrompt(t *testing.T) {
	seedReactReport(t, "909")
	stdinScript(t, "n\n")

	var out bytes.Buffer
	require.NoError(t, runReactApply(reactCheckCmd(&out), "909", true))

	text := out.String()
	assert.Contains(t, text, "Report for run 909:")
	assert.Contains(t, text, "- **Run ID**: 909")
	assert.NotContains(t, text, "Proceed with applying fix?")
	assert.NotContains(t, text, "Aborted.")
	assert.Contains(t, text, "Delegate to debugger agent")
}

// Declining the prompt must abort before any repository mutation or handoff message.
func TestRunReactApply_DeclinedPromptAborts(t *testing.T) {
	seedReactReport(t, "909")
	stdinScript(t, "n\n")

	var out bytes.Buffer
	require.NoError(t, runReactApply(reactCheckCmd(&out), "909", false))

	text := out.String()
	assert.Contains(t, text, "Proceed with applying fix?")
	assert.Contains(t, text, "Aborted.")
	assert.NotContains(t, text, "Delegate to debugger agent", "abort must not continue to the fix handoff")
	assert.NotContains(t, text, "stashed")
}

// Accepting the prompt must continue into the stash/handoff sequence.
func TestRunReactApply_AcceptedPromptContinuesToHandoff(t *testing.T) {
	seedReactReport(t, "909")
	stdinScript(t, "YES\n")

	var out bytes.Buffer
	require.NoError(t, runReactApply(reactCheckCmd(&out), "909", false))

	text := out.String()
	assert.NotContains(t, text, "Aborted.")
	assert.Contains(t, text, "Delegate to debugger agent")
	// Scratch dir is not a repository, so stash must degrade to a warning.
	assert.Contains(t, text, "git stash failed")
}

// The printed summary must keep only report headers and stay bounded.
func TestExtractReportSummary_KeepsHeadersAndCaps(t *testing.T) {
	t.Parallel()

	summary := extractReportSummary(reactSampleReport)
	lines := strings.Split(summary, "\n")

	assert.LessOrEqual(t, len(lines), 6, "summary must stay bounded")
	assert.Equal(t, "# CI Failure Report", lines[0])
	for _, line := range lines[1:] {
		assert.True(t, strings.HasPrefix(line, "- **"), "unexpected line kept: %q", line)
	}
	assert.NotContains(t, summary, "boom", "log body must not leak into the summary")
	assert.NotContains(t, summary, "## Failure Logs")
}

// Reports without header lines must yield an empty summary rather than echoing the body.
func TestExtractReportSummary_NoHeadersYieldsEmpty(t *testing.T) {
	t.Parallel()

	assert.Empty(t, extractReportSummary("plain text\nanother line\n"))
}
