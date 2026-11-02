package cli

// SPEC-REVIEWREUSE-001 S8 (REQ-RR-REUSE-01) for SPEC-STATUSEVIDENCE-001
// REQ-SEV-REVIEW-01: the review receipt records spec_fingerprint with the one
// shared helper, so status can prove by content that the SPEC is what the
// review saw.

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestRunSpecReview_ReceiptRecordsTheSpecFingerprint(t *testing.T) {
	fixture, _ := budgetFixture(t, config.ReviewBudgetConf{}, nil)
	before, err := specDocumentsFingerprint(fixture.specDir)
	require.NoError(t, err)

	runReviewOutput(t, fixture, specReviewOptions{})

	receipt := readSpecReviewReceipt(t, fixture.specDir)
	require.Equal(t, "approved", receipt.CurrentStatus, "the review promoted the SPEC")
	assert.Equal(t, before, receipt.SpecFingerprint, "the fingerprint of what the review read")
	after, err := specDocumentsFingerprint(fixture.specDir)
	require.NoError(t, err)
	assert.Equal(t, after, receipt.SpecFingerprint, "the promotion rewrote only status lines")

	review := readStatusReview(fixture.root, fixture.specDir)
	require.NotNil(t, review)
	assert.Equal(t, freshnessFresh, review.State, review.Reasons)

	appendFixtureFile(t, filepath.Join(fixture.specDir, "plan.md"), "\nOne more task.\n")
	assert.Equal(t, freshnessStale, readStatusReview(fixture.root, fixture.specDir).State)
}
