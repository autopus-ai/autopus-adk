package spec_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/spec"
)

func TestValidateSpecSet_LeanProfileAcceptsReducedSectionSet(t *testing.T) {
	t.Parallel()

	specDir, doc := writeAuthoringPreflightSpec(t, leanSpecFiles("bugfix_existing_contract"))

	assert.True(t, doc.IsLean())
	assert.Equal(t, "bugfix_existing_contract", doc.ChangeClass)
	for _, e := range spec.ValidateSpecSet(specDir, doc) {
		assert.NotEqual(t, "error", e.Level, "unexpected error: %s", e.Message)
	}
}

func TestValidateSpecSet_LeanProfileStillRequiresItsFloor(t *testing.T) {
	t.Parallel()

	files := leanSpecFiles("test_only")
	files["research.md"] = strings.ReplaceAll(files["research.md"], "## Reviewer Brief", "## Reviewer Notes")
	files["plan.md"] = strings.ReplaceAll(files["plan.md"], "## Tasks", "## Steps")
	specDir, doc := writeAuthoringPreflightSpec(t, files)

	errs := spec.ValidateSpecSet(specDir, doc)
	assertValidationError(t, errs, "research.md", "Reviewer Brief")
	assertValidationError(t, errs, "plan.md", "Tasks")
}

func TestValidateSpecSet_LeanProfileRequiresTraceRowsAndConcreteAcceptance(t *testing.T) {
	t.Parallel()

	files := leanSpecFiles("docs_only")
	files["spec.md"] = strings.ReplaceAll(files["spec.md"], "| REQ-001 | T1 | S1 |\n", "")
	files["acceptance.md"] = `# SPEC-LEAN-001 Acceptance

## Test Scenarios

### S1: Section exists
Priority: Must
Given the document
When it is rendered
Then the file exists
`
	specDir, doc := writeAuthoringPreflightSpec(t, files)

	errs := spec.ValidateSpecSet(specDir, doc)
	assertValidationError(t, errs, "spec.md", "Traceability Matrix")
	assertValidationError(t, errs, "acceptance.md", "structural-only")
}

func TestValidateSpecSet_LeanProfileRefusedForHighRiskOrMissingClass(t *testing.T) {
	t.Parallel()

	for _, class := range []string{"feature", "multi_domain", "security_or_data", "", "tiny"} {
		specDir, doc := writeAuthoringPreflightSpec(t, leanSpecFiles(class))
		errs := spec.ValidateSpecSet(specDir, doc)
		assertValidationError(t, errs, "spec.md", "lean profile")
	}
}

func TestValidateSpecSet_UnknownProfileIsRefused(t *testing.T) {
	t.Parallel()

	specDir, doc := writeAuthoringPreflightSpec(t, map[string]string{
		"spec.md": strings.Replace(validSpecMD(), "status: draft", "status: draft\nprofile: quick", 1),
	})

	assertValidationError(t, spec.ValidateSpecSet(specDir, doc), "spec.md", "profile")
}

func TestValidateSpecSet_FullProfileStillRequiresFullSet(t *testing.T) {
	t.Parallel()

	specDir, doc := writeAuthoringPreflightSpec(t, map[string]string{
		"plan.md": leanSpecFiles("test_only")["plan.md"],
	})

	assert.False(t, doc.IsLean())
	assertValidationError(t, spec.ValidateSpecSet(specDir, doc), "plan.md", "Risk-First Integration Probe")
}

func TestValidateSpecSet_LeanProfileRequiresDeclaredSurface(t *testing.T) {
	t.Parallel()

	specDir, doc := writeAuthoringPreflightSpec(t, leanSpecFilesWithSurface("bugfix_existing_contract", ""))
	assertValidationError(t, spec.ValidateSpecSet(specDir, doc), "spec.md", "surface")
}

// A self-declared low-risk class is checked against the declared paths with
// the classifier auto spec gates applies to the real change set.
func TestValidateSpecSet_LeanSurfaceContradictingTheClassIsRefused(t *testing.T) {
	t.Parallel()

	cases := map[string]struct{ class, surface, reason string }{
		"security path":       {"bugfix_existing_contract", "internal/auth/session.go", "security_or_data_surface_in_change_set"},
		"two module roots":    {"bugfix_existing_contract", "internal/cli/a.go, pkg/spec/b.go", "multi_domain_change_set"},
		"docs_only with code": {"docs_only", "README.md, internal/cli/spec_timing.go", "declared_docs_only_touches_code"},
		"test_only with code": {"test_only", "internal/cli/spec_timing.go", "declared_test_only_touches_production_source"},
	}
	for name, tc := range cases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			specDir, doc := writeAuthoringPreflightSpec(t, leanSpecFilesWithSurface(tc.class, tc.surface))
			assertValidationError(t, spec.ValidateSpecSet(specDir, doc), "spec.md", tc.reason)
			assert.False(t, doc.LeanEligible())
		})
	}
}

func TestParseSpecMetadata_SurfaceAcceptsFlowAndPlainLists(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"[a/b.go, 'c/d.go']", "a/b.go,c/d.go,", "`a/b.go`, \"c/d.go\""} {
		doc := spec.ParseSpecMetadata("---\nid: SPEC-X-001\nsurface: " + value + "\n---\n")
		assert.Equal(t, []string{"a/b.go", "c/d.go"}, doc.Surface, value)
	}
}

func leanSpecFiles(class string) map[string]string {
	return leanSpecFilesWithSurface(class, leanSurfaceFor(class))
}

// leanSurfaceFor returns a surface consistent with class, so a fixture fails
// only for the reason its test names.
func leanSurfaceFor(class string) string {
	switch class {
	case "test_only":
		return "internal/cli/spec_review_timing_test.go"
	case "docs_only":
		return "docs/guide/timing.md"
	default:
		return "internal/cli/spec_timing.go, internal/cli/spec_review_timing_test.go"
	}
}

func leanSpecFilesWithSurface(class, surface string) map[string]string {
	header := "profile: lean\n"
	if class != "" {
		header += "change_class: " + class + "\n"
	}
	if surface != "" {
		header += "surface: " + surface + "\n"
	}
	return map[string]string{
		"spec.md": `# SPEC-LEAN-001: Lean Fix

---
id: SPEC-LEAN-001
title: Lean Fix
version: 0.1.0
status: draft
` + header + `---

## Outcome Boundary

- Outcome Lock: the status line shows the elapsed review time.
- Explicit non-goals: changing the review verdict.

## Requirements

WHEN a review round finishes THEN the system SHALL print its elapsed time.

## Traceability Matrix

| Requirement | Plan Task | Acceptance Scenario |
|-------------|-----------|---------------------|
| REQ-001 | T1 | S1 |
`,
		"plan.md": `# SPEC-LEAN-001 Plan

## Tasks

- [ ] T1: Print the elapsed time of each review round.
`,
		"acceptance.md": `# SPEC-LEAN-001 Acceptance

## Test Scenarios

### S1: Round time is printed
Priority: Must
Given a review round that ran for 90 seconds
When the round finishes
Then expected stderr contains "1m30s"
`,
		"research.md": `# SPEC-LEAN-001 Research

## Outcome Lock

- User-visible outcome: each review round reports its elapsed time.

## Reviewer Brief

- Intended scope: one stderr line per round.
- Reviewer should focus on: correctness only.
`,
	}
}

// A prose mention of a heading must neither stand in for the section nor
// shadow the real section that follows it.
func TestValidateSpecSet_InlineHeadingMentionIsNotTheSection(t *testing.T) {
	t.Parallel()

	shadowed := strings.Replace(validPlanMD(), "## Implementation Strategy\n",
		"## Implementation Strategy\n\nT1 fills the `## Risk-First Integration Probe` rows A1 and A2.\n", 1)
	specDir, doc := writeAuthoringPreflightSpec(t, map[string]string{"plan.md": shadowed})
	for _, e := range spec.ValidateSpecSet(specDir, doc) {
		assert.NotEqual(t, "error", e.Level, "unexpected error: %s", e.Message)
	}

	mentionOnly := strings.Replace(validResearchMD(), "## Reviewer Brief", "Reviewer notes (see `## Reviewer Brief`)", 1)
	specDir, doc = writeAuthoringPreflightSpec(t, map[string]string{"research.md": mentionOnly})
	assertValidationError(t, spec.ValidateSpecSet(specDir, doc), "research.md", "Reviewer Brief")
}
