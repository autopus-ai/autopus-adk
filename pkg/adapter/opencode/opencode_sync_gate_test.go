package opencode

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestAdapter_Generate_AutoSyncUpdatesDocsFromTheChange(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir, WithCLIVersion("1.18.7"))

	_, err := a.Generate(context.Background(), config.DefaultFullConfig("demo"))
	require.NoError(t, err)

	autoSyncSkill, err := os.ReadFile(filepath.Join(dir, ".agents", "skills", "auto-sync", "SKILL.md"))
	require.NoError(t, err)
	body := string(autoSyncSkill)
	// Sync reads the diff, updates only the documents the change made stale,
	// and checks what they name; it no longer gates on SPEC status or @AX tags.
	assert.Contains(t, body, "git diff HEAD --stat")
	assert.Contains(t, body, "CHANGELOG.md")
	assert.Contains(t, body, "--help")
	assert.NotContains(t, body, "Completion Verdict")
	assert.NotContains(t, body, "현재 Codex 런타임 정책")
}
