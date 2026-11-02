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
	DeltaReasonContextChanged       = "context_changed"
	DeltaReasonInterfaceChange      = "interface_change"
	DeltaReasonUnclearImpact        = "unclear_impact"
	DeltaReasonOutOfScope           = "out_of_scope_change"
	DeltaReasonNoChange             = "no_change"
	DeltaReasonNoSaving             = "no_saving"
	DeltaReasonIntegrityGap         = "integrity_gap"
)

// DeltaVerifyInput is what the eligibility check compares: the baseline, the
// current SPEC documents and non-document digests, and the prior findings.
type DeltaVerifyInput struct {
	Baseline  *ReviewBaseline
	Documents map[string]string
	Context   map[string]string
	Findings  []ReviewFinding
}

// DocumentChange is one changed SPEC document and its hunks.
type DocumentChange struct {
	Name  string
	Hunks []DiffHunk
}

// DeltaPlan is the eligibility outcome. For a delta input it carries the
// changed documents and what the prompt builder needs to link the rest.
type DeltaPlan struct {
	Reason   string
	Changes  []DocumentChange
	scopes   []findingScope
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
		if SHA256Hex([]byte(document.Content)) != document.SHA256 {
			return fail(DeltaReasonBaselineHashMismatch)
		}
	}
	if !slices.Equal(slices.Sorted(slices.Values(baseline.OpenIDs)), OpenFindingIDs(in.Findings)) {
		return fail(DeltaReasonBaselineHashMismatch)
	}
	if !maps.Equal(baseline.Context, in.Context) {
		return fail(DeltaReasonContextChanged)
	}
	if !slices.Equal(slices.Sorted(maps.Keys(baseline.Documents)), slices.Sorted(maps.Keys(in.Documents))) {
		return fail(DeltaReasonInterfaceChange)
	}
	plan := DeltaPlan{current: map[string]docLayout{}, baseline: map[string]docLayout{}}
	for _, name := range slices.Sorted(maps.Keys(in.Documents)) {
		old, current := baseline.Documents[name].Content, in.Documents[name]
		plan.current[name], plan.baseline[name] = layoutDocument(current), layoutDocument(old)
		if old == current {
			continue
		}
		change := DocumentChange{Name: name, Hunks: buildHunks(diffLines(splitLines(old), splitLines(current)), 3)}
		for _, hunk := range change.Hunks {
			if plan.touchesRiskBoundary(name, hunk) {
				return fail(DeltaReasonInterfaceChange)
			}
		}
		plan.Changes = append(plan.Changes, change)
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
