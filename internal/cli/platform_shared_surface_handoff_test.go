package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

// The root marker no longer restates a per-platform file inventory. What it
// does carry, and what a platform transition must move, is the discovery path
// for each installed platform plus that platform's invocation policy. Both
// markers (codex-owned and opencode-owned) share these substrings, so the
// assertions below hold regardless of which adapter owns AGENTS.md.
const (
	codexDiscoveryEntry      = "- Codex: .codex/"
	openCodeDiscoveryEntry   = "- OpenCode: .opencode/"
	codexInvocationPolicy    = "$codex-auto"
	openCodeInvocationPolicy = "/auto-<route>"
)

func TestPlatformAddCodexRefreshesOpenCodeOwnedSharedSurface(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := config.DefaultFullConfig("platform-add-codex-handoff")
	cfg.Platforms = []string{"opencode"}
	seedPlatformTransitionSurface(t, root, cfg)

	before := readPlatformTransitionFile(t, root, "AGENTS.md")
	require.NotContains(t, before, codexDiscoveryEntry)

	dirFlag := root
	cmd := newPlatformAddCmd(&dirFlag)
	cmd.SetArgs([]string{"codex"})
	require.NoError(t, cmd.Execute())

	// The root marker is a discovery surface, not a file inventory: the
	// transition is observable as the newly owned platform appearing in the
	// installed-component paths and its invocation policy.
	after := readPlatformTransitionFile(t, root, "AGENTS.md")
	assert.Contains(t, after, codexDiscoveryEntry)
	assert.Contains(t, after, openCodeDiscoveryEntry)
	assert.Contains(t, after, codexInvocationPolicy)
	assert.Contains(t, after, openCodeInvocationPolicy)
}

func TestPlatformAddOpenCodeRelinquishesPreviousCodexRootClaim(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cfg := config.DefaultFullConfig("platform-add-opencode-handoff")
	cfg.Platforms = []string{"codex"}
	seedPlatformTransitionSurface(t, root, cfg)
	before, err := adapter.LoadManifest(root, "codex")
	require.NoError(t, err)
	require.Contains(t, before.Files, "AGENTS.md")

	dirFlag := root
	cmd := newPlatformAddCmd(&dirFlag)
	cmd.SetArgs([]string{"opencode"})
	require.NoError(t, cmd.Execute())

	codexManifest, err := adapter.LoadManifest(root, "codex")
	require.NoError(t, err)
	opencodeManifest, err := adapter.LoadManifest(root, "opencode")
	require.NoError(t, err)
	assert.NotContains(t, codexManifest.Files, "AGENTS.md")
	assert.Contains(t, opencodeManifest.Files, "AGENTS.md")
	agents := readPlatformTransitionFile(t, root, "AGENTS.md")
	assert.Contains(t, agents, codexDiscoveryEntry)
	assert.Contains(t, agents, openCodeDiscoveryEntry)
}

func TestPlatformAddOpenCodeOwnerFailureRollsBackBothPlatforms(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cfg := config.DefaultFullConfig("platform-add-opencode-rollback")
	cfg.Platforms = []string{"codex"}
	seedPlatformTransitionSurface(t, root, cfg)
	configPath := filepath.Join(root, ".codex", "config.toml")
	require.NoError(t, os.Remove(configPath))
	require.NoError(t, os.MkdirAll(configPath, 0o755))
	agentsBefore := readPlatformTransitionFile(t, root, "AGENTS.md")
	configBefore := readPlatformTransitionFile(t, root, "autopus.yaml")
	manifestBefore := readPlatformTransitionFile(t, root, ".autopus", "codex-manifest.json")

	dirFlag := root
	cmd := newPlatformAddCmd(&dirFlag)
	cmd.SetArgs([]string{"opencode"})
	require.Error(t, cmd.Execute())

	assert.Equal(t, agentsBefore, readPlatformTransitionFile(t, root, "AGENTS.md"))
	assert.Equal(t, configBefore, readPlatformTransitionFile(t, root, "autopus.yaml"))
	assert.Equal(t, manifestBefore, readPlatformTransitionFile(t, root, ".autopus", "codex-manifest.json"))
	assert.NoFileExists(t, filepath.Join(root, ".autopus", "opencode-manifest.json"))
	assert.DirExists(t, configPath)
}

func TestPlatformRemoveOpenCodeRegeneratesCodexOwnedAgents(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := config.DefaultFullConfig("platform-remove-opencode-handoff")
	cfg.Platforms = []string{"codex", "opencode"}
	seedPlatformTransitionSurface(t, root, cfg)

	dirFlag := root
	cmd := newPlatformRemoveCmd(&dirFlag)
	cmd.SetArgs([]string{"opencode"})
	require.NoError(t, cmd.Execute())

	after := readPlatformTransitionFile(t, root, "AGENTS.md")
	assert.Contains(t, after, codexDiscoveryEntry)
	assert.Contains(t, after, codexInvocationPolicy)
	assert.NotContains(t, after, openCodeDiscoveryEntry)
	assert.NotContains(t, after, openCodeInvocationPolicy)
}

func TestPlatformRemoveCodexRefreshesOpenCodeOnlySurface(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := config.DefaultFullConfig("platform-remove-codex-handoff")
	cfg.Platforms = []string{"codex", "opencode"}
	seedPlatformTransitionSurface(t, root, cfg)

	dirFlag := root
	cmd := newPlatformRemoveCmd(&dirFlag)
	cmd.SetArgs([]string{"codex"})
	require.NoError(t, cmd.Execute())

	after := readPlatformTransitionFile(t, root, "AGENTS.md")
	assert.Contains(t, after, openCodeDiscoveryEntry)
	assert.Contains(t, after, openCodeInvocationPolicy)
	assert.NotContains(t, after, codexDiscoveryEntry)
	assert.NotContains(t, after, codexInvocationPolicy)

	autoSkill := readPlatformTransitionFile(t, root, ".agents", "skills", "auto", "SKILL.md")
	assert.NotContains(t, autoSkill, "[OpenCode-only]")
}

func seedPlatformTransitionSurface(t *testing.T, root string, cfg *config.HarnessConfig) {
	t.Helper()
	require.NoError(t, config.Save(root, cfg))

	for _, platform := range []string{"codex", "opencode"} {
		if !containsPlatform(cfg.Platforms, platform) {
			continue
		}
		descriptor, ok := lookupPlatformDescriptor(platform)
		require.True(t, ok)
		_, err := descriptor.Generate(context.Background(), root, cfg)
		require.NoError(t, err)
	}
}

func readPlatformTransitionFile(t *testing.T, root string, elements ...string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(append([]string{root}, elements...)...))
	require.NoError(t, err)
	return string(data)
}
