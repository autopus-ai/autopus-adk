package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/spec"
	"github.com/insajin/autopus-adk/pkg/terminal"
)

var (
	specReviewRunOrchestra = runStructuredSpecReviewOrchestra
	// specReviewProviderAssembly and specReviewJudgeAssembly build the gated,
	// read-only reviewer set and judge; tests replace them to inject configs.
	specReviewProviderAssembly = assembleSpecReviewProviders
	specReviewJudgeAssembly    = assembleSpecReviewJudge
	// specReviewTerminalDetector reports the terminal the review runs in.
	specReviewTerminalDetector = detectStructuredTerminal
	// specReviewBackendFactory selects the subprocess base for the read-only
	// review config and routes providers configured with backend: omp to the
	// OMP review backend.
	specReviewBackendFactory func(orchestra.OrchestraConfig) orchestra.ExecutionBackend = selectRoutedBackend
)

// specReviewSubprocessNotice tells a pane-capable terminal why no provider
// pane opens: read-only review runs every CLI provider as a subprocess.
const specReviewSubprocessNotice = "spec review: read-only review runs providers in subprocess mode"

// announceSpecReviewSubprocessMode prints the subprocess notice once per
// review when the terminal could host provider panes (REQ-16).
func announceSpecReviewSubprocessMode(w io.Writer, term terminal.Terminal) {
	if term != nil && term.Name() != "plain" {
		fmt.Fprintln(w, specReviewSubprocessNotice)
	}
}

// shippedStatuses lists spec statuses that represent work already delivered.
// A PASS verdict from a fresh review must never silently regress these back
// to `approved` (issue #38).
var shippedStatuses = map[string]struct{}{
	"completed":   {},
	"implemented": {},
}

func syncReviewedSpecStatus(specDir string, result *spec.ReviewResult, allowDegraded bool) error {
	_, err := syncReviewedSpecStatusWithReceipt(specDir, result, allowDegraded)
	return err
}

func syncReviewedSpecStatusWithReceipt(
	specDir string,
	result *spec.ReviewResult,
	allowDegraded bool,
	runtimeEvidence ...specReviewRuntimeEvidence,
) (specReviewPromotionReceipt, error) {
	evidence := specReviewRuntimeEvidence{RunID: orchestra.NewSessionID(), FinishedAt: time.Now().UTC()}
	if len(runtimeEvidence) > 0 {
		if runtimeEvidence[0].RunID != "" {
			evidence.RunID = runtimeEvidence[0].RunID
		}
		if !runtimeEvidence[0].FinishedAt.IsZero() {
			evidence.FinishedAt = runtimeEvidence[0].FinishedAt.UTC()
		}
	}
	receipt := specReviewPromotionReceipt{
		Schema: specReviewPromotionReceiptSchema, RunID: evidence.RunID,
		FinishedAt: evidence.FinishedAt.Format(time.RFC3339Nano), DegradedReasons: []string{},
		GateStatus: "blocked",
	}
	if result == nil {
		return receipt, nil
	}
	if len(runtimeEvidence) > 0 {
		receipt.ProviderPolicy = append([]specReviewProviderPolicyRow(nil), runtimeEvidence[0].ProviderPolicy...)
	}
	receipt.SpecID = result.SpecID
	receipt.Verdict = string(result.Verdict)
	receipt.AnalysisVerdict = string(result.Verdict)
	receipt.DegradedReasons = append([]string(nil), result.DegradedReasons...)
	receipt.Providers = append([]spec.ProviderStatus(nil), result.ProviderStatuses...)
	receipt.CriticalVeto = hasCriticalSpecReviewVeto(result.Findings)
	applySpecReviewRepeatDiscovery(&receipt, result)
	applySpecReviewLoopEvidence(&receipt, result)
	if result.Judge != nil {
		receipt.Judge = &specReviewJudgeReceipt{
			Provider:    result.Judge.Provider,
			Family:      result.Judge.Family,
			Status:      result.Judge.Status,
			Verdict:     result.Judge.Verdict,
			Accepted:    result.Judge.Accepted,
			Rejected:    result.Judge.Rejected,
			Merged:      result.Judge.Merged,
			AcceptedIDs: append([]string{}, result.Judge.AcceptedIDs...),
			Rationale:   truncateSpecReviewJudgeRationale(result.Judge.Rationale),
			Reason:      result.Judge.Reason,
		}
	}

	doc, err := spec.Load(specDir)
	if err != nil {
		return receipt, fmt.Errorf("status gate: load spec: %w", err)
	}
	receipt.PreviousStatus = doc.Status
	receipt.CurrentStatus = doc.Status
	if result.Verdict != spec.VerdictPass || hasActiveFindings(result.Findings) {
		return receipt, nil
	}
	receipt.GateStatus = "passed"
	if len(result.DegradedReasons) > 0 {
		receipt.GateStatus = "degraded"
	}

	// Guard against status regression: a PASS review on a SPEC that is
	// already completed/implemented must not rewrite its status.
	if _, shipped := shippedStatuses[strings.ToLower(doc.Status)]; shipped {
		return receipt, nil
	}

	// SPEC-ADK-REVIEW-INTEGRITY-001 REQ-RINT-PROMO-06: a clean PASS auto-promotes
	// only when every auxiliary document was fully observed and the provider
	// quorum was met. Degraded observation blocks promotion unless the operator
	// passed --allow-degraded, which promotes via a recorded audit override.
	decision := evaluateIntegrityGate(result.DegradedReasons, allowDegraded)
	if !decision.promote {
		// Leave the prior status unchanged and surface why plus the remedy.
		fmt.Println(formatIntegrityBlockMessage(decision.reasons))
		return receipt, nil
	}
	if decision.viaOverride {
		result.OverridePromotion = true
		receipt.OverrideApplied = true
		receipt.GateStatus = "passed"
		fmt.Println(formatIntegrityOverrideAudit(decision.reasons))
		// Re-persist review.md so the override audit line (rendered by
		// review_persist from OverridePromotion) lands in the artifact.
		if err := spec.PersistReview(specDir, result); err != nil {
			return receipt, fmt.Errorf("status gate: persist override audit: %w", err)
		}
	}

	if err := spec.UpdateStatus(specDir, "approved"); err != nil {
		return receipt, err
	}
	receipt.CurrentStatus = "approved"
	receipt.StatusChanged = receipt.PreviousStatus != receipt.CurrentStatus
	return receipt, nil
}

func hasCriticalSpecReviewVeto(findings []spec.ReviewFinding) bool {
	for _, finding := range findings {
		if !spec.IsActiveBlockingFinding(finding) {
			continue
		}
		if strings.EqualFold(finding.Severity, "critical") &&
			(finding.Category == spec.FindingCategorySecurity || finding.Category == spec.FindingCategoryCorrectness) {
			return true
		}
	}
	return false
}
