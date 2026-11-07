package codex

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

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

}
