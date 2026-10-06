package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/spec"
)

// setThreeProviderQuorumConfig writes an autopus.yaml with three review
// providers and exclude_failed_from_denom=true so a single PASS among two
// timeouts still merges to PASS (denom excludes the failed pair) while remaining
// below the derived quorum of 2 (AC-RINT-QUORUM-4).
func setThreeProviderQuorumConfig(t *testing.T, dir string) {
	t.Helper()
	cfg := config.DefaultFullConfig("integrity-test")
	cfg.Spec.ReviewGate.Providers = []string{"claude", "gemini", "codex"}
	cfg.Spec.ReviewGate.ExcludeFailedFromDenom = true
	cfg.Spec.ReviewGate.MinProviders = 0 // unset -> derive majority (3 -> quorum 2)
	require.NoError(t, config.Save(dir, cfg))
}

// overrideReviewSeams points the review provider builder and orchestra runner at
// deterministic fakes so the gate is exercised without real providers.
func overrideReviewSeams(t *testing.T, providers []orchestra.ProviderConfig, responses []orchestra.ProviderResponse) {
	t.Helper()
	stubSpecReviewAssembly(t, func(_ *config.HarnessConfig, _ []string) []orchestra.ProviderConfig {
		return providers
	})

	origRunner := specReviewRunOrchestra
	specReviewRunOrchestra = func(_ context.Context, _ orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		return &orchestra.OrchestraResult{Responses: responses}, nil
	}
	t.Cleanup(func() { specReviewRunOrchestra = origRunner })
}

func passResponse(name string) orchestra.ProviderResponse {
	return orchestra.ProviderResponse{Provider: name, ExitCode: 0, Output: "VERDICT: PASS"}
}

func timeoutResponse(name string) orchestra.ProviderResponse {
	return orchestra.ProviderResponse{Provider: name, TimedOut: true}
}

// TestRunSpecReview_ObservationIntegrityQuorumGate drives the provider-quorum
// half of the promotion gate end-to-end through the review loop and status sync
// (REQ-RINT-QUORUM-05, REQ-RINT-PROMO-06, REQ-RINT-OVERRIDE-07).
func TestRunSpecReview_ObservationIntegrityQuorumGate(t *testing.T) {
	threeProviders := []orchestra.ProviderConfig{
		{Name: "claude", Binary: "claude"},
		{Name: "gemini", Binary: "gemini"},
		{Name: "codex", Binary: "codex"},
	}

	tests := []struct {
		name          string
		specID        string
		responses     []orchestra.ProviderResponse
		allowDegraded bool
		wantStatus    string
		wantReason    string // degraded reason expected in review.md; "" = none present
		wantOverride  bool
	}{
		{
			name:       "full quorum met promotes",
			specID:     "SPEC-RINT-QUORUM-FULL-001",
			responses:  []orchestra.ProviderResponse{passResponse("claude"), passResponse("gemini"), passResponse("codex")},
			wantStatus: "approved",
		},
		{
			name:       "sub-quorum blocks promotion",
			specID:     "SPEC-RINT-QUORUM-SUB-001",
			responses:  []orchestra.ProviderResponse{passResponse("claude"), timeoutResponse("gemini"), timeoutResponse("codex")},
			wantStatus: "draft",
			wantReason: spec.DegradedReasonProviderQuorum,
		},
		{
			name:          "sub-quorum with allow-degraded promotes and audits",
			specID:        "SPEC-RINT-QUORUM-OVR-001",
			responses:     []orchestra.ProviderResponse{passResponse("claude"), timeoutResponse("gemini"), timeoutResponse("codex")},
			allowDegraded: true,
			wantStatus:    "approved",
			wantReason:    spec.DegradedReasonProviderQuorum,
			wantOverride:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			specDir := scaffoldReviewSpec(t, dir, tt.specID)
			setThreeProviderQuorumConfig(t, dir)
			chdirForTest(t, dir)
			overrideReviewSeams(t, threeProviders, tt.responses)

			err := runSpecReviewWithOptions(context.Background(), tt.specID, "consensus", 10,
				specReviewOptions{allowDegraded: tt.allowDegraded})
			require.NoError(t, err)

			doc, err := spec.Load(specDir)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, doc.Status)

			reviewText := readReviewMd(t, specDir)
			if tt.wantReason != "" {
				assert.Contains(t, reviewText, tt.wantReason,
					"verdict line must record the degraded reason")
			} else {
				assert.NotContains(t, reviewText, spec.DegradedReasonProviderQuorum)
				assert.NotContains(t, reviewText, spec.DegradedReasonPartialDocContext)
			}
			if tt.wantOverride {
				assert.Contains(t, reviewText, "--allow-degraded",
					"override promotion records an audit line in review.md")
			}
		})
	}
}

// TestRunSpecReview_PartialDocContextBlocksPromotion drives the truncation half
// of the gate: an auxiliary document injected below 100% coverage annotates the
// verdict with partial_doc_context and holds the status at draft (AC-RINT-TRUNC-2).
func TestRunSpecReview_PartialDocContextBlocksPromotion(t *testing.T) {
	dir := t.TempDir()
	specID := "SPEC-RINT-TRUNC-001"
	specDir := scaffoldReviewSpec(t, dir, specID)
	oversizeResearchDoc(t, specDir)
	chdirForTest(t, dir)

	// One provider keeps quorum met (min 1), isolating document coverage as the
	// only degraded axis.
	single := []orchestra.ProviderConfig{{Name: "claude", Binary: "claude"}}
	overrideReviewSeams(t, single, []orchestra.ProviderResponse{passResponse("claude")})

	require.NoError(t, runSpecReview(context.Background(), specID, "consensus", 10))

	doc, err := spec.Load(specDir)
	require.NoError(t, err)
	assert.Equal(t, "draft", doc.Status, "partial-context PASS must not auto-promote")

	reviewText := readReviewMd(t, specDir)
	assert.Contains(t, reviewText, spec.DegradedReasonPartialDocContext,
		"verdict line must record the partial_doc_context reason")
}

// TestRunSpecReview_PartialDocContextOverridePromotes confirms --allow-degraded
// also releases the truncation half of the gate, not only the quorum half
// (REQ-RINT-OVERRIDE-07).
func TestRunSpecReview_PartialDocContextOverridePromotes(t *testing.T) {
	dir := t.TempDir()
	specID := "SPEC-RINT-TRUNC-OVR-001"
	specDir := scaffoldReviewSpec(t, dir, specID)
	oversizeResearchDoc(t, specDir)
	chdirForTest(t, dir)

	single := []orchestra.ProviderConfig{{Name: "claude", Binary: "claude"}}
	overrideReviewSeams(t, single, []orchestra.ProviderResponse{passResponse("claude")})

	require.NoError(t, runSpecReviewWithOptions(context.Background(), specID, "consensus", 10,
		specReviewOptions{allowDegraded: true}))

	doc, err := spec.Load(specDir)
	require.NoError(t, err)
	assert.Equal(t, "approved", doc.Status)

	reviewText := readReviewMd(t, specDir)
	assert.Contains(t, reviewText, "--allow-degraded")
	assert.Contains(t, reviewText, spec.DegradedReasonPartialDocContext)
}

func readReviewMd(t *testing.T, specDir string) string {
	t.Helper()
	reviewBytes, err := os.ReadFile(filepath.Join(specDir, "review.md"))
	require.NoError(t, err)
	return string(reviewBytes)
}

// oversizeResearchDoc overwrites research.md with more lines than the total aux
// budget so structure-preserving compaction injects it below full coverage.
func oversizeResearchDoc(t *testing.T, specDir string) {
	t.Helper()
	var sb strings.Builder
	for i := 0; i < spec.DefaultAuxTotalBudgetLines+200; i++ {
		sb.WriteString("filler line for oversized research document\n")
	}
	sb.WriteString("## Self-Verify Summary\n")
	sb.WriteString("Q-COMP-05 | status: PASS\n")
	require.NoError(t, os.WriteFile(filepath.Join(specDir, "research.md"), []byte(sb.String()), 0o644))
}
