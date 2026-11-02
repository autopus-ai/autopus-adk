package spec

import (
	"maps"
	"slices"
)

// Delta verify input reasons (SPEC-REVIEWREUSE-001 REQ-RR-DELTA-03,
// REQ-RR-DELTA-05). DeltaReasonDelta means the round uses the delta input;
// every other reason selects the full verify input.
const (
	DeltaReasonDelta                = "delta"
	DeltaReasonMissingBaseline      = "missing_baseline"
	DeltaReasonBaselineHashMismatch = "baseline_hash_mismatch"
	DeltaReasonIncompleteBaseline   = "incomplete_baseline"
	DeltaReasonContextChanged       = "context_changed"
	DeltaReasonInterfaceChange      = "interface_change"
	DeltaReasonUnclearImpact        = "unclear_impact"
	DeltaReasonOutOfScope           = "out_of_scope_change"
	DeltaReasonNoChange             = "no_change"
	DeltaReasonNoSaving             = "no_saving"
	DeltaReasonIntegrityGap         = "integrity_gap"
)

// DeltaVerifyInput is what the eligibility check compares: the baseline, the
// carried SPEC documents in their delivered representation, the digests of
// everything else the round reads, and the prior findings.
type DeltaVerifyInput struct {
	Baseline  *ReviewBaseline
	Documents map[string]ReviewInputDocument
	Context   map[string]string
	Findings  []ReviewFinding
}

// DocumentChange is one changed SPEC document and its hunks.
type DocumentChange struct {
	Name  string
	Hunks []DiffHunk
}

// DeltaPlan is the eligibility outcome. For a delta input it carries the
// changed documents, the delta scope ids, and the layouts the prompt builder
// and the manifest check read to decide what the request must carry.
type DeltaPlan struct {
	Reason   string
	Changes  []DocumentChange
	scopes   []findingScope
	ids      map[string]bool
	current  map[string]docLayout
	baseline map[string]docLayout
}

// PlanDeltaVerify checks, in the order of REQ-RR-DELTA-03, whether a verify
// round may use the delta input, and returns the first failed condition as
// the reason otherwise. Every check fails closed: anything the plan cannot
// account for selects the full input.
func PlanDeltaVerify(in DeltaVerifyInput) DeltaPlan {
	fail := func(reason string) DeltaPlan { return DeltaPlan{Reason: reason} }
	baseline := in.Baseline
	if baseline == nil || baseline.Schema != ReviewBaselineSchema {
		return fail(DeltaReasonMissingBaseline)
	}
	for _, document := range baseline.Documents {
		if SHA256Hex([]byte(document.Content)) != document.PromptSHA256 {
			return fail(DeltaReasonBaselineHashMismatch)
		}
	}
	if !slices.Equal(slices.Sorted(slices.Values(baseline.OpenIDs)), OpenFindingIDs(in.Findings)) {
		return fail(DeltaReasonBaselineHashMismatch)
	}
	// A document the baseline round did not deliver whole was never reviewed
	// whole, so nothing about it can stand in for sending it.
	for name := range baseline.Documents {
		if baseline.Delivered[name] != BaselineDeliveredComplete {
			return fail(DeltaReasonIncompleteBaseline)
		}
	}
	if !maps.Equal(baseline.Context, in.Context) {
		return fail(DeltaReasonContextChanged)
	}
	if !slices.Equal(slices.Sorted(maps.Keys(baseline.Documents)), slices.Sorted(maps.Keys(in.Documents))) {
		return fail(DeltaReasonInterfaceChange)
	}
	plan := DeltaPlan{current: map[string]docLayout{}, baseline: map[string]docLayout{}}
	for _, name := range slices.Sorted(maps.Keys(in.Documents)) {
		old, current := baseline.Documents[name], in.Documents[name]
		plan.current[name], plan.baseline[name] = layoutDocument(current.Content), layoutDocument(old.Content)
		sameSource, sameContent := old.SourceSHA256 == current.SourceSHA256, old.Content == current.Content
		if sameSource && sameContent {
			continue
		}
		// A source change the redaction hides, or a representation change
		// without one, cannot be shown as a diff of what reviewers read.
		if sameSource || sameContent {
			return fail(DeltaReasonIntegrityGap)
		}
		hunks := buildHunks(diffLines(splitLines(old.Content), splitLines(current.Content)), 3)
		plan.Changes = append(plan.Changes, DocumentChange{Name: name, Hunks: hunks})
	}
	for _, change := range plan.Changes {
		for _, hunk := range change.Hunks {
			if plan.touchesRiskBoundary(change.Name, hunk) {
				return fail(DeltaReasonInterfaceChange)
			}
		}
	}
	for _, finding := range in.Findings {
		if !isOpenOrRegressed(finding.Status) {
			continue
		}
		scope := newFindingScope(finding, plan.current["spec.md"])
		if !scope.resolvable(plan.current) {
			return fail(DeltaReasonUnclearImpact)
		}
		plan.scopes = append(plan.scopes, scope)
	}
	for _, change := range plan.Changes {
		for _, hunk := range change.Hunks {
			if !plan.hunkCovered(change.Name, hunk, plan.scopes) {
				return fail(DeltaReasonOutOfScope)
			}
		}
	}
	if len(plan.Changes) == 0 {
		return fail(DeltaReasonNoChange)
	}
	plan.ids = plan.deltaScopeIDs()
	plan.Reason = DeltaReasonDelta
	return plan
}

func (p DeltaPlan) touchesRiskBoundary(name string, hunk DiffHunk) bool {
	for _, i := range hunk.Removed {
		if p.baseline[name].riskBoundaryLine(name, i) {
			return true
		}
	}
	for _, i := range hunk.Added {
		if p.current[name].riskBoundaryLine(name, i) {
			return true
		}
	}
	return false
}

// hunkCovered reports a hunk whose every changed line lies in the scope of
// one of scopes.
func (p DeltaPlan) hunkCovered(name string, hunk DiffHunk, scopes []findingScope) bool {
	covered := func(layout docLayout, i int) bool {
		for _, scope := range scopes {
			if scope.covers(name, layout, i) {
				return true
			}
		}
		return false
	}
	for _, i := range hunk.Removed {
		if !covered(p.baseline[name], i) {
			return false
		}
	}
	for _, i := range hunk.Added {
		if !covered(p.current[name], i) {
			return false
		}
	}
	return true
}
