package opencode

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestAdapter_Update_SplitCompilerPrunesOpenCodeProjectLongTailWhenReturningToFullMode(t *testing.T) {
	t.Parallel()

	a := NewWithRoot(t.TempDir())
	splitCfg := config.DefaultFullConfig("split-opencode")
	splitCfg.Platforms = []string{"codex", "opencode"}
	splitCfg.Skills.SharedSurface = config.SharedSurfaceCore
	splitCfg.Skills.Compiler.Mode = config.SkillCompilerModeSplit
	// metrics is a `product` bundle skill: split mode relocates a long-tail
	// skill only once something selects it.
	splitCfg.Skills.Compiler.Bundles = []string{"product"}
	splitCfg.Skills.Compiler.OpenCodeLongTailTarget = config.SkillLongTailTargetProject

	_, err := a.Generate(context.Background(), splitCfg)
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(a.root, ".opencode", "skills", "metrics", "SKILL.md"), "acceptance S6: split compiler must materialize OpenCode long-tail skill under .opencode/skills")

	fullCfg := config.DefaultFullConfig("split-opencode")
	fullCfg.Platforms = []string{"codex", "opencode"}
	fullCfg.Skills.Compiler.Mode = config.SkillCompilerModeFull

	_, err = a.Update(context.Background(), fullCfg)
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(a.root, ".agents", "skills", "metrics", "SKILL.md"), "acceptance S2: returning to full mode must restore the full-compatible shared skill path")
	assert.NoFileExists(t, filepath.Join(a.root, ".opencode", "skills", "metrics", "SKILL.md"), "acceptance S4: stale OpenCode project-local long-tail artifact must be pruned when ownership returns to the full shared surface")
}
