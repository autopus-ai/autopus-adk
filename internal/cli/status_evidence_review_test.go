package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// S9: the review receipt reports its verdict and goes stale when a SPEC
// document is written after it; the review session is read when its schema
// is known and reported unknown otherwise, without failing status.
func TestStatusEvidence_ReviewReceiptAndSession(t *testing.T) {
	t.Parallel()
	const specID = "SPEC-REV-001"
	f := newStatusFixture(t)
	specDir := f.writeSpec(specID, "draft")
	f.write(".autopus/specs/"+specID+"/review-receipt.json", `{"schema":"spec_review_promotion_receipt.v1",`+
		`"run_id":"orch-1","finished_at":"2026-11-01T10:00:00Z","verdict":"REVISE","gate_status":"blocked",`+
		`"loop_status":"revisions_exhausted","blocking_reasons":[{"finding_id":"F-1"}]}`)
	past := time.Now().Add(-time.Hour)
	for _, name := range []string{"spec.md", "plan.md", "acceptance.md"} {
		require.NoError(t, os.Chtimes(filepath.Join(specDir, name), past, past))
	}

	review := fixtureEvidence(t, f, specID).Review
	require.NotNil(t, review)
	assert.Equal(t, "REVISE", review.Verdict)
	assert.Equal(t, "blocked", review.GateStatus)
	assert.Equal(t, "revisions_exhausted", review.LoopStatus)
	assert.Equal(t, "2026-11-01T10:00:00Z", review.FinishedAt)
	assert.Equal(t, "fresh", review.State)
	assert.Nil(t, review.Session)
	assert.Contains(t, f.runStatus(),
		"review: REVISE (gate blocked, loop revisions_exhausted) at 2026-11-01T10:00:00Z — fresh")

	future := time.Now().Add(time.Hour)
	require.NoError(t, os.Chtimes(filepath.Join(specDir, "plan.md"), future, future))
	review = fixtureEvidence(t, f, specID).Review
	assert.Equal(t, "stale", review.State)
	assert.Equal(t, []string{"SPEC documents changed after the review: plan.md"}, review.Reasons)

	sessionPath := ".autopus/specs/" + specID + "/review-session.json"
	f.write(sessionPath, `{"schema":"spec_review_session.v1","session_id":"s1","state":"budget_exhausted",`+
		`"attempts":[{"attempt_id":"a1"},{"attempt_id":"a2"}],`+
		`"calls":[{"call_id":"c1"},{"call_id":"c2"},{"call_id":"c3"},{"call_id":"c3"},{"call_id":"c4"},{"call_id":"c5"}]}`)
	session := fixtureEvidence(t, f, specID).Review.Session
	require.NotNil(t, session)
	assert.Equal(t, statusReviewSession{State: "budget_exhausted", Attempts: 2, Calls: 5}, *session)
	assert.Contains(t, f.runStatus(), "review session: budget_exhausted · 2 attempts · 5 calls")

	f.write(sessionPath, "not json")
	session = fixtureEvidence(t, f, specID).Review.Session
	assert.Equal(t, "unknown", session.State)
	assert.Contains(t, session.Reason, "review session unreadable")
	f.write(sessionPath, `{"schema":"spec_review_session.v9","state":"open"}`)
	session = fixtureEvidence(t, f, specID).Review.Session
	assert.Equal(t, `unrecognized review session schema "spec_review_session.v9"`, session.Reason)
	assert.Contains(t, f.runStatus(), `review session: unknown — unrecognized review session schema "spec_review_session.v9"`)

	f.write(".autopus/specs/"+specID+"/review-receipt.json", `{"schema":"spec_review_promotion_receipt.v7"}`)
	review = fixtureEvidence(t, f, specID).Review
	assert.Equal(t, "unknown", review.State)
	assert.Empty(t, review.Verdict)
	assert.Equal(t, []string{`unrecognized review receipt schema "spec_review_promotion_receipt.v7"`}, review.Reasons)
}
