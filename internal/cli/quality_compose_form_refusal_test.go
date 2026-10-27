package cli

// SPEC-MODELUX-002 refusal paths: an explicit OMP profile keeps its own
// models (OMP Tab Contract), the per-tool writer refuses what it never
// writes, a plan that cannot read the file writes nothing, and a detected
// platform the catalog does not know is reported instead of generated.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// saveQualityToolPreset refuses a preset name outside the vocabulary, a
// tool without a provider key, and an empty tier map, and writes nothing.
func TestSaveQualityToolPreset_RefusesWhatItNeverWrites(t *testing.T) {
	dir := writeComposeFixture(t, "claude-code", "codex")
	before := composeSessionHash(t, dir)
	cfg := composeBindLoad(t, dir)
	agents := map[string]string{"planner": "opus"}

	assert.Error(t, saveQualityToolPreset(dir, cfg, composeToolNamed(t, "codex"), "bad name", agents))
	assert.Error(t, saveQualityToolPreset(dir, cfg, composeToolNamed(t, "omp"), "omp-custom", agents))
	assert.Error(t, saveQualityToolPreset(dir, cfg, composeToolNamed(t, "codex"), "codex-custom", nil))
	assert.Equal(t, before, composeSessionHash(t, dir))
}

// A tool tab whose plan cannot read autopus.yaml for the rewrite notice
// returns the error and plans no write.
func TestComposeToolPlanner_UnreadableFilePlansNoWrite(t *testing.T) {
	dir := writeComposeFixture(t, "claude-code", "codex")
	run := newComposeToolRun(t, dir, composeSessionFile)
	planner := composeToolPlanner{run: run, tool: composeToolNamed(t, "codex")}
	path := filepath.Join(dir, "autopus.yaml")
	require.NoError(t, os.Chmod(path, 0o000))
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	plan, err := planner.PlanEdit(composeS5Edit(planner.Start()))

	require.Error(t, err)
	assert.Nil(t, plan.Apply)
}

// A detected platform the catalog does not know is reported as an error
// and leaves the config as it was.
func TestUpdateDetectedHarnessPlatformsWith_UnknownPlatformIsReported(t *testing.T) {
	dir := writeComposeFixture(t, "claude-code")
	cfg := composeBindLoad(t, dir)
	var out bytes.Buffer

	current, updated, codexUpdated, errs := updateDetectedHarnessPlatformsWith(
		context.Background(), dir, cfg, []string{"no-such-tool"}, globalFlags{}, &out, composeTableSnapshot(t, "CAT-A"),
	)

	assert.Same(t, cfg, current)
	assert.Zero(t, updated)
	assert.False(t, codexUpdated)
	assert.Equal(t, []string{"no-such-tool: unsupported platform"}, errs)
}
