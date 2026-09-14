package templates_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	tmpl "github.com/insajin/autopus-adk/pkg/template"
)

// A risk-first probe recorded on one platform and ignored on another is worse
// than no probe: the plan promises a pre-fan-out gate that the other platform
// never runs. Every plan / go / spec-writer / agent-pipeline surface must carry
// the probe section name, the Phase 1.9 gate that consumes it, the honest
// `not-run` status, and the `not_applicable` applicability value.
func TestRiskFirstProbeSurfaceParity(t *testing.T) {
	t.Parallel()

	root := templateRoot()
	e := tmpl.New()
	cfg := config.DefaultFullConfig("probe-project")
	probeTokens := []string{
		"Risk-First Integration Probe",
		"Phase 1.9",
		"not_applicable",
		"not-run",
	}

	cases := []struct {
		name   string
		path   string
		tokens []string
	}{
		{
			name: "spec-writer-content",
			path: filepath.Join(root, "..", "content", "agents", "spec-writer.md"),
		},
		{
			name: "codex-spec-writer-agent",
			path: filepath.Join(root, "codex", "agents", "spec-writer.toml.tmpl"),
		},
		{
			name: "gemini-spec-writer-agent",
			path: filepath.Join(root, "gemini", "agents", "spec-writer.md.tmpl"),
		},
		{
			name: "claude-workflows",
			path: filepath.Join(root, "claude", "commands", "auto-workflows.md.tmpl"),
		},
		{
			name: "codex-plan-skill",
			path: filepath.Join(root, "codex", "skills", "auto-plan.md.tmpl"),
		},
		{
			name: "codex-plan-prompt",
			path: filepath.Join(root, "codex", "prompts", "auto-plan.md.tmpl"),
		},
		{
			name: "gemini-plan-skill",
			path: filepath.Join(root, "gemini", "skills", "auto-plan", "SKILL.md.tmpl"),
		},
		{
			name: "codex-go-skill",
			path: filepath.Join(root, "codex", "skills", "auto-go.md.tmpl"),
		},
		{
			name: "codex-go-prompt",
			path: filepath.Join(root, "codex", "prompts", "auto-go.md.tmpl"),
		},
		{
			name: "gemini-go-skill",
			path: filepath.Join(root, "gemini", "skills", "auto-go", "SKILL.md.tmpl"),
		},
		{
			name: "agent-pipeline-content",
			path: filepath.Join(root, "..", "content", "skills", "agent-pipeline.md"),
		},
		{
			name: "gemini-agent-pipeline-template",
			path: filepath.Join(root, "gemini", "skills", "agent-pipeline", "SKILL.md.tmpl"),
		},
		{
			// The OMP entrypoint is a decision layer: it must reach the probe
			// contract, which the resource test below verifies in full.
			name:   "omp-agent-pipeline-template",
			path:   filepath.Join(root, "shared", "omp-agent-pipeline.md.tmpl"),
			tokens: []string{"references/phases.md"},
		},
		{
			name:   "spec-quality-rule",
			path:   filepath.Join(root, "..", "content", "rules", "spec-quality.md"),
			tokens: []string{"Risk-First Integration Probe", "Q-COMP-08", "not-run", "not_applicable"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tokens := tc.tokens
			if len(tokens) == 0 {
				tokens = probeTokens
			}
			text, err := semanticContractSurface(e, tc.path, cfg)
			require.NoError(t, err)
			for _, token := range tokens {
				assert.Contains(t, text, token, "%s should contain %q", tc.path, token)
			}
			// `reusable` is no longer disclaimed here: the applicability
			// vocabulary is asserted by TestPipelineRuntimeSurfaceParity, which
			// requires every go/pipeline surface to describe the receipt that
			// grants it.
		})
	}
}

// The row-by-row probe contract lives in the retrievable phase resource, so a
// surface that routes there must find it complete.
func TestRiskFirstProbeResourceCarriesTheRowContract(t *testing.T) {
	t.Parallel()

	surface := pipelineResourceSurface(t)
	for _, token := range []string{
		"Risk-First Integration Probe", "Phase 1.9", "not_applicable", "not-run",
		"assumption_id", "evidence ref",
	} {
		assert.Contains(t, surface, token,
			"the pipeline resources should contain the probe contract token %q", token)
	}
}
