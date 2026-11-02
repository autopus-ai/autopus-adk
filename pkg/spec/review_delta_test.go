package spec_test

// SPEC-REVIEWREUSE-001 S14, S15, S17 (REQ-RR-DELTA-02 to REQ-RR-DELTA-05): the
// delta plan falls back on the first failed condition, the delta prompt
// carries the changes and the context of the open findings, and a manifest
// that cannot account for a source is refused.

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
	replace := func(name, old, replacement string) func(map[string]string) {
		return func(current map[string]string) { current[name] = strings.Replace(current[name], old, replacement, 1) }
	}
	cases := []struct {
		name   string
		edit   func(current map[string]string)
		adjust func(in *spec.DeltaVerifyInput)
		reason string
	}{
		{"local fix", nil, nil, spec.DeltaReasonDelta},
		{"no baseline", nil, func(in *spec.DeltaVerifyInput) { in.Baseline = nil }, spec.DeltaReasonMissingBaseline},
		{"unknown baseline schema", nil, func(in *spec.DeltaVerifyInput) { in.Baseline.Schema = "spec_review_baseline.v9" }, spec.DeltaReasonMissingBaseline},
		{"stored content edited", nil, func(in *spec.DeltaVerifyInput) {
			doc := in.Baseline.Documents["plan.md"]
			doc.Content += "tampered\n"
			in.Baseline.Documents["plan.md"] = doc
		}, spec.DeltaReasonBaselineHashMismatch},
		{"other open findings", nil, func(in *spec.DeltaVerifyInput) { in.Baseline.OpenIDs = []string{"F-009"} }, spec.DeltaReasonBaselineHashMismatch},
		{"plan.md delivered partial", nil, func(in *spec.DeltaVerifyInput) {
			in.Baseline.Delivered["plan.md"] = spec.BaselineDeliveredPartial
		}, spec.DeltaReasonIncompleteBaseline},
		{"no delivered scope", nil, func(in *spec.DeltaVerifyInput) { in.Baseline.Delivered = nil }, spec.DeltaReasonIncompleteBaseline},
		{"checklist changed", nil, func(in *spec.DeltaVerifyInput) { in.Context["checklist"] = "c2" }, spec.DeltaReasonContextChanged},
		{"worktree changed", nil, func(in *spec.DeltaVerifyInput) { in.Context["code.worktree"] = "w2" }, spec.DeltaReasonContextChanged},
		{"document removed", func(current map[string]string) { delete(current, "research.md") }, nil, spec.DeltaReasonInterfaceChange},
		{"secret value swapped", nil, func(in *spec.DeltaVerifyInput) {
			doc := in.Documents["plan.md"]
			doc.SourceSHA256 = strings.Repeat("0", 64)
			in.Documents["plan.md"] = doc
		}, spec.DeltaReasonIntegrityGap},
		{"outcome boundary edited", replace("spec.md", "- In scope: the fixture.", "- In scope: the fixture and more."), nil, spec.DeltaReasonInterfaceChange},
		{"new requirement", replace("spec.md", "THE SYSTEM SHALL store input.\n",
			"THE SYSTEM SHALL store input.\n\n**REQ-004**\nPriority: Must\nTHE SYSTEM SHALL log input.\n"), nil, spec.DeltaReasonInterfaceChange},
		{"finding scope absent", nil, func(in *spec.DeltaVerifyInput) { in.Findings[0].ScopeRef = "REQ-999" }, spec.DeltaReasonUnclearImpact},
		{"unrelated requirement edited", replace("spec.md", "THE SYSTEM SHALL store input.", "THE SYSTEM SHALL store input twice."), nil, spec.DeltaReasonOutOfScope},
		{"sibling requirement naming the finding's requirement", replace("spec.md", "THE SYSTEM SHALL store input.\n",
			"THE SYSTEM SHALL store input.\nIt SHALL keep the REQ-002 validation result.\n"), nil, spec.DeltaReasonOutOfScope},
		{"sibling scenario edited", replace("acceptance.md", "Then it is stored.", "Then it is stored twice."), nil, spec.DeltaReasonOutOfScope},
		{"traceability row of the finding's requirement", replace("spec.md", "| REQ-002 | T2 | S2 |", "| REQ-002 | T2, T4 | S2 |"), nil, spec.DeltaReasonDelta},
		{"no change", func(current map[string]string) { maps.Copy(current, base) }, nil, spec.DeltaReasonNoChange},
	}
	for _, tc := range cases {
		current := deltaFix(base)
		if tc.edit != nil {
			tc.edit(current)
		}
		in := spec.DeltaVerifyInput{
			Baseline:  deltaBaseline(base),
			Documents: inputDocuments(current),
			Context:   map[string]string{"checklist": "c1", "code.worktree": "w1"},
			Findings:  []spec.ReviewFinding{deltaFinding()},
		}
		if tc.adjust != nil {
			tc.adjust(&in)
		}
		assert.Equal(t, tc.reason, spec.PlanDeltaVerify(in).Reason, tc.name)
	}
}

// deltaPromptFixture plans and builds the S14 delta prompt from raw round-0
// and round-1 documents.
func deltaPromptFixture(t *testing.T, base, current map[string]string) (string, []spec.DeltaSource, spec.DeltaManifestCheck) {
	t.Helper()
	in := spec.DeltaVerifyInput{Baseline: deltaBaseline(base), Documents: inputDocuments(current),
		Context: map[string]string{"checklist": "c1", "code.worktree": "w1"}, Findings: []spec.ReviewFinding{deltaFinding()}}
	plan := spec.PlanDeltaVerify(in)
	require.Equal(t, spec.DeltaReasonDelta, plan.Reason)
	sources := []spec.DeltaSource{deltaChecklist(), spec.IncludedDeltaSource("reference:ARCHITECTURE.md", "a1", "ARCHITECTURE-BODY")}
	prompt, manifest := spec.BuildDeltaVerifyPrompt(spec.DeltaPromptInput{
		Doc: &spec.SpecDocument{ID: "SPEC-DELTA-001", Title: "Delta fixture"}, Plan: plan, Baseline: in.Baseline,
		Documents: in.Documents, Sources: sources, PriorFindings: in.Findings,
	})
	check := spec.DeltaManifestCheck{Manifest: manifest, Required: spec.RequiredDeltaSources(in.Documents, sources),
		Baseline: in.Baseline, Plan: plan, Prompt: prompt}
	return prompt, manifest, check
}

func TestBuildDeltaVerifyPrompt_CarriesTheFixAndItsContext(t *testing.T) {
	t.Parallel()

	prompt, manifest, check := deltaPromptFixture(t, deltaDocuments(), deltaFix(deltaDocuments()))
	current := inputDocuments(deltaFix(deltaDocuments()))
	digests := func(name string) string {
		return name + " source_sha256:" + current[name].SourceSHA256 + " prompt_sha256:" + current[name].PromptSHA256
	}

	for _, want := range []string{
		"## SPEC: SPEC-DELTA-001 — Delta fixture",
		"### Delta Verify Input (spec_review_delta.v1)",
		"- " + digests("acceptance.md") + " changed\n",
		"- " + digests("spec.md") + " changed\n",
		"- " + digests("plan.md") + " linked (baseline run-1 r0",
		"- " + digests("research.md") + " linked (baseline run-1 r0",
		"- checklist source_sha256:c1 prompt_sha256:" + spec.SHA256Hex([]byte("CHECKLIST-BODY")) + " included\n",
		"\n-THE SYSTEM SHALL validate input.\n",
		"\n+THE SYSTEM SHALL validate input and refuse an empty value with error E1.\n",
		"**REQ-002**\nPriority: Must\nTHE SYSTEM SHALL validate input and refuse an empty value with error E1.\n",
		"### S2: AC-D-002 - Validate\nGiven bad input\nWhen it arrives\nThen it is refused with E1.\n",
		"| REQ-002 | T2 | S2 |",
		"## Tasks\n\n- T1\n- T2\n- T3\n",
		"## Outcome Boundary\n\n- In scope: the fixture.\n",
		"## Outcome Lock\n\nLock.\n",
		"ARCHITECTURE-BODY",
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
	assert.Len(t, manifest, 6)
	require.NoError(t, spec.ValidateDeltaManifest(check))
}

func TestValidateDeltaManifest_RefusesAnUnaccountedSource(t *testing.T) {
	t.Parallel()

	refuse := func(name string, edit func(check *spec.DeltaManifestCheck)) {
		_, _, check := deltaPromptFixture(t, deltaDocuments(), deltaFix(deltaDocuments()))
		edit(&check)
		assert.Error(t, spec.ValidateDeltaManifest(check), name)
	}
	without := func(manifest []spec.DeltaSource, name string) []spec.DeltaSource {
		var out []spec.DeltaSource
		for _, source := range manifest {
			if source.Name != name {
				out = append(out, source)
			}
		}
		return out
	}
	refuse("missing source", func(c *spec.DeltaManifestCheck) { c.Manifest = without(c.Manifest, "research.md") })
	refuse("linked source changed", func(c *spec.DeltaManifestCheck) {
		for i := range c.Required {
			if c.Required[i].Name == "research.md" {
				c.Required[i].SourceSHA256 = spec.SHA256Hex([]byte("changed research"))
			}
		}
		for i := range c.Manifest {
			if c.Manifest[i].Name == "research.md" {
				c.Manifest[i].SourceSHA256 = spec.SHA256Hex([]byte("changed research"))
			}
		}
	})
	refuse("changed source without its diff", func(c *spec.DeltaManifestCheck) {
		c.Prompt = strings.Replace(c.Prompt, "##### spec.md\n\n```diff", "##### spec.md\n\n```text", 1)
	})
	refuse("linked source the baseline did not deliver complete", func(c *spec.DeltaManifestCheck) {
		c.Baseline.Delivered["plan.md"] = spec.BaselineDeliveredPartial
	})
	refuse("linked document whose needed section is missing", func(c *spec.DeltaManifestCheck) {
		c.Prompt = strings.Replace(c.Prompt, "- T1\n- T2\n- T3\n", "", 1)
	})
	refuse("included source not in the prompt", func(c *spec.DeltaManifestCheck) {
		c.Prompt = strings.Replace(c.Prompt, "ARCHITECTURE-BODY", "", 1)
	})
	refuse("linked non-document source", func(c *spec.DeltaManifestCheck) {
		for i := range c.Manifest {
			if c.Manifest[i].Name == "reference:ARCHITECTURE.md" {
				c.Manifest[i].Delivery = spec.DeltaDeliveryLinked
			}
		}
	})
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
