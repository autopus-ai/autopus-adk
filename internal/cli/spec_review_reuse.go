package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/spec"
)

const (
	specReviewEndReasonReused = "reused"
	// specReviewReuseMaxAge bounds how long a recorded result may stand in
	// for a review (SPEC-REVIEWREUSE-001 REQ-RR-REUSE-02).
	specReviewReuseMaxAge = 168 * time.Hour
)

// specReviewReuse is the reuse decision: whether a previous receipt exists,
// whether its result is returned, the loop state it reports, and every reason
// it is not.
type specReviewReuse struct {
	found   bool
	reuse   bool
	receipt specReviewPromotionReceipt
	state   string
	reasons []string
}

// decideSpecReviewReuse compares the previous receipt with the current
// fingerprint (REQ-RR-REUSE-02 to REQ-RR-REUSE-04). Nothing in the receipt is
// trusted that can be recomputed: every input is re-hashed and the evidence
// files must still match their recorded digests.
func decideSpecReviewReuse(specDir string, current *specReviewFingerprint, now time.Time) specReviewReuse {
	data, err := os.ReadFile(filepath.Join(specDir, "review-receipt.json"))
	if errors.Is(err, os.ErrNotExist) {
		return specReviewReuse{}
	}
	decision := specReviewReuse{found: true}
	var receipt specReviewPromotionReceipt
	switch {
	case err != nil, json.Unmarshal(data, &receipt) != nil:
		decision.reasons = []string{"receipt: 손상"}
		return decision
	case receipt.Schema != specReviewPromotionReceiptSchema:
		decision.reasons = []string{"receipt: 알 수 없는 schema"}
		return decision
	}
	decision.receipt = receipt
	previous := receipt.Fingerprint
	switch {
	case previous == nil:
		decision.reasons = append(decision.reasons, "fingerprint: 없음")
	case previous.Schema != specReviewFingerprintSchema:
		decision.reasons = append(decision.reasons, "fingerprint: 알 수 없는 schema")
	case !previous.Complete:
		decision.reasons = append(decision.reasons, "fingerprint: 불완전 ("+strings.Join(previous.Incomplete, ", ")+")")
	}
	if !current.Complete {
		decision.reasons = append(decision.reasons, "fingerprint: 불완전 (현재 입력: "+strings.Join(current.Incomplete, ", ")+")")
	}
	if len(decision.reasons) == 0 {
		decision.reasons = append(decision.reasons, specReviewFingerprintChanges(*previous, *current)...)
		decision.reasons = append(decision.reasons, specReviewEvidenceChanges(specDir, previous.Evidence)...)
	}
	if finished, err := time.Parse(time.RFC3339Nano, receipt.FinishedAt); err != nil {
		decision.reasons = append(decision.reasons, "receipt: finished_at 없음")
	} else if now.Sub(finished) > specReviewReuseMaxAge {
		decision.reasons = append(decision.reasons, "168시간 초과")
	}
	state, reusable := reusableSpecReviewResult(receipt)
	if !reusable {
		decision.reasons = append(decision.reasons, fmt.Sprintf("결과: %s/%s 재사용 불가", receipt.Verdict, receipt.LoopStatus))
	} else if receipt.Verdict == string(spec.VerdictPass) && previous != nil {
		if aliases := specReviewUnpinnedProviders(previous.Providers); len(aliases) > 0 {
			decision.reasons = append(decision.reasons, "PASS: 모델 alias ("+strings.Join(aliases, ", ")+")")
		}
	}
	decision.state = state
	decision.reuse = len(decision.reasons) == 0
	return decision
}

// reusableSpecReviewResult returns the loop state a reusable result reports
// (REQ-RR-REUSE-03): a clean converged PASS, a REJECT that was not cut short,
// or a REVISE whose blockers wait for the author.
func reusableSpecReviewResult(receipt specReviewPromotionReceipt) (string, bool) {
	if len(receipt.DegradedReasons) > 0 {
		return "", false
	}
	switch receipt.Verdict {
	case string(spec.VerdictPass):
		if receipt.GateStatus == "passed" && receipt.LoopStatus == spec.LoopStatusConverged {
			return spec.LoopStatusConverged, true
		}
	case string(spec.VerdictReject):
		switch receipt.LoopStatus {
		case "", spec.LoopStatusBudgetExhausted, spec.LoopStatusProviderUnavailable:
		default:
			return receipt.LoopStatus, true
		}
	case string(spec.VerdictRevise):
		awaiting := receipt.LoopStatus == spec.LoopStatusAwaitingChanges || receipt.LoopStatus == spec.LoopStatusRevisionsExhausted
		if awaiting && !hasBlockingPolicy(receipt.BlockingReasons, spec.BlockingPolicyNoUsableReview, spec.BlockingPolicyBudgetExhausted) {
			return spec.LoopStatusAwaitingChanges, true
		}
	}
	return "", false
}

// specReviewFingerprintChanges names each changed component.
func specReviewFingerprintChanges(previous, current specReviewFingerprint) []string {
	var reasons []string
	if names := changedDigestKeys(previous.Documents, current.Documents); len(names) > 0 {
		reasons = append(reasons, "documents: "+strings.Join(names, ", "))
	}
	if names := changedDigestKeys(previous.References, current.References); len(names) > 0 {
		reasons = append(reasons, "references: "+strings.Join(names, ", "))
	}
	for name, pair := range map[string][2]string{"head": {previous.Code.Head, current.Code.Head},
		"worktree": {previous.Code.Worktree, current.Code.Worktree}, "context": {previous.Code.Context, current.Code.Context}} {
		if pair[0] != pair[1] {
			reasons = append(reasons, "code: "+name)
		}
	}
	if previous.Checklist != current.Checklist {
		reasons = append(reasons, "checklist")
	}
	if fields := changedJSONFields(previous.Policy, current.Policy); len(fields) > 0 {
		reasons = append(reasons, "policy: "+strings.Join(fields, ", "))
	}
	if previous.Config != current.Config {
		reasons = append(reasons, "config")
	}
	if names := specReviewDeliveryChanges(previous.Delivery, current.Delivery); len(names) > 0 {
		reasons = append(reasons, "delivery: "+strings.Join(names, ", "))
	}
	if previous.Contract != current.Contract {
		reasons = append(reasons, "contract")
	}
	if names := changedProviders(previous.Providers, current.Providers); len(names) > 0 {
		reasons = append(reasons, "providers: "+strings.Join(names, ", "))
	}
	slices.Sort(reasons)
	return reasons
}

// specReviewDeliveryChanges names a changed auxiliary budget and each
// document whose delivered excerpt changed.
func specReviewDeliveryChanges(previous, current specReviewDeliveryDigest) []string {
	var names []string
	if previous.AuxBudgetLines != current.AuxBudgetLines {
		names = append(names, "aux_budget_lines")
	}
	return append(names, changedDigestKeys(previous.Excerpts, current.Excerpts)...)
}

func specReviewEvidenceChanges(specDir string, evidence specReviewEvidenceDigest) []string {
	var reasons []string
	for name, recorded := range map[string]string{"review-findings.json": evidence.ReviewFindings, "review.md": evidence.ReviewMD} {
		data, err := os.ReadFile(filepath.Join(specDir, name))
		if err != nil || spec.SHA256Hex(data) != recorded {
			reasons = append(reasons, "evidence: "+name)
		}
	}
	slices.Sort(reasons)
	return reasons
}

func changedDigestKeys(previous, current map[string]string) []string {
	var names []string
	for _, name := range slices.Sorted(maps.Keys(mergedKeys(previous, current))) {
		if previous[name] != current[name] {
			names = append(names, name)
		}
	}
	return names
}

func mergedKeys[V any](a, b map[string]V) map[string]bool {
	keys := make(map[string]bool, len(a)+len(b))
	for key := range a {
		keys[key] = true
	}
	for key := range b {
		keys[key] = true
	}
	return keys
}

func changedJSONFields(previous, current any) []string {
	var before, after map[string]any
	first, _ := json.Marshal(previous)
	second, _ := json.Marshal(current)
	_ = json.Unmarshal(first, &before)
	_ = json.Unmarshal(second, &after)
	var fields []string
	for key := range mergedKeys(before, after) {
		if !reflect.DeepEqual(before[key], after[key]) {
			fields = append(fields, key)
		}
	}
	slices.Sort(fields)
	return fields
}

func changedProviders(previous, current []specReviewProviderIdentity) []string {
	index := func(list []specReviewProviderIdentity) map[string]specReviewProviderIdentity {
		out := make(map[string]specReviewProviderIdentity, len(list))
		for _, identity := range list {
			out[identity.Role+"/"+identity.Name] = identity
		}
		return out
	}
	before, after := index(previous), index(current)
	var names []string
	for key := range before {
		if identity, ok := after[key]; !ok || identity != before[key] {
			names = append(names, before[key].Name)
		}
	}
	for key := range after {
		if _, ok := before[key]; !ok {
			names = append(names, after[key].Name)
		}
	}
	slices.Sort(names)
	return slices.Compact(names)
}

func specReviewUnpinnedProviders(providers []specReviewProviderIdentity) []string {
	var aliases []string
	for _, provider := range providers {
		if !provider.Pinned {
			model := provider.Model
			if model == "" {
				model = "모델 미지정"
			}
			aliases = append(aliases, provider.Name+": "+model)
		}
	}
	return aliases
}

// printSpecReviewReuse states that the result was reused, from which run, and
// that no call was made, so a hit is never shown as a new review
// (REQ-RR-REUSE-05).
func printSpecReviewReuse(w io.Writer, specID, receiptPath string, decision specReviewReuse, findings []spec.ReviewFinding) {
	fmt.Fprintf(w, "SPEC 리뷰 재사용: %s — 입력이 %s 리뷰(run %s)와 같아 이전 결과를 반환합니다\n",
		specID, decision.receipt.FinishedAt, decision.receipt.RunID)
	fmt.Fprintf(w, "판정: %s\n", decision.receipt.Verdict)
	fmt.Fprintf(w, "리뷰 상태: %s\n", decision.state)
	fmt.Fprintf(w, "Review receipt: %s (변경 없음)\n", receiptPath)
	fmt.Fprintf(w, "모델 호출 0회 — 다시 리뷰하려면 --force-review (같은 세션 예산 사용)\n")
	if len(findings) > 0 {
		fmt.Fprintf(w, "발견 사항: %s\n", spec.SummarizeFindings(findings).Format())
	}
}
