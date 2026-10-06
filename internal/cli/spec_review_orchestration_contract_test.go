package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/spec"
)

func TestSpecReview_ConfiguredThreeResolvedOne_BlocksPromotion(t *testing.T) {
	root := t.TempDir()
	specID := "SPEC-ORCH-QUORUM-001"
	specDir := scaffoldReviewSpec(t, root, specID)

	cfg := config.DefaultFullConfig("orchestration-quorum")
	cfg.Spec.ReviewGate.Providers = []string{"claude", "codex", "gemini"}
	cfg.Spec.ReviewGate.MinProviders = 0
	cfg.Spec.ReviewGate.ExcludeFailedFromDenom = true
	require.NoError(t, config.Save(root, cfg))
	chdirForTest(t, root)

	originalRunner := specReviewRunOrchestra
	t.Cleanup(func() { specReviewRunOrchestra = originalRunner })

	stubSpecReviewAssembly(t, func(_ *config.HarnessConfig, names []string) []orchestra.ProviderConfig {
		assert.ElementsMatch(t, []string{"claude", "codex", "gemini"}, names)
		return []orchestra.ProviderConfig{{Name: "claude", Binary: "claude"}}
	})
	resolvedCount := 0
	specReviewRunOrchestra = func(_ context.Context, cfg orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		resolvedCount = len(cfg.Providers)
		return &orchestra.OrchestraResult{Responses: []orchestra.ProviderResponse{{
			Provider: "claude",
			ExitCode: 0,
			Output:   "VERDICT: PASS",
		}}}, nil
	}

	err := runSpecReviewWithOptions(context.Background(), specID, "consensus", 10, specReviewOptions{})

	require.NoError(t, err)
	assert.Equal(t, 1, resolvedCount)
	doc, loadErr := spec.Load(specDir)
	require.NoError(t, loadErr)
	assert.Equal(t, "draft", doc.Status)
	reviewBody, readErr := os.ReadFile(filepath.Join(specDir, "review.md"))
	require.NoError(t, readErr)
	assert.Contains(t, string(reviewBody), spec.DegradedReasonProviderQuorum)
}

func TestSpecReviewProviderQuorum_ConfiguredDenominatorRequiresTwo(t *testing.T) {
	t.Parallel()

	statuses := []spec.ProviderStatus{
		{Provider: "claude", Status: "success"},
		{Provider: "codex", Status: "error"},
		{Provider: "gemini", Status: "error"},
	}

	met, detail := spec.MeetsProviderQuorum(statuses, 3, 0)

	assert.False(t, met)
	assert.Equal(t, 2, spec.EffectiveMinProviders(3, 0))
	assert.Equal(t, "providers 1/3 < quorum 2", detail)
}
