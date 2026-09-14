package codex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestAdapter_Update_SplitCompilerPrunesRepoVisibleLongTailWhenMovingToPluginSurface(t *testing.T) {
	t.Parallel()

	a := NewWithRoot(t.TempDir())
	fullCfg := config.DefaultFullConfig("split-codex")
	fullCfg.Platforms = []string{"codex", "opencode"}
	// The first leg is the full-library baseline this fixture moves away from,
	// so it selects that library explicitly rather than riding the default.
	fullCfg.Skills.Compiler.Mode = config.SkillCompilerModeFull

	_, err := a.Generate(context.Background(), fullCfg)
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(a.root, ".codex", "skills", "codex-metrics", "SKILL.md"), "full mode must start with the native repo-visible Codex skill path")

	splitCfg := config.DefaultFullConfig("split-codex")
	splitCfg.Platforms = []string{"codex", "opencode"}
	splitCfg.Skills.SharedSurface = config.SharedSurfaceCore
	splitCfg.Skills.Compiler.Mode = config.SkillCompilerModeSplit
	// metrics is a `product` bundle skill: split mode relocates a long-tail
	// skill only once something selects it.
	splitCfg.Skills.Compiler.Bundles = []string{"product"}
	splitCfg.Skills.Compiler.CodexLongTailTarget = config.SkillLongTailTargetPlugin

	_, err = a.Update(context.Background(), splitCfg)
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(a.root, ".codex", "skills", "codex-planning", "SKILL.md"), "a core skill must remain repo-visible for Codex even when long-tail skills move to the plugin surface")
	assert.FileExists(t, filepath.Join(a.root, ".autopus", "plugins", "auto", "skills", "metrics", "SKILL.md"), "split compiler must materialize Codex long-tail skills in the plugin-scoped target")
	assert.NoFileExists(t, filepath.Join(a.root, ".codex", "skills", "codex-metrics", "SKILL.md"), "stale repo-visible Codex long-tail artifact must be pruned when ownership moves to the plugin surface")
}

func TestAdapter_Update_SplitCompilerWritesCodexPluginSkillFrontmatter(t *testing.T) {
	t.Parallel()

	a := NewWithRoot(t.TempDir())
	cfg := config.DefaultFullConfig("split-codex")
	cfg.Platforms = []string{"codex", "opencode"}
	cfg.Skills.SharedSurface = config.SharedSurfaceCore
	cfg.Skills.Compiler.Mode = config.SkillCompilerModeSplit
	cfg.Skills.Compiler.Bundles = []string{"product"}
	cfg.Skills.Compiler.CodexLongTailTarget = config.SkillLongTailTargetPlugin

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	body, err := os.ReadFile(filepath.Join(a.root, ".autopus", "plugins", "auto", "skills", "metrics", "SKILL.md"))
	require.NoError(t, err)
	text := string(body)
	assert.True(t, strings.HasPrefix(text, "---\n"), "Codex plugin skills must include YAML frontmatter")
	assert.Contains(t, text, "name: metrics")
	assert.Contains(t, text, "description: >")
}
