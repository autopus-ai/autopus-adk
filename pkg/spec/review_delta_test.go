package spec_test

// SPEC-REVIEWREUSE-001 S14, S15, S17 (REQ-RR-DELTA-02 to REQ-RR-DELTA-05): the
// delta plan falls back on the first failed condition, the delta prompt
// carries the changes and the linked context of the open findings, and a
// manifest that cannot account for a source is refused.

import (
	"maps"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/spec"
)

func TestPlanDeltaVerify_FallsBackOnTheFirstFailedCondition(t *testing.T) {
	t.Parallel()

	base := deltaDocuments()
	cases := []struct {
		name   string
		mutate func(in *spec.DeltaVerifyInput)
		reason string
	}{
		{"local fix", func(*spec.DeltaVerifyInput) {}, spec.DeltaReasonDelta},
		{"no baseline", func(in *spec.DeltaVerifyInput) { in.Baseline = nil }, spec.DeltaReasonMissingBaseline},
		{"unknown baseline schema", func(in *spec.DeltaVerifyInput) { in.Baseline.Schema = "spec_review_baseline.v9" }, spec.DeltaReasonMissingBaseline},
		{"stored content edited", func(in *spec.DeltaVerifyInput) {
			doc := in.Baseline.Documents["plan.md"]
			doc.Content += "tampered\n"
			in.Baseline.Documents["plan.md"] = doc
		}, spec.DeltaReasonBaselineHashMismatch},
		{"other open findings", func(in *spec.DeltaVerifyInput) { in.Baseline.OpenIDs = []string{"F-009"} }, spec.DeltaReasonBaselineHashMismatch},
		{"checklist changed", func(in *spec.DeltaVerifyInput) { in.Context["checklist"] = "c2" }, spec.DeltaReasonContextChanged},
		{"worktree changed", func(in *spec.DeltaVerifyInput) { in.Context["code.worktree"] = "w2" }, spec.DeltaReasonContextChanged},
		{"outcome boundary edited", func(in *spec.DeltaVerifyInput) {
			in.Documents["spec.md"] = strings.Replace(in.Documents["spec.md"], "- In scope: the fixture.", "- In scope: the fixture and more.", 1)
		}, spec.DeltaReasonInterfaceChange},
		{"new requirement", func(in *spec.DeltaVerifyInput) {
			in.Documents["spec.md"] = strings.Replace(in.Documents["spec.md"], "THE SYSTEM SHALL store input.\n",
				"THE SYSTEM SHALL store input.\n\n**REQ-004**\nPriority: Must\nTHE SYSTEM SHALL log input.\n", 1)
		}, spec.DeltaReasonInterfaceChange},
		{"new document", func(in *spec.DeltaVerifyInput) { in.Documents["notes.md"] = "# Notes\n" }, spec.DeltaReasonInterfaceChange},
		{"finding scope absent", func(in *spec.DeltaVerifyInput) { in.Findings[0].ScopeRef = "REQ-999" }, spec.DeltaReasonUnclearImpact},
		{"unrelated requirement edited", func(in *spec.DeltaVerifyInput) {
			in.Documents["spec.md"] = strings.Replace(in.Documents["spec.md"], "THE SYSTEM SHALL store input.", "THE SYSTEM SHALL store input twice.", 1)
		}, spec.DeltaReasonOutOfScope},
		{"no change", func(in *spec.DeltaVerifyInput) { in.Documents = maps.Clone(base) }, spec.DeltaReasonNoChange},
	}
	for _, tc := range cases {
		in := spec.DeltaVerifyInput{
			Baseline:  deltaBaseline(base),
			Documents: deltaFix(base),
			Context:   map[string]string{"checklist": "c1", "code.worktree": "w1"},
			Findings:  []spec.ReviewFinding{deltaFinding()},
		}
		tc.mutate(&in)
		assert.Equal(t, tc.reason, spec.PlanDeltaVerify(in).Reason, tc.name)
	}
}

func deltaPromptFixture(t *testing.T) (string, []spec.DeltaSource, spec.DeltaPlan) {
	t.Helper()
	base := deltaDocuments()
	current := deltaFix(base)
	in := spec.DeltaVerifyInput{Baseline: deltaBaseline(base), Documents: current,
		Context: map[string]string{"checklist": "c1", "code.worktree": "w1"}, Findings: []spec.ReviewFinding{deltaFinding()}}
	plan := spec.PlanDeltaVerify(in)
	require.Equal(t, spec.DeltaReasonDelta, plan.Reason)
	doc := &spec.SpecDocument{ID: "SPEC-DELTA-001", Title: "Delta fixture"}
	prompt, manifest := spec.BuildDeltaVerifyPrompt(spec.DeltaPromptInput{
		Doc: doc, Plan: plan, Baseline: in.Baseline, Documents: current, PriorFindings: in.Findings,
		Sources:   []spec.DeltaSource{{Name: "checklist", SHA256: "c1", Delivery: spec.DeltaDeliveryIncluded}},
		Checklist: "CHECKLIST-BODY",
	})
	return prompt, manifest, plan
}

func TestBuildDeltaVerifyPrompt_CarriesTheFixAndLinksTheRest(t *testing.T) {
	t.Parallel()

	prompt, manifest, _ := deltaPromptFixture(t)
	current := deltaFix(deltaDocuments())

	for _, want := range []string{
		"## SPEC: SPEC-DELTA-001 — Delta fixture",
		"### Delta Verify Input (spec_review_delta.v1)",
		"- acceptance.md sha256:" + spec.SHA256Hex([]byte(current["acceptance.md"])) + " changed\n",
		"- spec.md sha256:" + spec.SHA256Hex([]byte(current["spec.md"])) + " changed\n",
		"- plan.md sha256:" + spec.SHA256Hex([]byte(current["plan.md"])) + " linked (baseline run-1 r0)\n",
		"- research.md sha256:" + spec.SHA256Hex([]byte(current["research.md"])) + " linked (baseline run-1 r0)\n",
		"- checklist sha256:c1 included\n",
		"\n-THE SYSTEM SHALL validate input.\n",
		"\n+THE SYSTEM SHALL validate input and refuse an empty value with error E1.\n",
		"**REQ-002**\nPriority: Must\nTHE SYSTEM SHALL validate input and refuse an empty value with error E1.\n",
		"### S2: AC-D-002 - Validate\nGiven bad input\nWhen it arrives\nThen it is refused with E1.\n",
		"| REQ-002 | T2 | S2 |",
		"- F-001 [major] REQ-002: error path undefined\n  - acceptance.md @@",
		"\n  - spec.md @@",
		"## Quality Checklist\n\nCHECKLIST-BODY",
		"### Instructions (Verify Mode)",
		"#### Prior Findings Checklist",
	} {
		assert.Contains(t, prompt, want)
	}
	for _, absent := range []string{"PLAN-BODY-MARKER", "RESEARCH-BODY-MARKER", "THE SYSTEM SHALL store input."} {
		assert.NotContains(t, prompt, absent)
	}
	assert.Len(t, manifest, 5)
}

func TestValidateDeltaManifest_RefusesAnUnaccountedSource(t *testing.T) {
	t.Parallel()

	prompt, manifest, _ := deltaPromptFixture(t)
	base, current := deltaDocuments(), deltaFix(deltaDocuments())
	required := map[string]string{"checklist": "c1"}
	baseline := map[string]string{"checklist": "c1"}
	for name := range current {
		required[name] = spec.SHA256Hex([]byte(current[name]))
		baseline[name] = spec.SHA256Hex([]byte(base[name]))
	}
	require.NoError(t, spec.ValidateDeltaManifest(manifest, required, baseline, prompt))

	without := func(name string) []spec.DeltaSource {
		var out []spec.DeltaSource
		for _, source := range manifest {
			if source.Name != name {
				out = append(out, source)
			}
		}
		return out
	}
	assert.Error(t, spec.ValidateDeltaManifest(without("research.md"), required, baseline, prompt), "missing source")

	moved := maps.Clone(required)
	moved["research.md"] = spec.SHA256Hex([]byte("changed research"))
	assert.Error(t, spec.ValidateDeltaManifest(manifest, moved, baseline, prompt), "linked source changed")

	noDiff := strings.Replace(prompt, "##### spec.md\n\n```diff", "##### spec.md\n\n```text", 1)
	assert.Error(t, spec.ValidateDeltaManifest(manifest, required, baseline, noDiff), "changed source without its diff")
}

func TestReviewBaseline_PersistsBesideFindingsAndCoverage(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	findings := []spec.ReviewFinding{deltaFinding()}
	coverages := []spec.DocCoverage{{Name: "plan.md", Injected: 3, Total: 3, Percent: 100, Complete: true}}
	require.NoError(t, spec.PersistFindingsWithCoverage(dir, findings, coverages))
	baseline := deltaBaseline(deltaDocuments())
	require.NoError(t, spec.PersistReviewBaseline(dir, baseline))

	loadedFindings, loadedCoverage, err := spec.LoadFindingsWithCoverage(dir)
	require.NoError(t, err)
	assert.Equal(t, findings[0].ID, loadedFindings[0].ID)
	assert.Equal(t, coverages, loadedCoverage)
	loaded, err := spec.LoadReviewBaseline(dir)
	require.NoError(t, err)
	assert.Equal(t, baseline, loaded)

	empty := t.TempDir()
	missing, err := spec.LoadReviewBaseline(empty)
	require.NoError(t, err)
	assert.Nil(t, missing)
	require.NoError(t, os.WriteFile(empty+"/review-findings.json", []byte("{"), 0o644))
	_, err = spec.LoadReviewBaseline(empty)
	assert.Error(t, err)
}
