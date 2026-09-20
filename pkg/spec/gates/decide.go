package gates

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// DefaultMaxAge is the default freshness window for evidence reuse.
const DefaultMaxAge = 168 * time.Hour

// Reuse-failure conditions appended to a required gate's reason.
const (
	ReasonNoPriorEvidence = "no prior evidence"
	ReasonClosureChanged  = "input closure changed"
	ReasonPriorFail       = "prior status fail"
	ReasonPriorPartial    = "prior evidence partial"
	ReasonStale           = "evidence older than max-age"
	ReasonMissingInput    = "missing input"
)

// PriorEvidence is a recorded evidence receipt together with its input
// closure recomputed from the current tree.
type PriorEvidence struct {
	Receipt        EvidenceReceipt
	Path           string   // receipt path recorded in reused_evidence
	CurrentClosure string   // input_closure_sha256 recomputed from the current tree
	MissingInputs  []string // explicitly named inputs absent from the current tree
}

// DecisionInput carries everything Decide needs; it performs no IO.
type DecisionInput struct {
	SpecID         string
	Classification Classification
	Change         ChangeRisk // zero value is derived from Classification
	// AnnotationRequested opts the @AX annotation gate into evaluation. The
	// gate is not_applicable unless a caller asks for it, so ordinary code
	// work is never held behind an annotation pass it never requested.
	AnnotationRequested        bool
	AnnotationReferenceMissing bool // the @AX reference source is absent
	Prior                      map[GateID]PriorEvidence
	Now                        time.Time
	MaxAge                     time.Duration // zero selects DefaultMaxAge
	DisableReuse               bool          // force fresh checks when execution conditions changed
}

// Decide computes the applicability receipt for input. Base rules derive from
// the change class and the risk tier of the declared change class; evidence
// reuse then overlays required gates with reusable when the exact-input rule
// holds.
func Decide(input DecisionInput) ApplicabilityReceipt {
	if input.Change.Tier == "" {
		input.Change = AssessChange("", input.Classification, false)
	}
	receipt := ApplicabilityReceipt{
		Schema:       ApplicabilitySchema,
		SpecID:       input.SpecID,
		ChangeClass:  input.Classification.Class,
		ChangeRisk:   input.Change,
		ChangedPaths: append([]string{}, input.Classification.Paths...),
		GeneratedAt:  input.Now.UTC().Format(time.RFC3339),
		Decisions:    make([]GateDecision, 0, len(Catalog)),
	}
	maxAge := input.MaxAge
	if maxAge <= 0 {
		maxAge = DefaultMaxAge
	}
	for _, entry := range Catalog {
		decision := baseDecision(entry, input)
		if decision.Applicability == Required {
			if input.DisableReuse {
				decision.Reason += "; reuse disabled by caller"
			} else {
				overlayReuse(&decision, input.Prior[entry.ID], input.Now, maxAge)
			}
		}
		receipt.Decisions = append(receipt.Decisions, decision)
	}
	return receipt
}

func baseDecision(entry CatalogEntry, input DecisionInput) GateDecision {
	class := input.Classification.Class
	decision := GateDecision{Gate: entry.ID, Applicability: Required, Mandatory: entry.Mandatory}
	docOnly := class == ClassDocOnly
	crossBoundary := class == ClassSecurityOrData || class == ClassMultiDomain

	switch entry.ID {
	case GateSpecAuthoring:
		if input.Change.Tier == RiskLow {
			return notApplicable(decision, fmt.Sprintf("low-risk %s change: the compact change contract replaces the four-document SPEC set", input.Change.EffectiveClass))
		}
		decision.Reason = fmt.Sprintf("high-risk %s change requires the full SPEC set: %s", input.Change.EffectiveClass, strings.Join(input.Change.Reasons, ", "))
	case GateRiskFirstProbe:
		if docOnly {
			return notApplicable(decision, "no integration boundary")
		}
		if input.Change.Tier == RiskLow {
			return notApplicable(decision, fmt.Sprintf("low-risk %s change: no integration boundary requiring a probe", input.Change.EffectiveClass))
		}
		decision.Reason = "integration boundary in change set"
	case GateBuild, GateUnitTests:
		if docOnly {
			return notApplicable(decision, "documentation-only change set")
		}
		decision.Reason = "code change set"
	case GateIntegration:
		if !crossBoundary {
			return notApplicable(decision, "single-domain change set without security or data surface")
		}
		decision.Reason = string(class) + " change set crosses an integration boundary"
	case GateSecurity, GateValidation, GateDataLoss, GateDeterministicOracle:
		decision.Reason = "mandatory safety gate"
	case GateAccessibility, GateUXVerification:
		if !input.Classification.HasUI() {
			return notApplicable(decision, "no UI surface in change set")
		}
		decision.Reason = fmt.Sprintf("UI surface in change set: %d path(s)", len(input.Classification.UIPaths))
	case GateAnnotation:
		if !input.AnnotationRequested {
			return notApplicable(decision, "@AX annotation not requested for this change set")
		}
		if docOnly {
			return notApplicable(decision, "documentation-only change set")
		}
		if input.AnnotationReferenceMissing {
			decision.Applicability = Blocked
			decision.Reason = "reference source missing"
			return decision
		}
		decision.Reason = "code change set"
	case GateProviderReview:
		if docOnly {
			return notApplicable(decision, "documentation-only change set")
		}
		if crossBoundary {
			decision.Reason = string(class) + " change set requires provider review"
		} else {
			decision.Reason = "code change set requires provider review"
		}
	case GateDocSync:
		decision.Reason = "always required"
	}
	return decision
}

func notApplicable(decision GateDecision, reason string) GateDecision {
	decision.Applicability = NotApplicable
	decision.Reason = reason
	return decision
}

// overlayReuse turns a required decision into reusable when prior evidence
// satisfies the exact-input rule; otherwise it appends the failed condition.
func overlayReuse(decision *GateDecision, prior PriorEvidence, now time.Time, maxAge time.Duration) {
	if prior.Receipt.Schema == "" {
		decision.Reason += "; " + ReasonNoPriorEvidence
		return
	}
	decision.InputClosureSHA256 = prior.CurrentClosure
	condition, ok := reuseCondition(prior, now, maxAge)
	if !ok {
		decision.Reason += "; " + condition
		return
	}
	decision.Applicability = Reusable
	decision.Reason = "exact-input evidence matches current tree"
	decision.ReusedEvidence = &ReusedEvidence{
		Path:       prior.Path,
		ObservedAt: prior.Receipt.ObservedAt,
		Status:     prior.Receipt.Status,
	}
}

// reuseCondition returns the first failed reuse condition, or ok=true when
// every condition holds. Order: missing input, closure, status, completeness,
// freshness.
func reuseCondition(prior PriorEvidence, now time.Time, maxAge time.Duration) (string, bool) {
	if len(prior.MissingInputs) > 0 {
		return ReasonMissingInput + " " + prior.MissingInputs[0], false
	}
	if prior.CurrentClosure == "" || prior.CurrentClosure != prior.Receipt.InputClosureSHA256 {
		return ReasonClosureChanged, false
	}
	if prior.Receipt.Status == StatusFail {
		return ReasonPriorFail, false
	}
	if prior.Receipt.Status != StatusPass || !prior.Receipt.Complete {
		return ReasonPriorPartial, false
	}
	observed, err := time.Parse(time.RFC3339, prior.Receipt.ObservedAt)
	if err != nil || observed.After(now) || now.Sub(observed) > maxAge {
		return ReasonStale, false
	}
	return "", true
}

// RecomputeClosure re-resolves receipt's recorded globs and dynamic
// dependencies against root and returns the current closure hash together
// with any explicitly named inputs that no longer exist.
func RecomputeClosure(root string, receipt EvidenceReceipt) (closure string, missing []string, err error) {
	deps := make([]string, 0, len(receipt.DynamicDeps))
	for _, dep := range receipt.DynamicDeps {
		deps = append(deps, dep.Path)
	}
	inputs, dynamic, err := ResolveClosure(root, receipt.InputGlobs, deps)
	var missingErr *MissingInputError
	if errors.As(err, &missingErr) {
		missing = missingErr.Paths
	} else if err != nil {
		return "", nil, err
	}
	return ClosureSHA256(inputs, dynamic), missing, nil
}
