package cli

// SPEC-REVIEWREUSE-001 S7 (REQ-RR-JDG-07): the receipt links every judge
// decision to the reviewers whose finding it ruled on.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/spec"
)

func judgedEntry(id, decision, source, reason, mergeInto string) map[string]any {
	entry := map[string]any{"id": id, "severity": "major", "category": "completeness", "scope_ref": "REQ-001",
		"location": "spec.md", "description": "d " + id, "suggestion": "s", "decision": decision, "sources": []string{source}}
	if reason != "" {
		entry["reason"] = reason
	}
	if mergeInto != "" {
		entry["merge_into"] = mergeInto
	}
	return entry
}

func TestSpecReviewReceipt_RecordsEveryJudgeDecision(t *testing.T) {
	fixture, backend := judgeModeFixture(t, "", config.ReviewBudgetConf{})
	long := strings.Repeat("x", 600)
	judge, err := json.Marshal(map[string]any{"verdict": "REVISE", "rationale": "r", "findings": []any{
		judgedEntry("F-1", "accept", "Reviewer A", "", ""),
		judgedEntry("F-2", "reject", "Reviewer B", long, ""),
		judgedEntry("F-3", "merge", "Reviewer B", "same as F-1", "F-1"),
	}})
	require.NoError(t, err)
	backend.replies["claude/judge"] = []scriptedReply{{output: string(judge)}}

	runBudgetReview(t, fixture, specReviewOptions{})

	receipt := readSpecReviewReceipt(t, fixture.specDir)
	require.NotNil(t, receipt.Judge)
	assert.Equal(t, []spec.JudgeDecision{
		{ID: "F-1", Decision: "accept", Reviewers: []string{"claude"}, ScopeRef: "REQ-001"},
		{ID: "F-2", Decision: "reject", Reviewers: []string{"codex"}, ScopeRef: "REQ-001", Reason: strings.Repeat("x", 500)},
		{ID: "F-3", Decision: "merge", Reviewers: []string{"codex"}, ScopeRef: "REQ-001", Reason: "same as F-1", MergeInto: "F-1"},
	}, receipt.Judge.Decisions)

	plain, _ := judgeModeFixture(t, "", config.ReviewBudgetConf{})
	runBudgetReview(t, plain, specReviewOptions{})
	data, err := os.ReadFile(filepath.Join(plain.specDir, "review-receipt.json"))
	require.NoError(t, err)
	assert.NotContains(t, string(data), `"decisions"`)
}
