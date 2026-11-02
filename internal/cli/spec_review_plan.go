package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/spec"
	"github.com/insajin/autopus-adk/pkg/spec/gates"
)

// Values of `auto spec review --profile` and `--judge` (SPEC-REVIEWPOLICY-001
// REQ-RP-PLAN-01). The profile selector is the only way to change the review
// profile; --providers selects reviewers and never switches the profile.
const (
	specReviewProfileAuto = "auto"
	specReviewJudgeNone   = "none"
	// Judge modes. R8 adds conditional modes; the plan prints the mode so a
	// conditional judge can show its condition without a new plan shape.
	specReviewJudgeModeNone   = "none"
	specReviewJudgeModeAlways = "always"
)

// specReviewPlan is the resolved shape of one review attempt: the profile and
// why, the risk class, who reviews and judges and why, the quorum, and the
// round bound. It is computed before any readiness probe or model call,
// configures the existing read-only assembly, and is printed before the first
// model call of every attempt.
type specReviewPlan struct {
	SpecID         string
	Profile        string
	ProfileReason  string
	RiskDeclared   string
	RiskEffective  string
	RiskTier       string
	RiskReasons    []string
	Reviewers      []string
	ReviewerSource string
	Fallbacks      []string
	Judge          string
	JudgeMode      string
	JudgeReason    string
	Quorum         int
	MaxRounds      int
	Warnings       []string
}

// resolveSpecReviewPlan applies the precedence of REQ-RP-PLAN-02 and refuses
// the combinations of REQ-RP-PLAN-04 before anything executes.
func resolveSpecReviewPlan(
	doc *spec.SpecDocument, gate config.ReviewGateConf, opts specReviewOptions, multi bool, maxRevisions int,
) (specReviewPlan, error) {
	requested := strings.ToLower(strings.TrimSpace(opts.profile))
	if requested == "" {
		requested = specReviewProfileAuto
	}
	switch requested {
	case specReviewProfileAuto, spec.ReviewProfileFull, spec.ReviewProfileLean:
	default:
		return specReviewPlan{}, fmt.Errorf("--profile %q: expected one of auto, full, lean", opts.profile)
	}
	if opts.judgeSet && strings.TrimSpace(opts.judge) == "" {
		return specReviewPlan{}, fmt.Errorf("--judge needs a provider name or %q", specReviewJudgeNone)
	}
	if opts.newSession && opts.extendBudget {
		return specReviewPlan{}, fmt.Errorf("--new-session and --extend-budget cannot be combined: extend the current session or start a new one")
	}
	if opts.maxCalls < 0 || opts.maxMinutes < 0 {
		return specReviewPlan{}, fmt.Errorf("--max-calls and --max-minutes must be >= 0")
	}
	policy := spec.EffectiveReviewPolicy(doc)
	if requested == spec.ReviewProfileLean && !policy.Lean() {
		return specReviewPlan{}, fmt.Errorf("--profile lean: %s is not lean-eligible (%s); remove --profile lean or fix the spec.md frontmatter",
			doc.ID, leanIneligibleReason(doc, policy))
	}
	plan := specReviewPlan{SpecID: doc.ID, Profile: policy.Profile, MaxRounds: maxRevisions + 1}
	plan.describeRisk(doc)
	plan.ProfileReason = specReviewProfileReason(doc, policy)
	if requested == spec.ReviewProfileFull && policy.Lean() {
		plan.Profile = spec.ReviewProfileFull
		plan.ProfileReason = "--profile full (SPEC는 lean 적격)"
		plan.warn("--profile full: lean SPEC를 full panel(설정된 리뷰어 전원과 judge)로 리뷰합니다. 이전에는 --providers가 lean 정책을 끄는 방법이었습니다")
	}
	if doc.IsLean() && !policy.Lean() {
		plan.warn(fmt.Sprintf("lean profile 조건을 충족하지 않아 full 리뷰로 진행합니다: %s", policy.EscalationReason))
	}
	if plan.Profile == spec.ReviewProfileLean {
		plan.resolveLeanReviewers(gate, opts, multi)
	} else {
		plan.resolveFullReviewers(gate, opts, multi)
	}
	plan.resolveJudge(gate, opts)
	return plan, nil
}

func (p *specReviewPlan) resolveLeanReviewers(gate config.ReviewGateConf, opts specReviewOptions, multi bool) {
	p.Quorum = 1
	if len(opts.providers) > 0 {
		p.Reviewers, p.Fallbacks, p.ReviewerSource = opts.providers[:1], slices.Clone(opts.providers[1:]), "--providers"
		if len(opts.providers) > 1 {
			p.warn(fmt.Sprintf("--providers %s: lean profile은 리뷰어 1명으로 실행합니다 — %s가 리뷰하고 %s는 실행 실패 시 fallback입니다. 이전에는 --providers가 lean 정책(리뷰어 1명, judge 없음, quorum 1)을 끄고 full panel을 돌렸습니다; panel은 --profile full로 요청하세요",
				strings.Join(opts.providers, ","), opts.providers[0], strings.Join(opts.providers[1:], ", ")))
		}
		return
	}
	candidates := leanSpecReviewers(gate)
	if len(candidates) == 0 {
		candidates = defaultProviders()
	}
	p.Reviewers, p.Fallbacks, p.ReviewerSource = candidates[:1], slices.Clone(candidates[1:]), "spec.review_gate (judge 우선 순서)"
	if multi {
		p.warn("--multi: lean profile은 리뷰어 1명으로 실행합니다; panel은 --profile full로 요청하세요")
	}
}

func (p *specReviewPlan) resolveFullReviewers(gate config.ReviewGateConf, opts specReviewOptions, multi bool) {
	p.Quorum = gate.MinProviders
	if len(opts.providers) > 0 {
		p.Reviewers, p.ReviewerSource = slices.Clone(opts.providers), "--providers"
		return
	}
	p.ReviewerSource = "spec.review_gate.providers"
	if multi {
		p.ReviewerSource += " + --multi discovery"
	}
}

func (p *specReviewPlan) resolveJudge(gate config.ReviewGateConf, opts specReviewOptions) {
	judge := strings.TrimSpace(opts.judge)
	switch {
	case opts.judgeSet && strings.EqualFold(judge, specReviewJudgeNone):
		p.JudgeReason = "--judge none"
	case opts.judgeSet:
		p.Judge, p.JudgeReason = judge, "--judge"
		if p.Profile == spec.ReviewProfileLean {
			p.warn(fmt.Sprintf("--judge %s: lean profile은 judge 없이 실행하지만 --judge로 judge를 추가합니다 (라운드당 호출 1회 추가)", judge))
		}
	case p.Profile == spec.ReviewProfileLean:
		p.JudgeReason = "lean profile 기본값"
	case strings.TrimSpace(gate.Judge) != "":
		p.Judge, p.JudgeReason = strings.TrimSpace(gate.Judge), "spec.review_gate.judge"
	default:
		p.JudgeReason = "spec.review_gate.judge 미설정"
	}
	p.JudgeMode = specReviewJudgeModeNone
	if p.Judge != "" {
		p.JudgeMode = specReviewJudgeModeAlways
	}
}

func (p *specReviewPlan) describeRisk(doc *spec.SpecDocument) {
	p.RiskTier = "unknown"
	if strings.TrimSpace(doc.ChangeClass) == "" {
		p.RiskDeclared, p.RiskEffective = "undeclared", "undeclared"
		return
	}
	kind, err := gates.ParseChangeKind(doc.ChangeClass)
	if err != nil {
		p.RiskDeclared, p.RiskEffective = doc.ChangeClass, "unknown"
		return
	}
	risk := gates.AssessChange(kind, gates.Classify(doc.Surface, nil), false)
	p.RiskDeclared, p.RiskEffective = string(kind), string(risk.EffectiveClass)
	p.RiskTier, p.RiskReasons = string(risk.Tier), risk.Reasons
}

func (p *specReviewPlan) warn(line string) { p.Warnings = append(p.Warnings, line) }

// applySpecReviewPlan turns the plan into the gate and options the read-only
// assembly reads: the planned reviewers, the planned judge (empty for none),
// and quorum 1 on lean. It returns the lean fallbacks.
func applySpecReviewPlan(plan specReviewPlan, gate config.ReviewGateConf, opts specReviewOptions) (config.ReviewGateConf, specReviewOptions, []string) {
	if plan.Reviewers != nil {
		opts.providers = slices.Clone(plan.Reviewers)
	}
	gate.Judge = plan.Judge
	if plan.Profile == spec.ReviewProfileLean {
		gate.MinProviders = plan.Quorum
	}
	return gate, opts, slices.Clone(plan.Fallbacks)
}

func specReviewProfileReason(doc *spec.SpecDocument, policy spec.ReviewPolicy) string {
	switch {
	case policy.Lean():
		return fmt.Sprintf("spec.md profile: lean (change_class %s)", doc.ChangeClass)
	case doc.IsLean():
		return "lean 선언이 거절되어 full"
	case doc.Profile != "":
		return fmt.Sprintf("알 수 없는 profile %q라 full", doc.Profile)
	}
	return "spec.md에 profile이 없어 full"
}

func leanIneligibleReason(doc *spec.SpecDocument, policy spec.ReviewPolicy) string {
	if policy.EscalationReason != "" {
		return policy.EscalationReason
	}
	if !doc.IsLean() {
		return "spec.md declares no profile: lean"
	}
	return "the lean profile rules refuse it"
}

// specReviewReceiptProfile is the receipt's review_profile: "lean" only when
// the review actually ran lean, empty (full) otherwise, as before.
func specReviewReceiptProfile(plan specReviewPlan) string {
	if plan.Profile == spec.ReviewProfileLean {
		return spec.ProfileLean
	}
	return ""
}
