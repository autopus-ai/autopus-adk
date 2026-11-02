package cli

// SPEC-REVIEWREUSE-001 S5 (REQ-RR-JDG-05): without a judge, a minority
// critical or data-loss claim survives clean PASS votes and a majority finding,
// while an ordinary minority finding keeps today's handling.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/spec"
)

func TestRunSpecReview_MinoritySafetyClaimSurvivesTheMerge(t *testing.T) {
	rollback := reviewerWith("PASS", "critical", "correctness", "REQ-001", "Rollback loses committed rows")
	errorPath := reviewerWith("REVISE", "major", "completeness", "REQ-002", "Error path undefined")
	cases := []struct {
		name          string
		claudeCodex   string
		gemini        string
		verdict       string
		findingScopes map[string][]string
	}{
		{"pass vote with a critical claim", passNoFinding, rollback, "REVISE", map[string][]string{"REQ-001": {"gemini"}}},
		{"pass vote with a data loss claim", passNoFinding,
			reviewerWith("PASS", "minor", "data_loss", "REQ-001", "Purge drops the archive"), "REVISE", map[string][]string{"REQ-001": {"gemini"}}},
		{"critical claim beside a majority finding", errorPath,
			reviewerWith("REVISE", "critical", "correctness", "REQ-001", "Rollback loses committed rows"), "REVISE",
			map[string][]string{"REQ-002": {"claude", "codex"}, "REQ-001": {"gemini"}}},
		{"ordinary minority finding", passNoFinding, errorPath, "REVISE", map[string][]string{"REQ-002": {"gemini"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture, backend := budgetFixture(t, config.ReviewBudgetConf{}, func(cfg *config.HarnessConfig) {
				cfg.Spec.ReviewGate.Providers = []string{"claude", "codex", "gemini"}
				cfg.Spec.ReviewGate.VerdictThreshold = 0.67
			})
			backend.replies["claude/reviewer"] = []scriptedReply{{output: tc.claudeCodex}}
			backend.replies["codex/reviewer"] = []scriptedReply{{output: tc.claudeCodex}}
			backend.replies["gemini/reviewer"] = []scriptedReply{{output: tc.gemini}}

			runBudgetReview(t, fixture, specReviewOptions{})

			assert.Equal(t, 3, backend.count())
			assert.Equal(t, tc.verdict, readSpecReviewReceipt(t, fixture.specDir).Verdict)
			findings, err := spec.LoadFindings(fixture.specDir)
			require.NoError(t, err)
			got := map[string][]string{}
			for _, finding := range findings {
				got[finding.ScopeRef] = finding.Sources
			}
			assert.Equal(t, tc.findingScopes, got)
		})
	}
}
