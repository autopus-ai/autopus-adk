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

func leanSpecFiles(class string) map[string]string {
	header := "profile: lean\n"
	if class != "" {
		header += "change_class: " + class + "\n"
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
