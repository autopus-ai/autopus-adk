package cli

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

func TestRunSpecReview_AddsVerdictCompletionHints(t *testing.T) {
	dir := t.TempDir()
	scaffoldReviewSpec(t, dir, "SPEC-REVIEW-HINTS-001")
	setFakeProviderOnPath(t, dir, "claude")

	origWD, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWD) }()
	require.NoError(t, os.Chdir(dir))

	stubSpecReviewAssembly(t, func(_ *config.HarnessConfig, names []string) []orchestra.ProviderConfig {
		return []orchestra.ProviderConfig{{Name: "claude", Binary: "claude"}}
	})

	var captured orchestra.OrchestraConfig
	origRunner := specReviewRunOrchestra
	specReviewRunOrchestra = func(_ context.Context, cfg orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		captured = cfg
		return &orchestra.OrchestraResult{Responses: []orchestra.ProviderResponse{{Provider: "claude", Output: "VERDICT: PASS"}}}, nil
	}
	defer func() { specReviewRunOrchestra = origRunner }()

	require.NoError(t, runSpecReview(context.Background(), "SPEC-REVIEW-HINTS-001", "consensus", 10))
	require.Len(t, captured.Providers, 1)
	assert.Contains(t, captured.Providers[0].ResultReadyPatterns, "VERDICT:")
	assert.Equal(t, specReviewResultReadyGrace, captured.Providers[0].ResultReadyGrace)
}

func TestNewSpecReviewCmd_RegistersPaneBypassFlags(t *testing.T) {
	cmd := newSpecReviewCmd()
	assert.NotNil(t, cmd.Flags().Lookup("subprocess"))
	assert.NotNil(t, cmd.Flags().Lookup("plain"))
}

// SPEC review must never reach the pane backend, which adds a permission
// bypass flag and auto-approves tool prompts; it is always read-only.
func TestRunSpecReview_AlwaysReadOnlySubprocessBackend(t *testing.T) {
	dir := t.TempDir()
	scaffoldReviewSpec(t, dir, "SPEC-REVIEW-SUBPROCESS-001")
	setFakeProviderOnPath(t, dir, "claude")

	origWD, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWD) }()
	require.NoError(t, os.Chdir(dir))

	stubSpecReviewAssembly(t, func(_ *config.HarnessConfig, names []string) []orchestra.ProviderConfig {
		return []orchestra.ProviderConfig{{Name: "claude", Binary: "claude"}}
	})

	var captured orchestra.OrchestraConfig
	origRunner := specReviewRunOrchestra
	specReviewRunOrchestra = func(_ context.Context, cfg orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		captured = cfg
		return &orchestra.OrchestraResult{Responses: []orchestra.ProviderResponse{{Provider: "claude", Output: "VERDICT: PASS"}}}, nil
	}
	defer func() { specReviewRunOrchestra = origRunner }()

	err = runSpecReviewWithOptions(context.Background(), "SPEC-REVIEW-SUBPROCESS-001", "consensus", 10, specReviewOptions{})
	require.NoError(t, err)
	assert.True(t, captured.SubprocessMode)
	assert.True(t, captured.ReadOnly)
}
