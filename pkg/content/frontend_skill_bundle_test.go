package content_test

import (
	"context"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/adapter/antigravity"
	"github.com/insajin/autopus-adk/pkg/adapter/claude"
	"github.com/insajin/autopus-adk/pkg/adapter/codex"
	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/adapter/opencode"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/content"
)

// frontendSkillSections are the sections of content/skills/frontend-skill.md
// that the golden tasks on the default split surface never see, each with a
// sentence that only its own body carries.
var frontendSkillSections = map[string]string{
	"## Reference-First": "Take the grammar, never the content",
	"## Banned Looks":    "Each one present keeps its rubric key below 8.",
	"## Critique Loop":   "Stop when every score is 8 or more, or after round 3.",
}

// frontend-skill ships only in the frontend bundle, so the default surface
// cannot cover it. Compiling that bundle must deliver the whole skill on every
// platform: a transform that drops or rewrites one of these sections would
// otherwise go unnoticed.
func TestFrontendBundle_GeneratesTheFullFrontendSkillOnEveryPlatform(t *testing.T) {
	t.Parallel()
	require.False(t, content.IsCoreSkill("frontend-skill"), "frontend-skill stays an opt-in bundle skill")

	cfg := config.DefaultFullConfig("frontend-bundle")
	cfg.Skills.Compiler.Bundles = []string{"frontend"}
	generators := map[string]func(root string) adapter.PlatformAdapter{
		"claude-code":     func(root string) adapter.PlatformAdapter { return claude.NewWithRoot(root) },
		"codex":           func(root string) adapter.PlatformAdapter { return codex.NewWithRoot(root) },
		"antigravity-cli": func(root string) adapter.PlatformAdapter { return antigravity.NewWithRoot(root) },
		"opencode": func(root string) adapter.PlatformAdapter {
			return opencode.NewWithRoot(root, opencode.WithCLIVersion("1.18.7"))
		},
		"omp": func(root string) adapter.PlatformAdapter { return omp.NewWithRoot(root) },
	}

	for platform, newAdapter := range generators {
		pf, err := newAdapter(t.TempDir()).Generate(context.Background(), cfg)
		require.NoError(t, err, platform)

		found := 0
		for _, file := range pf.Files {
			target := strings.ReplaceAll(file.TargetPath, "\\", "/")
			if path.Base(target) != "SKILL.md" || strings.TrimPrefix(path.Base(path.Dir(target)), "codex-") != "frontend-skill" {
				continue
			}
			found++
			body := string(file.Content)
			for heading, sentence := range frontendSkillSections {
				assert.Contains(t, body, "\n"+heading+"\n", "%s %s lost the %s section", platform, target, heading)
				assert.Contains(t, body, sentence, "%s %s lost the body of %s", platform, target, heading)
			}
		}
		assert.NotZero(t, found, "%s compiles frontend-skill once the frontend bundle is selected", platform)
	}
}
