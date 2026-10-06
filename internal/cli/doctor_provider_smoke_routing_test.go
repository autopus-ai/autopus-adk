package cli

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// smokeRouteRecorder keeps the real routed backend's decisions: CLI providers
// execute through it, while an OMP-routed request is recorded instead of
// starting an OMP session.
type smokeRouteRecorder struct {
	routed orchestra.ExecutionBackend
	args   map[string][]string
	routes map[string]string
}

func (r *smokeRouteRecorder) Execute(ctx context.Context, req orchestra.ProviderRequest) (*orchestra.ProviderResponse, error) {
	r.args[req.Provider] = append([]string(nil), req.Config.Args...)
	r.routes[req.Provider] = specReviewProviderBackendName(r.routed, req.Config)
	if r.routes[req.Provider] == config.ProviderBackendOMP {
		return &orchestra.ProviderResponse{Provider: req.Provider, Output: providerSmokeMarker}, nil
	}
	return r.routed.Execute(ctx, req)
}

func (r *smokeRouteRecorder) Name() string { return r.routed.Name() }

// smokeReviewGateConfig is fixture F-default as the review gate of a full config.
func smokeReviewGateConfig(extra ...string) *config.HarnessConfig {
	cfg := config.DefaultFullConfig("provider-smoke")
	cfg.Spec.ReviewGate.Providers = append([]string{"claude", "codex", "gemini"}, extra...)
	cfg.Orchestra.Providers = fDefaultSpecReviewConfig().Orchestra.Providers
	return cfg
}

// S14 (REQ-08): the smoke builds review-gate providers through the spec
// review assembly and projection and runs them through the same routing:
// CLI providers as subprocesses, an OMP-backed provider on the OMP review
// backend and never as a plain omp subprocess.
func TestRunProviderTransportSmoke_UsesSpecReviewAssemblyAndRouting(t *testing.T) {
	evidence := installReadOnlyArgvRecorders(t, "claude", "codex", "agy", "omp")
	countCodexCatalogProbes(t)
	cfg := smokeReviewGateConfig("opus")
	cfg.Orchestra.Providers["opus"] = config.ProviderEntry{Backend: config.ProviderBackendOMP, Model: ompAnthropicModel}
	recorder := &smokeRouteRecorder{args: map[string][]string{}, routes: map[string]string{}}
	var factoryCfg orchestra.OrchestraConfig
	original := providerSmokeBackendFactory
	providerSmokeBackendFactory = func(runCfg orchestra.OrchestraConfig) orchestra.ExecutionBackend {
		factoryCfg = runCfg
		recorder.routed = original(runCfg)
		return recorder
	}
	t.Cleanup(func() { providerSmokeBackendFactory = original })

	results := runProviderTransportSmoke(context.Background(), cfg, 10*time.Second)

	require.Len(t, results, 4)
	assert.True(t, factoryCfg.SubprocessMode)
	assert.True(t, factoryCfg.ReadOnly)
	assert.Equal(t, map[string]string{"claude": "subprocess", "codex": "subprocess", "gemini": "subprocess", "opus": "omp"}, recorder.routes)
	pClaude := withClaudeReadOnlySuffix("--print", "--model", "claude-fable-5-1", "--effort", "max")
	assert.Equal(t, pClaude, recorder.args["claude"])
	assert.Equal(t, []string{
		"exec", "--json", "--sandbox", "read-only", "-m", "gpt-5.6-sol", "-c", `model_reasoning_effort="max"`,
		"--ephemeral", "--ignore-user-config", "--ignore-rules",
	}, recorder.args["codex"])
	assert.Equal(t, []string{"--print", "", "--mode", "plan", "--sandbox", "--disable-slash-commands"}, recorder.args["gemini"])
	assert.Equal(t, pClaude, readRecordedArgv(t, evidence, "claude"), "claude executed the projected argv")
	assert.Equal(t, []string{"--print", providerSmokePrompt(), "--mode", "plan", "--sandbox", "--disable-slash-commands"},
		readRecordedArgv(t, evidence, "agy"))
	assert.NoFileExists(t, filepath.Join(evidence, "omp.argv"), "the OMP provider never runs as a plain omp subprocess")
}

// S14: a smoke over a config the read-only policy rejects reports the Error
// Contract once and executes nothing.
func TestRunProviderTransportSmoke_PolicyViolationFailsWithoutExecution(t *testing.T) {
	evidence := installReadOnlyArgvRecorders(t, "claude", "codex", "agy")
	cfg := smokeReviewGateConfig()
	editProvider(cfg, "claude", func(e *config.ProviderEntry) { e.Args = []string{"--print", "--verbose"} })
	factoryCalls := 0
	original := providerSmokeBackendFactory
	providerSmokeBackendFactory = func(orchestra.OrchestraConfig) orchestra.ExecutionBackend {
		factoryCalls++
		return fakeProviderSmokeBackend{}
	}
	t.Cleanup(func() { providerSmokeBackendFactory = original })

	results := runProviderTransportSmoke(context.Background(), cfg, time.Second)

	assert.Equal(t, []providerSmokeResult{{Provider: "review_gate", Status: "fail",
		Detail: `spec review: provider "claude" rejected by the read-only policy: contains unsupported argv "--verbose" (config key: orchestra.providers.claude.args; remedy: remove "--verbose" from orchestra.providers.claude.args)`}},
		results)
	assert.Zero(t, factoryCalls)
	recorded, err := filepath.Glob(filepath.Join(evidence, "*.argv"))
	require.NoError(t, err)
	assert.Empty(t, recorded)
}
