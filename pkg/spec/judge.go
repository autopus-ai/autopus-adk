package spec

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// Judge dismissal scopes. A judge clears a reviewer's verdict-scoped blocker
// only by a `reject` decision with a reason whose scope_ref names it: the
// reviewer's REJECT vote ("verdict") or one checklist item id.
const JudgeDismissalVerdict = "verdict"

// JudgeDismissal is a judge's explicit rejection of one reviewer's
// verdict-scoped blocker (SPEC-REVIEWPOLICY-001 REQ-RP-POL-04).
type JudgeDismissal struct {
	Reviewer string `json:"reviewer"`
	ScopeRef string `json:"scope_ref"`
	Reason   string `json:"reason"`
}

// JudgeDecision is one ruling of the judge on a reviewer finding, linked to
// the reviewers that raised it (SPEC-REVIEWREUSE-001 REQ-RR-JDG-07).
type JudgeDecision struct {
	ID        string   `json:"id,omitempty"`
	Decision  string   `json:"decision"`
	Reviewers []string `json:"reviewers,omitempty"`
	ScopeRef  string   `json:"scope_ref,omitempty"`
	Reason    string   `json:"reason,omitempty"`
	MergeInto string   `json:"merge_into,omitempty"`
}

// Judge statuses beyond the parsed outcome: a conditional judge mode that
// found nothing to arbitrate records skipped with the reason.
const JudgeStatusSkipped = "skipped"

// JudgeSummary records the typed judge outcome.
type JudgeSummary struct {
	Provider    string   `json:"provider"`
	Family      string   `json:"family"`
	Status      string   `json:"status"`
	Verdict     string   `json:"verdict"`
	Accepted    int      `json:"accepted"`
	Rejected    int      `json:"rejected"`
	Merged      int      `json:"merged"`
	AcceptedIDs []string `json:"accepted_ids,omitempty"`
	Rationale   string   `json:"rationale,omitempty"`
	Reason      string   `json:"reason,omitempty"`
	// Dismissals are the verdict-scoped blockers the judge explicitly
	// rejected; anything else a reviewer raised outside findings still blocks.
	Dismissals []JudgeDismissal `json:"dismissals,omitempty"`
	// Mode is the judge mode the round ran under; Decisions are the judge's
	// rulings in its own order (SPEC-REVIEWREUSE-001 REQ-RR-JDG-07).
	Mode      string          `json:"mode,omitempty"`
	Decisions []JudgeDecision `json:"decisions,omitempty"`
}

// Dismisses reports whether the judge rejected reviewer's blocker scope.
func (j *JudgeSummary) Dismisses(reviewer, scopeRef string) bool {
	if j == nil {
		return false
	}
	for _, dismissal := range j.Dismissals {
		if dismissal.Reviewer == reviewer && strings.EqualFold(strings.TrimSpace(dismissal.ScopeRef), strings.TrimSpace(scopeRef)) {
			return true
		}
	}
	return false
}

// JudgedFindingsToReview converts accepted judge findings and applies verify-mode lifecycle transitions.
// A `merge` finding contributes only its sources: they are folded into the
// accepted finding named by its merge_into and never open a finding of their
// own. In verify mode a prior finding accepted again keeps its identity; one
// the judge had previously resolved comes back as regressed.
func JudgedFindingsToReview(
	_ string,
	revision int,
	out *orchestra.ReviewJudgeOutput,
	aliasToProvider map[string]string,
	prior []ReviewFinding,
) []ReviewFinding {
	if out == nil {
		return nil
	}
	out = foldMergedJudgeSources(out)
	if len(prior) == 0 {
		return discoverJudgedFindings(revision, out, aliasToProvider)
	}

	priorByID := make(map[string]ReviewFinding, len(prior))
	nextID := 1
	for _, finding := range prior {
		id := strings.TrimSpace(finding.ID)
		if id != "" {
			priorByID[id] = finding
		}
		if strings.HasPrefix(id, "F-") {
			if number, err := strconv.Atoi(strings.TrimPrefix(id, "F-")); err == nil && number >= nextID {
				nextID = number + 1
			}
		}
	}

	acceptedPrior := make(map[string]struct{}, len(prior))
	result := make([]ReviewFinding, 0, len(out.Findings)+len(prior))
	for _, finding := range out.Findings {
		if !strings.EqualFold(strings.TrimSpace(finding.Decision), "accept") {
			continue
		}
		id := strings.TrimSpace(finding.ID)
		if existing, ok := priorByID[id]; ok {
			current := reviewFindingFromJudge(id, revision, finding, aliasToProvider)
			current.Provider = unionProviders(existing.Provider, current.Provider)
			if existing.Status == FindingStatusResolved || existing.Status == FindingStatusRegressed {
				current.Status = FindingStatusRegressed
			}
			current.FirstSeenRev = existing.FirstSeenRev
			current.EscapeHatch = existing.EscapeHatch
			if strings.TrimSpace(finding.Category) == "" {
				current.Category = existing.Category
			}
			if current.ScopeRef == "" {
				current.ScopeRef = existing.ScopeRef
			}
			if current.Description == "" {
				current.Description = existing.Description
			}
			result = append(result, current)
			acceptedPrior[id] = struct{}{}
			continue
		}
		id = fmt.Sprintf("F-%03d", nextID)
		nextID++
		result = append(result, reviewFindingFromJudge(id, revision, finding, aliasToProvider))
	}
	for _, finding := range prior {
		if _, accepted := acceptedPrior[strings.TrimSpace(finding.ID)]; accepted {
			continue
		}
		finding.Status = FindingStatusResolved
		finding.LastSeenRev = revision
		result = append(result, finding)
	}
	return result
}

// foldMergedJudgeSources returns a copy whose accepted findings carry the
// sources of every merge finding that names the same id.
func foldMergedJudgeSources(out *orchestra.ReviewJudgeOutput) *orchestra.ReviewJudgeOutput {
	mergedSources := make(map[string][]string)
	for _, finding := range out.Findings {
		target := strings.TrimSpace(finding.MergeInto)
		if target != "" && strings.EqualFold(strings.TrimSpace(finding.Decision), "merge") {
			mergedSources[target] = append(mergedSources[target], finding.Sources...)
		}
	}
	if len(mergedSources) == 0 {
		return out
	}
	folded := *out
	folded.Findings = make([]orchestra.JudgedFinding, len(out.Findings))
	for i, finding := range out.Findings {
		folded.Findings[i] = finding
		extra := mergedSources[strings.TrimSpace(finding.ID)]
		if len(extra) == 0 || !strings.EqualFold(strings.TrimSpace(finding.Decision), "accept") {
			continue
		}
		sources := append([]string(nil), finding.Sources...)
		seen := make(map[string]struct{}, len(sources))
		for _, source := range sources {
			seen[strings.TrimSpace(source)] = struct{}{}
		}
		for _, source := range extra {
			if _, dup := seen[strings.TrimSpace(source)]; dup || strings.TrimSpace(source) == "" {
				continue
			}
			seen[strings.TrimSpace(source)] = struct{}{}
			sources = append(sources, source)
		}
		folded.Findings[i].Sources = sources
	}
	return &folded
}

func discoverJudgedFindings(
	revision int,
	out *orchestra.ReviewJudgeOutput,
	aliasToProvider map[string]string,
) []ReviewFinding {
	usedIDs := make(map[string]struct{}, len(out.Findings))
	for _, finding := range out.Findings {
		if strings.EqualFold(strings.TrimSpace(finding.Decision), "accept") && strings.TrimSpace(finding.ID) != "" {
			usedIDs[strings.TrimSpace(finding.ID)] = struct{}{}
		}
	}
	var result []ReviewFinding
	nextID := 1
	for _, finding := range out.Findings {
		if !strings.EqualFold(strings.TrimSpace(finding.Decision), "accept") {
			continue
		}
		id := strings.TrimSpace(finding.ID)
		if id == "" {
			for {
				id = fmt.Sprintf("F-%03d", nextID)
				nextID++
				if _, exists := usedIDs[id]; !exists {
					usedIDs[id] = struct{}{}
					break
				}
			}
		}
		result = append(result, reviewFindingFromJudge(id, revision, finding, aliasToProvider))
	}
	return result
}

func reviewFindingFromJudge(
	id string,
	revision int,
	finding orchestra.JudgedFinding,
	aliasToProvider map[string]string,
) ReviewFinding {
	scopeRef := strings.TrimSpace(finding.ScopeRef)
	if scopeRef == "" {
		scopeRef = strings.TrimSpace(finding.Location)
	}
	return ReviewFinding{
		ID:           id,
		Provider:     restoreJudgeSources(finding.Sources, aliasToProvider),
		Severity:     normalizeStructuredSeverity(finding.Severity),
		Category:     normalizeStructuredCategory(finding.Category),
		ScopeRef:     scopeRef,
		Description:  strings.TrimSpace(finding.Description),
		Status:       FindingStatusOpen,
		FirstSeenRev: revision,
		LastSeenRev:  revision,
	}
}

// unionProviders joins two comma-separated provider lists without duplicates,
// keeping the prior attribution first.
func unionProviders(prior, current string) string {
	seen := make(map[string]struct{})
	var out []string
	for _, list := range []string{prior, current} {
		for _, item := range strings.Split(list, ",") {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			if _, dup := seen[item]; dup {
				continue
			}
			seen[item] = struct{}{}
			out = append(out, item)
		}
	}
	return strings.Join(out, ", ")
}

func restoreJudgeSources(sources []string, aliasToProvider map[string]string) string {
	restored := make([]string, 0, len(sources))
	for _, source := range sources {
		source = strings.TrimSpace(source)
		if source == "" {
			continue
		}
		if provider := strings.TrimSpace(aliasToProvider[source]); provider != "" {
			source = provider
		}
		restored = append(restored, source)
	}
	return strings.Join(restored, ", ")
}
