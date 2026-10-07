package cli

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// captureSpecReviewRouting records the run config and, for every reviewer
// and then the judge, the backend the real routed factory selects.
func captureSpecReviewRouting(t *testing.T) (*orchestra.OrchestraConfig, *[]string) {
	t.Helper()
	captured, names := &orchestra.OrchestraConfig{}, &[]string{}
	original := specReviewRunOrchestra
	specReviewRunOrchestra = func(_ context.Context, cfg orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		*captured = cfg
		backend := specReviewBackendFactory(cfg)
		for _, provider := range append(append([]orchestra.ProviderConfig(nil), cfg.Providers...), *cfg.JudgeConfig) {
			*names = append(*names, provider.Name+"="+specReviewProviderBackendName(backend, provider))
		}
		return &orchestra.OrchestraResult{Responses: []orchestra.ProviderResponse{
			passResponse("claude"), passResponse("codex"), passResponse("gemini"),
		}}, nil
	}
	t.Cleanup(func() { specReviewRunOrchestra = original })
	return captured, names
}

// S2 (REQ-17) with SPEC-PANERM-001 REQ-01: in the context that used to select
// pane execution, spec review builds a read-only config, routes every
// provider and the judge to the subprocess backend, and calls no terminal.
// The pane backend is retired, so no subprocess notice is printed either.
func TestRunSpecReview_PaneCapableTerminalRunsProvidersAsSubprocesses(t *testing.T) {
	fixture := newReadOnlyReviewFixture(t, nil)
	useHermeticReadiness(t)
	tmuxLog := usePaneCapableContext(t)
	captured, names := captureSpecReviewRouting(t)

	stderr := captureSpecReviewStderr(t, func() {
		require.NoError(t, runSpecReviewWithOptions(context.Background(), fixture.specID, "", 0, specReviewOptions{}))
	})

	assert.True(t, captured.ReadOnly)
	assert.Equal(t, []string{"claude=subprocess", "codex=subprocess", "gemini=subprocess", "claude=subprocess"}, *names)
	assert.NoFileExists(t, tmuxLog, "spec review must not call the terminal")
	assert.NotContains(t, stderr, "read-only review runs providers in subprocess mode")
}

func TestRunSpecReview_PlainTerminalPrintsNoSubprocessNotice(t *testing.T) {
	fixture := newReadOnlyReviewFixture(t, nil)
	fixture.useFakeBackend(t)
	useHermeticReadiness(t)
	for _, key := range []string{"TMUX", "CMUX_SOCKET_PATH", "CMUX_WORKSPACE_ID", "CMUX_SURFACE_ID", "CMUX_PANE_ID"} {
		t.Setenv(key, "")
	}

	stderr := captureSpecReviewStderr(t, func() {
		require.NoError(t, runSpecReviewWithOptions(context.Background(), fixture.specID, "", 0, specReviewOptions{}))
	})

	assert.NotContains(t, stderr, "read-only review runs providers in subprocess mode")
}

// S2: an OMP-backed claude keeps the OMP review backend route.
func TestRunSpecReview_OMPBackedProviderRoutesToOMPReviewBackend(t *testing.T) {
	fixture := newReadOnlyReviewFixture(t, func(cfg *config.HarnessConfig) {
		cfg.Orchestra.Providers["claude"] = config.ProviderEntry{Backend: config.ProviderBackendOMP, Model: ompAnthropicModel}
	})
	installReviewJSONRecorders(t, "omp", "codex", "agy")
	useHermeticReadiness(t)
	usePaneCapableContext(t)
	_, names := captureSpecReviewRouting(t)

	require.NoError(t, runSpecReviewWithOptions(context.Background(), fixture.specID, "", 0, specReviewOptions{}))

	assert.Equal(t, []string{"claude=omp", "codex=subprocess", "gemini=subprocess", "claude=omp"}, *names)
}
