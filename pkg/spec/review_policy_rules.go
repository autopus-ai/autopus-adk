package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// reviewVerdictRuleLines is the one statement of the finding disposition
// rules (SPEC-REVIEWPOLICY-001 REQ-RP-POL-03). The default reviewer prompt
// writes these lines verbatim, templates/shared/spec-review-judge.md.tmpl
// carries the same lines (a parity test fails when they drift), and
// FindingBlockingPolicy is their runtime form. Change all three together.
var reviewVerdictRuleLines = []string{
	"- Blocking at any severity: an open or regressed finding in category correctness, contract, data_loss, or security, and any finding raised through the critical/security escape hatch.",
	"- Blocking: any other critical or major finding, and a minor finding outside the style, suggestion, and improvement categories.",
	"- Advisory, never blocking by itself: severity suggestion outside correctness, contract, data_loss, and security, and category style, suggestion, or improvement at minor or suggestion severity.",
	"- A failing checklist item blocks: vote REVISE and name it.",
	"- PASS only when nothing blocks. REVISE when something blocks and the design can be fixed in place. REJECT only for a stated fundamental design problem that requires redesign.",
	"- A requirement conflict, an undecided choice that changes what gets built, an acceptance criterion that cannot be verified, and a security or data-loss risk always block: report them as major or above, or under correctness, contract, data_loss, or security.",
	"- Wording polish, a nicer structure, and choices the SPEC delegates to the implementer are advice: report them with severity suggestion.",
}

// ReviewVerdictRuleLines returns a copy of the canonical verdict rule lines.
func ReviewVerdictRuleLines() []string {
	return append([]string(nil), reviewVerdictRuleLines...)
}

// ApplyChecklistWaivers rewrites a FAIL on a waived checklist item to N/A while
// every section the waiver names is absent from files (REQ-RP-POL-07). A
// document that could not be read never confirms an absence, so a waiver needs
// positive evidence; every other outcome is returned unchanged.
func (p ReviewPolicy) ApplyChecklistWaivers(outcomes []ChecklistOutcome, files map[string]string) []ChecklistOutcome {
	if len(p.ChecklistWaivers) == 0 || len(outcomes) == 0 {
		return outcomes
	}
	updated := append([]ChecklistOutcome(nil), outcomes...)
	for i, outcome := range updated {
		if outcome.Status != ChecklistStatusFail {
			continue
		}
		for _, waiver := range p.ChecklistWaivers {
			if !strings.EqualFold(strings.TrimSpace(outcome.ID), waiver.ID) || !waiver.omittedIn(files) {
				continue
			}
			updated[i].Status = ChecklistStatusNA
			updated[i].Reason = fmt.Sprintf("profile %s omits %s in %s; the reviewer reported: %s",
				p.Profile, strings.Join(waiver.Sections, " and "), waiver.Document, strings.TrimSpace(outcome.Reason))
			break
		}
	}
	return updated
}

func (w ChecklistWaiver) omittedIn(files map[string]string) bool {
	content, read := files[w.Document]
	if !read {
		return false
	}
	for _, section := range w.Sections {
		if hasSection(content, section) {
			return false
		}
	}
	return true
}

// ApplyReviewChecklistPolicy applies doc's effective policy to reviewer
// checklist outcomes, reading the SPEC documents from specDir.
func ApplyReviewChecklistPolicy(specDir string, doc *SpecDocument, outcomes []ChecklistOutcome) []ChecklistOutcome {
	policy := EffectiveReviewPolicy(doc)
	if len(policy.ChecklistWaivers) == 0 || len(outcomes) == 0 {
		return outcomes
	}
	files := make(map[string]string, len(policy.Documents))
	for _, name := range policy.Documents {
		if data, err := os.ReadFile(filepath.Join(specDir, name)); err == nil {
			files[name] = string(data)
		}
	}
	return policy.ApplyChecklistWaivers(outcomes, files)
}

// writeReviewPolicySection states what the review is for and, for an
// eligible lean SPEC, which sections its profile lets the author omit
// (REQ-RP-POL-06).
func writeReviewPolicySection(sb *strings.Builder, policy ReviewPolicy) {
	fmt.Fprintf(sb, "### Review Policy (profile %s)\n\n", policy.Profile)
	sb.WriteString("Answer one question: can implementation start from this SPEC without guessing? Block on what would make an implementer build the wrong thing, ship an unsafe change, or be unable to verify acceptance; everything else is advice.\n")
	sb.WriteString("Judge whether each requirement and its evidence are present, not whether a heading with a particular name exists. A missing heading is a finding only when the content it would carry is required by this profile and absent everywhere.\n")
	if policy.EscalationReason != "" {
		fmt.Fprintf(sb, "This SPEC declares a profile it does not qualify for (%s); review it against the full profile.\n", policy.EscalationReason)
	}
	if policy.Lean() {
		fmt.Fprintf(sb, "\nAllowed omissions (profile lean): %s. Their absence is not a finding.\n", strings.Join(policy.OmittedHeadings(), ", "))
		var items []string
		for _, waiver := range policy.ChecklistWaivers {
			items = append(items, fmt.Sprintf("%s (%s)", waiver.ID, strings.Join(waiver.Sections, ", ")))
		}
		fmt.Fprintf(sb, "Answer these checklist items N/A while their section is absent: %s. Judge Q-COMP-05 on concrete acceptance only; an absent Semantic Invariant Inventory is not a FAIL.\n", strings.Join(items, "; "))
	}
	sb.WriteString("\n")
}
