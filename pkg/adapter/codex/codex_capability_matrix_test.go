package codex

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodexCapabilityMatrixProjectsEveryConsumer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		catalog          string
		rendered         config.CodexProfile
		provider         config.CodexProfile
		providerReason   config.CodexResolutionReason
		supervisor       config.CodexProfile
		supervisorReason config.CodexResolutionReason
		agent            config.CodexProfile
		agentReason      config.CodexResolutionReason
	}{
		{
			name: "full support",
			catalog: `{"models":[
				{"slug":"gpt-6-astra","supported_reasoning_levels":[{"effort":"xhigh"},{"effort":"max"},{"effort":"ultra"}]},
				{"slug":"gpt-6.1-sol","supported_reasoning_levels":[{"effort":"xhigh"}]},
				{"slug":"gpt-5.5","supported_reasoning_levels":[{"effort":"xhigh"}]}
			]}`,
			rendered:         config.CodexProfile{Model: config.CodexAstraModel, Effort: config.CodexEffortUltra},
			provider:         config.CodexProfile{Model: config.CodexAstraModel, Effort: config.CodexEffortMax},
			providerReason:   config.CodexResolutionSupported,
			supervisor:       config.CodexProfile{Model: config.CodexAstraModel, Effort: config.CodexEffortUltra},
			supervisorReason: config.CodexResolutionSupported,
			agent:            config.CodexProfile{Model: config.CodexSolModel, Effort: config.CodexEffortXHigh},
			agentReason:      config.CodexResolutionSupported,
		},
		{
			name: "effort downgrade",
			catalog: `{"models":[
				{"slug":"gpt-6-astra","supported_reasoning_levels":[{"effort":"xhigh"},{"effort":"max"}]},
				{"slug":"gpt-6.1-sol","supported_reasoning_levels":[{"effort":"xhigh"}]},
				{"slug":"gpt-5.5","supported_reasoning_levels":[{"effort":"xhigh"}]}
			]}`,
			rendered:         config.CodexProfile{Model: config.CodexAstraModel, Effort: config.CodexEffortMax},
			provider:         config.CodexProfile{Model: config.CodexAstraModel, Effort: config.CodexEffortMax},
			providerReason:   config.CodexResolutionSupported,
			supervisor:       config.CodexProfile{Model: config.CodexAstraModel, Effort: config.CodexEffortMax},
			supervisorReason: config.CodexResolutionEffortUnavailable,
			agent:            config.CodexProfile{Model: config.CodexSolModel, Effort: config.CodexEffortXHigh},
			agentReason:      config.CodexResolutionSupported,
		},
		{
			name:             "runtime default",
			catalog:          `{"models":[{"slug":"other-model","supported_reasoning_levels":[{"effort":"medium"}]}]}`,
			providerReason:   config.CodexResolutionRuntimeDefault,
			supervisorReason: config.CodexResolutionRuntimeDefault,
			agentReason:      config.CodexResolutionRuntimeDefault,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			a := NewWithRoot(t.TempDir())
			a.codexCatalogProbed = true
			a.codexCatalogJSON = []byte(tt.catalog)
			a.codexFallbackWriter = nil
			cfg := config.DefaultFullConfig("capability-matrix")
			cfg.Quality.Default = "ultra"
			cfg.Quality.SupervisorModelPolicy = "quality"

			rootFiles, err := a.prepareConfigFile(cfg)
			require.NoError(t, err)
			agentFiles, err := a.generateAgents(cfg)
			require.NoError(t, err)
			root := strings.SplitN(string(rootFiles[0].Content), "[agents]", 2)[0]
			executor := codexAgentMappingContent(t, agentFiles, "executor.toml")
			assertCodexRenderedProfile(t, root, tt.rendered)
			assertCodexRenderedProfile(t, executor, tt.agent)

			provider, providerResolution := config.ResolveCodexProviderProfile(
				config.CodexProviderEntryForQuality(cfg.Quality),
				[]byte(tt.catalog),
			)
			assertCodexProviderProfile(t, provider.Args, tt.provider)
			assert.Equal(t, tt.providerReason, providerResolution.Reason)

			rootResolution := config.ResolveCodexProfile(cfg.Quality.CodexSupervisorProfile(), []byte(tt.catalog))
			agentResolution := config.ResolveCodexProfile(cfg.Quality.CodexAgentProfile("executor", "sonnet", "medium"), []byte(tt.catalog))
			assert.Equal(t, tt.supervisorReason, rootResolution.Reason)
			assert.Equal(t, tt.agentReason, agentResolution.Reason)
			assert.Equal(t, tt.supervisor, rootResolution.Effective)
			assert.Equal(t, tt.agent, agentResolution.Effective)
		})
	}
}

func codexAgentMappingContent(t *testing.T, files []adapter.FileMapping, name string) string {
	t.Helper()
	for _, file := range files {
		if filepath.Base(file.TargetPath) == name {
			return string(file.Content)
		}
	}
	require.FailNow(t, "managed agent mapping not found", name)
	return ""
}

func assertCodexRenderedProfile(t *testing.T, content string, profile config.CodexProfile) {
	t.Helper()
	var modelLine, effortLine string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "model = ") {
			modelLine = trimmed
		}
		if strings.HasPrefix(trimmed, "model_reasoning_effort = ") {
			effortLine = trimmed
		}
	}
	if profile.Model == "" {
		assert.Empty(t, modelLine)
		assert.Empty(t, effortLine)
		return
	}
	assert.Equal(t, `model = "`+profile.Model+`"`, modelLine)
	assert.Equal(t, `model_reasoning_effort = "`+profile.Effort+`"`, effortLine)
}

func assertCodexProviderProfile(t *testing.T, args []string, profile config.CodexProfile) {
	t.Helper()
	joined := strings.Join(args, "\x00")
	if profile.Model == "" {
		assert.NotContains(t, joined, "-m")
		assert.NotContains(t, joined, "model_reasoning_effort")
		return
	}
	assert.Contains(t, joined, profile.Model)
	assert.Contains(t, joined, `model_reasoning_effort="`+profile.Effort+`"`)
}
