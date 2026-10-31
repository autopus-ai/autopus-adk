package spec

import (
	"fmt"
	"strings"

	"github.com/insajin/autopus-adk/pkg/spec/gates"
)

// ProfileLean is the reduced authoring profile for a new SPEC whose declared
// change class is low risk. It keeps the outcome, requirements, traceability,
// test scenarios and reviewer brief, and drops the sections that only pay off
// on high-risk work (invariant inventory, minimality matrix, probe table,
// visual brief, self-verify ledger). The empty profile is the full SPEC set.
const ProfileLean = "lean"

// IsLean reports whether the document declares the lean authoring profile.
func (d *SpecDocument) IsLean() bool {
	return d != nil && d.Profile == ProfileLean
}

// leanRequiredSections is the section floor of a lean SPEC set.
var leanRequiredSections = map[string][]string{
	"spec.md":       {"## Outcome Boundary", "## Requirements", "## Traceability Matrix"},
	"plan.md":       {"## Tasks"},
	"acceptance.md": {"## Test Scenarios"},
	"research.md":   {"## Outcome Lock", "## Reviewer Brief"},
}

// validateProfile refuses an unknown profile and a lean profile whose declared
// change class is missing or high risk, so a feature, multi-domain or
// security/data SPEC can never opt out of the full set.
func validateProfile(doc *SpecDocument) []ValidationError {
	if doc == nil || doc.Profile == "" {
		return nil
	}
	if doc.Profile != ProfileLean {
		return []ValidationError{{
			Field:   "spec.md",
			Message: fmt.Sprintf("알 수 없는 SPEC profile: %q (허용: %s)", doc.Profile, ProfileLean),
			Level:   "error",
		}}
	}
	kind, err := gates.ParseChangeKind(doc.ChangeClass)
	if err != nil {
		return []ValidationError{{
			Field:   "spec.md",
			Message: fmt.Sprintf("lean profile은 frontmatter change_class가 필요합니다: %v", err),
			Level:   "error",
		}}
	}
	if gates.HighRiskKind(kind) {
		return []ValidationError{{
			Field:   "spec.md",
			Message: fmt.Sprintf("change_class %s는 고위험이라 lean profile을 쓸 수 없습니다; profile을 지우고 full SPEC set을 작성하세요", kind),
			Level:   "error",
		}}
	}
	return validateLeanSurface(doc, kind)
}

// validateLeanSurface checks the declared class against the paths the SPEC
// says it will change, with the same deterministic classifier `auto spec
// gates` applies to the real change set later. A self-declared low-risk class
// cannot hide a security/data path, a public contract, production code in two
// module roots, or code under a docs/test-only declaration.
func validateLeanSurface(doc *SpecDocument, kind gates.ChangeKind) []ValidationError {
	if len(doc.Surface) == 0 {
		return []ValidationError{{
			Field:   "spec.md",
			Message: "lean profile은 frontmatter surface에 변경할 경로 목록이 필요합니다 (예: surface: internal/cli/foo.go, internal/cli/foo_test.go)",
			Level:   "error",
		}}
	}
	risk := gates.AssessChange(kind, gates.Classify(doc.Surface, nil), false)
	if !risk.Escalated() {
		return nil
	}
	return []ValidationError{{
		Field: "spec.md",
		Message: fmt.Sprintf("lean profile 거절: 선언한 surface가 %s로 판정됩니다 (%s); profile을 지우고 full SPEC set을 작성하세요",
			risk.EffectiveClass, strings.Join(risk.Reasons, ", ")),
		Level: "error",
	}}
}

// validateLeanSpecSet is the strict preflight of a lean SPEC set. The checks
// that remain are the ones a low-risk change still needs: required sections,
// unresolved placeholders, traceability rows and concrete acceptance.
func validateLeanSpecSet(files map[string]string) []ValidationError {
	var errs []ValidationError
	errs = append(errs, validateSectionSet(files, leanRequiredSections)...)
	errs = append(errs, validateAuthoringPlaceholders(files)...)
	errs = append(errs, validateLeanTraceability(files)...)
	errs = append(errs, validateCompletionDebtSeparation(files["research.md"])...)
	return errs
}

// validateLeanTraceability keeps the full invariant cross-check when the author
// chose to write an inventory, and otherwise only requires trace rows.
func validateLeanTraceability(files map[string]string) []ValidationError {
	if hasSection(files["research.md"], "## Semantic Invariant Inventory") {
		return validateTraceability(files["spec.md"], files["research.md"], files["acceptance.md"])
	}
	var errs []ValidationError
	if len(tableRows(sectionBody(files["spec.md"], "## Traceability Matrix"))) == 0 {
		errs = append(errs, ValidationError{
			Field:   "spec.md",
			Message: "Traceability Matrix에 Requirement, Plan Task, Acceptance Scenario 연결 행이 없습니다",
			Level:   "error",
		})
	}
	if acceptanceLooksStructuralOnly(files["acceptance.md"]) {
		errs = append(errs, ValidationError{
			Field:   "acceptance.md",
			Message: "Must acceptance가 structural-only 신호만 포함합니다. concrete expected output 또는 explicit tolerance가 필요합니다",
			Level:   "error",
		})
	}
	return errs
}

// LeanEligible reports whether the document declares the lean profile and its
// declared change class allows it. Consumers that relax a gate for lean SPECs
// check this, not IsLean, so a mislabelled high-risk SPEC keeps the full gate.
func (d *SpecDocument) LeanEligible() bool {
	return d.IsLean() && len(validateProfile(d)) == 0
}
