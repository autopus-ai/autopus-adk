package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/codex"
	"github.com/insajin/autopus-adk/pkg/config"
)

// composeCatalogA advertises every managed rung; composeCatalogB swaps GPT-6.1
// Sol for GPT-6 Sol, so an ultra opus agent lands on a visible substitute.
const (
	composeCatalogA = `{"models":[` +
		`{"slug":"gpt-6-astra","supported_reasoning_levels":[{"effort":"xhigh"},{"effort":"max"},{"effort":"ultra"}]},` +
		`{"slug":"gpt-6.1-sol","supported_reasoning_levels":[{"effort":"xhigh"},{"effort":"max"}]},` +
		`{"slug":"gpt-6-luna","supported_reasoning_levels":[{"effort":"medium"},{"effort":"max"}]}]}`
	composeCatalogB = `{"models":[` +
		`{"slug":"gpt-6-astra","supported_reasoning_levels":[{"effort":"xhigh"},{"effort":"max"},{"effort":"ultra"}]},` +
		`{"slug":"gpt-6-sol","supported_reasoning_levels":[{"effort":"high"},{"effort":"xhigh"}]},` +
		`{"slug":"gpt-6-luna","supported_reasoning_levels":[{"effort":"medium"},{"effort":"max"}]}]}`
)

type composeProbeCall struct {
	binary  string
	timeout time.Duration
}

// stubComposeCatalogProbe replaces the run's single catalog probe and records
// every call.
func stubComposeCatalogProbe(t *testing.T, catalog string, probeErr error) *[]composeProbeCall {
	t.Helper()
	original := composeCodexCatalogProbe
	t.Cleanup(func() { composeCodexCatalogProbe = original })
	calls := &[]composeProbeCall{}
	composeCodexCatalogProbe = func(_ context.Context, binary string, timeout time.Duration) ([]byte, error) {
		*calls = append(*calls, composeProbeCall{binary: binary, timeout: timeout})
		if probeErr != nil {
			return nil, probeErr
		}
		return []byte(catalog), nil
	}
	return calls
}

// installComposeFakeCodex shadows the codex CLI with a script that logs every
// invocation and answers `debug models` with catalog, so a test can tell an
// adapter that probed for itself from one that read the run's snapshot.
func installComposeFakeCodex(t *testing.T, catalog string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the codex stand-in is a POSIX shell script")
	}
	binDir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "codex-calls.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + marker + "'\n" +
		"if [ \"$1\" = debug ] && [ \"$2\" = models ]; then printf '%s' '" + catalog + "'; fi\n"
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "codex"), []byte(script), 0o755))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return marker
}

func composeCodexCalls(t *testing.T, marker string) string {
	t.Helper()
	data, err := os.ReadFile(marker)
	if errors.Is(err, os.ErrNotExist) {
		return ""
	}
	require.NoError(t, err)
	return string(data)
}

func composeSnapshotConfig(mode string, platforms ...string) *config.HarnessConfig {
	cfg := config.DefaultFullConfig("compose-snapshot")
	cfg.Platforms = platforms
	cfg.Quality.Default = mode
	return cfg
}

// readComposeCodexProfile reads the model and effort a generated agent TOML carries.
func readComposeCodexProfile(t *testing.T, dir, agent string) config.CodexProfile {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".codex", "agents", agent+".toml"))
	require.NoError(t, err)
	var profile config.CodexProfile
	for _, line := range strings.Split(string(data), "\n") {
		if value, ok := strings.CutPrefix(line, "model = "); ok {
			profile.Model = strings.Trim(value, `"`)
		}
		if value, ok := strings.CutPrefix(line, "model_reasoning_effort = "); ok {
			profile.Effort = strings.Trim(value, `"`)
		}
	}
	return profile
}

func newComposeTestCommand() (*cobra.Command, *bytes.Buffer) {
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	return cmd, &out
}

func TestCaptureComposeSnapshot_CodexInstalled_ProbesOnceAndFreezesCatalog(t *testing.T) {
	calls := stubComposeCatalogProbe(t, composeCatalogB, nil)

	snapshot := captureComposeSnapshot(context.Background(), composeSnapshotConfig("ultra", "claude-code", "codex"))

	require.Len(t, *calls, 1)
	assert.Equal(t, composeProbeCall{binary: "codex", timeout: 5 * time.Second}, (*calls)[0])
	assert.Equal(t, []byte(composeCatalogB), snapshot.codexCatalog)
	assert.True(t, snapshot.codexCatalogFixed)
}

func TestCaptureComposeSnapshot_WithoutCodex_ProbesNothing(t *testing.T) {
	calls := stubComposeCatalogProbe(t, composeCatalogB, nil)

	snapshot := captureComposeSnapshot(context.Background(), composeSnapshotConfig("ultra", "claude-code", "omp"))

	assert.Empty(t, *calls)
	assert.Equal(t, driftGenerationSnapshot{}, snapshot)
}

// A failed probe still freezes the run: generation must not probe again and
// see a different catalog than the table did.
func TestCaptureComposeSnapshot_ProbeFailure_FreezesUnreadableCatalog(t *testing.T) {
	stubComposeCatalogProbe(t, "", errors.New("codex: not signed in"))

	snapshot := captureComposeSnapshot(context.Background(), composeSnapshotConfig("ultra", "codex"))

	assert.Equal(t, driftGenerationSnapshot{codexCatalogFixed: true}, snapshot)
}

// The *With seams generate Codex from the run's snapshot: the generated file is
// what the table's resolver computes from that snapshot, and the adapter never
// asks the CLI for a second catalog.
func TestComposeWithSeams_RunSnapshot_GenerationMatchesResolverWithoutSecondProbe(t *testing.T) {
	original := qualityPlatformUpdater
	t.Cleanup(func() { qualityPlatformUpdater = original })
	qualityPlatformUpdater = func(context.Context, string, string, *config.HarnessConfig) (bool, error) {
		t.Error("applyQualityHarnessWith must build real adapters from the snapshot")
		return true, nil
	}

	seams := []struct {
		name     string
		generate func(t *testing.T, dir string, cfg *config.HarnessConfig, snapshot driftGenerationSnapshot) string
	}{
		{
			name: "update",
			generate: func(t *testing.T, dir string, cfg *config.HarnessConfig, snapshot driftGenerationSnapshot) string {
				supported, err := updateHarnessPlatformWith(context.Background(), dir, "codex", cfg, snapshot)
				require.True(t, supported)
				require.NoError(t, err)
				return ""
			},
		},
		{
			name: "init",
			generate: func(t *testing.T, dir string, cfg *config.HarnessConfig, snapshot driftGenerationSnapshot) string {
				cmd, out := newComposeTestCommand()
				require.NoError(t, generatePlatformFilesWith(context.Background(), dir, cfg, cmd, snapshot))
				return out.String()
			},
		},
		{
			name: "quality apply",
			generate: func(t *testing.T, dir string, cfg *config.HarnessConfig, snapshot driftGenerationSnapshot) string {
				cmd, out := newComposeTestCommand()
				require.NoError(t, applyQualityHarnessWith(cmd, dir, cfg, "auto quality --apply", snapshot))
				return out.String()
			},
		},
	}
	for _, seam := range seams {
		t.Run(seam.name, func(t *testing.T) {
			marker := installComposeFakeCodex(t, composeCatalogA)
			calls := stubComposeCatalogProbe(t, composeCatalogB, nil)
			dir := t.TempDir()
			cfg := composeSnapshotConfig("ultra", "codex")
			snapshot := captureComposeSnapshot(context.Background(), cfg)
			cell, err := codex.ResolveAgentProfile(cfg.Quality, snapshot.codexCatalog, "annotator", "sonnet", "medium")
			require.NoError(t, err)

			output := seam.generate(t, dir, cfg, snapshot)

			generated := readComposeCodexProfile(t, dir, "annotator")
			assert.Equal(t, config.CodexProfile{Model: "gpt-6-sol", Effort: "xhigh"}, generated)
			assert.Equal(t, cell.Effective, generated)
			assert.Equal(t, config.CodexProfile{Model: "gpt-6-astra", Effort: "max"}, readComposeCodexProfile(t, dir, "planner"))
			assert.Len(t, *calls, 1)
			assert.NotContains(t, composeCodexCalls(t, marker), "debug models")
			if seam.name == "quality apply" {
				assert.Contains(t, output, "  applied codex\n")
				assert.Contains(t, output, "quality.applied_platforms = 1\n")
				assert.Contains(t, output, "Start a new Codex session to load the updated managed agents.")
			}
		})
	}
}

// The original entry points keep today's behavior: no snapshot, so the Codex
// adapter probes the CLI catalog for itself.
func TestComposeWithSeams_OriginalEntryPoints_AdapterStillProbesItself(t *testing.T) {
	entries := []struct {
		name     string
		generate func(t *testing.T, dir string, cfg *config.HarnessConfig)
	}{
		{
			name: "updateHarnessPlatform",
			generate: func(t *testing.T, dir string, cfg *config.HarnessConfig) {
				supported, err := updateHarnessPlatform(context.Background(), dir, "codex", cfg)
				require.True(t, supported)
				require.NoError(t, err)
			},
		},
		{
			name: "generatePlatformFiles",
			generate: func(t *testing.T, dir string, cfg *config.HarnessConfig) {
				cmd, _ := newComposeTestCommand()
				require.NoError(t, generatePlatformFiles(context.Background(), dir, cfg, cmd))
			},
		},
		{
			name: "applyQualityHarness",
			generate: func(t *testing.T, dir string, cfg *config.HarnessConfig) {
				cmd, _ := newComposeTestCommand()
				require.NoError(t, applyQualityHarness(cmd, dir, cfg, "auto quality --apply"))
			},
		},
	}
	for _, entry := range entries {
		t.Run(entry.name, func(t *testing.T) {
			marker := installComposeFakeCodex(t, composeCatalogA)
			dir := t.TempDir()

			entry.generate(t, dir, composeSnapshotConfig("ultra", "codex"))

			assert.Equal(t, config.CodexProfile{Model: "gpt-6.1-sol", Effort: "xhigh"}, readComposeCodexProfile(t, dir, "annotator"))
			assert.Contains(t, composeCodexCalls(t, marker), "debug models")
		})
	}
}

func TestComposeWithSeams_UnknownPlatform_KeepsExistingHandling(t *testing.T) {
	stubComposeCatalogProbe(t, composeCatalogA, nil)
	cfg := composeSnapshotConfig("balanced", "unknown-tool")
	snapshot := captureComposeSnapshot(context.Background(), cfg)
	dir := t.TempDir()

	supported, err := updateHarnessPlatformWith(context.Background(), dir, "unknown-tool", cfg, snapshot)
	assert.False(t, supported)
	assert.NoError(t, err)

	cmd, out := newComposeTestCommand()
	require.NoError(t, generatePlatformFilesWith(context.Background(), dir, cfg, cmd, snapshot))
	assert.Contains(t, out.String(), "unknown-tool")

	cmd, out = newComposeTestCommand()
	require.NoError(t, applyQualityHarnessWith(cmd, dir, cfg, "auto quality --apply", snapshot))
	assert.Contains(t, out.String(), "  skipped unknown platform: unknown-tool\n")
	assert.Contains(t, out.String(), "quality.applied_platforms = 0\n")
}
