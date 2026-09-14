package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeCoverageProject(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	content := "mode: full\nproject_name: test\nplatforms:\n  - claude-code\n" + body
	require.NoError(t, os.WriteFile(filepath.Join(dir, "autopus.yaml"), []byte(content), 0o644))
	return dir
}

func TestPipelineCoverageThreshold_DeclaredValueReachesRun(t *testing.T) {
	t.Parallel()

	got := pipelineCoverageThreshold(writeCoverageProject(t, "workflow:\n  coverage_threshold: 70\n"))

	assert.Equal(t, 70.0, got)
}

func TestPipelineCoverageThreshold_UndeclaredUsesFloor(t *testing.T) {
	t.Parallel()

	got := pipelineCoverageThreshold(writeCoverageProject(t, ""))

	assert.Equal(t, 85.0, got)
}

// Zero is the project's opt-out and must survive the resolver, otherwise the
// fallback would re-impose a floor the project explicitly removed.
func TestPipelineCoverageThreshold_ExplicitZeroStaysOff(t *testing.T) {
	t.Parallel()

	got := pipelineCoverageThreshold(writeCoverageProject(t, "workflow:\n  coverage_threshold: 0\n"))

	assert.Equal(t, 0.0, got)
}

// An unreadable config must not silently drop the gate: "could not tell" is
// not the same statement as "no gate".
func TestPipelineCoverageThreshold_UnreadableConfigKeepsFloor(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "autopus.yaml"),
		[]byte("mode: [unterminated\n"),
		0o644,
	))

	got := pipelineCoverageThreshold(dir)

	assert.Equal(t, 85.0, got)
}
