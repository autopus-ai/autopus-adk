package codex

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestGenerateCodexSurfacesUseCodexProviderOverride(t *testing.T) {
	t.Parallel()

	// tester takes the native Luna/max placement under balanced and the Sol
	// ladder rung under ultra, while planner anchors on Astra in both.
	tests := []struct {
		name              string
		global            string
		codex             string
		wantRootEffort    string
		wantPlannerEffort string
		wantMidModel      string
		wantMidEffort     string
	}{
		{
			name:              "global ultra codex balanced",
			global:            "ultra",
			codex:             "balanced",
			wantRootEffort:    config.CodexEffortXHigh,
			wantPlannerEffort: config.CodexEffortMax,
			wantMidModel:      config.CodexLunaModel,
			wantMidEffort:     config.CodexEffortMax,
		},
		{
			name:              "global balanced codex ultra",
			global:            "balanced",
			codex:             "ultra",
			wantRootEffort:    config.CodexEffortUltra,
			wantPlannerEffort: config.CodexEffortMax,
			wantMidModel:      config.CodexSolModel,
			wantMidEffort:     config.CodexEffortXHigh,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			adapter := NewWithRoot(dir)
			cfg := config.DefaultFullConfig("provider-quality")
			cfg.Quality.Default = tt.global
			cfg.Quality.Providers = map[string]string{
				config.QualityProviderClaude: tt.global,
				config.QualityProviderCodex:  tt.codex,
			}
			cfg.Quality.SupervisorModelPolicy = config.SupervisorModelPolicyQuality

			configFiles, err := adapter.prepareConfigFile(cfg)
			require.NoError(t, err)
			root := strings.SplitN(string(configFiles[0].Content), "[agents]", 2)[0]
			assert.Contains(t, root, `model = "`+config.CodexAstraModel+`"`)
			assert.Contains(t, root, `model_reasoning_effort = "`+tt.wantRootEffort+`"`)

			agentFiles, err := adapter.generateAgents(cfg)
			require.NoError(t, err)
			byPath := make(map[string]string, len(agentFiles))
			for _, file := range agentFiles {
				byPath[file.TargetPath] = string(file.Content)
			}
			planner := byPath[filepath.Join(".codex", "agents", "reviewer.toml")]
			tester := byPath[filepath.Join(".codex", "agents", "ux-validator.toml")]
			assert.Contains(t, planner, `model = "`+config.CodexAstraModel+`"`)
			assert.Contains(t, planner, `model_reasoning_effort = "`+tt.wantPlannerEffort+`"`)
			assert.Contains(t, tester, `model = "`+tt.wantMidModel+`"`)
			assert.Contains(t, tester, `model_reasoning_effort = "`+tt.wantMidEffort+`"`)
		})
	}
}
