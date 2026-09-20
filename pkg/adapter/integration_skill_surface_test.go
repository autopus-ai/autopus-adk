package adapter_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/antigravity"
	"github.com/insajin/autopus-adk/pkg/adapter/claude"
	"github.com/insajin/autopus-adk/pkg/adapter/codex"
	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/adapter/opencode"
	"github.com/insajin/autopus-adk/pkg/config"
)

// polishSkillName is long-tail: the default surface leaves it uninstalled, so
// this fixture opts it in explicitly. That is the path a user takes to get it,
// and it keeps the subject here where it belongs — the per-platform native path
// and body projection of one reusable skill.
const polishSkillName = "make-interfaces-feel-better"

func TestE2EInitMakeInterfacesFeelBetterSkill_AllPlatforms(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		generate func(context.Context, string) error
		path     string
	}{
		{
			name: "claude-code",
			generate: func(ctx context.Context, dir string) error {
				cfg := config.DefaultFullConfig("polish-claude")
				cfg.Platforms = []string{"claude-code"}
				cfg.Skills.Compiler.ExplicitSkills = []string{polishSkillName}
				_, err := claude.NewWithRoot(dir).Generate(ctx, cfg)
				return err
			},
			path: filepath.Join(".claude", "skills", "make-interfaces-feel-better", "SKILL.md"),
		},
		{
			name: "codex",
			generate: func(ctx context.Context, dir string) error {
				cfg := config.DefaultFullConfig("polish-codex")
				cfg.Platforms = []string{"codex"}
				cfg.Skills.Compiler.ExplicitSkills = []string{polishSkillName}
				cfg.Skills.Compiler.CodexLongTailTarget = config.SkillLongTailTargetRepo
				_, err := codex.NewWithRoot(dir).Generate(ctx, cfg)
				return err
			},
			path: filepath.Join(".codex", "skills", "codex-make-interfaces-feel-better", "SKILL.md"),
		},
		{
			name: "gemini",
			generate: func(ctx context.Context, dir string) error {
				cfg := config.DefaultFullConfig("polish-gemini")
				cfg.Platforms = []string{"gemini-cli"}
				cfg.Skills.Compiler.ExplicitSkills = []string{polishSkillName}
				_, err := antigravity.NewWithRoot(dir).Generate(ctx, cfg)
				return err
			},
			path: filepath.Join(".gemini", "skills", "autopus", "make-interfaces-feel-better", "SKILL.md"),
		},
		{
			name: "opencode",
			generate: func(ctx context.Context, dir string) error {
				cfg := config.DefaultFullConfig("polish-opencode")
				cfg.Platforms = []string{"opencode"}
				cfg.Skills.Compiler.ExplicitSkills = []string{polishSkillName}
				cfg.Skills.Compiler.OpenCodeLongTailTarget = config.SkillLongTailTargetShared
				_, err := opencode.NewWithRoot(dir, opencode.WithCLIVersion("1.18.7")).Generate(ctx, cfg)
				return err
			},
			path: filepath.Join(".agents", "skills", "make-interfaces-feel-better", "SKILL.md"),
		},
		{
			name: "omp",
			generate: func(ctx context.Context, dir string) error {
				cfg := config.DefaultFullConfig("polish-omp")
				cfg.Platforms = []string{"omp"}
				cfg.Skills.Compiler.ExplicitSkills = []string{polishSkillName}
				_, err := omp.NewWithRoot(dir).Generate(ctx, cfg)
				return err
			},
			path: filepath.Join(".omp", "skills", "make-interfaces-feel-better", "SKILL.md"),
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			require.NoError(t, tc.generate(context.Background(), dir))

			data, err := os.ReadFile(filepath.Join(dir, tc.path))
			require.NoError(t, err, "expected generated skill at %s", tc.path)
			assert.Contains(t, string(data), "## Detail Pass")
			assert.Contains(t, string(data), "Do not use `transition: all`")
		})
	}
}
