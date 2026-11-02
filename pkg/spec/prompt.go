package spec

import (
	"fmt"
	"os"
	"strings"
)

const trimNoticeFormat = "[Review-context notice: %d additional lines were omitted from this injected excerpt because of the prompt context limit. This notice is not a source document defect and must not be used as SPEC quality finding evidence.]"

// BuildReviewPrompt constructs a review prompt from a SPEC document and code context.
// specDir (optional variadic): path to the spec directory for loading plan.md/research.md/acceptance.md.
// opts.Mode controls whether a discover (open-ended) or verify (checklist) prompt is generated.
// The full spec.md content is included to prevent false positives from parser truncation
// (e.g., multi-line requirements with SQL schemas or itemized lists after the EARS header).
func BuildReviewPrompt(doc *SpecDocument, codeContext string, opts ReviewPromptOptions, specDir ...string) string {
	if opts.RequireCompleteDocuments && len(opts.completeDocuments) == 0 {
		documents, err := loadCompleteReviewDocuments(resolveReviewSpecDir(opts, specDir))
		if err != nil {
			return ""
		}
		frozenDoc, err := freezeCompleteReviewSpec(doc, documents)
		if err != nil {
			return ""
		}
		doc = frozenDoc
		opts.completeDocuments = documents
	}
	var sb strings.Builder

	sb.WriteString("You are reviewing a SPEC to decide whether implementation can start from it: check correctness, completeness, and feasibility only as far as they decide that.\n\n")
	fmt.Fprintf(&sb, "## SPEC: %s — %s\n\n", doc.ID, doc.Title)
	if opts.RequireCompleteDocuments {
		injectCompleteContextDocs(&sb, opts.completeDocuments)
	}

	// Include full spec.md content to avoid parser-induced truncation.
	// The EARS parser only captures single-line descriptions, missing multi-line
	// requirement bodies (SQL schemas, itemized lists, etc.).
	if opts.RequireCompleteDocuments {
		if !injectCompleteSpecDoc(&sb, opts.completeDocuments) {
			return ""
		}
	} else if doc.RawContent != "" {
		sb.WriteString("### Full SPEC Document\n\n")
		sb.WriteString(doc.RawContent)
		sb.WriteString("\n\n")
	} else {
		// Fallback: use parsed requirements when raw content is unavailable.
		if len(doc.Requirements) > 0 {
			sb.WriteString("### Requirements\n\n")
			for _, req := range doc.Requirements {
				fmt.Fprintf(&sb, "- **%s** [%s]: %s\n", req.ID, req.Type, req.Description)
			}
			sb.WriteString("\n")
		}
	}

	if len(doc.AcceptanceCriteria) > 0 {
		sb.WriteString("### Acceptance Criteria\n\n")
		for _, ac := range doc.AcceptanceCriteria {
			fmt.Fprintf(&sb, "- %s: %s\n", ac.ID, ac.Description)
		}
		sb.WriteString("\n")
	}

	// Inject auxiliary documentation context (plan.md, research.md, acceptance.md).
	dir := opts.SpecDir
	if len(specDir) > 0 && specDir[0] != "" {
		dir = specDir[0]
	}
	if dir != "" {
		if opts.RequireCompleteDocuments {
			injectCompleteAuxDocs(&sb, opts.completeDocuments)
		} else {
			injectAuxDocs(&sb, dir, ResolveAuxTotalBudget(opts.DocContextMaxLines))
		}
	}

	if codeContext != "" {
		sb.WriteString("### Existing Code Context\n\n")
		sb.WriteString("```\n")
		sb.WriteString(codeContext)
		sb.WriteString("\n```\n\n")
	}

	checklistIncluded := false
	maxLines := opts.DocContextMaxLines
	if opts.RequireCompleteDocuments {
		maxLines = int(^uint(0) >> 1)
	}
	if checklistBody, checklistPath, err := loadChecklistForPrompt(opts); err != nil {
		fmt.Fprintf(os.Stderr, "경고: 체크리스트 로드 실패 (%s): %v\n", checklistPath, err)
	} else {
		InjectChecklistSection(&sb, checklistBody, maxLines)
		checklistIncluded = true
	}

	// One policy states the review purpose and any profile allowances before
	// the verdict rules (SPEC-REVIEWPOLICY-001 REQ-RP-POL-06).
	writeReviewPolicySection(&sb, EffectiveReviewPolicy(doc))

	if opts.Mode == ReviewModeVerify || len(opts.PriorFindings) > 0 {
		buildVerifyInstructions(&sb, opts.PriorFindings, opts.PassCriteria, checklistIncluded)
	} else {
		buildDiscoverInstructions(&sb, opts.StaticFindings, opts.PassCriteria, checklistIncluded)
	}

	return sb.String()
}

// injectAuxDocs injects plan.md, research.md, and acceptance.md into the prompt.
// Missing files are silently skipped. Documents inject in full while their
// combined line count fits totalBudget; over budget, the largest document is
// compacted with tail-critical sections preserved rather than head-trimmed
// (REQ-RINT-FULL-02, REQ-RINT-STRUCT-03). The shared planning core also backs
// AuxDocCoverages so coverage records match what was actually injected.
func injectAuxDocs(sb *strings.Builder, specDir string, totalBudget int) {
	for _, plan := range planAuxDocInjection(specDir, totalBudget) {
		sb.WriteString(plan.header)
		sb.WriteString("\n\n")
		sb.WriteString(plan.excerpt)
		sb.WriteString("\n\n")
	}
}

// trimToLines truncates content to maxLines lines.
// Appends a trim notice when truncation occurs.
func trimToLines(content string, maxLines int) string {
	lines := strings.Split(content, "\n")
	if len(lines) <= maxLines {
		return content
	}
	trimmed := strings.Join(lines[:maxLines], "\n")
	extra := len(lines) - maxLines
	return fmt.Sprintf("%s\n"+trimNoticeFormat, trimmed, extra)
}

// buildVerifyInstructions writes checklist-based instructions for verify mode.
func buildVerifyInstructions(sb *strings.Builder, priorFindings []ReviewFinding, passCriteria string, checklistIncluded bool) {
	sb.WriteString("### Verdict Decision Rules\n\n")
	writeVerdictRules(sb, passCriteria)
	sb.WriteString("\n")

	if checklistIncluded {
		sb.WriteString("### Checklist Response Format\n\n")
		writeChecklistExamples(sb)
		sb.WriteString("\n")
	}

	sb.WriteString("### Finding Format Examples\n\n")
	writeFindingExamples(sb)
	sb.WriteString("\n")

	sb.WriteString("### Instructions (Verify Mode)\n\n")
	sb.WriteString("For each finding below, report its current status.\n\n")
	sb.WriteString("Do NOT stop early, narrow the scope on your own, or replace the requested output with progress notes.\n")
	sb.WriteString("Review all prior findings in one pass; do not drip-feed one optional suggestion per revision.\n")
	sb.WriteString("If auxiliary docs include `## Outcome Lock`, `## Completion Debt`, `## Evolution Ideas`, or `## Reviewer Brief`, use them to block only Completion Debt and keep Evolution Ideas advisory.\n")
	writeTrimNoticeInstruction(sb)
	sb.WriteString("A `suggestion` is advisory and must not be the only reason for REVISE. If VERDICT is PASS, do not keep suggestion-only findings open.\n")
	sb.WriteString("Responses that omit the required VERDICT/FINDING_STATUS lines are treated as malformed review output.\n\n")

	if len(priorFindings) > 0 {
		sb.WriteString("#### Prior Findings Checklist\n\n")
		for _, f := range priorFindings {
			fmt.Fprintf(sb, "- %s [%s] %s: %s\n", f.ID, f.Severity, f.ScopeRef, f.Description)
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Respond with:\n")
	sb.WriteString("1. VERDICT: PASS, REVISE, or REJECT\n")
	if checklistIncluded {
		sb.WriteString("2. For each quality checklist item, write: CHECKLIST: <항목 ID> | PASS or CHECKLIST: <항목 ID> | FAIL | <reason>\n")
		sb.WriteString("3. For each prior finding, write: FINDING_STATUS: F-{id} | {open|resolved|regressed} | {reason}\n")
		sb.WriteString("4. Report any regression or newly broken behavior caused by fixes, even if not in the checklist.\n")
	} else {
		sb.WriteString("2. For each prior finding, write: FINDING_STATUS: F-{id} | {open|resolved|regressed} | {reason}\n")
		sb.WriteString("3. Report any regression or newly broken behavior caused by fixes, even if not in the checklist.\n")
	}
	sb.WriteString("   For new critical/security issues: FINDING: [severity] [category] [scope_ref] description\n")
}

// buildDiscoverInstructions writes open-ended instructions for discover mode.
func buildDiscoverInstructions(sb *strings.Builder, staticFindings []ReviewFinding, passCriteria string, checklistIncluded bool) {
	sb.WriteString("### Verdict Decision Rules\n\n")
	writeVerdictRules(sb, passCriteria)
	sb.WriteString("\n")

	if checklistIncluded {
		sb.WriteString("### Checklist Response Format\n\n")
		writeChecklistExamples(sb)
		sb.WriteString("\n")
	}

	sb.WriteString("### Finding Format Examples\n\n")
	writeFindingExamples(sb)
	sb.WriteString("\n")

	sb.WriteString("### Instructions\n\n")
	sb.WriteString("Do NOT stop early, narrow the scope on your own, or replace the requested output with progress notes.\n")
	sb.WriteString("Review the whole SPEC in one pass and return the full set of actionable issues together.\n")
	sb.WriteString("If auxiliary docs include `## Outcome Lock`, `## Completion Debt`, `## Evolution Ideas`, or `## Reviewer Brief`, use them to prioritize Completion Debt blockers over optional deeper-layer suggestions.\n")
	writeTrimNoticeInstruction(sb)
	sb.WriteString("Use `suggestion` only for non-blocking advisory improvements; suggestions alone must not drive REVISE.\n")
	sb.WriteString("Responses that omit the required VERDICT/FINDING lines are treated as malformed review output.\n\n")

	if len(staticFindings) > 0 {
		sb.WriteString("#### Already Discovered Static Analysis Issues\n\n")
		for _, f := range staticFindings {
			fmt.Fprintf(sb, "- [%s] %s: %s\n", f.Severity, f.ScopeRef, f.Description)
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Review the SPEC and respond with:\n")
	sb.WriteString("1. VERDICT: PASS, REVISE, or REJECT\n")
	if checklistIncluded {
		sb.WriteString("2. For each quality checklist item, write: CHECKLIST: <항목 ID> | PASS or CHECKLIST: <항목 ID> | FAIL | <reason>\n")
		sb.WriteString("3. For each issue found, write: FINDING: [severity] [category] [scope_ref] description\n")
		sb.WriteString("4. Provide reasoning for your verdict.\n")
	} else {
		sb.WriteString("2. For each issue found, write: FINDING: [severity] [category] [scope_ref] description\n")
		sb.WriteString("3. Provide reasoning for your verdict.\n")
	}
	sb.WriteString("   Severity levels: critical, major, minor, suggestion\n")
	sb.WriteString("   Category: correctness, completeness, feasibility, style, security\n")
}

func writeTrimNoticeInstruction(sb *strings.Builder) {
	sb.WriteString("If an injected excerpt contains a review-context notice about omitted lines, treat it as prompt-budget metadata, not as a source-document defect or finding evidence.\n")
}

// writeVerdictRules writes the canonical verdict rules the judge and the
// runtime blocker share. A configured pass_criteria is appended as additional
// project criteria: it can add requirements but never replace the rules the
// runtime enforces anyway (SPEC-REVIEWPOLICY-001 REQ-RP-POL-03, POL-06).
func writeVerdictRules(sb *strings.Builder, passCriteria string) {
	sb.WriteString("Apply the following rules to determine your VERDICT:\n")
	for _, line := range reviewVerdictRuleLines {
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	if strings.TrimSpace(passCriteria) != "" {
		sb.WriteString("\nAdditional project criteria (they cannot relax the rules above):\n")
		sb.WriteString(passCriteria)
		sb.WriteString("\n")
	}
}

// writeFindingExamples writes positive and negative few-shot examples for the FINDING format.
func writeFindingExamples(sb *strings.Builder) {
	sb.WriteString("Use the structured FINDING format exactly as shown:\n\n")
	sb.WriteString("GOOD (structured format — use this):\n")
	sb.WriteString("  FINDING: [major] [correctness] pkg/foo/bar.go:42 The retry logic does not handle timeout errors.\n")
	sb.WriteString("  FINDING: [critical] [security] pkg/auth/handler.go:88 JWT secret is logged in plaintext.\n\n")
	sb.WriteString("AVOID (legacy format — do NOT use this):\n")
	sb.WriteString("  FINDING: [major] The retry logic is broken.\n")
}

// Context collection functions are in context_collect.go.
