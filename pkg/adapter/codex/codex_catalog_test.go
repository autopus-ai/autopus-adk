package codex

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateConfig_CatalogDowngradesEffortOnSameModel(t *testing.T) {
	t.Parallel()

	a := NewWithRoot(t.TempDir())
	a.codexCatalogProbed = true
	a.codexCatalogJSON = []byte(`{"models":[{"slug":"gpt-6-astra","supported_reasoning_levels":[{"effort":"xhigh"},{"effort":"max"}]}]}`)
	var warnings bytes.Buffer
	a.codexFallbackWriter = &warnings
	cfg := config.DefaultFullConfig("catalog-project")
	cfg.Quality.Default = "ultra"
	cfg.Quality.SupervisorModelPolicy = "quality"

	files, err := a.prepareConfigFile(cfg)
	require.NoError(t, err)
	root := strings.SplitN(string(files[0].Content), "[agents]", 2)[0]
	assert.Contains(t, root, `model = "gpt-6-astra"`)
	assert.Contains(t, root, `model_reasoning_effort = "max"`)
	assert.Contains(t, warnings.String(), "requested=gpt-6-astra/ultra")
	assert.Contains(t, warnings.String(), "selected=gpt-6-astra/max")
	assert.Contains(t, warnings.String(), "reason=effort_unavailable")
	assert.Equal(t, 1, strings.Count(warnings.String(), "reason=effort_unavailable"))
}

func TestGenerateConfig_CatalogFallsBackToPreviousSol(t *testing.T) {
	t.Parallel()

	a := NewWithRoot(t.TempDir())
	a.codexCatalogProbed = true
	a.codexCatalogJSON = []byte(`{"models":[{"slug":"gpt-5.6-sol","supported_reasoning_levels":[{"effort":"xhigh"}]},{"slug":"gpt-5.5","supported_reasoning_levels":[{"effort":"xhigh"}]}]}`)
	cfg := config.DefaultFullConfig("legacy-project")
	cfg.Quality.Default = "ultra"
	cfg.Quality.SupervisorModelPolicy = "quality"

	files, err := a.prepareConfigFile(cfg)
	require.NoError(t, err)
	root := strings.SplitN(string(files[0].Content), "[agents]", 2)[0]
	assert.Contains(t, root, `model = "gpt-5.6-sol"`)
	assert.Contains(t, root, `model_reasoning_effort = "xhigh"`)
}

func TestGenerateConfig_CatalogUnknownUsesFallbackModel(t *testing.T) {
	t.Parallel()

	a := NewWithRoot(t.TempDir())
	a.codexCatalogProbed = true
	var warnings bytes.Buffer
	a.codexFallbackWriter = &warnings
	cfg := config.DefaultFullConfig("unknown-catalog-project")
	cfg.Quality.Default = "ultra"
	cfg.Quality.SupervisorModelPolicy = "quality"

	files, err := a.prepareConfigFile(cfg)
	require.NoError(t, err)
	root := strings.SplitN(string(files[0].Content), "[agents]", 2)[0]
	assert.Contains(t, root, `model = "gpt-5.6-sol"`)
	assert.Contains(t, root, `model_reasoning_effort = "ultra"`)
	assert.Contains(t, warnings.String(), "reason=catalog_unknown")
}

func TestGenerateConfig_CatalogUsesRuntimeDefaultWhenNoCompatibleModel(t *testing.T) {
	t.Parallel()

	a := NewWithRoot(t.TempDir())
	a.codexCatalogProbed = true
	a.codexCatalogJSON = []byte(`{"models":[{"slug":"other-model","supported_reasoning_levels":[{"effort":"medium"}]}]}`)
	cfg := config.DefaultFullConfig("runtime-default-project")

	files, err := a.prepareConfigFile(cfg)
	require.NoError(t, err)
	root := strings.SplitN(string(files[0].Content), "[agents]", 2)[0]
	assert.NotContains(t, root, "\nmodel =")
	assert.NotContains(t, root, "model_reasoning_effort")
}

func TestGenerateConfig_RuntimeDefaultStillPreservesUserModel(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	a := NewWithRoot(dir)
	a.codexCatalogProbed = true
	a.codexCatalogJSON = []byte(`{"models":[{"slug":"other-model","supported_reasoning_levels":[{"effort":"medium"}]}]}`)
	configPath := filepath.Join(dir, ".codex", "config.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0755))
	require.NoError(t, os.WriteFile(configPath, []byte("model = \"custom-model\"\nmodel_reasoning_effort = \"ultra\"\n"), 0644))

	files, err := a.prepareConfigFile(config.DefaultFullConfig("preserve-project"))
	require.NoError(t, err)
	root := strings.SplitN(string(files[0].Content), "[agents]", 2)[0]
	assert.Contains(t, root, `model = "custom-model"`)
	assert.Contains(t, root, `model_reasoning_effort = "ultra"`)
}

func TestCodexRenderContext_ResolvesAgentModelWithDeclaredEffort(t *testing.T) {
	t.Parallel()

	a := NewWithRoot(t.TempDir())
	a.codexCatalogProbed = true
	a.codexCatalogJSON = []byte(`{"models":[{"slug":"gpt-6-luna","supported_reasoning_levels":[{"effort":"medium"},{"effort":"max"}]}]}`)
	var warnings bytes.Buffer
	a.codexFallbackWriter = &warnings
	cfg := config.DefaultFullConfig("tuple-project")
	data := codexRenderContext{HarnessConfig: cfg, adapter: a}

	// A non-canonical agent has no native balanced placement, so its declared
	// tier still decides the tuple; the sonnet tier runs Luna at max.
	model, err := data.CodexAgentModel("synthetic", "sonnet", "medium")
	require.NoError(t, err)
	effort, err := data.CodexAgentEffort("synthetic", "sonnet", "medium")
	require.NoError(t, err)

	assert.Equal(t, config.CodexLunaModel, model)
	assert.Equal(t, config.CodexEffortMax, effort)
	assert.Empty(t, warnings.String())
}

func TestGenerateAgents_AppliesCatalogFallbackProfiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		catalog       string
		wantModel     string
		wantEffort    string
		wantReason    string
		wantOmissions bool
	}{
		{
			name:       "previous Sol fallback",
			catalog:    `{"models":[{"slug":"gpt-5.6-sol","supported_reasoning_levels":[{"effort":"max"}]},{"slug":"gpt-5.5","supported_reasoning_levels":[{"effort":"xhigh"}]}]}`,
			wantModel:  config.CodexPreviousSolModel,
			wantEffort: config.CodexEffortMax,
			wantReason: "model_unavailable",
		},
		{
			name:       "effort downgrade",
			catalog:    `{"models":[{"slug":"gpt-6-astra","supported_reasoning_levels":[{"effort":"xhigh"}]}]}`,
			wantModel:  config.CodexAstraModel,
			wantEffort: config.CodexEffortXHigh,
			wantReason: "effort_unavailable",
		},
		{
			name:          "runtime default",
			catalog:       `{"models":[{"slug":"other-model","supported_reasoning_levels":[{"effort":"medium"}]}]}`,
			wantReason:    "runtime_default",
			wantOmissions: true,
		},
		{
			name:       "catalog unknown",
			wantModel:  config.CodexFallbackModel,
			wantEffort: config.CodexEffortMax,
			wantReason: "catalog_unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			a := NewWithRoot(t.TempDir())
			a.codexCatalogProbed = true
			a.codexCatalogJSON = []byte(tt.catalog)
			var warnings bytes.Buffer
			a.codexFallbackWriter = &warnings
			cfg := config.DefaultFullConfig("agent-fallback-project")
			cfg.Quality.Default = "ultra"

			files, err := a.generateAgents(cfg)
			require.NoError(t, err)
			var planner string
			for _, file := range files {
				if file.TargetPath == filepath.Join(".codex", "agents", "planner.toml") {
					planner = string(file.Content)
					break
				}
			}
			require.NotEmpty(t, planner)
			if tt.wantOmissions {
				assert.NotContains(t, planner, "\nmodel =")
				assert.NotContains(t, planner, "model_reasoning_effort")
				assert.Contains(t, warnings.String(), "reason="+tt.wantReason)
				return
			}
			assert.Contains(t, planner, `model = "`+tt.wantModel+`"`)
			assert.Contains(t, planner, `model_reasoning_effort = "`+tt.wantEffort+`"`)
			assert.Contains(t, warnings.String(), "reason="+tt.wantReason)
		})
	}
}
