package learn

import "sort"

// addEvidenceSummary fills the SPEC-LEARN-003 parts of a summary: counts by
// effective scope and evidence kind, the process lessons an execution
// contrast backs (candidates for ADK templates, never promoted
// automatically), and how many entries harness intake could turn into golden
// tasks.
func addEvidenceSummary(summary *Summary, entries []LearningEntry, topN int) {
	summary.ScopeCounts = map[string]int{}
	summary.EvidenceCounts = map[EvidenceKind]int{}
	var candidates []LearningEntry
	for _, e := range entries {
		summary.ScopeCounts[e.EffectiveScope()]++
		if kind := e.EvidenceKindOf(); kind != "" {
			summary.EvidenceCounts[kind]++
		}
		if e.EffectiveScope() == ScopeProcess && e.EvidenceKindOf() == EvidenceDeterministic && e.Pattern != "" {
			candidates = append(candidates, e)
		}
		if e.Expected != "" && e.Actual != "" {
			summary.IntakeEligible++
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].ReuseCount != candidates[j].ReuseCount {
			return candidates[i].ReuseCount > candidates[j].ReuseCount
		}
		return candidates[i].Timestamp.After(candidates[j].Timestamp)
	})
	if topN < len(candidates) {
		candidates = candidates[:topN]
	}
	for _, c := range candidates {
		summary.TemplateCandidates = append(summary.TemplateCandidates, PatternStat{Pattern: c.Pattern, ReuseCount: c.ReuseCount})
	}
}
