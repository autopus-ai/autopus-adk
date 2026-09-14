package templates_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	tmpl "github.com/insajin/autopus-adk/pkg/template"
)

func TestQAMESHGuidanceSourceContracts(t *testing.T) {
	t.Parallel()

	root := templateRoot()
	staticPaths := []string{
		filepath.Join(root, "..", "content", "skills", "testing-strategy.md"),
		filepath.Join(root, "codex", "prompts", "auto-qa.md.tmpl"),
		filepath.Join(root, "codex", "skills", "auto-qa.md.tmpl"),
		filepath.Join(root, "gemini", "commands", "auto", "qa.toml.tmpl"),
		filepath.Join(root, "gemini", "skills", "auto-qa", "SKILL.md.tmpl"),
	}
	for _, path := range staticPaths {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			body, err := os.ReadFile(path)
			require.NoError(t, err)
			assertQAMESHGuidance(t, string(body))
		})
	}
}

func TestQAMESHRouterTemplateGuidance(t *testing.T) {
	t.Parallel()

	e := tmpl.New()
	cfg := config.DefaultFullConfig("qa-project")
	paths := []string{
		filepath.Join(templateRoot(), "claude", "commands", "auto-router.md.tmpl"),
		filepath.Join(templateRoot(), "codex", "prompts", "auto.md.tmpl"),
		filepath.Join(templateRoot(), "gemini", "commands", "auto-router.md.tmpl"),
	}
	for _, tmplPath := range paths {
		tmplPath := tmplPath
		t.Run(filepath.Base(filepath.Dir(tmplPath))+"-"+filepath.Base(tmplPath), func(t *testing.T) {
			t.Parallel()
			result, err := semanticContractSurface(e, tmplPath, cfg)
			require.NoError(t, err)
			assertQAMESHGuidance(t, result)
		})
	}
}

// TestQAMESHVisualReportGuidance keeps the human-facing evidence report
// discoverable on every detailed QA surface. Router surfaces only need to reach
// the `qa` namespace, so they are intentionally excluded.
func TestQAMESHVisualReportGuidance(t *testing.T) {
	t.Parallel()

	root := templateRoot()
	paths := []string{
		filepath.Join(root, "claude", "commands", "auto-workflows.md.tmpl"),
		filepath.Join(root, "codex", "prompts", "auto-qa.md.tmpl"),
		filepath.Join(root, "codex", "skills", "auto-qa.md.tmpl"),
		filepath.Join(root, "gemini", "commands", "auto", "qa.toml.tmpl"),
		filepath.Join(root, "gemini", "skills", "auto-qa", "SKILL.md.tmpl"),
	}
	for _, path := range paths {
		path := path
		t.Run(filepath.Base(filepath.Dir(path))+"-"+filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			body, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Contains(t, string(body), "auto qa report")
			assert.Contains(t, string(body), "report.html")
		})
	}
}

// TestQAMESHCaptureContractGuidance keeps the typed GUI capture contract and its
// publication boundary documented wherever the capture policy is authored, so an
// agent cannot invent artifact kinds the harness no longer accepts.
func TestQAMESHCaptureContractGuidance(t *testing.T) {
	t.Parallel()

	root := templateRoot()
	paths := []string{
		filepath.Join(root, "claude", "commands", "auto-workflows.md.tmpl"),
		filepath.Join(root, "codex", "prompts", "auto-qa.md.tmpl"),
		filepath.Join(root, "codex", "skills", "auto-qa.md.tmpl"),
		filepath.Join(root, "gemini", "skills", "auto-qa", "SKILL.md.tmpl"),
	}
	tokens := []string{
		"gui.capture",
		"capture-index.json",
		"capture_index",
		"AUTOPUS_QAMESH_GUI_CAPTURE_",
		"gui-capture-contract",
		"local-redacted-local-media",
		"guard receipt",
		"--embed-media",
		".autopus/qa/capture/README.md",
		"unenforceable_forbidden_actions",
		"network_stopped",
	}
	for _, path := range paths {
		path := path
		t.Run(filepath.Base(filepath.Dir(path))+"-"+filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			body, err := os.ReadFile(path)
			require.NoError(t, err)
			for _, token := range tokens {
				assert.Contains(t, string(body), token)
			}
		})
	}
}

// TestQAMESHScenarioContractGuidance keeps the scenario authoring contract
// documented wherever QA commands are authored. The load-bearing facts are that
// the harness does not invent assertions and that the step vocabulary is closed:
// an agent that believes it can emit a click step would author scenarios the
// compiler rejects.
func TestQAMESHScenarioContractGuidance(t *testing.T) {
	t.Parallel()

	root := templateRoot()
	paths := []string{
		filepath.Join(root, "claude", "commands", "auto-workflows.md.tmpl"),
		filepath.Join(root, "codex", "prompts", "auto-qa.md.tmpl"),
		filepath.Join(root, "codex", "skills", "auto-qa.md.tmpl"),
		filepath.Join(root, "gemini", "skills", "auto-qa", "SKILL.md.tmpl"),
	}
	tokens := []string{
		"auto qa scenario",
		".autopus/qa/scenarios",
		"qamesh.scenario.v1",
		"expect_role",
		"expect_count",
		"autopus-screen",
		"screen_ref",
		"gui.screen_matrix",
		"allowed_origins[0]",
	}
	for _, path := range paths {
		path := path
		t.Run(filepath.Base(filepath.Dir(path))+"-"+filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			body, err := os.ReadFile(path)
			require.NoError(t, err)
			for _, token := range tokens {
				assert.Contains(t, string(body), token)
			}
		})
	}
}

func TestAutoGoQAMESHScopeBudgetGuidance(t *testing.T) {
	t.Parallel()

	e := tmpl.New()
	cfg := config.DefaultFullConfig("qa-project")
	root := templateRoot()
	// Route surfaces carry the full budget rule because they run the lanes.
	paths := []string{
		filepath.Join(root, "claude", "commands", "auto-router.md.tmpl"),
		filepath.Join(root, "codex", "prompts", "auto-go.md.tmpl"),
		filepath.Join(root, "codex", "skills", "auto-go.md.tmpl"),
		filepath.Join(root, "gemini", "commands", "auto-router.md.tmpl"),
		filepath.Join(root, "gemini", "skills", "auto-go", "SKILL.md.tmpl"),
	}
	for _, path := range paths {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			body, err := renderOrReadTemplate(e, path, cfg)
			require.NoError(t, err)
			assert.Contains(t, body, "affected/fast/smoke QAMESH")
			assert.Contains(t, body, "auto qa plan --lane fast --format json")
			assert.Contains(t, body, "full GUI/native/release matrix")
			assert.Contains(t, body, "auto canary")
			assert.Contains(t, body, "post-deploy smoke/status")
		})
	}

	// The pipeline entrypoint defers the lane budget to its verification
	// resource, which must still name the commands that bound it.
	t.Run("pipeline-resources", func(t *testing.T) {
		t.Parallel()
		surface := pipelineResourceSurface(t)
		assert.Contains(t, surface, "auto qa plan --lane fast --format json",
			"the verification resource must name the lane-planning command")
		assert.Contains(t, surface, "auto canary",
			"the verification resource must keep canary as a post-deploy gate")
		assert.Contains(t, surface, "auto qa init --local-only --format json",
			"the verification resource must keep the local-only scaffold escape hatch")
	})
}

func assertQAMESHGuidance(t *testing.T, body string) {
	t.Helper()
	assert.Contains(t, body, "QAMESH")
	assert.Contains(t, body, "auto qa init")
	assert.Contains(t, body, "auto qa plan")
	assert.Contains(t, body, "auto qa run")
	assert.Contains(t, body, "auto qa explore")
	assert.Contains(t, body, "auto qa release")
	assert.Contains(t, body, "auto qa evidence")
	assert.Contains(t, body, "auto qa feedback")
	assert.Contains(t, body, "ADK is a harness")
	assert.Contains(t, body, "project-local Journey Pack")
	assert.Contains(t, body, "QAMESH is the default project QA orchestration layer")
	assert.Contains(t, body, "Playwright")
	assert.Contains(t, body, "not a competing")
	assert.Contains(t, body, "choose between QAMESH and Playwright")
	assert.Contains(t, body, "canary-explicit")
	assert.Contains(t, body, "post-deploy smoke")
}

func renderOrReadTemplate(e *tmpl.Engine, path string, cfg *config.HarnessConfig) (string, error) {
	return semanticContractSurface(e, path, cfg)
}
