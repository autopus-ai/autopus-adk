package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/spec"
)

// Observation-integrity promotion gate (SPEC-ADK-REVIEW-INTEGRITY-001).
//
// A multi-provider review can return PASS while a reviewer never actually saw a
// truncated auxiliary document, or while only a minority of providers responded.
// This file aggregates both signals into machine-readable degraded reasons and
// decides whether a clean PASS may auto-promote a SPEC to "approved". The render
// halves (verdict-line suffix, Observation Coverage section, override audit line)
// live in pkg/spec/review_persist.go; the enforcement logic stays here in the
// platform-neutral Go runtime.

// integrityRemedyHint is the operator remedy appended to the promotion-hold
// message when the gate blocks auto-promotion. It names both escape routes: fix
// the underlying observation gap, or re-run with the explicit override flag
// (REQ-RINT-PROMO-06).
const integrityRemedyHint = "문서/프로바이더 관측을 보강하거나 --allow-degraded 로 재실행하세요"

// observationDegradedReasons returns the machine-readable degraded reasons for a
// single revision. The order is stable — coverage before quorum — so
// review.md, the findings sidecar, and the audit log stay diff-stable across
// runs (REQ-RINT-TRUNC-04, REQ-RINT-QUORUM-05).
// @AX:NOTE: [AUTO] subtle invariant — reasons are appended coverage-then-quorum; reordering or sorting this slice elsewhere changes the rendered verdict-line and audit-log text and breaks diff-stability across review runs
func observationDegradedReasons(coverages []spec.DocCoverage, statuses []spec.ProviderStatus, configuredProviders, minProviders int) []string {
	var reasons []string
	if !spec.AllDocumentsFullyObserved(coverages) {
		reasons = append(reasons, spec.DegradedReasonPartialDocContext)
	}
	if met, _ := spec.MeetsProviderQuorum(statuses, configuredProviders, minProviders); !met {
		reasons = append(reasons, spec.DegradedReasonProviderQuorum)
	}
	return reasons
}

// applyObservationIntegrity computes per-document injection coverage and the
// provider quorum for one revision, records both on the result, and returns
// the coverage set for the findings sidecar. The budget is resolved solely
// through spec.ResolveAuxTotalBudget so the coverage the gate reads matches what
// the prompt injected (REQ-RINT-FULL-02). The promotion gate later reads
// result.DegradedReasons to decide whether a clean PASS may auto-promote.
func applyObservationIntegrity(result *spec.ReviewResult, specDir string, gate config.ReviewGateConf, configuredProviders int) []spec.DocCoverage {
	budget := spec.ResolveAuxTotalBudget(gate.DocContextMaxLines)
	coverages := spec.AuxDocCoverages(specDir, budget)
	result.DocCoverages = coverages
	result.DegradedReasons = observationDegradedReasons(coverages, result.ProviderStatuses, configuredProviders, gate.MinProviders)
	return coverages
}

// integrityGateDecision is the outcome of the observation-integrity promotion
// gate for a PASS review that already cleared the verdict and findings checks.
type integrityGateDecision struct {
	promote     bool     // status may advance to approved
	viaOverride bool     // promotion proceeds only because --allow-degraded was set
	reasons     []string // degraded reasons blocking (or overridden by the operator)
}

// evaluateIntegrityGate decides whether a clean PASS may auto-promote to
// "approved". With no degraded reasons the SPEC promotes normally. With degraded
// reasons present, promotion is blocked unless allowDegraded is set, in which
// case it promotes via a recorded override (REQ-RINT-PROMO-06, REQ-RINT-OVERRIDE-07).
// @AX:NOTE: [AUTO] fail-closed default — the final branch (promote: false) fires whenever reasons is non-empty and allowDegraded is false; flipping this default would silently reintroduce the unqualified-PASS promotion bug SPEC-ADK-REVIEW-INTEGRITY-001 exists to close
func evaluateIntegrityGate(reasons []string, allowDegraded bool) integrityGateDecision {
	if len(reasons) == 0 {
		return integrityGateDecision{promote: true}
	}
	// A review the session budget cut short did not make every planned call,
	// so no override turns it into an approval (SPEC-REVIEWPOLICY-001
	// REQ-RP-BUD-05). The loop already resolves such a round as non-PASS; this
	// keeps the gate itself from ever promoting it.
	if slices.Contains(reasons, spec.DegradedReasonBudgetExhausted) {
		return integrityGateDecision{promote: false, reasons: reasons}
	}
	if allowDegraded {
		return integrityGateDecision{promote: true, viaOverride: true, reasons: reasons}
	}
	return integrityGateDecision{promote: false, reasons: reasons}
}

// formatIntegrityBlockMessage renders the stable stdout line shown when the gate
// holds a PASS at its prior status because observation integrity was not met.
func formatIntegrityBlockMessage(reasons []string) string {
	return fmt.Sprintf("승격 보류: 관측 무결성 미충족 (사유: %s) — %s",
		strings.Join(reasons, ", "), integrityRemedyHint)
}

// formatIntegrityOverrideAudit renders the stable stdout audit line shown when
// --allow-degraded promotes a degraded-observation PASS.
func formatIntegrityOverrideAudit(reasons []string) string {
	return fmt.Sprintf("승격 override: --allow-degraded 로 degraded PASS 승격 (사유: %s)",
		strings.Join(reasons, ", "))
}
