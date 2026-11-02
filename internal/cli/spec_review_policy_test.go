package cli

// SPEC-REVIEWPOLICY-001 runtime regressions (S3, S5). The reviewer prompt,
// the judge, and the runtime must map one finding set to one verdict, and an
// eligible lean SPEC must not be blocked by a section its profile omits while
// its real blockers still block.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/spec"
)

// writeLeanReviewSpec writes a lean SPEC set under dir and returns its spec
// directory. extraPlan is appended to plan.md so a test can add a section the
// lean profile omits.
func writeLeanReviewSpec(t *testing.T, dir, specID, class, surface, extraPlan string) string {
	t.Helper()
	specDir := filepath.Join(dir, ".autopus", "specs", specID)
	require.NoError(t, os.MkdirAll(specDir, 0o755))
	files := map[string]string{
		"spec.md": "# " + specID + ": Lean Fix\n\n---\nid: " + specID + "\ntitle: Lean Fix\nversion: 0.1.0\nstatus: draft\n" +
			"profile: lean\nchange_class: " + class + "\nsurface: " + surface + "\n---\n\n" +
			"## Outcome Boundary\n\n- Outcome Lock: the status line shows the elapsed review time.\n\n" +
			"## Requirements\n\nWHEN a review round finishes THEN the system SHALL print its elapsed time.\n\n" +
			"## Traceability Matrix\n\n| Requirement | Plan Task | Acceptance Scenario |\n|---|---|---|\n| REQ-001 | T1 | S1 |\n",
		"plan.md": "# " + specID + " Plan\n\n## Tasks\n\n- [ ] T1: Print the elapsed time of each review round.\n" + extraPlan,
		"acceptance.md": "# " + specID + " Acceptance\n\n## Test Scenarios\n\n### S1: Round time is printed\nPriority: Must\n" +
			"Given a review round that ran for 90 seconds\nWhen the round finishes\nThen expected stderr contains \"1m30s\"\n",
		"research.md": "# " + specID + " Research\n\n## Outcome Lock\n\n- User-visible outcome: each round reports its time.\n\n" +
			"## Reviewer Brief\n\n- Intended scope: one stderr line per round.\n",
	}
	for name, body := range files {
		require.NoError(t, os.WriteFile(filepath.Join(specDir, name), []byte(body), 0o644))
	}
	return specDir
}

// runSingleReviewerLoop runs one round in which one reviewer answers output.
func runSingleReviewerLoop(t *testing.T, specID, specDir, output string) *spec.ReviewResult {
	t.Helper()
	doc, err := spec.Load(specDir)
	require.NoError(t, err)
	original := specReviewRunOrchestra
	specReviewRunOrchestra = func(context.Context, orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		return &orchestra.OrchestraResult{Responses: []orchestra.ProviderResponse{{Provider: "claude", Output: output}}}, nil
	}
	t.Cleanup(func() { specReviewRunOrchestra = original })
	params := reviewLoopParams(specID, specDir)
	params.maxRevisions = 0
	params.providers = []orchestra.ProviderConfig{{Name: "claude", Binary: "claude"}}
	result, err := runSpecReviewLoop(params, doc, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	return result
}

const probeChecklistFail = "VERDICT: REVISE\nCHECKLIST: Q-COMP-08 | FAIL | missing Risk-First Integration Probe\n"

// S5: an eligible lean SPEC omits the Risk-First Integration Probe on purpose,
// so a reviewer FAIL on Q-COMP-08 is recorded N/A instead of blocking. On
// 109c7ea4 this resolved REVISE with reviewer_checklist_fail.
func TestRunSpecReviewLoop_LeanOmittedSectionChecklistFailDoesNotBlock(t *testing.T) {
	const specID = "SPEC-LEANWAIVE-001"
	specDir := writeLeanReviewSpec(t, t.TempDir(), specID, "bugfix_existing_contract", "internal/cli/spec_timing.go", "")

	result := runSingleReviewerLoop(t, specID, specDir, probeChecklistFail)

	assert.Equal(t, spec.VerdictPass, result.Verdict)
	assert.Empty(t, result.BlockingReasons)
	assert.Equal(t, spec.LoopStatusConverged, result.LoopStatus)
	require.Len(t, result.ChecklistOutcomes, 1)
	outcome := result.ChecklistOutcomes[0]
	assert.Equal(t, spec.ChecklistStatusNA, outcome.Status)
	assert.Contains(t, outcome.Reason, "lean")
	assert.Contains(t, outcome.Reason, "Risk-First Integration Probe")
}

// The waiver is narrow: a full SPEC, an ineligible lean SPEC, and an eligible
// lean SPEC that does carry the section all keep the FAIL as a blocker.
func TestRunSpecReviewLoop_ChecklistWaiverNeverReachesOtherSpecs(t *testing.T) {
	cases := map[string]func(dir string) string{
		"full SPEC": func(dir string) string {
			return scaffoldReviewSpec(t, dir, "SPEC-LEANWAIVE-002")
		},
		"high-risk lean": func(dir string) string {
			return writeLeanReviewSpec(t, dir, "SPEC-LEANWAIVE-002", "feature", "internal/cli/spec_timing.go", "")
		},
		"lean with the section present": func(dir string) string {
			return writeLeanReviewSpec(t, dir, "SPEC-LEANWAIVE-002", "bugfix_existing_contract", "internal/cli/spec_timing.go",
				"\n## Risk-First Integration Probe\n\n| assumption_id | status |\n|---|---|\n| A1 | not-run |\n")
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			specDir := build(t.TempDir())
			result := runSingleReviewerLoop(t, "SPEC-LEANWAIVE-002", specDir, probeChecklistFail)

			assert.Equal(t, spec.VerdictRevise, result.Verdict)
			require.NotEmpty(t, result.BlockingReasons)
			assert.Equal(t, spec.BlockingPolicyReviewerChecklist, result.BlockingReasons[0].Policy)
		})
	}
}

// S5: a lean SPEC is reviewed for its real blockers exactly like a full one.
func TestRunSpecReviewLoop_LeanRealBlockersStillBlock(t *testing.T) {
	cases := map[string]struct {
		output string
		policy string
	}{
		"security at suggestion severity": {singleFindingReviewOutput("suggestion", "security", "Token lifetime is unbounded"), spec.BlockingPolicyOpenSecurity},
		"missing acceptance oracle":       {singleFindingReviewOutput("major", "completeness", "acceptance.md S1 has no expected output"), spec.BlockingPolicyOpenBlocking},
		"data loss from a JSON reviewer":  {singleFindingReviewOutput("suggestion", "data_loss", "Retry drops queued rows"), spec.BlockingPolicyOpenCorrectness},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			const specID = "SPEC-LEANBLOCK-001"
			specDir := writeLeanReviewSpec(t, t.TempDir(), specID, "bugfix_existing_contract", "internal/cli/spec_timing.go", "")

			result := runSingleReviewerLoop(t, specID, specDir, tc.output)

			assert.Equal(t, spec.VerdictRevise, result.Verdict)
			require.Len(t, result.BlockingReasons, 1)
			assert.Equal(t, tc.policy, result.BlockingReasons[0].Policy)
		})
	}
}

// S3: a judge PASS used to waive an accepted minor correctness finding that
// blocks the same review when no judge runs. The judge must reject or
// downgrade a finding to clear it; accepting it keeps it blocking.
func TestRunSpecReviewLoop_JudgePassCannotWaiveBlockingFinding(t *testing.T) {
	result, _ := runJudgeMergeLoop(t, validStructuredJudgeOutput("PASS", "minor"), nil)

	assert.Equal(t, spec.VerdictRevise, result.Verdict)
	require.Len(t, result.BlockingReasons, 1)
	assert.Equal(t, "F-001", result.BlockingReasons[0].FindingID)
	assert.Equal(t, spec.BlockingPolicyOpenCorrectness, result.BlockingReasons[0].Policy)
	require.Len(t, result.Findings, 1)
	assert.Equal(t, spec.FindingStatusOpen, result.Findings[0].Status)
}

// S3: the finding sets of the policy table resolve to the same verdict whether
// a reviewer votes alone or a judge PASS accepts the findings.
func TestRunSpecReviewLoop_ReviewerAndJudgeShareOneDisposition(t *testing.T) {
	cases := []struct {
		severity, category string
		verdict            spec.ReviewVerdict
		policy             string
	}{
		{"suggestion", "completeness", spec.VerdictPass, ""},
		{"minor", "style", spec.VerdictPass, ""},
		{"minor", "completeness", spec.VerdictRevise, spec.BlockingPolicyOpenBlocking},
		{"major", "completeness", spec.VerdictRevise, spec.BlockingPolicyOpenBlocking},
		{"minor", "correctness", spec.VerdictRevise, spec.BlockingPolicyOpenCorrectness},
		{"suggestion", "data_loss", spec.VerdictRevise, spec.BlockingPolicyOpenCorrectness},
		{"suggestion", "security", spec.VerdictRevise, spec.BlockingPolicyOpenSecurity},
	}
	for _, tc := range cases {
		t.Run(tc.severity+"_"+tc.category, func(t *testing.T) {
			vote := "REVISE"
			if tc.verdict == spec.VerdictPass {
				vote = "PASS"
			}
			reviewer := strings.Replace(singleFindingReviewOutput(tc.severity, tc.category, "finding under test"),
				`"verdict":"REVISE"`, `"verdict":"`+vote+`"`, 1)
			specDir := scaffoldReviewSpec(t, t.TempDir(), "SPEC-DISPOSE-001")
			alone := runSingleReviewerLoop(t, "SPEC-DISPOSE-001", specDir, reviewer)
			judged, _ := runJudgeMergeLoop(t, judgeOutputAccepting("PASS", tc.severity, tc.category), nil)

			for name, result := range map[string]*spec.ReviewResult{"reviewer": alone, "judge": judged} {
				assert.Equal(t, tc.verdict, result.Verdict, name)
				if tc.policy == "" {
					assert.Empty(t, result.BlockingReasons, name)
					continue
				}
				require.NotEmpty(t, result.BlockingReasons, name)
				assert.Equal(t, tc.policy, result.BlockingReasons[0].Policy, name)
			}
		})
	}
}

func judgeOutputAccepting(verdict, severity, category string) string {
	return `{"verdict":"` + verdict + `","findings":[{"severity":"` + severity + `","category":"` + category +
		`","scope_ref":"REQ-JUDGE-001","location":"spec.md:54","description":"Judge finding","suggestion":"Fix it",` +
		`"decision":"accept","sources":["Reviewer A"]}],"rationale":"reviewed"}`
}
