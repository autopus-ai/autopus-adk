package spec

import (
	"slices"
	"strings"

	"github.com/insajin/autopus-adk/pkg/spec/gates"
)

// Review policy profiles (SPEC-REVIEWPOLICY-001 REQ-RP-POL-01). Compact is the
// change.md contract recorded against an existing SPEC, lean the reduced
// four-document set of a new low-risk SPEC, and full the complete set.
const (
	ReviewProfileCompact = "compact"
	ReviewProfileLean    = ProfileLean
	ReviewProfileFull    = "full"
)

// ChecklistWaiver names a quality checklist item whose only evidence is a
// section the profile lets an author omit. It waives a FAIL only while every
// named section is absent, so a section the author did write is still checked.
type ChecklistWaiver struct {
	ID       string
	Document string
	Sections []string
}

// ReviewExecution is the review shape a profile asks for. Reviewers 0 means
// every configured reviewer and Quorum 0 the majority default; Review false
// means the profile has no SPEC review of its own.
type ReviewExecution struct {
	Review    bool
	Reviewers int
	Judge     bool
	Quorum    int
}

// ReviewPolicy is the single definition, per authoring profile, of what a SPEC
// set must contain, what it may omit, which checklist items an omission makes
// not applicable, which gates no profile can waive, and how it is reviewed.
// The validator, the reviewer prompt, the runtime checklist handling, and the
// CLI review plan all read it; nothing else restates these tables.
type ReviewPolicy struct {
	Profile          string
	Documents        []string
	RequiredSections map[string][]string
	OmittedSections  map[string][]string
	ChecklistWaivers []ChecklistWaiver
	MandatoryGates   []gates.GateID
	RiskFirstProbe   bool
	SelfVerify       bool
	Execution        ReviewExecution
	// EscalationReason says why a lean declaration fell back to this policy;
	// empty unless the SPEC declared a profile it does not qualify for.
	EscalationReason string
}

var specSetDocuments = []string{"spec.md", "plan.md", "acceptance.md", "research.md"}

var fullRequiredSections = map[string][]string{
	"spec.md":       {"## Outcome Boundary", "## Requirements", "## Traceability Matrix"},
	"plan.md":       {"## Implementation Strategy", "## Visual Planning Brief", "## Feature Completion Scope", probeSectionHeading, "## Tasks"},
	"acceptance.md": {"## Test Scenarios", "## Oracle Acceptance Notes"},
	"research.md": {
		"## Outcome Lock",
		"## Visual Planning Brief",
		"## Semantic Invariant Inventory",
		"## Feature Coverage Map",
		"## Completion Debt",
		"## Evolution Ideas",
		"## Reference Discipline",
		"## Reviewer Brief",
		"## Self-Verify Summary",
	},
}

// leanRequiredSections is the section floor of a lean SPEC set.
var leanRequiredSections = map[string][]string{
	"spec.md":       {"## Outcome Boundary", "## Requirements", "## Traceability Matrix"},
	"plan.md":       {"## Tasks"},
	"acceptance.md": {"## Test Scenarios"},
	"research.md":   {"## Outcome Lock", "## Reviewer Brief"},
}

// leanOmittedSections are the procedure sections a lean SPEC may leave out:
// every full-profile section lean does not require, plus the Minimality
// Decision Matrix the full writer guidance asks for. A review must not report
// their absence.
var leanOmittedSections = map[string][]string{
	"plan.md":       {"## Implementation Strategy", "## Visual Planning Brief", "## Feature Completion Scope", probeSectionHeading},
	"acceptance.md": {"## Oracle Acceptance Notes"},
	"research.md": {
		"## Visual Planning Brief",
		"## Semantic Invariant Inventory",
		"## Minimality Decision Matrix",
		"## Feature Coverage Map",
		"## Completion Debt",
		"## Evolution Ideas",
		"## Reference Discipline",
		"## Self-Verify Summary",
	},
}

var leanChecklistWaivers = []ChecklistWaiver{
	{ID: "Q-COMP-08", Document: "plan.md", Sections: []string{probeSectionHeading}},
	{ID: "Q-COMP-07", Document: "research.md", Sections: []string{"## Completion Debt", "## Evolution Ideas"}},
	{ID: "Q-CORR-04", Document: "research.md", Sections: []string{"## Reference Discipline"}},
}

// compactRequiredSections are the sections of change.md (see
// internal/cli/spec_change_doc.go); requirements stay in the referenced SPEC.
var compactRequiredSections = map[string][]string{
	"change.md": {"## Referenced SPEC", "## Intended Surface", "## Verification Plan", "## Gate Applicability"},
}

// mandatoryReviewGates are never not_applicable on any profile.
var mandatoryReviewGates = []gates.GateID{gates.GateSecurity, gates.GateValidation, gates.GateDataLoss, gates.GateDeterministicOracle}

// ReviewPolicyFor returns a copy of the named profile's policy.
func ReviewPolicyFor(profile string) (ReviewPolicy, bool) {
	switch profile {
	case ReviewProfileFull:
		return ReviewPolicy{
			Profile: ReviewProfileFull, Documents: slices.Clone(specSetDocuments),
			RequiredSections: cloneSections(fullRequiredSections), MandatoryGates: slices.Clone(mandatoryReviewGates),
			RiskFirstProbe: true, SelfVerify: true,
			Execution: ReviewExecution{Review: true, Judge: true},
		}, true
	case ReviewProfileLean:
		return ReviewPolicy{
			Profile: ReviewProfileLean, Documents: slices.Clone(specSetDocuments),
			RequiredSections: cloneSections(leanRequiredSections), OmittedSections: cloneSections(leanOmittedSections),
			ChecklistWaivers: cloneWaivers(leanChecklistWaivers), MandatoryGates: slices.Clone(mandatoryReviewGates),
			Execution: ReviewExecution{Review: true, Reviewers: 1, Quorum: 1},
		}, true
	case ReviewProfileCompact:
		return ReviewPolicy{
			Profile: ReviewProfileCompact, Documents: []string{"change.md"},
			RequiredSections: cloneSections(compactRequiredSections), MandatoryGates: slices.Clone(mandatoryReviewGates),
		}, true
	}
	return ReviewPolicy{}, false
}

// EffectiveReviewPolicy returns the policy a SPEC is held to. Only a lean
// declaration the profile rules accept gets the lean policy; any other SPEC,
// including a high-risk or otherwise refused lean declaration, gets the full
// one, so a self-declared profile can never skip a full gate.
func EffectiveReviewPolicy(doc *SpecDocument) ReviewPolicy {
	if doc.LeanEligible() {
		policy, _ := ReviewPolicyFor(ReviewProfileLean)
		return policy
	}
	policy, _ := ReviewPolicyFor(ReviewProfileFull)
	if doc != nil && doc.Profile != "" {
		var reasons []string
		for _, err := range validateProfile(doc) {
			reasons = append(reasons, err.Message)
		}
		policy.EscalationReason = strings.Join(reasons, "; ")
	}
	return policy
}

// Lean reports whether the policy is the lean profile.
func (p ReviewPolicy) Lean() bool { return p.Profile == ReviewProfileLean }

// RequiredHeadings lists the required section headings in document order.
func (p ReviewPolicy) RequiredHeadings() []string { return p.headings(p.RequiredSections) }

// OmittedHeadings lists the allowed omissions in document order, each heading
// once even when two documents may omit it.
func (p ReviewPolicy) OmittedHeadings() []string { return p.headings(p.OmittedSections) }

func (p ReviewPolicy) headings(sections map[string][]string) []string {
	var out []string
	for _, name := range p.Documents {
		for _, heading := range sections[name] {
			if !slices.Contains(out, heading) {
				out = append(out, heading)
			}
		}
	}
	return out
}

func cloneSections(in map[string][]string) map[string][]string {
	out := make(map[string][]string, len(in))
	for name, sections := range in {
		out[name] = slices.Clone(sections)
	}
	return out
}

func cloneWaivers(in []ChecklistWaiver) []ChecklistWaiver {
	out := make([]ChecklistWaiver, len(in))
	for i, waiver := range in {
		out[i] = waiver
		out[i].Sections = slices.Clone(waiver.Sections)
	}
	return out
}
