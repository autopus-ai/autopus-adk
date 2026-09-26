package cli

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/adapter/antigravity"
	"github.com/insajin/autopus-adk/pkg/adapter/claude"
	"github.com/insajin/autopus-adk/pkg/adapter/codex"
	"github.com/insajin/autopus-adk/pkg/config"
)

// claudeDriftConfig returns a full-mode config scoped to claude-code so the
// content-drift fixtures generate the deterministic workflow surface (S1-S3).
func claudeDriftConfig() *config.HarnessConfig {
	cfg := config.DefaultFullConfig("drift-proj")
	cfg.Platforms = []string{"claude-code"}
	return cfg
}

// generateClaudeInstall runs the claude adapter's Generate against dir, writing a
// fresh install plus its manifest — the "installed" gate collectContentDrift uses.
func generateClaudeInstall(t *testing.T, dir string, cfg *config.HarnessConfig) {
	t.Helper()
	_, err := claude.NewWithRoot(dir).Generate(context.Background(), cfg)
	require.NoError(t, err)
}

func findContentDrift(results []contentDriftResult, platform string) (contentDriftResult, bool) {
	for _, r := range results {
		if r.Platform == platform {
			return r, true
		}
	}
	return contentDriftResult{}, false
}

// TestContentDrift_TamperedWorkflow_WarnCountOne is the S1 oracle: a deterministic
// installed file whose model id was rolled back to claude-sonnet-4-6 while the
// current binary generates claude-sonnet-5 drifts with count exactly 1, and the
// JSON check reports warn/warning with the file path and the auto update hint.
func TestContentDrift_TamperedWorkflow_WarnCountOne(t *testing.T) {
	dir := t.TempDir()
	cfg := claudeDriftConfig()
	generateClaudeInstall(t, dir, cfg)

	wf := filepath.Join(dir, ".claude", "workflows", "route_team.workflow.js")
	original, err := os.ReadFile(wf)
	require.NoError(t, err)
	tampered := strings.Replace(string(original), "claude-sonnet-5", "claude-sonnet-4-6", 1)
	require.NotEqual(t, string(original), tampered, "fixture must actually change the installed bytes")
	require.NoError(t, os.WriteFile(wf, []byte(tampered), 0o644))

	res, ok := findContentDrift(collectContentDrift(dir, cfg), "claude-code")
	require.True(t, ok, "claude-code content drift check must be present")
	assert.Equal(t, 1, res.DriftCount, "exactly one deterministic file drifts")
	require.Len(t, res.DriftPaths, 1)
	assert.Contains(t, res.DriftPaths[0], "route_team.workflow.js")

	check := contentDriftCheck(res)
	assert.Equal(t, "doctor.drift.content.claude-code", check.ID)
	assert.Equal(t, "warn", check.Status)
	assert.Equal(t, "warning", check.Severity)
	assert.Contains(t, check.Detail, "route_team.workflow.js")
	assert.Contains(t, check.Detail, "auto update")
}

// TestContentDrift_UserStatusline_PassCountZero is the S2 oracle: a freshly
// generated install that carries a user statusline value must report count 0 —
// the determinism gate excludes the environment-dependent
// statusline-user-command.txt so it never contributes drift.
func TestContentDrift_UserStatusline_PassCountZero(t *testing.T) {
	dir := t.TempDir()
	cfg := claudeDriftConfig()

	// Seed merge-mode statusline state so the generated install preserves a user
	// command value in statusline-user-command.txt.
	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"),
		[]byte(`{"statusLine":{"command":".claude/statusline-combined.sh"}}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "statusline-user-command.txt"),
		[]byte("echo user-value\n"), 0o644))

	generateClaudeInstall(t, dir, cfg)

	// The install genuinely carries the user value — otherwise the test is vacuous.
	userCmd, err := os.ReadFile(filepath.Join(claudeDir, "statusline-user-command.txt"))
	require.NoError(t, err)
	require.Contains(t, string(userCmd), "user-value")

	res, ok := findContentDrift(collectContentDrift(dir, cfg), "claude-code")
	require.True(t, ok)
	assert.Equal(t, 0, res.DriftCount, "no drift despite the user statusline value")
	assert.Greater(t, res.Compared, 0, "the deterministic subset is non-empty")
	for _, p := range res.DriftPaths {
		assert.NotContains(t, p, "statusline-user-command.txt",
			"the environment-dependent file must be excluded from comparison")
	}

	check := contentDriftCheck(res)
	assert.Equal(t, "pass", check.Status)
}

// TestContentDrift_MarkerMergeExcluded is the S3 oracle: user content in the
// marker file (CLAUDE.md) and merge files (.mcp.json, settings.json) must not
// appear as drift, because those policies are excluded from the comparison set.
func TestContentDrift_MarkerMergeExcluded(t *testing.T) {
	dir := t.TempDir()
	cfg := claudeDriftConfig()
	generateClaudeInstall(t, dir, cfg)

	// Tamper the marker and merge surfaces with user edits.
	appendToFile(t, filepath.Join(dir, "CLAUDE.md"), "\nuser preface outside markers\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mcp.json"),
		[]byte(`{"mcpServers":{"user":{"command":"x"}}}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "settings.json"),
		[]byte(`{"permissions":{"allow":["UserRule(*)"]}}`), 0o644))

	res, ok := findContentDrift(collectContentDrift(dir, cfg), "claude-code")
	require.True(t, ok)
	assert.Equal(t, 0, res.DriftCount, "marker/merge edits are excluded by policy")
	for _, p := range res.DriftPaths {
		assert.NotContains(t, p, "CLAUDE.md")
		assert.NotContains(t, p, ".mcp.json")
		assert.NotContains(t, p, "settings.json")
	}
}

func appendToFile(t *testing.T, path, extra string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append(data, []byte(extra)...), 0o644))
}

// TestContentDrift_AntigravityBaselineNeverInstallsPlugin guards the doctor's
// read-only boundary. Generating an isolated comparison baseline must not turn
// into an external `agy plugin install` call merely because agy is on PATH.
func TestContentDrift_AntigravityBaselineNeverInstallsPlugin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake PATH executable uses a POSIX shell")
	}
	dir := t.TempDir()
	binDir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "agy-called")
	cfg := config.DefaultFullConfig("drift-antigravity")
	cfg.Platforms = []string{"antigravity-cli"}

	// Seed an installed manifest while agy is absent, then expose a marker
	// executable only for the doctor baseline generation under test.
	t.Setenv("PATH", binDir)
	_, err := antigravity.NewWithRoot(dir).Generate(context.Background(), cfg)
	require.NoError(t, err)
	writeMarkerExecutable(t, filepath.Join(binDir, "agy"), marker, "{}")

	results := collectContentDrift(dir, cfg)
	_, ok := findContentDrift(results, "antigravity-cli")
	require.True(t, ok, "antigravity baseline must still be compared")
	_, err = os.Stat(marker)
	assert.ErrorIs(t, err, os.ErrNotExist,
		"doctor content drift must never invoke external plugin installation")
}

// TestContentDrift_CodexCatalogProbedOnce ensures both seeded roots share one
// immutable runtime catalog snapshot without depending on subprocess timing.
func TestContentDrift_CodexCatalogProbedOnce(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultFullConfig("drift-codex")
	cfg.Platforms = []string{"codex"}

	_, err := codex.NewWithRoot(dir, codex.WithModelCatalog(nil)).Generate(context.Background(), cfg)
	require.NoError(t, err)
	payload := []byte(`{"models":[{"slug":"gpt-6-astra","supported_reasoning_levels":[{"effort":"max"}]},{"slug":"gpt-6-luna","supported_reasoning_levels":[{"effort":"max"}]}]}`)
	probeCalls := 0
	var snapshots [][]byte
	deps := defaultDriftContentDeps()
	deps.probeCatalog = func(context.Context, string, time.Duration) ([]byte, error) {
		probeCalls++
		return append([]byte(nil), payload...), nil
	}
	productionGenerate := deps.generateBaseline
	deps.generateBaseline = func(
		ctx context.Context,
		platform string,
		cfg *config.HarnessConfig,
		snapshot driftGenerationSnapshot,
		seed func(string) error,
	) (*adapter.PlatformFiles, bool) {
		snapshots = append(snapshots, append([]byte(nil), snapshot.codexCatalog...))
		return productionGenerate(ctx, platform, cfg, snapshot, seed)
	}

	results := collectContentDriftWithDeps(context.Background(), dir, cfg, deps)
	_, ok := findContentDrift(results, "codex")
	require.True(t, ok, "codex baseline must still be compared")
	assert.Equal(t, 1, probeCalls, "one catalog probe must serve both roots")
	require.Len(t, snapshots, 2)
	assert.Equal(t, payload, snapshots[0])
	assert.Equal(t, snapshots[0], snapshots[1], "both roots must reuse the same immutable snapshot")
}

func writeMarkerExecutable(t *testing.T, path, marker, stdout string) {
	t.Helper()
	script := "#!/bin/sh\nprintf 'called\\n' >> '" + marker + "'\nprintf '%s' '" + stdout + "'\n"
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
}
