package cli

// SPEC-REVIEWREUSE-001 S13 to S16 and S18 (REQ-RR-DELTA-01 to REQ-RR-DELTA-07):
// a local fix is verified from the delta input, every fallback runs the full
// input on the same budget, and a delta round still opens a new safety claim.

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/spec"
)

const deltaReviewSpec = `# %s: Delta fixture

**Status**: draft

## Purpose

Delta fixture purpose.

## Outcome Boundary

- In scope: the fixture.

## Requirements

### Core

**REQ-001**
Priority: Must
THE SYSTEM SHALL accept input.

**REQ-002**
Priority: Must
THE SYSTEM SHALL validate input.

**REQ-003**
Priority: Must
THE SYSTEM SHALL store input.

## Traceability Matrix

| Requirement | Task | Scenario |
|-------------|------|----------|
| REQ-001 | T1 | S1 |
| REQ-002 | T2 | S2 |
| REQ-003 | T3 | S3 |
`

const deltaReviewAcceptance = "# Acceptance\n\n## Test Scenarios\n\n### S1: AC-D-001 - Accept\nGiven input\nWhen it arrives\nThen it is accepted.\n\n" +
	"### S2: AC-D-002 - Validate\nGiven bad input\nWhen it arrives\nThen it is refused.\n\n### S3: AC-D-003 - Store\nGiven input\nWhen it is accepted\nThen it is stored.\n"

const reviseOnREQ002 = `{"verdict":"REVISE","summary":"gap","findings":[{"severity":"major","category":"completeness","scope_ref":"REQ-002","location":"spec.md","description":"error path undefined","suggestion":"define it"}]}`

// deltaFixture is a git-tracked review project under verify_input delta whose
// fake reviewers open F-001 on REQ-002 in round 0; edit runs during the last
// round-0 call, before round 1 builds its input, as the author would.
func deltaFixture(t *testing.T, edit func(specDir string)) (*readOnlyReviewFixture, *scriptedBudgetBackend) {
	t.Helper()
	return deltaFixtureWith(t, nil, 400, edit)
}

// deltaFixtureWith is deltaFixture with a configuration edit and a number of
// unchanged filler lines in plan.md's and research.md's Notes sections, which
// name no id of the fix, so a delta request links them.
func deltaFixtureWith(t *testing.T, configure func(*config.HarnessConfig), filler int, edit func(specDir string)) (*readOnlyReviewFixture, *scriptedBudgetBackend) {
	t.Helper()
	reviewers := 0
	fixture, backend := reuseFixture(t, config.ReviewBudgetConf{}, func(cfg *config.HarnessConfig) {
		cfg.Spec.ReviewGate.VerifyInput = "delta"
		cfg.Spec.ReviewGate.MaxRevisions = new(1)
		if configure != nil {
			configure(cfg)
		}
		reviewers = len(cfg.Spec.ReviewGate.Providers)
	})
	write := func(name, body string) {
		require.NoError(t, os.WriteFile(filepath.Join(fixture.specDir, name), []byte(body), 0o644))
	}
	write("spec.md", strings.Replace(deltaReviewSpec, "%s", fixture.specID, 1))
	write("acceptance.md", deltaReviewAcceptance)
	// The unchanged documents outweigh the checklist and instructions, as in
	// a real SPEC, so linking them saves input.
	notes := strings.Repeat("Unchanged planning detail that the baseline round already reviewed.\n", filler)
	write("plan.md", "# Plan\n\n## Tasks\n\n- T1\n- T2: validate in internal/x.go\n- T3\n\n## Notes\n\nPLAN-BODY-MARKER\n\n"+notes)
	write("research.md", "# Research\n\n## Outcome Lock\n\nLock.\n\n## Notes\n\nRESEARCH-BODY-MARKER\n\n"+notes)
	commitFixtureRepo(t, fixture.root, "delta fixture")
	backend.replies["claude/reviewer"] = []scriptedReply{{output: reviseOnREQ002}, {output: passNoFinding}}
	backend.replies["codex/reviewer"] = []scriptedReply{{output: reviseOnREQ002}, {output: passNoFinding}}
	var calls atomic.Int32
	backend.onCall = func(orchestra.ProviderRequest) {
		if int(calls.Add(1)) == reviewers && edit != nil {
			edit(fixture.specDir)
		}
	}
	return fixture, backend
}

func replaceInFile(t *testing.T, path, old, replacement string) {
	t.Helper()
	data := string(readFixtureFile(t, path))
	require.Contains(t, data, old)
	require.NoError(t, os.WriteFile(path, []byte(strings.Replace(data, old, replacement, 1)), 0o644))
}

func localFix(t *testing.T) func(string) {
	return func(specDir string) {
		replaceInFile(t, filepath.Join(specDir, "spec.md"), "THE SYSTEM SHALL validate input.",
			"THE SYSTEM SHALL validate input and refuse an empty value with error E1.")
		replaceInFile(t, filepath.Join(specDir, "acceptance.md"), "Then it is refused.", "Then it is refused with E1.")
	}
}

func TestRunSpecReview_VerifyInputFlagIsValidated(t *testing.T) {
	fixture, backend := budgetFixture(t, config.ReviewBudgetConf{}, nil)
	err := runSpecReviewCommand(t, fixture, "--verify-input", "partial")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `--verify-input "partial": expected one of full, delta`)
	assert.Zero(t, backend.count())
}

// S14 and S16: a local fix is verified from the delta input.
func TestRunSpecReview_LocalFixIsVerifiedFromTheDelta(t *testing.T) {
	fixture, backend := deltaFixture(t, nil)
	var baseline *spec.ReviewBaseline
	fix := localFix(t)
	var calls atomic.Int32
	backend.onCall = func(orchestra.ProviderRequest) {
		switch calls.Add(1) {
		case 2:
			fix(fixture.specDir)
		case 3:
			baseline, _ = spec.LoadReviewBaseline(fixture.specDir)
		}
	}

	_, stderr := runReviewOutput(t, fixture, specReviewOptions{})

	require.Equal(t, 4, backend.count())
	require.NotNil(t, baseline, "round 0 stored a baseline")
	assert.Equal(t, spec.ReviewBaselineSchema, baseline.Schema)
	assert.Equal(t, 0, baseline.Revision)
	assert.Len(t, baseline.Documents, 4)
	assert.Equal(t, []string{"F-001"}, baseline.OpenIDs)
	ref := baseline.RunID + " r0"
	round0, round1 := backend.requests[0].Prompt, backend.requests[2].Prompt
	current := spec.NewReviewInputDocument(readFixtureFile(t, filepath.Join(fixture.specDir, "spec.md")))
	for _, want := range []string{
		"### Delta Verify Input (spec_review_delta.v1)",
		"- plan.md source_sha256:", " linked (baseline " + ref + ": unchanged", "- checklist source_sha256:",
		"- spec.md source_sha256:" + current.SourceSHA256 + " prompt_sha256:" + current.PromptSHA256 + " changed\n",
		"\n-THE SYSTEM SHALL validate input.\n", "\n+THE SYSTEM SHALL validate input and refuse an empty value with error E1.\n",
		"**REQ-002**\nPriority: Must\n", "### S2: AC-D-002 - Validate\n", "| REQ-002 | T2 | S2 |",
		"- F-001 [major] REQ-002: error path undefined", "Instructions (Verify Mode)",
	} {
		assert.Contains(t, round1, want)
	}
	for _, absent := range []string{"PLAN-BODY-MARKER", "RESEARCH-BODY-MARKER"} {
		assert.Contains(t, round0, absent)
		assert.NotContains(t, round1, absent)
	}
	assert.Less(t, len(round1), len(round0))
	assert.Contains(t, stderr, "SPEC 리뷰 입력: full — discover\n")
	assert.Contains(t, stderr, "SPEC 리뷰 입력: delta (baseline "+ref+")\n")
	rounds := readSpecReviewReceipt(t, fixture.specDir).Rounds
	require.Len(t, rounds, 2)
	assert.Equal(t, [2]string{"full", "discover"}, [2]string{rounds[0].Input, rounds[0].InputReason})
	assert.Equal(t, [2]string{"delta", "delta"}, [2]string{rounds[1].Input, rounds[1].InputReason})
}

// S15 and S16: a change the delta cannot account for falls back to the full
// input on the same budget.
func TestRunSpecReview_DeltaFallsBackToTheFullInput(t *testing.T) {
	cases := map[string]struct {
		edit   func(t *testing.T, specDir string)
		reason string
	}{
		"outcome boundary": {func(t *testing.T, specDir string) {
			localFix(t)(specDir)
			replaceInFile(t, filepath.Join(specDir, "spec.md"), "- In scope: the fixture.", "- In scope: the fixture and more.")
		}, spec.DeltaReasonInterfaceChange},
		"no saving": {func(t *testing.T, specDir string) {
			replaceInFile(t, filepath.Join(specDir, "spec.md"), "THE SYSTEM SHALL validate input.",
				"THE SYSTEM SHALL validate input."+strings.Repeat("\nREQ-002 detail line for the validation contract.", 3000))
		}, spec.DeltaReasonNoSaving},
		"unrelated requirement": {func(t *testing.T, specDir string) {
			localFix(t)(specDir)
			replaceInFile(t, filepath.Join(specDir, "spec.md"), "THE SYSTEM SHALL store input.", "THE SYSTEM SHALL store input twice.")
		}, spec.DeltaReasonOutOfScope},
		"sibling requirement naming REQ-002": {func(t *testing.T, specDir string) {
			localFix(t)(specDir)
			replaceInFile(t, filepath.Join(specDir, "spec.md"), "THE SYSTEM SHALL store input.\n",
				"THE SYSTEM SHALL store input.\nIt SHALL keep the REQ-002 validation result.\n")
		}, spec.DeltaReasonOutOfScope},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fixture, backend := deltaFixture(t, func(specDir string) { tc.edit(t, specDir) })

			_, stderr := runReviewOutput(t, fixture, specReviewOptions{})

			assert.Equal(t, 4, backend.count(), "the fallback round spends the same budget")
			assert.NotContains(t, backend.requests[2].Prompt, "spec_review_delta.v1")
			assert.Contains(t, stderr, "SPEC 리뷰 입력: full — "+tc.reason+"\n")
			rounds := readSpecReviewReceipt(t, fixture.specDir).Rounds
			require.Len(t, rounds, 2)
			assert.Equal(t, [2]string{"full", tc.reason}, [2]string{rounds[1].Input, rounds[1].InputReason})
			assert.Len(t, readTestSpecReviewSession(t, fixture.specDir).Calls, 4)
		})
	}
}

// S18: a delta round still opens a new critical security finding.
func TestRunSpecReview_DeltaRoundStillOpensANewSafetyClaim(t *testing.T) {
	fixture, backend := deltaFixture(t, localFix(t))
	backend.replies["codex/reviewer"] = []scriptedReply{{output: reviseOnREQ002},
		{output: reviewerWith("REVISE", "critical", "security", "REQ-001", "Token logged")}}

	runReviewOutput(t, fixture, specReviewOptions{})

	assert.Contains(t, backend.requests[3].Prompt, "spec_review_delta.v1")
	receipt := readSpecReviewReceipt(t, fixture.specDir)
	assert.Equal(t, "REVISE", receipt.Verdict)
	assert.Contains(t, blockingPolicies(receipt.BlockingReasons), spec.BlockingPolicyOpenSecurity)
	findings, err := spec.LoadFindings(fixture.specDir)
	require.NoError(t, err)
	var security *spec.ReviewFinding
	for i := range findings {
		if findings[i].Description == "Token logged" {
			security = &findings[i]
		}
	}
	require.NotNil(t, security)
	assert.True(t, security.EscapeHatch)
	assert.Equal(t, spec.FindingStatusOpen, security.Status)
}
