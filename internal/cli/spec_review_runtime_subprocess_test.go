package cli

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/terminal"
)

// countingPaneTerminal is a pane-capable terminal that counts pane launches.
type countingPaneTerminal struct {
	fakeWiringTerminal
	splits, longTexts atomic.Int32
}

func (term *countingPaneTerminal) SplitPane(context.Context, terminal.Direction) (terminal.PaneID, error) {
	term.splits.Add(1)
	return "pane-1", nil
}

func (term *countingPaneTerminal) SendLongText(context.Context, terminal.PaneID, string) error {
	term.longTexts.Add(1)
	return nil
}

func useSpecReviewTerminal(t *testing.T, term terminal.Terminal) {
	t.Helper()
	original := specReviewTerminalDetector
	specReviewTerminalDetector = func() terminal.Terminal { return term }
	t.Cleanup(func() { specReviewTerminalDetector = original })
}

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

// S2 (REQ-16, REQ-17): on a pane-capable terminal with subprocess mode
// disabled in config, spec review builds a read-only subprocess config,
// routes every provider and the judge to the subprocess backend, opens no
// pane, and says so once.
func TestRunSpecReview_PaneCapableTerminalRunsProvidersAsSubprocesses(t *testing.T) {
	fixture := newReadOnlyReviewFixture(t, func(cfg *config.HarnessConfig) { cfg.Orchestra.Subprocess.Enabled = false })
	useHermeticReadiness(t)
	term := &countingPaneTerminal{fakeWiringTerminal: fakeWiringTerminal{name: "cmux"}}
	useSpecReviewTerminal(t, term)
	captured, names := captureSpecReviewRouting(t)

	stderr := captureSpecReviewStderr(t, func() {
		require.NoError(t, runSpecReviewWithOptions(context.Background(), fixture.specID, "", 0, specReviewOptions{}))
	})

	assert.True(t, captured.ReadOnly)
	assert.True(t, captured.SubprocessMode)
	assert.Equal(t, []string{"claude=subprocess", "codex=subprocess", "gemini=subprocess", "claude=subprocess"}, *names)
	assert.Zero(t, term.splits.Load())
	assert.Zero(t, term.longTexts.Load())
	assert.Equal(t, 1, strings.Count(stderr, "spec review: read-only review runs providers in subprocess mode\n"))
}

func TestRunSpecReview_PlainTerminalPrintsNoSubprocessNotice(t *testing.T) {
	fixture := newReadOnlyReviewFixture(t, nil)
	fixture.useFakeBackend(t)
	useHermeticReadiness(t)
	useSpecReviewTerminal(t, fakeWiringTerminal{name: "plain"})

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
	useSpecReviewTerminal(t, &countingPaneTerminal{fakeWiringTerminal: fakeWiringTerminal{name: "tmux"}})
	_, names := captureSpecReviewRouting(t)

	require.NoError(t, runSpecReviewWithOptions(context.Background(), fixture.specID, "", 0, specReviewOptions{}))

	assert.Equal(t, []string{"claude=omp", "codex=subprocess", "gemini=subprocess", "claude=omp"}, *names)
}
