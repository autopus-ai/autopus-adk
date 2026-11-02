package cli

import (
	"regexp"
	"strings"

	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/spec"
)

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

// specChecklistItemRe matches a quality checklist item id such as Q-COMP-08.
var specChecklistItemRe = regexp.MustCompile(`(?i)^Q-[A-Z]+-[0-9]+$`)

// specReviewJudgeDismissals collects the judge's `reject` entries that name a
// reviewer's verdict-scoped blocker: scope_ref "verdict" for its REJECT vote,
// or a checklist item id for its FAIL on that item. Sources are restored to
// provider names; any other rejected entry is an ordinary rejected finding
// (SPEC-REVIEWPOLICY-001 REQ-RP-POL-04).
func specReviewJudgeDismissals(out *orchestra.ReviewJudgeOutput, aliasToProvider map[string]string) []spec.JudgeDismissal {
	if out == nil {
		return nil
	}
	var dismissals []spec.JudgeDismissal
	for _, finding := range out.Findings {
		scope := strings.TrimSpace(finding.ScopeRef)
		if finding.Decision != "reject" || (!strings.EqualFold(scope, spec.JudgeDismissalVerdict) && !specChecklistItemRe.MatchString(scope)) {
			continue
		}
		for _, source := range finding.Sources {
			if provider := aliasToProvider[strings.TrimSpace(source)]; provider != "" {
				dismissals = append(dismissals, spec.JudgeDismissal{Reviewer: provider, ScopeRef: scope, Reason: strings.TrimSpace(finding.Reason)})
			}
		}
	}
	return dismissals
}
