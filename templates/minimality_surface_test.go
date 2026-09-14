package templates_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	tmpl "github.com/insajin/autopus-adk/pkg/template"
)

func TestMinimalityDisciplineSourceSurfaceParity(t *testing.T) {
	t.Parallel()

	root := templateRoot()
	e := tmpl.New()
	cfg := config.DefaultFullConfig("minimality-project")
	reviewTokens := []string{
		"Correctness/Security Findings",
		"Complexity Findings",
		"delete",
		"stdlib",
		"native",
		"yagni",
		"shrink",
		"existing-helper",
		"existing-dependency",
		"Tested",
		"Readable",
		"Unified",
		"Secured",
		"Trackable",
	}
	cases := []struct {
		name   string
		path   string
		tokens []string
	}{
		{
			name:   "codex-plan-skill",
			path:   filepath.Join(root, "codex", "skills", "auto-plan.md.tmpl"),
			tokens: []string{"Minimality Decision Matrix", "new dependency", "new abstraction", "minimum sufficient verification"},
		},
		{
			name:   "codex-plan-prompt",
			path:   filepath.Join(root, "codex", "prompts", "auto-plan.md.tmpl"),
			tokens: []string{"Minimality Decision Matrix", "new dependency", "new abstraction", "minimum sufficient verification"},
		},
		{
			name:   "gemini-plan-skill",
			path:   filepath.Join(root, "gemini", "skills", "auto-plan", "SKILL.md.tmpl"),
			tokens: []string{"Minimality Decision Matrix", "new dependency", "new abstraction", "minimum sufficient verification"},
		},
		{
			name:   "claude-router",
			path:   filepath.Join(root, "claude", "commands", "auto-router.md.tmpl"),
			tokens: []string{"Minimality Decision Matrix", "minimality ladder", "shared root-cause", "Correctness/Security Findings"},
		},
		{
			name:   "gemini-router",
			path:   filepath.Join(root, "gemini", "commands", "auto-router.md.tmpl"),
			tokens: []string{"Minimality Decision Matrix", "minimality ladder", "shared root-cause", "Correctness/Security Findings"},
		},
		{
			name:   "spec-writer-content",
			path:   filepath.Join(root, "..", "content", "agents", "spec-writer.md"),
			tokens: []string{"Minimality Decision Matrix", "new dependency", "new abstraction", "minimum sufficient verification"},
		},
		{
			name:   "codex-spec-writer-agent",
			path:   filepath.Join(root, "codex", "agents", "spec-writer.toml.tmpl"),
			tokens: []string{"Minimality Decision Matrix", "new dependency", "new abstraction", "minimum sufficient verification"},
		},
		{
			name:   "gemini-spec-writer-agent",
			path:   filepath.Join(root, "gemini", "agents", "spec-writer.md.tmpl"),
			tokens: []string{"Minimality Decision Matrix", "new dependency", "new abstraction", "minimum sufficient verification"},
		},
		{
			// The entrypoint states the principle; the ladder is retrievable
			// beside it and is asserted on the resources below.
			name:   "agent-pipeline-content",
			path:   filepath.Join(root, "..", "content", "skills", "agent-pipeline.md"),
			tokens: []string{"Minimality", "smallest change", "receipt", "references/delegation.md"},
		},
		{
			name:   "gemini-agent-pipeline-template",
			path:   filepath.Join(root, "gemini", "skills", "agent-pipeline", "SKILL.md.tmpl"),
			tokens: []string{"Minimality", "smallest change", "receipt", "references/delegation.md"},
		},
		{
			name:   "codex-go-skill",
			path:   filepath.Join(root, "codex", "skills", "auto-go.md.tmpl"),
			tokens: []string{"minimality ladder", "existing code/helper/pattern", "minimum sufficient verification", "receipt"},
		},
		{
			name:   "codex-go-prompt",
			path:   filepath.Join(root, "codex", "prompts", "auto-go.md.tmpl"),
			tokens: []string{"minimality ladder", "existing code/helper/pattern", "minimum sufficient verification", "receipt"},
		},
		{
			name:   "gemini-go-skill",
			path:   filepath.Join(root, "gemini", "skills", "auto-go", "SKILL.md.tmpl"),
			tokens: []string{"minimality ladder", "existing code/helper/pattern", "minimum sufficient verification", "receipt"},
		},
		{
			name:   "codex-fix-skill",
			path:   filepath.Join(root, "codex", "skills", "auto-fix.md.tmpl"),
			tokens: []string{"caller", "shared root-cause", "revise-target", "receipt"},
		},
		{
			name:   "codex-fix-prompt",
			path:   filepath.Join(root, "codex", "prompts", "auto-fix.md.tmpl"),
			tokens: []string{"caller", "shared root-cause", "revise-target", "receipt"},
		},
		{
			name:   "gemini-fix-skill",
			path:   filepath.Join(root, "gemini", "skills", "auto-fix", "SKILL.md.tmpl"),
			tokens: []string{"caller", "shared root-cause", "revise-target", "receipt"},
		},
		{
			name:   "debugger-content",
			path:   filepath.Join(root, "..", "content", "agents", "debugger.md"),
			tokens: []string{"caller", "shared root-cause", "revise-target"},
		},
		{
			name:   "debugging-content",
			path:   filepath.Join(root, "..", "content", "skills", "debugging.md"),
			tokens: []string{"caller", "shared root-cause", "revise-target"},
		},
		{
			name:   "codex-debugger-agent",
			path:   filepath.Join(root, "codex", "agents", "debugger.toml.tmpl"),
			tokens: []string{"caller", "shared root-cause", "revise-target"},
		},
		{
			name:   "gemini-debugger-agent",
			path:   filepath.Join(root, "gemini", "agents", "debugger.md.tmpl"),
			tokens: []string{"caller", "shared root-cause", "revise-target"},
		},
		{
			name:   "codex-debugging-skill",
			path:   filepath.Join(root, "codex", "skills", "debugging.md.tmpl"),
			tokens: []string{"caller", "shared root-cause", "revise-target"},
		},
		{
			name:   "gemini-debugging-skill",
			path:   filepath.Join(root, "gemini", "skills", "debugging", "SKILL.md.tmpl"),
			tokens: []string{"caller", "shared root-cause", "revise-target"},
		},
		{
			name:   "codex-review-skill",
			path:   filepath.Join(root, "codex", "skills", "auto-review.md.tmpl"),
			tokens: reviewTokens,
		},
		{
			name:   "codex-review-prompt",
			path:   filepath.Join(root, "codex", "prompts", "auto-review.md.tmpl"),
			tokens: reviewTokens,
		},
		{
			name:   "gemini-review-skill",
			path:   filepath.Join(root, "gemini", "skills", "auto-review", "SKILL.md.tmpl"),
			tokens: reviewTokens,
		},
		{
			name:   "reviewer-content",
			path:   filepath.Join(root, "..", "content", "agents", "reviewer.md"),
			tokens: reviewTokens,
		},
		{
			name:   "review-content",
			path:   filepath.Join(root, "..", "content", "skills", "review.md"),
			tokens: reviewTokens,
		},
		{
			name:   "codex-reviewer-agent",
			path:   filepath.Join(root, "codex", "agents", "reviewer.toml.tmpl"),
			tokens: reviewTokens,
		},
		{
			name:   "gemini-reviewer-agent",
			path:   filepath.Join(root, "gemini", "agents", "reviewer.md.tmpl"),
			tokens: reviewTokens,
		},
		{
			name:   "codex-review-skill-template",
			path:   filepath.Join(root, "codex", "skills", "review.md.tmpl"),
			tokens: reviewTokens,
		},
		{
			name:   "gemini-review-skill-template",
			path:   filepath.Join(root, "gemini", "skills", "review", "SKILL.md.tmpl"),
			tokens: reviewTokens,
		},
		{
			name:   "shared-orchestra-reviewer",
			path:   filepath.Join(root, "shared", "orchestra-reviewer.md.tmpl"),
			tokens: []string{"complexity", "unnecessary dependency", "duplicate helper", "single-implementation abstraction", "YAGNI", "correctness/security"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			text, err := semanticContractSurface(e, tc.path, cfg)
			require.NoError(t, err)
			for _, token := range tc.tokens {
				assert.Contains(t, text, token, "%s should contain %q", tc.path, token)
			}
		})
	}
}

// The ladder is the enforceable part of minimality discipline. It moved out of
// the pipeline entrypoint into the resource a worker actually opens, so it is
// checked there rather than duplicated back into every body.
func TestMinimalityLadderLivesInThePipelineResources(t *testing.T) {
	t.Parallel()

	surface := pipelineResourceSurface(t)
	for _, token := range []string{
		"minimality ladder",
		"existing code/helper/pattern",
		"minimum sufficient verification",
		"stdlib/native",
	} {
		assert.Contains(t, surface, token,
			"the pipeline resources should contain the minimality token %q", token)
	}
}
