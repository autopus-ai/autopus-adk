package spec_test

// SPEC-REVIEWPOLICY-001 REQ-RP-POL-07 (codex review F-005): which findings an
// eligible lean SPEC may treat as omission-only. The rule is narrow on
// purpose; every counter-case keeps blocking.

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/spec"
)

func TestOmissionOnlyFinding_IsNarrow(t *testing.T) {
	t.Parallel()

	lean, _ := spec.ReviewPolicyFor(spec.ReviewProfileLean)
	full, _ := spec.ReviewPolicyFor(spec.ReviewProfileFull)
	omitted := map[string]string{"plan.md": "# Plan\n\n## Tasks\n", "research.md": "# Research\n\n## Outcome Lock\n"}
	withProbe := map[string]string{"plan.md": "# Plan\n\n## Tasks\n\n## Risk-First Integration Probe\n", "research.md": "# Research\n"}
	finding := func(severity, category, scope, description string) spec.ReviewFinding {
		return spec.ReviewFinding{Severity: severity, Category: spec.FindingCategory(category), ScopeRef: scope,
			Description: description, Status: spec.FindingStatusOpen}
	}

	cases := []struct {
		name   string
		policy spec.ReviewPolicy
		files  map[string]string
		f      spec.ReviewFinding
		want   bool
	}{
		{"missing probe", lean, omitted, finding("major", "completeness", "plan.md", "Risk-First Integration Probe missing"), true},
		{"Korean marker", lean, omitted, finding("major", "completeness", "research.md", "Self-Verify Summary 섹션이 누락되었다"), true},
		{"scope names the section", lean, omitted, finding("minor", "style", "plan.md#Visual Planning Brief", "absent"), true},
		{"full policy", full, omitted, finding("major", "completeness", "plan.md", "Risk-First Integration Probe missing"), false},
		{"section present", lean, withProbe, finding("major", "completeness", "plan.md", "Risk-First Integration Probe missing"), false},
		{"document unread", lean, map[string]string{}, finding("major", "completeness", "plan.md", "Risk-First Integration Probe missing"), false},
		{"impact category", lean, omitted, finding("minor", "data_loss", "plan.md", "Risk-First Integration Probe missing"), false},
		{"cites a requirement", lean, omitted, finding("major", "completeness", "plan.md", "Risk-First Integration Probe missing for REQ-002"), false},
		{"cites an acceptance scenario", lean, omitted, finding("major", "completeness", "acceptance.md", "Oracle Acceptance Notes missing so S1 is unverifiable"), false},
		{"names a floor section", lean, omitted, finding("major", "completeness", "plan.md", "Tasks miss a step and the Risk-First Integration Probe is missing"), false},
		{"no missing marker", lean, omitted, finding("major", "completeness", "plan.md", "Risk-First Integration Probe rows are wrong"), false},
		{"unrelated gap", lean, omitted, finding("major", "completeness", "spec.md", "the rollback behaviour is undecided"), false},
	}
	for _, tc := range cases {
		escape := tc.f
		assert.Equal(t, tc.want, tc.policy.OmissionOnlyFinding(tc.f, tc.files), tc.name)
		escape.EscapeHatch = true
		assert.False(t, tc.policy.OmissionOnlyFinding(escape, tc.files), tc.name+" (escape hatch)")
	}
}
