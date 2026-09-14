package templates_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	tmpl "github.com/insajin/autopus-adk/pkg/template"
)

func TestTechnologyStackFreshnessTemplateContracts(t *testing.T) {
	t.Parallel()

	e := tmpl.New()
	cfg := config.DefaultFullConfig("techstack-project")
	root := templateRoot()
	templatePaths := map[string][]string{
		filepath.Join(root, "..", "content", "rules", "techstack-freshness.md"): {
			"Technology Stack Decision",
			"greenfield",
			"source refs",
			"checked_at",
			"allow_prerelease=true",
			"current intended source state",
			"start and end source SHA",
			"artifact receipts",
			"installed older app",
			"concrete immutable version, SHA, or digest",
			"`latest-stable`",
			"`repo-compatible-pin`",
			"`source-exact`",
			"fail closed",
			"explicitly offline development",
		},
		filepath.Join(root, "shared", "prd-standard.md.tmpl"): {
			"Technology Stack Decision",
			"Resolved versions",
			"Source refs",
			"Checked at",
		},
		filepath.Join(root, "shared", "prd-minimal.md.tmpl"): {
			"Technology Stack Decision",
			"current stable versions",
			"checked-at date",
		},
		filepath.Join(root, "claude", "commands", "auto-router.md.tmpl"): {
			"Technology Stack Decision",
			"techstack-freshness",
			"version/source_ref/checked_at",
		},
		filepath.Join(root, "codex", "prompts", "auto-plan.md.tmpl"): {
			"Technology Stack Decision",
			"techstack-freshness",
			"checked_at",
		},
		filepath.Join(root, "codex", "agents", "executor.toml.tmpl"): {
			"Technology Stack Decision",
			"version/source_ref/checked_at",
			"BLOCKED",
		},
		filepath.Join(root, "gemini", "commands", "auto-router.md.tmpl"): {
			"Technology Stack Decision",
			"techstack-freshness",
		},
		filepath.Join(root, "gemini", "agents", "executor.md.tmpl"): {
			"Technology Stack Decision",
			"version/source_ref/checked_at",
			"BLOCKED",
		},
	}

	for path, expected := range templatePaths {
		path, expected := path, expected
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			result := renderOrReadTechstackContract(t, e, cfg, path)
			for _, phrase := range expected {
				assert.Contains(t, result, phrase)
			}
		})
	}
}

func renderOrReadTechstackContract(t *testing.T, e *tmpl.Engine, cfg *config.HarnessConfig, path string) string {
	t.Helper()
	result, err := semanticContractSurface(e, path, cfg)
	require.NoError(t, err)
	return result
}

// The stack-metadata fields the documentation step must preserve moved into the
// pipeline's phase resource. They are identifiers a greenfield manifest depends
// on, so they are checked where they now live instead of in every prompt body.
func TestTechnologyStackFreshnessResourceKeepsVersionEvidence(t *testing.T) {
	t.Parallel()

	surface := pipelineResourceSurface(t)
	for _, field := range []string{"version", "source_ref", "checked_at"} {
		assert.Contains(t, surface, field,
			"the pipeline resources must preserve the stack evidence field %q", field)
	}
}
