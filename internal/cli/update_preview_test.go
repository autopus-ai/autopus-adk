package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

func TestUpdateCmd_PlanShowsConfigAndCreatesWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	binDir := t.TempDir()
	makeDummyBinary(t, binDir, "opencode")
	t.Setenv("PATH", binDir)

	initCmd := newTestRootCmd()
	initCmd.SetArgs([]string{"init", "--dir", dir, "--project", "preview-proj", "--platforms", "claude-code", "--yes"})
	require.NoError(t, initCmd.Execute())

	beforeConfig, err := os.ReadFile(filepath.Join(dir, "autopus.yaml"))
	require.NoError(t, err)

	var out bytes.Buffer
	updateCmd := newTestRootCmd()
	updateCmd.SetOut(&out)
	updateCmd.SetErr(&out)
	updateCmd.SetArgs([]string{"update", "--dir", dir, "--plan"})
	require.NoError(t, updateCmd.Execute())

	afterConfig, err := os.ReadFile(filepath.Join(dir, "autopus.yaml"))
	require.NoError(t, err)
	assert.Equal(t, string(beforeConfig), string(afterConfig), "preview must not rewrite config")
	assert.NoFileExists(t, filepath.Join(dir, "opencode.json"), "preview must not create new platform files")

	output := out.String()
	assert.Contains(t, output, "autopus.yaml")
	assert.Contains(t, output, "opencode.json")
	assert.Contains(t, output, "[config] update")
	assert.Contains(t, output, "[config] emit opencode.json")
	assert.Contains(t, output, "[generated_surface] emit .opencode/agents/annotator.md")
}

func TestUpdateCmd_PlanShowsSkipAndPreserveWithoutWriting(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	initCmd := newTestRootCmd()
	initCmd.SetArgs([]string{"init", "--dir", dir, "--project", "preview-proj", "--platforms", "claude-code", "--yes"})
	require.NoError(t, initCmd.Execute())

	deletePath, preservePath := selectManagedPreviewTargets(t, dir, "claude-code")
	require.NoError(t, os.Remove(filepath.Join(dir, deletePath)))
	require.NoError(t, os.WriteFile(filepath.Join(dir, preservePath), []byte("user-modified"), 0o644))

	var out bytes.Buffer
	updateCmd := newTestRootCmd()
	updateCmd.SetOut(&out)
	updateCmd.SetErr(&out)
	updateCmd.SetArgs([]string{"update", "--dir", dir, "--plan"})
	require.NoError(t, updateCmd.Execute())

	assert.NoFileExists(t, filepath.Join(dir, deletePath), "preview must not recreate deleted file")
	data, err := os.ReadFile(filepath.Join(dir, preservePath))
	require.NoError(t, err)
	assert.Equal(t, "user-modified", string(data), "preview must not overwrite modified file")

	output := out.String()
	assert.Contains(t, output, "retain "+filepath.ToSlash(deletePath))
}

func TestUpdateCmd_PlanShowsLegacyConfigNormalizationWithoutWriting(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	initCmd := newTestRootCmd()
	initCmd.SetArgs([]string{"init", "--dir", dir, "--project", "preview-proj", "--platforms", "claude-code", "--yes"})
	require.NoError(t, initCmd.Execute())

	configPath := filepath.Join(dir, "autopus.yaml")
	before, err := os.ReadFile(configPath)
	require.NoError(t, err)

	legacy := strings.Replace(string(before), "claude-code", "claude", 1)
	require.NoError(t, os.WriteFile(configPath, []byte(legacy), 0o644))

	var out bytes.Buffer
	updateCmd := newTestRootCmd()
	updateCmd.SetOut(&out)
	updateCmd.SetErr(&out)
	updateCmd.SetArgs([]string{"update", "--dir", dir, "--plan", "--yes"})
	require.NoError(t, updateCmd.Execute())

	after, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Equal(t, legacy, string(after), "preview must not normalize config in place")
	assert.Contains(t, out.String(), "autopus.yaml")
	assert.Contains(t, out.String(), "legacy platform names would be normalized")
}

func TestUpdateCmd_PlanShowsDesignBackfillWithoutWriting(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "autopus.yaml"), []byte(`mode: full
project_name: legacy-design-proj
platforms:
  - claude-code
`), 0o644))

	var out bytes.Buffer
	updateCmd := newTestRootCmd()
	updateCmd.SetOut(&out)
	updateCmd.SetErr(&out)
	updateCmd.SetArgs([]string{"update", "--dir", dir, "--plan", "--yes"})
	require.NoError(t, updateCmd.Execute())

	assert.NoFileExists(t, filepath.Join(dir, "DESIGN.md"), "preview must not create DESIGN.md")
	configData, err := os.ReadFile(filepath.Join(dir, "autopus.yaml"))
	require.NoError(t, err)
	assert.NotContains(t, string(configData), "design:", "preview must not rewrite config")

	output := out.String()
	assert.Contains(t, output, "DESIGN.md")
	assert.Contains(t, output, "starter design context would be created")
	assert.Contains(t, output, "design defaults would be persisted in autopus.yaml")
}

func TestUpdateCmd_PlanShowsStatusLineKeepHint(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	initCmd := newTestRootCmd()
	initCmd.SetArgs([]string{"init", "--dir", dir, "--project", "preview-proj", "--platforms", "claude-code", "--yes"})
	require.NoError(t, initCmd.Execute())

	settingsPath := filepath.Join(dir, ".claude", "settings.json")
	require.NoError(t, os.WriteFile(settingsPath, []byte(`{
  "statusLine": {
    "type": "command",
    "command": "node ~/.claude/hud/omc-hud.mjs",
    "padding": 2
  }
}`), 0o644))

	var out bytes.Buffer
	updateCmd := newTestRootCmd()
	updateCmd.SetOut(&out)
	updateCmd.SetErr(&out)
	updateCmd.SetArgs([]string{"update", "--dir", dir, "--plan", "--yes"})
	require.NoError(t, updateCmd.Execute())

	output := out.String()
	assert.Contains(t, output, ".claude/settings.json")
	assert.Contains(t, output, "statusLine would be preserved")
	assert.Contains(t, output, "--statusline-mode merge or replace")
}

func TestUpdateCmd_PlanShowsSplitCompilerEmitRetainPruneAndChecksumDiff(t *testing.T) {
	dir := t.TempDir()
	configurePreviewBinaries(t, "codex", "opencode")
	initMixedPreviewHarness(t, dir)
	installFullSkillLibrary(t, dir)

	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "metrics", "SKILL.md"), "acceptance S2 baseline: the full library must start with the shared long-tail path before split opt-in")

	cfg, err := loadConfigFromDir(dir)
	require.NoError(t, err)
	cfg.Skills.SharedSurface = config.SharedSurfaceCore
	cfg.Skills.Compiler.Mode = config.SkillCompilerModeSplit
	// metrics is a `product` bundle skill. Split mode relocates a long-tail
	// skill only once a bundle selects it; with nothing selected the compact
	// surface simply drops it and there is no relocation left to preview.
	cfg.Skills.Compiler.Bundles = []string{"product"}
	cfg.Skills.Compiler.OpenCodeLongTailTarget = config.SkillLongTailTargetProject
	cfg.Skills.Compiler.CodexLongTailTarget = config.SkillLongTailTargetPlugin
	require.NoError(t, config.Save(dir, cfg))

	var out bytes.Buffer
	updateCmd := newTestRootCmd()
	updateCmd.SetOut(&out)
	updateCmd.SetErr(&out)
	updateCmd.SetArgs([]string{"update", "--dir", dir, "--plan"})
	require.NoError(t, updateCmd.Execute())

	output := out.String()
	assert.Contains(t, output, "emit .opencode/skills/metrics/SKILL.md", "acceptance S6/S9: split preview must emit the OpenCode long-tail target path")
	assert.Contains(t, output, "emit .autopus/plugins/auto/skills/metrics/SKILL.md", "acceptance S7/S9: split preview must emit the Codex plugin-scoped long-tail target path")
	assert.Contains(t, output, "retain .agents/skills/planning/SKILL.md", "acceptance S3/S9: split preview must retain shared core skills that stay visible")
	assert.Contains(t, output, "prune .agents/skills/metrics/SKILL.md", "acceptance S4/S9: split preview must expose stale shared artifacts that would be pruned after ownership changes")
	assert.Contains(t, output, "checksum diff", "acceptance S9: split preview must expose checksum or manifest diffs before apply")
}

func TestUpdateCmd_PlanKeepsFullCompatibleSharedSkillPathUnderExplicitFullMode(t *testing.T) {
	dir := t.TempDir()
	configurePreviewBinaries(t, "codex", "opencode")
	initMixedPreviewHarness(t, dir)
	installFullSkillLibrary(t, dir)

	var out bytes.Buffer
	updateCmd := newTestRootCmd()
	updateCmd.SetOut(&out)
	updateCmd.SetErr(&out)
	updateCmd.SetArgs([]string{"update", "--dir", dir, "--plan"})
	require.NoError(t, updateCmd.Execute())

	output := out.String()
	assert.Contains(t, output, "retain .agents/skills/metrics/SKILL.md", "acceptance S2: explicit full mode must keep the existing full-compatible shared skill path when split compiler mode is not enabled")
	assert.NotContains(t, output, ".opencode/skills/metrics/SKILL.md", "acceptance S2: full mode must not route shared long-tail skills to split OpenCode targets")
	assert.NotContains(t, output, ".autopus/plugins/auto/skills/metrics/SKILL.md", "acceptance S2: full mode must not route shared long-tail skills to split Codex plugin targets")
}

func TestUpdateCmd_PlanRetainsCodexNativeWorkflowSkills(t *testing.T) {
	dir := t.TempDir()
	configurePreviewBinaries(t, "codex", "opencode")
	initMixedPreviewHarness(t, dir)

	applyCmd := newTestRootCmd()
	applyCmd.SetArgs([]string{"update", "--dir", dir, "--yes"})
	require.NoError(t, applyCmd.Execute())

	var out bytes.Buffer
	updateCmd := newTestRootCmd()
	updateCmd.SetOut(&out)
	updateCmd.SetErr(&out)
	updateCmd.SetArgs([]string{"update", "--dir", dir, "--plan"})
	require.NoError(t, updateCmd.Execute())

	output := out.String()
	for _, name := range []string{
		"auto-dev", "auto-doctor", "auto-goal", "auto-map", "auto-secure",
		"auto-status", "auto-test", "auto-update", "auto-verify", "auto-why",
	} {
		path := ".codex/skills/codex-" + name + "/SKILL.md"
		assert.Contains(t, output, "retain "+path)
		assert.NotContains(t, output, "prune "+path)
	}
}

func selectManagedPreviewTargets(t *testing.T, dir, platform string) (string, string) {
	t.Helper()

	manifest, err := adapter.LoadManifest(dir, platform)
	require.NoError(t, err)
	require.NotNil(t, manifest)

	paths := make([]string, 0, len(manifest.Files))
	for path, meta := range manifest.Files {
		if meta.Policy == adapter.OverwriteMarker {
			continue
		}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	require.GreaterOrEqual(t, len(paths), 2, "need at least two non-marker managed files")

	return paths[0], paths[1]
}

func configurePreviewBinaries(t *testing.T, names ...string) {
	t.Helper()

	binDir := t.TempDir()
	for _, name := range names {
		makeDummyBinary(t, binDir, name)
	}
	t.Setenv("PATH", binDir)
}

func initMixedPreviewHarness(t *testing.T, dir string) {
	t.Helper()

	initCmd := newTestRootCmd()
	initCmd.SetArgs([]string{"init", "--dir", dir, "--project", "preview-proj", "--platforms", "codex,opencode", "--yes"})
	require.NoError(t, initCmd.Execute())
}

// installFullSkillLibrary switches the workspace onto the opt-in full skill
// library and applies it, so a fixture whose subject is a transition away from
// the full surface actually has that surface on disk first. `auto init` writes
// the compact default, which installs the core surface only.
func installFullSkillLibrary(t *testing.T, dir string) {
	t.Helper()

	cfg, err := loadConfigFromDir(dir)
	require.NoError(t, err)
	cfg.Skills.Compiler.Mode = config.SkillCompilerModeFull
	require.NoError(t, config.Save(dir, cfg))

	updateCmd := newTestRootCmd()
	updateCmd.SetArgs([]string{"update", "--dir", dir, "--yes"})
	require.NoError(t, updateCmd.Execute())
}
