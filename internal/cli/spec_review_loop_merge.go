package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/spec"
)

// mergeVerifyFindings folds provider verify output into the prior checklist and
// returns the restatements it absorbed. A finding whose content already exists
// in the prior checklist is not added again, but it is reported as a repeat
// discovery so the receipt can show the review re-found known ground.
func mergeVerifyFindings(
	providerFindings [][]spec.ReviewFinding,
	priorFindings []spec.ReviewFinding,
	totalProviders int,
	threshold float64,
	revision int,
) ([]spec.ReviewFinding, []spec.RepeatDiscovery) {
	if len(providerFindings) == 0 {
		return append([]spec.ReviewFinding(nil), priorFindings...), nil
	}

	priorIDs := make(map[string]struct{}, len(priorFindings))
	for _, f := range priorFindings {
		if f.ID != "" {
			priorIDs[f.ID] = struct{}{}
		}
	}

	priorStatusInputs := make([][]spec.ReviewFinding, 0, len(providerFindings))
	var newFindings []spec.ReviewFinding
	var repeats []spec.RepeatDiscovery
	for _, findings := range providerFindings {
		var priorStatuses []spec.ReviewFinding
		for _, f := range findings {
			if _, ok := priorIDs[f.ID]; ok {
				priorStatuses = append(priorStatuses, f)
				continue
			}
			if matched, ok := priorFindingContentMatch(f, priorFindings); ok {
				repeats = append(repeats, spec.RepeatDiscovery{
					FindingID: f.ID, MatchedPriorID: matched.ID, Revision: revision,
				})
				continue
			}
			newFindings = append(newFindings, f)
		}
		priorStatusInputs = append(priorStatusInputs, priorStatuses)
	}

	merged := spec.MergeFindingStatuses(priorStatusInputs, threshold)
	mergedNew := spec.MergeSupermajority(newFindings, totalProviders, threshold)
	mergedNew = spec.DeduplicateFindings(mergedNew)
	assignNewFindingIDs(mergedNew, maxFindingNumber(priorFindings)+1)
	return append(merged, mergedNew...), spec.MergeRepeatDiscoveries(repeats)
}

type findingContentKey struct {
	scopeRef    string
	category    spec.FindingCategory
	description string
}

// priorFindingContentMatch returns the prior finding an incoming finding
// duplicates verbatim (same scope, category and normalized description).
func priorFindingContentMatch(f spec.ReviewFinding, priorFindings []spec.ReviewFinding) (spec.ReviewFinding, bool) {
	k := findingKey(f)
	for _, prior := range priorFindings {
		if findingKey(prior) == k {
			return prior, true
		}
	}
	return spec.ReviewFinding{}, false
}

func findingKey(f spec.ReviewFinding) findingContentKey {
	return findingContentKey{
		scopeRef:    spec.NormalizeScopeRef(f.ScopeRef, ""),
		category:    f.Category,
		description: normalizedFindingDescription(f.Description),
	}
}

func normalizedFindingDescription(description string) string {
	return strings.ToLower(strings.Join(strings.Fields(description), " "))
}

func assignNewFindingIDs(findings []spec.ReviewFinding, start int) {
	for i := range findings {
		findings[i].ID = fmt.Sprintf("F-%03d", start+i)
	}
}

func maxFindingNumber(findings []spec.ReviewFinding) int {
	maxID := 0
	for _, f := range findings {
		raw := strings.TrimPrefix(f.ID, "F-")
		n, err := strconv.Atoi(raw)
		if err != nil {
			continue
		}
		if n > maxID {
			maxID = n
		}
	}
	return maxID
}

func parseSpecReviewerResults(
	result *orchestra.OrchestraResult,
	specID string,
	revision int,
	priorFindings []spec.ReviewFinding,
) ([]spec.ReviewResult, []orchestra.ProviderResponse) {
	if result == nil {
		return nil, nil
	}
	failedProviderNames := make(map[string]struct{}, len(result.FailedProviders))
	for _, failed := range result.FailedProviders {
		failedProviderNames[failed.Name] = struct{}{}
	}
	reviews := make([]spec.ReviewResult, 0, len(result.Responses))
	responses := make([]orchestra.ProviderResponse, 0, len(result.Responses))
	for _, response := range result.Responses {
		if isSpecReviewJudgeResponse(response) {
			continue
		}
		if _, failed := failedProviderNames[response.Provider]; failed {
			continue
		}
		if response.TimedOut || response.ExitCode != 0 || response.EmptyOutput {
			continue
		}
		reviews = append(reviews, spec.ParseVerdict(
			specID, response.Output, response.Provider, revision, nilIfEmpty(priorFindings),
		))
		responses = append(responses, response)
	}
	return reviews, responses
}

func applySpecReviewJudge(
	merged *spec.ReviewResult,
	result *orchestra.OrchestraResult,
	reviewerResponses []orchestra.ProviderResponse,
	judgeProvider string,
	revision int,
	priorFindings []spec.ReviewFinding,
) {
	if merged == nil || result == nil {
		return
	}
	_, aliasToProvider := orchestra.AnonymizeReviewParticipants(reviewerResponses)
	for _, response := range result.Responses {
		if !isSpecReviewJudgeResponse(response) {
			continue
		}
		out, err := (&orchestra.OutputParser{}).ParseReviewJudge(response.Output)
		if err == nil {
			err = validateStructuredReviewJudge(out, aliasToProvider)
		}
		if err != nil {
			merged.Judge = &spec.JudgeSummary{
				Provider: trimSpecReviewJudgeSuffix(response.Provider, judgeProvider),
				Family:   response.ModelFamily,
				Status:   "invalid",
				Reason:   err.Error(),
			}
			return
		}
		accepted, rejected, combined := reviewJudgeDecisionCounts(out)
		judgedFindings := spec.JudgedFindingsToReview(
			merged.SpecID, revision, out, aliasToProvider, priorFindings,
		)
		merged.Judge = &spec.JudgeSummary{
			Provider:    trimSpecReviewJudgeSuffix(response.Provider, judgeProvider),
			Family:      response.ModelFamily,
			Status:      "ok",
			Verdict:     out.Verdict,
			Accepted:    accepted,
			Rejected:    rejected,
			Merged:      combined,
			AcceptedIDs: acceptedJudgeFindingIDs(judgedFindings, accepted),
			Rationale:   out.Rationale,
			Dismissals:  specReviewJudgeDismissals(out, aliasToProvider),
			Decisions:   specReviewJudgeDecisions(out, aliasToProvider),
		}
		merged.Verdict = spec.ReviewVerdict(out.Verdict)
		// A judge PASS never waives an accepted blocking finding: the finding
		// stays open and resolveReviewVerdict names it (SPEC-REVIEWPOLICY-001
		// REQ-RP-POL-04). Advisory findings are deferred by the caller.
		merged.Findings = judgedFindings
		return
	}
	applyFailedSpecReviewJudgeSummary(merged, result.FailedProviders, judgeProvider)
}

// JudgedFindingsToReview emits accepted findings before resolved prior findings.
func acceptedJudgeFindingIDs(findings []spec.ReviewFinding, accepted int) []string {
	ids := make([]string, 0, accepted)
	for i := 0; i < len(findings) && i < accepted; i++ {
		ids = append(ids, findings[i].ID)
	}
	return ids
}

func applyFailedSpecReviewJudgeSummary(
	merged *spec.ReviewResult,
	failedProviders []orchestra.FailedProvider,
	judgeProvider string,
) {
	for _, failed := range failedProviders {
		if failed.Role != "judge" && !strings.HasSuffix(strings.TrimSpace(failed.Name), specReviewJudgeSuffix) {
			continue
		}
		merged.Judge = &spec.JudgeSummary{
			Provider: trimSpecReviewJudgeSuffix(failed.Name, judgeProvider),
			Family:   failed.ModelFamily,
			Status:   specReviewJudgeFailureStatus(failed),
			Reason:   specReviewJudgeFailureReason(failed),
		}
		return
	}
}

func specReviewJudgeFailureStatus(failed orchestra.FailedProvider) string {
	class := strings.ToLower(strings.TrimSpace(failed.FailureClass))
	reason := strings.ToLower(failed.Error)
	if strings.Contains(class, "invalid") ||
		strings.Contains(reason, "invalid review judge") ||
		strings.Contains(reason, "parse review_judge") {
		return "invalid"
	}
	return "failed"
}

func specReviewJudgeFailureReason(failed orchestra.FailedProvider) string {
	if reason := strings.TrimSpace(failed.Error); reason != "" {
		return reason
	}
	return strings.TrimSpace(failed.FailureClass)
}

func trimSpecReviewJudgeSuffix(provider, fallback string) string {
	provider = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(provider), specReviewJudgeSuffix))
	if provider == "" {
		return strings.TrimSpace(fallback)
	}
	return provider
}
