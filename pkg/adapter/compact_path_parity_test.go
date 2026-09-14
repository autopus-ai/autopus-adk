package adapter_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	contentfs "github.com/insajin/autopus-adk/content"
)

// A platform whose route surface loses the compact-path policy silently sends
// its agents back to authoring a four-document SPEC set for test-only or
// small-UI work. These lists pin that regression per platform.
//
// compactPathAuthoringSources author or refuse the contract, so they must name
// the command that writes it.
var compactPathAuthoringSources = []string{
	"templates/claude/commands/auto-workflows.md.tmpl",
	"templates/codex/skills/auto-plan.md.tmpl",
	"templates/codex/skills/auto-fix.md.tmpl",
	"templates/codex/prompts/auto-plan.md.tmpl",
	"templates/codex/prompts/auto-fix.md.tmpl",
	"templates/gemini/skills/auto-plan/SKILL.md.tmpl",
	"templates/gemini/skills/auto-fix/SKILL.md.tmpl",
	"templates/gemini/skills/agent-pipeline/SKILL.md.tmpl",
}

// compactPathExecutionSources consume an existing contract, so they must name
// the declared-class flag and the contract document they read.
var compactPathExecutionSources = []string{
	"templates/claude/commands/auto-workflows.md.tmpl",
	"templates/codex/skills/auto-go.md.tmpl",
	"templates/codex/prompts/auto-go.md.tmpl",
	"templates/gemini/skills/auto-go/SKILL.md.tmpl",
}

func compactPathSources() []string {
	return append(append([]string{}, compactPathAuthoringSources...), compactPathExecutionSources...)
}

func repoRelativeFile(t *testing.T, rel string) string {
	t.Helper()
	path := filepath.Join("..", "..", filepath.FromSlash(rel))
	body, err := os.ReadFile(path)
	require.NoError(t, err, "missing surface %s", rel)
	return string(body)
}

func TestCompactPath_EveryRouteSurfaceDescribesTheLowRiskDefault(t *testing.T) {
	t.Parallel()

	for _, rel := range compactPathSources() {
		body := repoRelativeFile(t, rel)
		for _, claim := range []string{"escalate_to_full_spec", "bugfix_existing_contract"} {
			assert.Contains(t, body, claim, "%s omits %q", rel, claim)
		}
	}
	for _, rel := range compactPathAuthoringSources {
		assert.Contains(t, repoRelativeFile(t, rel), "auto spec change",
			"%s omits the command that records the compact contract", rel)
	}
	for _, rel := range compactPathExecutionSources {
		body := repoRelativeFile(t, rel)
		assert.Contains(t, body, "--change-class", "%s omits the declared-class flag", rel)
		assert.Contains(t, body, "change.md", "%s omits the contract document it must read", rel)
	}
}

// The pipeline surfaces additionally carry the gate catalog and the merged
// final verification contract, because they are what a supervisor reads.
func TestCompactPath_PipelineSurfacesCarryAuthoringGateAndMergedVerification(t *testing.T) {
	t.Parallel()

	pipeline, err := contentfs.FS.ReadFile("skills/agent-pipeline.md")
	require.NoError(t, err)
	surfaces := map[string]string{
		"content/skills/agent-pipeline.md":                     string(pipeline),
		"templates/gemini/skills/agent-pipeline/SKILL.md.tmpl": repoRelativeFile(t, "templates/gemini/skills/agent-pipeline/SKILL.md.tmpl"),
	}
	for name, body := range surfaces {
		assert.Contains(t, body, "spec_authoring", "%s omits the authoring gate", name)
		assert.Contains(t, body, "awaiting_changes", "%s omits the review-loop terminal status", name)
		for _, retained := range []string{"security", "validation", "data_loss", "deterministic_oracle"} {
			assert.Contains(t, body, retained, "%s stopped naming the retained safety gate %q", name, retained)
		}
	}
}

// An entrypoint that defers the compact-path detail must still reach it. The
// contract itself is checked in the resource, not duplicated in every body:
// duplicating it is exactly what let the two copies drift apart before.
func TestCompactPath_DeferringEntrypointsReachTheGateResource(t *testing.T) {
	t.Parallel()

	for _, rel := range []string{
		"templates/shared/omp-agent-pipeline.md.tmpl",
		"content/skills/agent-pipeline.md",
	} {
		body := repoRelativeFile(t, rel)
		assert.Contains(t, body, "references/gates.md",
			"%s must route to the retrievable gate/change-class contract", rel)
	}

	gates, err := contentfs.FS.ReadFile("skills/references/agent-pipeline/gates.md")
	require.NoError(t, err)
	for _, claim := range []string{
		"escalate_to_full_spec", "bugfix_existing_contract", "auto spec change",
		"spec_authoring", "security", "validation", "data_loss", "deterministic_oracle",
	} {
		assert.Contains(t, string(gates), claim, "the gate resource omits %q", claim)
	}

	review, err := contentfs.FS.ReadFile("skills/references/agent-pipeline/review.md")
	require.NoError(t, err)
	assert.Contains(t, string(review), "awaiting_changes",
		"the review resource omits the loop terminal status")
}

// The gate catalog listing is a closed set; a platform that lists it must list
// spec_authoring too, or its workers will treat the decision as unknown.
func TestCompactPath_GateCatalogListingsIncludeSpecAuthoring(t *testing.T) {
	t.Parallel()

	const marker = "risk_first_probe, build, unit_tests"
	for _, rel := range append([]string{
		"content/skills/agent-pipeline.md",
		"content/rules/spec-quality.md",
	}, compactPathSources()...) {
		body := repoRelativeFile(t, rel)
		index := strings.Index(body, marker)
		if index < 0 {
			continue
		}
		assert.Contains(t, body[:index], "spec_authoring",
			"%s lists the gate catalog without spec_authoring", rel)
	}
}
