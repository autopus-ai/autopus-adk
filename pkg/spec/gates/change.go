package gates

import (
	"fmt"
	"strings"
)

// ChangeKind is the declared class of a change. It is orthogonal to
// ChangeClass: ChangeKind carries what the author says the work is,
// ChangeClass carries what the touched paths actually show. Path signals may
// only raise the declared class, never lower it.
type ChangeKind string

const (
	KindTestOnly       ChangeKind = "test_only"
	KindDocsOnly       ChangeKind = "docs_only"
	KindSmallUI        ChangeKind = "small_ui"
	KindBugfix         ChangeKind = "bugfix_existing_contract"
	KindFeature        ChangeKind = "feature"
	KindMultiDomain    ChangeKind = "multi_domain"
	KindSecurityOrData ChangeKind = "security_or_data"
)

// ChangeKinds lists every declarable change class in escalation order.
var ChangeKinds = []ChangeKind{
	KindTestOnly, KindDocsOnly, KindSmallUI, KindBugfix,
	KindFeature, KindMultiDomain, KindSecurityOrData,
}

// RiskTier is the risk band a change class falls into. Only low-risk work may
// take the compact change-contract path.
type RiskTier string

const (
	RiskLow  RiskTier = "low"
	RiskHigh RiskTier = "high"
)

// ChangeDecision is the authoring path a change set is allowed to take.
type ChangeDecision string

const (
	// DecisionCompact allows a compact change.md instead of the four-document
	// SPEC set.
	DecisionCompact ChangeDecision = "compact_contract"
	// DecisionEscalate requires the full SPEC set plus a risk-first
	// integration probe.
	DecisionEscalate ChangeDecision = "escalate_to_full_spec"
)

// Escalation reasons. Every escalation names at least one of these, so a
// refused compact contract is never a bare verdict.
const (
	EscalationDeclaredHighRisk = "declared_class_is_high_risk"
	EscalationSecuritySurface  = "security_or_data_surface_in_change_set"
	EscalationMultiDomain      = "multi_domain_change_set"
	EscalationNewContract      = "new_exported_api_or_contract"
	EscalationContractSurface  = "public_contract_surface_in_change_set"
	EscalationTestOnlyCode     = "declared_test_only_touches_production_source"
	EscalationDocsOnlyCode     = "declared_docs_only_touches_code"
	EscalationSmallUINonUI     = "declared_small_ui_touches_non_ui_source"
)

// ChangeRisk is the deterministic risk decision for a change set.
type ChangeRisk struct {
	DeclaredClass  ChangeKind     `json:"declared_class"`
	EffectiveClass ChangeKind     `json:"effective_class"`
	Tier           RiskTier       `json:"risk_tier"`
	Decision       ChangeDecision `json:"decision"`
	Reasons        []string       `json:"reasons,omitempty"`
}

// Escalated reports whether the compact path was refused.
func (r ChangeRisk) Escalated() bool { return r.Decision == DecisionEscalate }

// Compact reports whether this risk record authorizes the compact route. It
// is not enough for the record to claim a low tier and a compact decision:
// the classes it names must be recognised, must be low risk on their own,
// must not have been escalated, and the effective class must not sit below
// the declared one, because path signals only ever raise a class. A record
// that fails any of those is internally inconsistent, and an inconsistent
// record is exactly what a forged or stale one looks like.
func (r ChangeRisk) Compact() bool {
	if r.Decision != DecisionCompact || r.Tier != RiskLow || len(r.Reasons) > 0 {
		return false
	}
	for _, kind := range []ChangeKind{r.DeclaredClass, r.EffectiveClass} {
		if !KnownChangeKind(kind) || HighRiskKind(kind) {
			return false
		}
	}
	return kindRank(r.EffectiveClass) >= kindRank(r.DeclaredClass)
}

// KnownChangeKind reports whether kind is one of the declarable classes.
func KnownChangeKind(kind ChangeKind) bool {
	for _, known := range ChangeKinds {
		if known == kind {
			return true
		}
	}
	return false
}

// ParseChangeKind resolves a declared change class name.
func ParseChangeKind(value string) (ChangeKind, error) {
	normalized := ChangeKind(strings.ToLower(strings.TrimSpace(value)))
	names := make([]string, 0, len(ChangeKinds))
	for _, kind := range ChangeKinds {
		if kind == normalized {
			return kind, nil
		}
		names = append(names, string(kind))
	}
	return "", fmt.Errorf("unknown change class %q: expected one of %s", value, strings.Join(names, ", "))
}

// kindRank orders change classes by risk so that a path signal can raise the
// effective class without ever lowering it.
func kindRank(kind ChangeKind) int {
	switch kind {
	case KindTestOnly, KindDocsOnly:
		return 0
	case KindSmallUI:
		return 1
	case KindBugfix:
		return 2
	case KindFeature:
		return 3
	case KindMultiDomain:
		return 4
	case KindSecurityOrData:
		return 5
	default:
		return 3
	}
}

// HighRiskKind reports whether a change class is high risk on its own, before
// any path signal. Feature work is high risk because it introduces behaviour
// the existing acceptance criteria do not cover.
func HighRiskKind(kind ChangeKind) bool {
	switch kind {
	case KindFeature, KindMultiDomain, KindSecurityOrData:
		return true
	default:
		return false
	}
}

// DeriveChangeKind reads the change class off the touched paths. It is the
// declared class used when a caller supplies none, and never resolves to a
// class lower than the surface justifies.
func DeriveChangeKind(c Classification) ChangeKind {
	switch {
	case c.Class == ClassDocOnly:
		return KindDocsOnly
	case len(c.SecurityPaths) > 0:
		return KindSecurityOrData
	case len(c.Paths) > 0 && !hasNonTestCode(c):
		return KindTestOnly
	case len(productionRoots(c)) >= 2:
		return KindMultiDomain
	case c.Class == ClassUIOnly:
		return KindSmallUI
	default:
		return KindFeature
	}
}

// AssessChange decides whether declared may take the compact change-contract
// path. An empty declared class is derived from the change set, in which case
// no declaration can be contradicted. newContract records an author-declared
// new exported API or contract; path signals are evaluated independently.
func AssessChange(declared ChangeKind, c Classification, newContract bool) ChangeRisk {
	if declared == "" {
		declared = DeriveChangeKind(c)
	}
	risk := ChangeRisk{DeclaredClass: declared, EffectiveClass: declared}
	raise := func(to ChangeKind, reason string) {
		risk.Reasons = append(risk.Reasons, reason)
		if kindRank(to) > kindRank(risk.EffectiveClass) {
			risk.EffectiveClass = to
		}
	}

	// Path signals: a security, data, or migration surface outranks any lower
	// declaration, as does production code spanning several module roots.
	// Documentation and test material cannot change behaviour across a
	// boundary, so they never raise the multi-domain signal on their own.
	if len(c.SecurityPaths) > 0 && declared != KindSecurityOrData {
		raise(KindSecurityOrData, EscalationSecuritySurface)
	}
	if len(productionRoots(c)) >= 2 && declared != KindMultiDomain {
		raise(KindMultiDomain, EscalationMultiDomain)
	}
	if !HighRiskKind(declared) {
		if newContract {
			raise(KindFeature, EscalationNewContract)
		}
		if len(contractPaths(c)) > 0 {
			raise(KindFeature, EscalationContractSurface)
		}
	}

	// Declaration consistency: a compact class that turns out to touch
	// production behaviour beyond its declared surface escalates.
	if reason := declaredSurfaceViolation(declared, c); reason != "" {
		raise(KindFeature, reason)
	}

	risk.Tier = RiskLow
	risk.Decision = DecisionCompact
	if HighRiskKind(risk.EffectiveClass) || len(risk.Reasons) > 0 {
		risk.Tier = RiskHigh
		risk.Decision = DecisionEscalate
	}
	if risk.Escalated() && len(risk.Reasons) == 0 {
		risk.Reasons = []string{EscalationDeclaredHighRisk}
	}
	return risk
}
