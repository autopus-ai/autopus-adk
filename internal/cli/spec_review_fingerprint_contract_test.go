package cli

// SPEC-REVIEWREUSE-001 S8 and S10 (REQ-RR-REUSE-01, REQ-RR-REUSE-02): the
// fingerprint covers what a reviewer actually receives and the settings that
// shape it: every production prompt variant through the structured wrapper,
// the effective configuration wherever it was loaded from, and the excerpt
// each document delivers under the effective budget.

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestSpecReviewContract_CoversEveryProductionPromptVariant(t *testing.T) {
	contract := specReviewContractText()
	for _, want := range []string{
		"Review the full SPEC in one pass and include all actionable findings together",
		"In verify mode, scope the review to the prior findings and checklist statuses requested above.",
		"Required JSON schema:",
		"[Review document metadata: source_ref=",
		"### Delta Verify Input (spec_review_delta.v1)",
		"[REDACTED_SECRET]",
		"[NEUTRALIZED_INJECTION]",
	} {
		assert.Contains(t, contract, want)
	}

	before := specReviewContractDigest()
	original := specReviewStructuredPrompt
	t.Cleanup(func() { specReviewStructuredPrompt = original })
	specReviewStructuredPrompt = func(base, schema string, inline bool) string {
		return original(base, schema, inline) + "\nOne more wrapper rule.\n"
	}
	assert.NotEqual(t, before, specReviewContractDigest(), "a wrapper-only change changes the contract")
}

// reuseRefusals returns the reasons of the stderr refusal line.
func reuseRefusals(t *testing.T, stderr string) []string {
	t.Helper()
	for _, line := range strings.Split(stderr, "\n") {
		if reasons, ok := strings.CutPrefix(line, "리뷰 재사용 안 함: "); ok {
			return strings.Split(reasons, "; ")
		}
	}
	t.Fatalf("no reuse refusal in stderr:\n%s", stderr)
	return nil
}

// S10 row: a structured wrapper change alone re-reviews.
func TestRunSpecReview_WrapperOnlyChangeInvalidates(t *testing.T) {
	fixture, backend := reuseFixture(t, config.ReviewBudgetConf{}, nil)
	backend.replies["claude/reviewer"] = []scriptedReply{{output: majorCorrectnessRevise}}
	backend.replies["codex/reviewer"] = []scriptedReply{{output: majorCorrectnessRevise}}
	runReviewOutput(t, fixture, specReviewOptions{})
	require.Equal(t, 2, backend.count())

	original := specReviewStructuredPrompt
	t.Cleanup(func() { specReviewStructuredPrompt = original })
	specReviewStructuredPrompt = func(base, schema string, inline bool) string {
		return original(base, schema, inline) + "\nOne more wrapper rule.\n"
	}
	_, stderr := runReviewOutput(t, fixture, specReviewOptions{})

	assert.Equal(t, 4, backend.count())
	assert.Equal(t, []string{"contract"}, reuseRefusals(t, stderr))
}

// S10 row: an external --config change re-reviews although the repository is
// unchanged.
func TestRunSpecReview_ExternalConfigChangeInvalidates(t *testing.T) {
	fixture, backend := reuseFixture(t, config.ReviewBudgetConf{}, nil)
	external := t.TempDir()
	cfg, err := config.Load(fixture.root)
	require.NoError(t, err)
	require.NoError(t, config.Save(external, cfg))
	ctx := withGlobalFlags(context.Background(), globalFlags{ConfigPath: filepath.Join(external, "autopus.yaml")})
	run := func() string {
		return captureSpecReviewStderr(t, func() {
			_ = captureStdout(t, func() {
				require.NoError(t, runSpecReviewWithOptions(ctx, fixture.specID, "", 0, specReviewOptions{}))
			})
		})
	}
	backend.replies["claude/reviewer"] = []scriptedReply{{output: majorCorrectnessRevise}}
	backend.replies["codex/reviewer"] = []scriptedReply{{output: majorCorrectnessRevise}}
	run()
	run()
	require.Equal(t, 2, backend.count(), "an unchanged external config is reused")

	editFixtureConfig(t, external, func(cfg *config.HarnessConfig) { cfg.Spec.ReviewGate.DocContextMaxLines = 4500 })
	stderr := run()

	assert.Equal(t, 4, backend.count())
	reasons := reuseRefusals(t, stderr)
	assert.Contains(t, reasons, "config")
	assert.Contains(t, reasons, "policy: doc_context_max_lines")
	assert.False(t, slices.ContainsFunc(reasons, func(reason string) bool { return strings.HasPrefix(reason, "code:") }),
		"the repository did not change: %v", reasons)
}

// S8: the excerpt a document delivers under the effective budget is part
// of the fingerprint, so a compaction boundary that moves re-reviews.
func TestSpecReviewFingerprint_RecordsTheDeliveredExcerpts(t *testing.T) {
	fixture, _ := reuseFixture(t, config.ReviewBudgetConf{}, nil)
	long := "# Plan\n\n## Tasks\n\n" + strings.Repeat("- planning detail line\n", 4200)
	require.NoError(t, os.WriteFile(filepath.Join(fixture.specDir, "plan.md"), []byte(long), 0o644))
	fingerprint := func(lines int) *specReviewFingerprint {
		return computeSpecReviewFingerprint(specReviewFingerprintInput{
			projectRoot: fixture.root, specDir: fixture.specDir, policy: specReviewPolicyIdentity{DocContextMaxLines: lines},
		})
	}
	compacted, whole := fingerprint(200), fingerprint(5000)

	assert.Equal(t, 4000, compacted.Delivery.AuxBudgetLines)
	assert.Equal(t, 5000, whole.Delivery.AuxBudgetLines)
	assert.NotEqual(t, compacted.Delivery.Excerpts["plan.md"], whole.Delivery.Excerpts["plan.md"])
	assert.Equal(t, compacted.Delivery.Excerpts["acceptance.md"], whole.Delivery.Excerpts["acceptance.md"])
	assert.Contains(t, specReviewFingerprintChanges(*compacted, *whole), "delivery: aux_budget_lines, plan.md")
}
