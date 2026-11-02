package spec_test

// SPEC-REVIEWPOLICY-001 S1, S3, S4: the policy table, the shared verdict rule
// lines, and the profile-aware reviewer prompt.

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/spec"
	"github.com/insajin/autopus-adk/pkg/spec/gates"
)

func TestReviewPolicyFor_TableIsExact(t *testing.T) {
	t.Parallel()

	lean, ok := spec.ReviewPolicyFor(spec.ReviewProfileLean)
	require.True(t, ok)
	full, ok := spec.ReviewPolicyFor(spec.ReviewProfileFull)
	require.True(t, ok)
	compact, ok := spec.ReviewPolicyFor(spec.ReviewProfileCompact)
	require.True(t, ok)
	_, ok = spec.ReviewPolicyFor("quick")
	assert.False(t, ok)

	assert.Equal(t, map[string][]string{
		"spec.md":       {"## Outcome Boundary", "## Requirements", "## Traceability Matrix"},
		"plan.md":       {"## Tasks"},
		"acceptance.md": {"## Test Scenarios"},
		"research.md":   {"## Outcome Lock", "## Reviewer Brief"},
	}, lean.RequiredSections)
	assert.Equal(t, []string{
		"## Implementation Strategy", "## Visual Planning Brief", "## Feature Completion Scope",
		"## Risk-First Integration Probe", "## Oracle Acceptance Notes", "## Semantic Invariant Inventory",
		"## Minimality Decision Matrix", "## Feature Coverage Map", "## Completion Debt", "## Evolution Ideas",
		"## Reference Discipline", "## Self-Verify Summary",
	}, lean.OmittedHeadings())
	for name, sections := range full.RequiredSections {
		for _, section := range sections {
			if !contains(lean.RequiredSections[name], section) {
				assert.Contains(t, lean.OmittedSections[name], section, "%s %s is neither required nor omitted on lean", name, section)
			}
		}
		for _, omitted := range lean.OmittedSections[name] {
			assert.NotContains(t, lean.RequiredSections[name], omitted)
		}
	}
	var waivers []string
	for _, waiver := range lean.ChecklistWaivers {
		waivers = append(waivers, waiver.ID+" "+waiver.Document+" "+strings.Join(waiver.Sections, "+"))
	}
	assert.Equal(t, []string{
		"Q-COMP-08 plan.md ## Risk-First Integration Probe",
		"Q-COMP-07 research.md ## Completion Debt+## Evolution Ideas",
		"Q-CORR-04 research.md ## Reference Discipline",
	}, waivers)

	assert.Len(t, full.RequiredHeadings(), 18)
	assert.Empty(t, full.OmittedSections)
	assert.True(t, full.RiskFirstProbe && full.SelfVerify)
	assert.False(t, lean.RiskFirstProbe || lean.SelfVerify)
	assert.Equal(t, map[string][]string{
		"change.md": {"## Referenced SPEC", "## Intended Surface", "## Verification Plan", "## Gate Applicability"},
	}, compact.RequiredSections)

	mandatory := []gates.GateID{gates.GateSecurity, gates.GateValidation, gates.GateDataLoss, gates.GateDeterministicOracle}
	for _, policy := range []spec.ReviewPolicy{lean, full, compact} {
		assert.Equal(t, mandatory, policy.MandatoryGates, policy.Profile)
	}
	assert.Equal(t, spec.ReviewExecution{Review: true, Reviewers: 1, Quorum: 1}, lean.Execution)
	assert.Equal(t, spec.ReviewExecution{Review: true, Judge: true}, full.Execution)
	assert.Equal(t, spec.ReviewExecution{}, compact.Execution)

	// Callers get copies: mutating one cannot leak into the next lookup.
	lean.RequiredSections["plan.md"][0] = "## Mutated"
	again, _ := spec.ReviewPolicyFor(spec.ReviewProfileLean)
	assert.Equal(t, "## Tasks", again.RequiredSections["plan.md"][0])
}

func TestEffectiveReviewPolicy_OnlyEligibleLeanIsLean(t *testing.T) {
	t.Parallel()

	cases := []struct {
		doc     *spec.SpecDocument
		profile string
		reason  string
	}{
		{&spec.SpecDocument{}, spec.ReviewProfileFull, ""},
		{nil, spec.ReviewProfileFull, ""},
		{&spec.SpecDocument{Profile: "lean", ChangeClass: "bugfix_existing_contract", Surface: []string{"internal/cli/foo.go"}}, spec.ReviewProfileLean, ""},
		{&spec.SpecDocument{Profile: "lean", ChangeClass: "feature", Surface: []string{"internal/cli/foo.go"}}, spec.ReviewProfileFull, "feature"},
		{&spec.SpecDocument{Profile: "lean", ChangeClass: "test_only", Surface: []string{"internal/auth/session.go"}}, spec.ReviewProfileFull, "security_or_data_surface_in_change_set"},
		{&spec.SpecDocument{Profile: "lean", ChangeClass: "docs_only"}, spec.ReviewProfileFull, "surface"},
	}
	for _, tc := range cases {
		policy := spec.EffectiveReviewPolicy(tc.doc)
		assert.Equal(t, tc.profile, policy.Profile)
		if tc.reason == "" {
			assert.Empty(t, policy.EscalationReason)
		} else {
			assert.Contains(t, policy.EscalationReason, tc.reason)
		}
	}
}

// S3: the reviewer prompt (discover and verify) and the judge template carry
// the same rule lines, and the judge template no longer differs on majors.
func TestReviewVerdictRules_PromptAndJudgeTemplateCarryTheSameLines(t *testing.T) {
	t.Parallel()

	lines := spec.ReviewVerdictRuleLines()
	require.Len(t, lines, 7)
	discover := spec.BuildReviewPrompt(&spec.SpecDocument{ID: "SPEC-RULES-002"}, "", spec.ReviewPromptOptions{})
	verify := spec.BuildReviewPrompt(&spec.SpecDocument{ID: "SPEC-RULES-002"}, "", spec.ReviewPromptOptions{
		Mode: spec.ReviewModeVerify, PriorFindings: []spec.ReviewFinding{{ID: "F-001", Severity: "major"}},
	})
	builder, err := orchestra.NewPromptBuilder()
	require.NoError(t, err)
	judge, err := builder.BuildReviewJudge(orchestra.ReviewJudgeData{SpecID: "SPEC-RULES-002", Mode: "discover", SchemaJSON: "{}"})
	require.NoError(t, err)

	for _, line := range lines {
		assert.Contains(t, discover, line+"\n")
		assert.Contains(t, verify, line+"\n")
		assert.Contains(t, judge, line+"\n")
	}
	for _, text := range []string{discover, verify, judge} {
		assert.NotContains(t, text, "major <= 2")
		assert.NotContains(t, text, "Otherwise, any accepted critical or major finding requires")
	}
}

// S4: every prompt states the purpose; only an eligible lean SPEC lists its
// allowed omissions; pass_criteria adds to the rules instead of replacing them.
func TestBuildReviewPrompt_PolicySectionPerProfile(t *testing.T) {
	t.Parallel()

	docs := map[string]*spec.SpecDocument{
		"lean":    {ID: "SPEC-P-001", Profile: "lean", ChangeClass: "test_only", Surface: []string{"internal/cli/x_test.go"}},
		"refused": {ID: "SPEC-P-002", Profile: "lean", ChangeClass: "feature", Surface: []string{"internal/cli/x.go"}},
		"full":    {ID: "SPEC-P-003"},
	}
	for name, doc := range docs {
		prompt := spec.BuildReviewPrompt(doc, "", spec.ReviewPromptOptions{PassCriteria: "CUSTOM_RULE"})
		assert.Contains(t, prompt, "can implementation start", name)
		rules := strings.Index(prompt, spec.ReviewVerdictRuleLines()[0])
		extra := strings.Index(prompt, "Additional project criteria (they cannot relax the rules above):\nCUSTOM_RULE")
		require.Positive(t, rules, name)
		assert.Greater(t, extra, rules, name)
		if name != "lean" {
			assert.NotContains(t, prompt, "Allowed omissions", name)
			continue
		}
		start := strings.Index(prompt, "Allowed omissions (profile lean): ")
		require.Positive(t, start)
		allowance := prompt[start : start+strings.Index(prompt[start:], "\n\n")]
		lean, _ := spec.ReviewPolicyFor(spec.ReviewProfileLean)
		for _, heading := range lean.OmittedHeadings() {
			assert.Contains(t, allowance, heading)
		}
		for _, id := range []string{"Q-COMP-08", "Q-COMP-07", "Q-CORR-04"} {
			assert.Contains(t, allowance, id)
		}
	}
	refused := spec.BuildReviewPrompt(docs["refused"], "", spec.ReviewPromptOptions{})
	assert.Contains(t, refused, "review it against the full profile")
}

func TestApplyChecklistWaivers_NeedsTheSectionToBeAbsent(t *testing.T) {
	t.Parallel()

	lean, _ := spec.ReviewPolicyFor(spec.ReviewProfileLean)
	outcomes := []spec.ChecklistOutcome{
		{ID: "Q-COMP-08", Status: spec.ChecklistStatusFail, Reason: "no probe"},
		{ID: "Q-COMP-07", Status: spec.ChecklistStatusFail, Reason: "no debt split"},
		{ID: "Q-COMP-05", Status: spec.ChecklistStatusFail, Reason: "acceptance is structural"},
		{ID: "Q-CORR-04", Status: spec.ChecklistStatusPass},
	}
	files := map[string]string{
		"plan.md":     "# Plan\n\n## Tasks\n",
		"research.md": "# Research\n\n## Completion Debt\n\nNone\n",
	}

	got := lean.ApplyChecklistWaivers(outcomes, files)
	assert.Equal(t, spec.ChecklistStatusNA, got[0].Status)
	assert.Contains(t, got[0].Reason, "profile lean omits ## Risk-First Integration Probe in plan.md")
	assert.Contains(t, got[0].Reason, "no probe")
	assert.Equal(t, spec.ChecklistStatusFail, got[1].Status, "one waived section is present")
	assert.Equal(t, spec.ChecklistStatusFail, got[2].Status, "Q-COMP-05 is never waived")
	assert.Equal(t, spec.ChecklistStatusPass, got[3].Status)
	assert.Equal(t, spec.ChecklistStatusFail, outcomes[0].Status, "input is not mutated")

	unread := lean.ApplyChecklistWaivers(outcomes, map[string]string{})
	assert.Equal(t, spec.ChecklistStatusFail, unread[0].Status, "an unread document never confirms an absence")
	full, _ := spec.ReviewPolicyFor(spec.ReviewProfileFull)
	assert.Equal(t, outcomes, full.ApplyChecklistWaivers(outcomes, files))
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}
