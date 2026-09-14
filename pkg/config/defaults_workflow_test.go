package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const workflowTestBase = `
mode: full
project_name: test
platforms:
  - claude-code
`

func writeWorkflowConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "autopus.yaml"),
		[]byte(workflowTestBase+body),
		0o644,
	))
	return dir
}

func TestDefaultFullConfig_DeclaresCoverageFloor(t *testing.T) {
	t.Parallel()

	cfg := DefaultFullConfig("test-project")
	require.NotNil(t, cfg)

	assert.Equal(t, DefaultCoverageThreshold, cfg.Workflow.CoverageThreshold)
	assert.Equal(t, 85, DefaultCoverageThreshold)
}

// A project that never mentions workflow is held to the floor, not to zero.
// Zero is the opt-out, so an unstated threshold must not read as one.
func TestLoad_AbsentWorkflowSectionInheritsCoverageFloor(t *testing.T) {
	t.Parallel()

	cfg, err := Load(writeWorkflowConfig(t, ""))

	require.NoError(t, err)
	assert.Equal(t, 85, cfg.Workflow.CoverageThreshold)
}

// A workflow section that omits the key is the same statement as omitting the
// section: the floor still applies. The failure mode this locks is a section
// present for some other reason silently disabling the gate.
func TestLoad_WorkflowSectionWithoutThresholdInheritsCoverageFloor(t *testing.T) {
	t.Parallel()

	cfg, err := Load(writeWorkflowConfig(t, "workflow:\n  team_default: false\n"))

	require.NoError(t, err)
	assert.Equal(t, 85, cfg.Workflow.CoverageThreshold)
}

// An explicit 0 is how a project turns the numeric gate off, so the backfill
// must not overwrite it.
func TestLoad_ExplicitZeroThresholdDisablesCoverageGate(t *testing.T) {
	t.Parallel()

	cfg, err := Load(writeWorkflowConfig(t, "workflow:\n  coverage_threshold: 0\n"))

	require.NoError(t, err)
	assert.Equal(t, 0, cfg.Workflow.CoverageThreshold)
}

func TestLoad_ExplicitThresholdOverridesCoverageFloor(t *testing.T) {
	t.Parallel()

	cfg, err := Load(writeWorkflowConfig(t, "workflow:\n  coverage_threshold: 70\n"))

	require.NoError(t, err)
	assert.Equal(t, 70, cfg.Workflow.CoverageThreshold)
}

// A missing config file resolves through DefaultFullConfig rather than the
// unmarshal path, so it needs its own row.
func TestLoad_MissingConfigFileInheritsCoverageFloor(t *testing.T) {
	t.Parallel()

	cfg, err := Load(t.TempDir())

	require.NoError(t, err)
	assert.Equal(t, 85, cfg.Workflow.CoverageThreshold)
}
