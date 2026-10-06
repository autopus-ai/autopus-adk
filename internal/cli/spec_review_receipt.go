package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/spec"
)

const specReviewPromotionReceiptSchema = "spec_review_promotion_receipt.v1"

type specReviewJudgeReceipt struct {
	Provider    string   `json:"provider"`
	Family      string   `json:"family"`
	Status      string   `json:"status"`
	Verdict     string   `json:"verdict"`
	Accepted    int      `json:"accepted"`
	Rejected    int      `json:"rejected"`
	Merged      int      `json:"merged"`
	AcceptedIDs []string `json:"accepted_ids"`
	Rationale   string   `json:"rationale"`
	Reason      string   `json:"reason"`
}

type specReviewPromotionReceipt struct {
	Schema          string                  `json:"schema"`
	RunID           string                  `json:"run_id"`
	FinishedAt      string                  `json:"finished_at"`
	SpecID          string                  `json:"spec_id,omitempty"`
	Verdict         string                  `json:"verdict,omitempty"`
	AnalysisVerdict string                  `json:"analysis_verdict"`
	GateStatus      string                  `json:"gate_status"`
	CriticalVeto    bool                    `json:"critical_veto"`
	PreviousStatus  string                  `json:"previous_status,omitempty"`
	CurrentStatus   string                  `json:"current_status,omitempty"`
	StatusChanged   bool                    `json:"status_changed"`
	DegradedReasons []string                `json:"degraded_reasons"`
	OverrideApplied bool                    `json:"override_applied"`
	Providers       []spec.ProviderStatus   `json:"providers,omitempty"`
	Judge           *specReviewJudgeReceipt `json:"judge,omitempty"`

	// Review-convergence evidence (issue #186). All omitempty so receipts from
	// converging runs keep their existing bytes.
	RepeatDiscoveries       []spec.RepeatDiscovery `json:"repeat_discoveries,omitempty"`
	RepeatDiscoveryCount    int                    `json:"repeat_discovery_count,omitempty"`
	SameInputReReview       bool                   `json:"same_input_rereview,omitempty"`
	DiscoveryRepeatDetected bool                   `json:"discovery_repeat_detected,omitempty"`

	// Verdict-consistency evidence (issue #187). LoopStatus tells the operator
	// how the revision loop ended, BlockingReasons why a REVISE/REJECT happened.
	LoopStatus      string                `json:"loop_status,omitempty"`
	BlockingReasons []spec.BlockingReason `json:"blocking_reasons,omitempty"`

	// Provider execution policy (SPEC-REVIEWRO-001 REQ-07), omitempty so a
	// receipt of a run without provider execution keeps its existing bytes.
	ProviderPolicy []specReviewProviderPolicyRow `json:"provider_policy,omitempty"`
}

// specReviewProviderPolicyRow records how one reviewer, excluded provider, or
// judge ran. SandboxMode is judged from the executed argv and stays empty for
// a provider that never executed; Readiness is the preflight status token.
type specReviewProviderPolicyRow struct {
	Provider    string `json:"provider"`
	Role        string `json:"role"`
	SandboxMode string `json:"sandbox_mode"`
	Readiness   string `json:"readiness"`
	Excluded    bool   `json:"excluded"`
}

type specReviewRuntimeEvidence struct {
	RunID          string
	FinishedAt     time.Time
	ProviderPolicy []specReviewProviderPolicyRow
}

// applySpecReviewRepeatDiscovery projects the loop's repeat-discovery evidence
// onto the promotion receipt. DiscoveryRepeatDetected is the operator-facing
// verdict: the review either re-found known ground or re-reviewed unchanged
// input, and either way the loop is not converging on new information.
func applySpecReviewRepeatDiscovery(receipt *specReviewPromotionReceipt, result *spec.ReviewResult) {
	if receipt == nil || result == nil {
		return
	}
	receipt.RepeatDiscoveries = append([]spec.RepeatDiscovery(nil), result.RepeatDiscoveries...)
	receipt.RepeatDiscoveryCount = len(result.RepeatDiscoveries)
	receipt.SameInputReReview = result.SameInputReReview
	receipt.DiscoveryRepeatDetected = receipt.RepeatDiscoveryCount > 0 || receipt.SameInputReReview
}

// applySpecReviewLoopEvidence projects the loop termination state and the stated
// blockers onto the promotion receipt, so a reader can tell why a REVISE
// happened and whether re-running without edits is pointless.
func applySpecReviewLoopEvidence(receipt *specReviewPromotionReceipt, result *spec.ReviewResult) {
	if receipt == nil || result == nil {
		return
	}
	receipt.LoopStatus = result.LoopStatus
	receipt.BlockingReasons = append([]spec.BlockingReason(nil), result.BlockingReasons...)
}

// blockingPolicySummary joins the distinct policies that blocked, for one-line
// operator output.
func blockingPolicySummary(reasons []spec.BlockingReason) string {
	if len(reasons) == 0 {
		return "none"
	}
	seen := make(map[string]struct{}, len(reasons))
	policies := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		if _, ok := seen[reason.Policy]; ok {
			continue
		}
		seen[reason.Policy] = struct{}{}
		policies = append(policies, reason.Policy)
	}
	return strings.Join(policies, ", ")
}

func persistSpecReviewPromotionReceipt(specDir string, receipt specReviewPromotionReceipt) (string, error) {
	path := filepath.Join(specDir, "review-receipt.json")
	data, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal SPEC review promotion receipt: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("write SPEC review promotion receipt: %w", err)
	}
	return path, nil
}

func truncateSpecReviewJudgeRationale(value string) string {
	const maxRunes = 500
	value = strings.TrimSpace(value)
	count := 0
	for index := range value {
		if count == maxRunes {
			return value[:index]
		}
		count++
	}
	return value
}
