package spec_test

// Fixture of the SPEC-REVIEWREUSE-001 delta verify tests: a small SPEC whose
// round 0 opened F-001 on REQ-002, and the author's local fix. REQ-001 to
// REQ-003 share one H2 heading, so a change is attributed to its requirement
// block, never to the heading section around it (REQ-RR-DELTA-03).

import (
	"maps"
	"strings"

	"github.com/insajin/autopus-adk/pkg/spec"
)

const deltaSpec = `# SPEC-DELTA-001: Delta fixture

**Status**: draft

## Purpose

Delta fixture purpose.

## Outcome Boundary

- In scope: the fixture.

## Requirements

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

const deltaAcceptance = `# SPEC-DELTA-001 Acceptance Criteria

## Test Scenarios

### S1: AC-D-001 - Accept
Given input
When it arrives
Then it is accepted.

### S2: AC-D-002 - Validate
Given bad input
When it arrives
Then it is refused.

### S3: AC-D-003 - Store
Given input
When it is accepted
Then it is stored.
`

// deltaDocuments are the raw SPEC documents of round 0. plan.md names T2,
// which shares the REQ-002 traceability row, in its Tasks section; its Notes
// section and research.md's name no id of the fix.
func deltaDocuments() map[string]string {
	return map[string]string{
		"spec.md":       deltaSpec,
		"acceptance.md": deltaAcceptance,
		"plan.md":       "# Plan\n\n## Tasks\n\n- T1\n- T2\n- T3\n\n## Notes\n\nPLAN-BODY-MARKER\n",
		"research.md":   "# Research\n\n## Outcome Lock\n\nLock.\n\n## Notes\n\nRESEARCH-BODY-MARKER\n",
	}
}

// inputDocuments is raw in the representation a review request delivers.
func inputDocuments(raw map[string]string) map[string]spec.ReviewInputDocument {
	documents := make(map[string]spec.ReviewInputDocument, len(raw))
	for name, content := range raw {
		documents[name] = spec.NewReviewInputDocument([]byte(content))
	}
	return documents
}

// deltaFix is the author's local fix of F-001: the REQ-002 statement and the
// S2 outcome.
func deltaFix(documents map[string]string) map[string]string {
	fixed := maps.Clone(documents)
	fixed["spec.md"] = strings.Replace(fixed["spec.md"], "THE SYSTEM SHALL validate input.",
		"THE SYSTEM SHALL validate input and refuse an empty value with error E1.", 1)
	fixed["acceptance.md"] = strings.Replace(fixed["acceptance.md"], "Then it is refused.", "Then it is refused with E1.", 1)
	return fixed
}

// deltaBaseline is what round 0 recorded: every document delivered complete.
func deltaBaseline(raw map[string]string) *spec.ReviewBaseline {
	documents := inputDocuments(raw)
	delivered := map[string]string{}
	for name := range documents {
		delivered[name] = spec.BaselineDeliveredComplete
	}
	return &spec.ReviewBaseline{
		Schema: spec.ReviewBaselineSchema, RunID: "run-1", Revision: 0, RecordedAt: "2026-11-02T09:00:00Z",
		Documents: spec.BaselineDocuments(documents), Delivered: delivered,
		Context: map[string]string{"checklist": "c1", "code.worktree": "w1"},
		OpenIDs: []string{"F-001"},
	}
}

// deltaChecklist is the checklist source of a delta round, sent in full.
func deltaChecklist() spec.DeltaSource {
	return spec.IncludedDeltaSource("checklist", "c1", "CHECKLIST-BODY")
}

func deltaFinding() spec.ReviewFinding {
	return spec.ReviewFinding{ID: "F-001", Severity: "major", Category: spec.FindingCategoryCompleteness,
		ScopeRef: "REQ-002", Description: "error path undefined", Status: spec.FindingStatusOpen}
}
