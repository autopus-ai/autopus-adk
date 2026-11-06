package adapter_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/adapter/antigravity"
	"github.com/insajin/autopus-adk/pkg/adapter/claude"
	"github.com/insajin/autopus-adk/pkg/adapter/codex"
	"github.com/insajin/autopus-adk/pkg/config"
)

// TestParity_ClassifyFileCountsConditionalBodies is the S16 oracle for
// REQ-CONDRULE-VERIFY-02. Both relocated classes must keep counting as rules,
// or the parity census would report a relocation as a lost rule.
func TestParity_ClassifyFileCountsConditionalBodies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want string
	}{
		{".claude/hooks/autopus/conditional/lore-commit.md", "rules"},
		{".claude/hooks/autopus/conditional/shell-portability.md", "rules"},
		{".claude/hooks/autopus/conditional/worktree-safety.md", "rules"},
		{".claude/hooks/autopus/conditional/spec-quality.md", "rules"},
		{".claude/hooks/autopus/conditional/techstack-freshness.md", "rules"},
		{".claude/hooks/autopus/conditional-rules.json", ""},
		{".claude/hooks/autopus/task-created-validate.sh", ""},
		{".claude/rules/autopus/language-policy.md", "rules"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, classifyFile(adapter.FileMapping{TargetPath: tt.path}))
		})
	}
}

// TestParity_RelocationConservesRuleCount is the S16 count-conservation oracle
// for REQ-CONDRULE-VERIFY-01.
func TestParity_RelocationConservesRuleCount(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := config.DefaultFullConfig("parity-conditional")

	claudeFiles, err := claude.NewWithRoot(t.TempDir()).Generate(ctx, cfg)
	require.NoError(t, err)
	codexFiles, err := codex.NewWithRoot(t.TempDir()).Generate(ctx, cfg)
	require.NoError(t, err)
	geminiFiles, err := antigravity.NewWithRoot(t.TempDir()).Generate(ctx, cfg)
	require.NoError(t, err)

	claudeCounts := countFeatures(claudeFiles)
	assert.Equal(t, 3, claudeCounts.Rules, "relocation must conserve the claude rule count")
	assert.Zero(t, countFeatures(codexFiles).Rules,
		"Codex 0.149.1 must not receive inert markdown under .codex/rules")
	assert.Equal(t, 3+len(platformRuleAdditions["gemini"]), countFeatures(geminiFiles).Rules,
		"the 3 source rules plus the Antigravity-only REA guard")

	var baseline, relocated int
	for _, file := range claudeFiles.Files {
		target := filepath.ToSlash(file.TargetPath)
		switch {
		case strings.HasPrefix(target, ".claude/rules/autopus/"):
			baseline++
		case strings.Contains(target, "hooks/autopus/conditional/"):
			relocated++
		}
	}
	assert.Equal(t, 2, baseline, "two rules remain in baseline context")
	assert.Equal(t, 1, relocated,
		"the hook-fired lore-commit rule moves under the conditional body root")
}
