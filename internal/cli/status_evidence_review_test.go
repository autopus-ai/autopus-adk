package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeReviewReceipt writes a REVISE receipt with the given fingerprint and
// dates the file apart from its finished_at.
func writeReviewReceipt(t *testing.T, f *statusFixture, specID, fingerprint string, written time.Time) {
	t.Helper()
	field := ""
	if fingerprint != "" {
		field = `,"spec_fingerprint":"` + fingerprint + `"`
	}
	rel := ".autopus/specs/" + specID + "/review-receipt.json"
	f.write(rel, `{"schema":"spec_review_promotion_receipt.v1","run_id":"orch-1",`+
		`"finished_at":"2026-11-01T10:00:00Z","verdict":"REVISE","gate_status":"blocked",`+
		`"loop_status":"revisions_exhausted"`+field+`}`)
	require.NoError(t, os.Chtimes(filepath.Join(f.root, rel), written, written))
}

// S9: with a recorded fingerprint the review is compared by content: a
// status-only edit keeps it fresh and a requirement change makes it stale.
func TestStatusEvidence_ReviewFreshnessComparesContent(t *testing.T) {
	t.Parallel()
	const specID = "SPEC-REV-001"
	f := newStatusFixture(t)
	specDir := f.writeSpec(specID, "draft")
	fingerprint, err := specDocumentsFingerprint(specDir)
	require.NoError(t, err)
	// The receipt file is written two hours after finished_at.
	writeReviewReceipt(t, f, specID, fingerprint, time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC))

	review := f.evidence(specID).Review
	require.NotNil(t, review)
	assert.Equal(t, statusReview{
		State: "fresh", Source: ".autopus/specs/SPEC-REV-001/review-receipt.json", RunID: "orch-1",
		Verdict: "REVISE", GateStatus: "blocked", LoopStatus: "revisions_exhausted", FinishedAt: "2026-11-01T10:00:00Z",
	}, *review)
	assert.Contains(t, f.runStatus(),
		"review: REVISE (gate blocked, loop revisions_exhausted, run orch-1) at 2026-11-01T10:00:00Z — fresh")

	spec := f.read(".autopus/specs/" + specID + "/spec.md")
	f.write(".autopus/specs/"+specID+"/spec.md", strings.Replace(spec, "**Status**: draft", "**Status**: approved", 1))
	assert.Equal(t, "fresh", f.evidence(specID).Review.State, "promotion rewrites only the status line")

	f.write(".autopus/specs/"+specID+"/plan.md", "# Plan\n\n## Tasks\n\n- T1: build it differently\n")
	review = f.evidence(specID).Review
	assert.Equal(t, "stale", review.State)
	assert.Equal(t, []string{"SPEC requirements changed after the review"}, review.Reasons)

	writeReviewReceipt(t, f, specID, "sha256:not-a-digest", time.Now())
	assert.Equal(t, []string{"the receipt's spec_fingerprint is malformed"}, f.evidence(specID).Review.Reasons)
}

// S9: a receipt without a fingerprint is never fresh. A document written
// after it is stale; anything else, including a change that kept its old
// modification time, is unknown, and so is a SPEC that cannot be read.
func TestStatusEvidence_ReviewWithoutFingerprintIsNeverFresh(t *testing.T) {
	t.Parallel()
	const specID = "SPEC-REV-002"
	f := newStatusFixture(t)
	specDir := f.writeSpec(specID, "draft")
	past, written := time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour)
	for _, name := range []string{"spec.md", "plan.md", "acceptance.md"} {
		require.NoError(t, os.Chtimes(filepath.Join(specDir, name), past, past))
	}
	writeReviewReceipt(t, f, specID, "", written)

	review := f.evidence(specID).Review
	assert.Equal(t, "unknown", review.State)
	assert.Equal(t, []string{"the receipt records no SPEC fingerprint, so an unchanged SPEC cannot be proven"}, review.Reasons)

	// A content change that preserves the old modification time.
	f.write(".autopus/specs/"+specID+"/plan.md", "# Plan\n\n## Tasks\n\n- T1: something else\n")
	require.NoError(t, os.Chtimes(filepath.Join(specDir, "plan.md"), past, past))
	assert.Equal(t, "unknown", f.evidence(specID).Review.State, "mtime alone never proves the SPEC unchanged")

	require.NoError(t, os.Chtimes(filepath.Join(specDir, "plan.md"), time.Now(), time.Now()))
	review = f.evidence(specID).Review
	assert.Equal(t, "stale", review.State)
	assert.Equal(t, []string{"SPEC documents written after the review: plan.md"}, review.Reasons)

	require.NoError(t, os.Remove(filepath.Join(specDir, "spec.md")))
	review = f.evidence(specID).Review
	assert.Equal(t, "unknown", review.State)
	require.Len(t, review.Reasons, 1)
	assert.Contains(t, review.Reasons[0], "SPEC documents unreadable")
}

// S9: the review session is read when its schema is known and reported
// unknown otherwise, without failing status; so is a foreign receipt.
func TestStatusEvidence_ReviewSessionIsReadTolerantly(t *testing.T) {
	t.Parallel()
	const specID = "SPEC-REV-003"
	f := newStatusFixture(t)
	f.writeSpec(specID, "draft")
	sessionPath := ".autopus/specs/" + specID + "/review-session.json"
	f.write(sessionPath, `{"schema":"spec_review_session.v1","session_id":"s1","state":"budget_exhausted",`+
		`"attempts":[{"attempt_id":"a1"},{"attempt_id":"a2"}],`+
		`"calls":[{"call_id":"c1"},{"call_id":"c2"},{"call_id":"c3"},{"call_id":"c3"},{"call_id":"c4"},{"call_id":"c5"}]}`)
	review := f.evidence(specID).Review
	require.NotNil(t, review)
	assert.Equal(t, "none", review.State, "a session without a receipt is no verdict")
	assert.Equal(t, statusReviewSession{State: "budget_exhausted", Attempts: 2, Calls: 5}, *review.Session)
	assert.Contains(t, f.runStatus(), "review session: budget_exhausted · 2 attempts · 5 calls")

	f.write(sessionPath, "not json")
	session := f.evidence(specID).Review.Session
	assert.Equal(t, "unknown", session.State)
	assert.Contains(t, session.Reason, "review session unreadable")
	f.write(sessionPath, `{"schema":"spec_review_session.v9","state":"open"}`)
	assert.Contains(t, f.runStatus(), `review session: unknown — unrecognized review session schema "spec_review_session.v9"`)

	f.write(".autopus/specs/"+specID+"/review-receipt.json", `{"schema":"spec_review_promotion_receipt.v7","verdict":"PASS"}`)
	review = f.evidence(specID).Review
	assert.Equal(t, "unknown", review.State)
	assert.Empty(t, review.Verdict)
	assert.Equal(t, []string{`unrecognized review receipt schema "spec_review_promotion_receipt.v7"`}, review.Reasons)
}
