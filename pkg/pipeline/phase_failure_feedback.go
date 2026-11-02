package pipeline

import (
	"fmt"
	"strings"

	"github.com/insajin/autopus-adk/pkg/promptlayer"
)

// remediationFeedbackLayerID names the manifest entry a fix dispatch's
// feedback renders as (SPEC-PIPELINE-RESUME-001 REQ-PRR-FAIL-10).
const remediationFeedbackLayerID = "phase:remediation-feedback"

// renderRemediationFeedback is the prompt section a fix dispatch receives.
func renderRemediationFeedback(f *PhaseFailure) string {
	var b strings.Builder
	b.WriteString("## Remediation Feedback\n")
	fmt.Fprintf(&b, "The %s phase failed (%s, attempt %d, fingerprint %s): %s\n",
		f.PhaseID, f.Kind, f.Attempt, f.Fingerprint, f.Summary)
	if f.Kind == FailureReviewRejection {
		b.WriteString("Resolve every finding below. Validation and review run again on the changed code; their earlier results are not reused.\n")
	} else {
		fmt.Fprintf(&b, "Fix the cause below. The %s phase runs again on the changed code; its earlier result is not reused.\n", f.PhaseID)
	}
	for _, item := range f.Evidence {
		if item.ID != "" {
			fmt.Fprintf(&b, "- Finding %s: %s\n", item.ID, item.Detail)
			if item.Fix != "" {
				fmt.Fprintf(&b, "  Fix condition: %s\n", item.Fix)
			}
			continue
		}
		if item.Command != "" {
			fmt.Fprintf(&b, "- Failing command: %s\n", item.Command)
		} else {
			b.WriteString("- Failing check\n")
		}
		if item.Expected != "" {
			fmt.Fprintf(&b, "  Expected: %s\n", item.Expected)
		}
		if item.Actual != "" {
			fmt.Fprintf(&b, "  Actual: %s\n", item.Actual)
		}
	}
	if f.EvidenceOmitted > 0 {
		fmt.Fprintf(&b, "- %d more not shown: only the first %d items are carried; the fingerprint covers all of them.\n",
			f.EvidenceOmitted, failureEvidenceMaxItems)
	}
	if f.Excerpt != "" {
		b.WriteString("Log excerpt (secrets redacted):\n")
		b.WriteString(f.Excerpt)
	}
	return strings.TrimRight(b.String(), "\n")
}

// appendRemediationFeedback adds the feedback to a prompt that no SPEC
// directory frames, which has no layer manifest to carry it.
func appendRemediationFeedback(prompt string, feedback *PhaseFailure) string {
	if feedback == nil {
		return prompt
	}
	return prompt + "\n\n" + renderRemediationFeedback(feedback)
}

// remediationFeedbackLayer carries the feedback as an ephemeral task-context
// layer, so the manifest names it and its hash changes with the failure.
func remediationFeedbackLayer(feedback *PhaseFailure) promptlayer.Layer {
	sanitized := sanitizePromptContent(renderRemediationFeedback(feedback))
	return promptlayer.Layer{
		ID: remediationFeedbackLayerID, Kind: promptlayer.KindEphemeral,
		Group: promptlayer.GroupTaskContext, SourceRef: "remediation:" + feedback.Fingerprint,
		Content: sanitized.Content, CacheEligible: false,
		RedactionStatus: sanitized.RedactionStatus, InvalidationReason: sanitized.InvalidationReason,
	}
}
