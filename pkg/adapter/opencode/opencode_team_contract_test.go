package opencode

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratedOpenCodeDevPreservesExplicitTeamTopology(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	_, err := NewWithRoot(root, WithCLIVersion("1.18.7")).Generate(context.Background(), config.DefaultFullConfig("team-contract"))
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(root, ".agents", "skills", "auto-dev", "SKILL.md"))
	require.NoError(t, err)
	body := string(data)
	assert.Contains(t, body, "current runtime")
	assert.Contains(t, body, "native team lifecycle")
	assert.Contains(t, body, "unsupported-mode")
	assert.Contains(t, body, "do not substitute")
	assert.Contains(t, body, "`--multi` is a review modifier")
	assert.NotContains(t, body, "reserved compatibility flag")
}

func TestNormalizeOpenCodeExplicitTeamRequiresLiveCapability(t *testing.T) {
	t.Parallel()
	body := normalizeOpenCodeMarkdown("- `--team`: `.codex/skills/codex-agent-teams/SKILL.md`의 Codex team profile 적용")
	assert.Contains(t, body, "current runtime")
	assert.Contains(t, body, "unsupported-mode")
	assert.Contains(t, body, "do not substitute")
	assert.NotContains(t, body, "별도 native team을 만들지 않고")
}
