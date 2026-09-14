package templates_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	tmpl "github.com/insajin/autopus-adk/pkg/template"
)

// A go/pipeline surface that names a runtime on one platform and omits it on
// another is a silent behavioral fork: the supervisor on the quiet platform
// re-runs verified work, self-assigns applicability, screenshots in no-capture
// mode, re-discovers the same findings, and reports no lead time. Every surface
// that drives `go` must name all four runtimes the pipeline now consumes.
func TestPipelineRuntimeSurfaceParity(t *testing.T) {
	t.Parallel()

	root := templateRoot()
	e := tmpl.New()
	cfg := config.DefaultFullConfig("pipeline-runtime-project")
	runtimeTokens := []string{
		// gate applicability + exact-input evidence reuse
		"auto spec gates",
		"gate-applicability.json",
		"reusable",
		// no-capture UX verification
		"no-capture",
		// repeat-discovery re-review
		"discovery_repeat_detected",
		// lead-time telemetry
		"first_vertical_slice",
		"auto telemetry leadtime",
	}

	// Route surfaces drive a run end to end, so they name every runtime the
	// pipeline consumes. Pipeline entrypoints are decision layers: they name the
	// classifier command and route to the detail, which is asserted on the
	// resources below rather than duplicated into every body.
	routeCases := []struct {
		name string
		path string
	}{
		{
			name: "claude-workflows",
			path: filepath.Join(root, "claude", "commands", "auto-workflows.md.tmpl"),
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
	}
	entrypointCases := []struct {
		name string
		path string
	}{
		{
			name: "agent-pipeline-content",
			path: filepath.Join(root, "..", "content", "skills", "agent-pipeline.md"),
		},
		{
			name: "gemini-agent-pipeline-template",
			path: filepath.Join(root, "gemini", "skills", "agent-pipeline", "SKILL.md.tmpl"),
		},
		{
			name: "omp-agent-pipeline-template",
			path: filepath.Join(root, "shared", "omp-agent-pipeline.md.tmpl"),
		},
	}

	for _, tc := range routeCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			text, err := semanticContractSurface(e, tc.path, cfg)
			require.NoError(t, err)
			for _, token := range runtimeTokens {
				assert.Contains(t, text, token, "%s should contain %q", tc.path, token)
			}
		})
	}

	for _, tc := range entrypointCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			text, err := semanticContractSurface(e, tc.path, cfg)
			require.NoError(t, err)
			assert.Contains(t, text, "auto spec gates",
				"%s must name the classifier that decides applicability", tc.path)
			assert.Contains(t, text, "references/gates.md",
				"%s must route to the gate and telemetry detail", tc.path)
		})
	}

	t.Run("pipeline-resources", func(t *testing.T) {
		t.Parallel()
		surface := pipelineResourceSurface(t)
		for _, token := range runtimeTokens {
			assert.Contains(t, surface, token,
				"the retrievable pipeline resources should contain %q", token)
		}
	})
}

// The applicability vocabulary is a closed set shared by the classifier and
// every prompt surface. A surface that keeps the pre-receipt wording tells the
// supervisor that reuse is impossible, so it re-runs evidence the classifier
// already accepted.
func TestPipelineRuntimeSurfaceDropsReusableDisclaimer(t *testing.T) {
	t.Parallel()

	root := templateRoot()
	e := tmpl.New()
	cfg := config.DefaultFullConfig("pipeline-runtime-project")

	paths := []string{
		filepath.Join(root, "..", "content", "skills", "agent-pipeline.md"),
		filepath.Join(root, "..", "content", "agents", "spec-writer.md"),
		filepath.Join(root, "gemini", "skills", "agent-pipeline", "SKILL.md.tmpl"),
		filepath.Join(root, "shared", "omp-agent-pipeline.md.tmpl"),
		filepath.Join(root, "claude", "commands", "auto-workflows.md.tmpl"),
		filepath.Join(root, "codex", "skills", "auto-go.md.tmpl"),
		filepath.Join(root, "codex", "prompts", "auto-go.md.tmpl"),
		filepath.Join(root, "gemini", "skills", "auto-go", "SKILL.md.tmpl"),
		filepath.Join(root, "codex", "skills", "auto-plan.md.tmpl"),
		filepath.Join(root, "codex", "prompts", "auto-plan.md.tmpl"),
		filepath.Join(root, "gemini", "skills", "auto-plan", "SKILL.md.tmpl"),
	}

	for _, path := range paths {
		path := path
		t.Run(filepath.Base(filepath.Dir(path))+"/"+filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			text, err := semanticContractSurface(e, path, cfg)
			require.NoError(t, err)
			for _, stale := range []string{
				"not a valid value in this release",
				"유효한 값이 아닙니다",
				"no exact-input evidence engine exists",
			} {
				assert.NotContains(t, text, stale,
					"%s still disclaims `reusable`; the evidence engine now grants it", path)
			}
		})
	}
}
