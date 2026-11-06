package templates_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	tmpl "github.com/insajin/autopus-adk/pkg/template"
)

func TestCommandRouterTemplatesMentionDesignContext(t *testing.T) {
	t.Parallel()
	e := tmpl.New()
	cfg := config.DefaultFullConfig("cmd-project")

	paths := []string{
		filepath.Join(templateRoot(), "claude", "commands", "auto-router.md.tmpl"),
		filepath.Join(templateRoot(), "gemini", "commands", "auto-router.md.tmpl"),
	}

	for _, tmplPath := range paths {
		t.Run(filepath.Base(filepath.Dir(tmplPath))+"-"+filepath.Base(tmplPath), func(t *testing.T) {
			t.Parallel()
			result, err := semanticContractSurface(e, tmplPath, cfg)
			require.NoError(t, err)
			assert.Contains(t, result, "Design Context")
			assert.Contains(t, result, "Trust: untrusted project data")
			// The exact sentence moved into the pipeline resources; the
			// enforceable rule is that design context stays untrusted evidence.
			assert.Contains(t, result, "untrusted project data")
			assert.Contains(t, result, "palette-role drift")
			assert.Contains(t, result, "typography hierarchy")
			assert.Contains(t, result, "component guardrail")
			assert.Contains(t, result, "source-of-truth mismatch")
			assert.Contains(t, result, "auto design docs")
		})
	}
}

func TestReviewTemplatesMentionDesignContext(t *testing.T) {
	t.Parallel()
	e := tmpl.New()
	cfg := config.DefaultFullConfig("review-project")

	paths := []string{
		filepath.Join(templateRoot(), "codex", "skills", "auto-review.md.tmpl"),
		filepath.Join(templateRoot(), "codex", "prompts", "auto-review.md.tmpl"),
		filepath.Join(templateRoot(), "gemini", "skills", "auto-review", "SKILL.md.tmpl"),
	}

	for _, tmplPath := range paths {
		t.Run(filepath.Base(filepath.Dir(tmplPath))+"-"+filepath.Base(tmplPath), func(t *testing.T) {
			t.Parallel()
			result, err := e.RenderFile(tmplPath, cfg)
			require.NoError(t, err)
			assert.Contains(t, result, "Design Context")
			assert.Contains(t, result, "palette-role drift")
			assert.Contains(t, result, "source-of-truth mismatch")
			assert.Contains(t, result, "auto design docs")
		})
	}
}

func TestFrontendVerifyTemplatesMentionDesignContext(t *testing.T) {
	t.Parallel()
	e := tmpl.New()
	cfg := config.DefaultFullConfig("verify-project")

	paths := []string{
		filepath.Join(templateRoot(), "codex", "skills", "frontend-verify.md.tmpl"),
		filepath.Join(templateRoot(), "gemini", "skills", "frontend-verify", "SKILL.md.tmpl"),
	}

	for _, tmplPath := range paths {
		t.Run(filepath.Base(filepath.Dir(tmplPath))+"-"+filepath.Base(tmplPath), func(t *testing.T) {
			t.Parallel()
			result, err := e.RenderFile(tmplPath, cfg)
			require.NoError(t, err)
			assert.Contains(t, result, "## Design Context")
			assert.Contains(t, result, "Trust: untrusted project data")
			assert.Contains(t, result, "palette-role drift")
			assert.Contains(t, result, "typography hierarchy")
			assert.Contains(t, result, "component guardrail")
			assert.Contains(t, result, "source-of-truth")
			assert.Contains(t, result, "auto design docs")
		})
	}
}

func TestDesignContextExamplesMentionTrustLabel(t *testing.T) {
	t.Parallel()
	e := tmpl.New()
	cfg := config.DefaultFullConfig("trust-project")

	paths := []string{
		filepath.Join(templateRoot(), "codex", "agents", "frontend-specialist.toml.tmpl"),
		filepath.Join(templateRoot(), "codex", "agents", "reviewer.toml.tmpl"),
		filepath.Join(templateRoot(), "gemini", "agents", "frontend-specialist.md.tmpl"),
		filepath.Join(templateRoot(), "gemini", "agents", "reviewer.md.tmpl"),
		filepath.Join(templateRoot(), "claude", "skills", "frontend-verify-report.md.tmpl"),
	}

	for _, tmplPath := range paths {
		t.Run(filepath.Base(filepath.Dir(tmplPath))+"-"+filepath.Base(tmplPath), func(t *testing.T) {
			t.Parallel()
			result, err := e.RenderFile(tmplPath, cfg)
			require.NoError(t, err)
			assert.Contains(t, result, "Trust: untrusted project data")
		})
	}
}

// normalizeTemplateWhitespace collapses the line wrapping used in prompt bodies
// so a contract sentence is matched by its words, not by where it was wrapped.
func normalizeTemplateWhitespace(body string) string {
	return strings.Join(strings.Fields(body), " ")
}
