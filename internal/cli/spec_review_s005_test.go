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

// TestRunSpecReview_FailedProviderRejectIgnored covers SPEC-SPECREV-001 S-005
// hardening: a failed provider (TimedOut or ExitCode != 0) emitting a partial
// "VERDICT: REJECT" in stdout MUST NOT trigger the REJECT short-circuit. The
// surviving providers' verdicts decide the merged outcome.
func TestRunSpecReview_FailedProviderRejectIgnored(t *testing.T) {
	dir := t.TempDir()
	specDir := scaffoldReviewSpec(t, dir, "SPEC-S005-GUARD-001")
	setFakeProviderOnPath(t, dir, "claude")

	origWD, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWD) }()
	require.NoError(t, os.Chdir(dir))

	stubSpecReviewAssembly(t, func(_ *config.HarnessConfig, _ []string) []orchestra.ProviderConfig {
		return []orchestra.ProviderConfig{
			{Name: "claude", Binary: "claude"},
			{Name: "gemini", Binary: "gemini"},
		}
	})

	origRunner := specReviewRunOrchestra
	specReviewRunOrchestra = func(_ context.Context, _ orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		// The default policy requests claude, codex, and gemini. Only claude and
		// gemini resolve in this fixture; codex must remain in the denominator.
		// Claude succeeds with PASS; gemini failed (ExitCode=1) but its partial
		// stdout contains a spurious "VERDICT: REJECT" and must be ignored.
		return &orchestra.OrchestraResult{Responses: []orchestra.ProviderResponse{
			{Provider: "claude", ExitCode: 0, Output: "VERDICT: PASS"},
			{Provider: "gemini", ExitCode: 1, Error: "subprocess crash", Output: "partial output\nVERDICT: REJECT"},
		}}, nil
	}
	defer func() { specReviewRunOrchestra = origRunner }()

	require.NoError(t, runSpecReview(context.Background(), "SPEC-S005-GUARD-001", "consensus", 10))

	// Read review.md and verify the verdict was NOT short-circuited to REJECT.
	reviewBytes, err := os.ReadFile(filepath.Join(specDir, "review.md"))
	require.NoError(t, err)
	reviewText := string(reviewBytes)
	assert.NotContains(t, reviewText, "**Verdict**: REJECT",
		"failed provider's partial REJECT must not short-circuit the merged verdict")
	// gemini failed and codex was unresolved: 1/3 policy coverage is degraded.
	assert.Contains(t, reviewText, "(degraded — 1/3 providers responded)",
		"degraded label must retain all three configured providers")
	assert.Contains(t, reviewText, spec.DegradedReasonProviderQuorum)
}

func TestRunSpecReview_StderrWarningsDoNotDegradeSuccessfulProviders(t *testing.T) {
	dir := t.TempDir()
	specDir := scaffoldReviewSpec(t, dir, "SPEC-STDERR-WARN-001")

	origWD, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWD) }()
	require.NoError(t, os.Chdir(dir))

	stubSpecReviewAssembly(t, func(_ *config.HarnessConfig, _ []string) []orchestra.ProviderConfig {
		return []orchestra.ProviderConfig{
			{Name: "codex", Binary: "codex"},
			{Name: "gemini", Binary: "gemini"},
		}
	})

	origRunner := specReviewRunOrchestra
	specReviewRunOrchestra = func(_ context.Context, _ orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		return &orchestra.OrchestraResult{Responses: []orchestra.ProviderResponse{
			{
				Provider: "codex",
				ExitCode: 0,
				Output:   `{"verdict":"PASS","summary":"ok","findings":[]}`,
				Error:    "warning: --full-auto is deprecated; use --sandbox workspace-write instead",
			},
			{
				Provider: "gemini",
				ExitCode: 0,
				Output:   `{"verdict":"PASS","summary":"ok","findings":[]}`,
				Error:    "Ripgrep is not available. Falling back to GrepTool.\nSkill conflict detected: \"find-skills\"",
			},
		}}, nil
	}
	defer func() { specReviewRunOrchestra = origRunner }()

	require.NoError(t, runSpecReview(context.Background(), "SPEC-STDERR-WARN-001", "consensus", 10))

	reviewBytes, err := os.ReadFile(filepath.Join(specDir, "review.md"))
	require.NoError(t, err)
	reviewText := string(reviewBytes)
	assert.Contains(t, reviewText, "**Verdict**: PASS")
	assert.NotContains(t, reviewText, "degraded",
		"stderr-only provider warnings must not mark successful reviews degraded")
	assert.Contains(t, reviewText, "| codex | success | - |")
	assert.Contains(t, reviewText, "| gemini | success | - |")

	doc, err := spec.Load(specDir)
	require.NoError(t, err)
	assert.Equal(t, "approved", doc.Status)
}
