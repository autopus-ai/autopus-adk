package cli

import "github.com/insajin/autopus-adk/pkg/spec"

// applySpecReviewChecklistPolicy rewrites, in place, the reviewer checklist
// FAILs the SPEC's effective review policy waives: on an eligible lean SPEC a
// FAIL whose only evidence is a section the profile omits, while that section
// is absent, is recorded N/A instead of blocking (SPEC-REVIEWPOLICY-001
// REQ-RP-POL-07). It runs before the verdict merge, so a waived item can no
// longer produce a reviewer_checklist_fail blocker.
func applySpecReviewChecklistPolicy(reviews []spec.ReviewResult, specDir string, doc *spec.SpecDocument) {
	for i := range reviews {
		reviews[i].ChecklistOutcomes = spec.ApplyReviewChecklistPolicy(specDir, doc, reviews[i].ChecklistOutcomes)
	}
}
