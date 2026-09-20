package opencode

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestInjectMarkerSection_MixedModeAdvertisesLatestCodexNativeSurface(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultFullConfig("demo")
	cfg.Platforms = []string{"codex", "opencode"}

	section, err := NewWithRoot(t.TempDir(), WithCLIVersion("1.18.7")).injectMarkerSection(cfg)
	require.NoError(t, err)

	// The marker advertises where each installed platform lives and how to
	// invoke it, not a file inventory or a tool list.
	assert.Contains(t, section, "- Codex: .codex/")
	assert.Contains(t, section, "- OpenCode: .opencode/")
	assert.Contains(t, section, "- Shared skills: .agents/skills/")
	assert.Contains(t, section, "@auto")
	assert.Contains(t, section, "$codex-auto")
	assert.Contains(t, section, "/auto-<route>")
	// Retired and foreign surfaces must never reappear.
	assert.NotContains(t, section, ".codex/rules")
	assert.NotContains(t, section, ".codex/prompts")
	assert.NotContains(t, section, "send_input")
	assert.NotContains(t, section, "resume_agent")
	assert.NotContains(t, section, "close_agent")
}

func TestInjectMarkerSection_UpdatePreservesCodexDollarInvocation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	existing := "user\n" + markerBegin + "\nold\n" + markerEnd + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(existing), 0o644))
	cfg := config.DefaultFullConfig("demo")
	cfg.Platforms = []string{"codex", "opencode"}

	section, err := NewWithRoot(root, WithCLIVersion("1.18.7")).injectMarkerSection(cfg)

	require.NoError(t, err)
	assert.Contains(t, section, "$codex-auto")
	assert.NotContains(t, section, "/ -auto-<route>")
}
