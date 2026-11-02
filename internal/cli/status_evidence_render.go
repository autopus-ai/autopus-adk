package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/insajin/autopus-adk/internal/cli/tui"
	"github.com/insajin/autopus-adk/pkg/pipeline"
)

// statusReport is everything status shows beyond the declared SPEC states.
// Text and JSON render the same report, so they carry the same meaning
// (REQ-SEV-JSON-01).
type statusReport struct {
	evidence []statusEvidence
	env      statusEnvironment
	actions  []statusNextAction
}

func collectStatusReport(dir string, specs []specEntry) statusReport {
	reader := newStatusEvidenceReader(dir, specs)
	evidence := make([]statusEvidence, len(specs))
	for i, entry := range specs {
		evidence[i] = reader.read(entry)
	}
	env := collectStatusEnvironment(dir, newStatusProbe())
	return statusReport{evidence: evidence, env: env, actions: buildStatusNextActions(dir, specs, evidence, env)}
}

// evidenceCounts counts the SPECs under every label, zeros included.
func (r statusReport) evidenceCounts() map[string]int {
	counts := make(map[string]int, len(evidenceLabelOrder))
	for _, label := range evidenceLabelOrder {
		counts[label] = 0
	}
	for _, evidence := range r.evidence {
		counts[evidence.Label]++
	}
	return counts
}

// evidenceSummaryText always names verified, then every other label a SPEC
// carries.
func (r statusReport) evidenceSummaryText() string {
	counts := r.evidenceCounts()
	parts := []string{fmt.Sprintf("%d %s", counts[evidenceVerified], evidenceVerified)}
	for _, label := range evidenceLabelOrder[1:] {
		if counts[label] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[label], label))
		}
	}
	return "Evidence: " + strings.Join(parts, " · ")
}

func renderStatusEvidenceDetails(out io.Writer, evidence statusEvidence) {
	for _, line := range statusEvidenceLines(evidence) {
		_, _ = fmt.Fprintf(out, "      %s\n", tui.MutedStyle.Render(line))
	}
}

func statusEvidenceLines(evidence statusEvidence) []string {
	var lines []string
	if execution := evidence.Execution; execution != nil {
		if execution.Error != "" {
			lines = append(lines, "run: "+execution.Error)
		} else {
			lines = append(lines, statusExecutionLines(execution)...)
		}
	}
	if verification := evidence.Verification; verification.State != freshnessNone {
		line := "verification: " + verification.State
		if verification.RunID != "" {
			line += " (run " + verification.RunID + ")"
		}
		if len(verification.Reasons) > 0 {
			line += " — " + strings.Join(verification.Reasons, "; ")
		}
		lines = append(lines, line)
	}
	if review := evidence.Review; review != nil {
		lines = append(lines, statusReviewLines(review)...)
	}
	return lines
}

func statusExecutionLines(execution *statusExecution) []string {
	terminal := execution.Terminal
	if terminal == "" {
		terminal = "no terminal state (still running, killed, or recorded without a run receipt)"
	}
	if execution.StopReason != "" && execution.StopReason != execution.Terminal {
		terminal += " (" + execution.StopReason + ")"
	}
	runID := execution.RunID
	if runID == "" {
		runID = "unrecorded"
	}
	if execution.Attempt > 0 {
		runID += fmt.Sprintf(" · attempt %d", execution.Attempt)
	}
	phases := execution.Phases
	lines := []string{
		fmt.Sprintf("run: %s · %s", runID, terminal),
		fmt.Sprintf("phases: %d executed · %d restored · %d failed · %d pending · %d dispatches (%d cumulative)",
			phases.Executed, phases.Restored, phases.Failed, phases.Pending,
			execution.Dispatches, execution.CumulativeDispatches),
	}
	if execution.Blocker != "" {
		lines = append(lines, "blocker: "+execution.Blocker)
	}
	if attempt := execution.BlockedAttempt; attempt != nil {
		lines = append(lines, "last attempt blocked before dispatch: "+attempt.Blocker)
	}
	if pipelineRunStopped(pipeline.TerminalState(execution.Terminal)) {
		if execution.Resumable {
			lines = append(lines, fmt.Sprintf("resumable: yes — %s (%s)", execution.ResumeCommand, execution.ResumeReason))
		} else {
			lines = append(lines, "resumable: no — "+execution.ResumeReason)
		}
	}
	return lines
}

func statusReviewLines(review *statusReview) []string {
	var lines []string
	switch review.State {
	case freshnessUnknown:
		lines = append(lines, "review: unknown — "+strings.Join(review.Reasons, "; "))
	case freshnessNone:
		lines = append(lines, "review: no review receipt")
	default:
		verdict := review.Verdict
		if verdict == "" {
			verdict = "no verdict"
		}
		var facts []string
		if review.GateStatus != "" {
			facts = append(facts, "gate "+review.GateStatus)
		}
		if review.LoopStatus != "" {
			facts = append(facts, "loop "+review.LoopStatus)
		}
		line := "review: " + verdict
		if len(facts) > 0 {
			line += " (" + strings.Join(facts, ", ") + ")"
		}
		if review.FinishedAt != "" {
			line += " at " + review.FinishedAt
		}
		line += " — " + review.State
		if len(review.Reasons) > 0 {
			line += ": " + strings.Join(review.Reasons, "; ")
		}
		lines = append(lines, line)
	}
	if session := review.Session; session != nil {
		if session.State == freshnessUnknown {
			lines = append(lines, "review session: unknown — "+session.Reason)
		} else {
			lines = append(lines, fmt.Sprintf("review session: %s · %d attempts · %d calls",
				session.State, session.Attempts, session.Calls))
		}
	}
	return lines
}

// applyStatusEvidenceJSON carries the report into the JSON payload, and turns
// a stopped run or stale verification into a warning finding.
func applyStatusEvidenceJSON(
	data *statusJSONData,
	report statusReport,
	warnings *[]jsonMessage,
	status *jsonEnvelopeStatus,
) {
	for i := range data.Specs {
		if i >= len(report.evidence) {
			break
		}
		evidence := report.evidence[i]
		data.Specs[i].Evidence = &evidence
		finding, ok := statusEvidenceFinding(data.Specs[i], evidence)
		if !ok {
			continue
		}
		data.Findings = append(data.Findings, finding)
		*warnings = append(*warnings, jsonMessage{Code: finding.Code, Message: finding.Message})
		*status = jsonStatusWarn
	}
	counts := report.evidenceCounts()
	data.Summary.Verified, data.Summary.Evidence = counts[evidenceVerified], counts
	env := report.env
	data.Environment, data.NextActions = &env, report.actions
}

func statusEvidenceFinding(spec statusJSONSpec, evidence statusEvidence) (statusJSONFinding, bool) {
	finding := statusJSONFinding{SpecID: spec.ID, Module: spec.Module, Severity: "warning"}
	switch evidence.Label {
	case evidenceFailed:
		finding.Code, finding.Status = "run_failed", evidenceFailed
		finding.Message = fmt.Sprintf("%s: the last pipeline run stopped (%s)", spec.DisplayID, evidence.Execution.StopReason)
	case evidenceStale:
		finding.Code, finding.Status = "verification_stale", evidenceStale
		finding.Message = fmt.Sprintf("%s: verification is stale (%s)", spec.DisplayID,
			strings.Join(evidence.Verification.Reasons, "; "))
	default:
		return statusJSONFinding{}, false
	}
	return finding, true
}
