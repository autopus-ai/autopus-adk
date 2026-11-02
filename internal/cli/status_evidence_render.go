package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/insajin/autopus-adk/internal/cli/tui"
)

// statusReport is everything status shows beyond the declared SPEC states.
// Text and JSON render the same report, so they carry the same meaning
// (REQ-SEV-JSON-01).
type statusReport struct {
	evidence []statusEvidence
	env      statusEnvironment
	actions  []statusNextAction
}

func collectStatusReport(dir string, specs []specEntry, probe statusProbe) statusReport {
	env := collectStatusEnvironment(dir, probe)
	reader := newStatusEvidenceReader(dir, specs, env)
	evidence := make([]statusEvidence, len(specs))
	for i, entry := range specs {
		evidence[i] = reader.read(entry)
	}
	return statusReport{evidence: evidence, env: env, actions: buildStatusNextActions(specs, evidence, env)}
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
			lines = append(lines, "run: "+execution.Error+" · "+execution.Source)
		} else {
			lines = append(lines, statusExecutionLines(execution)...)
		}
		if len(execution.OtherRoots) > 0 {
			lines = append(lines, "older evidence also in: "+statusSingleLine(strings.Join(execution.OtherRoots, ", ")))
		}
	}
	if verification := evidence.Verification; verification.State != freshnessNone {
		line := "verification: " + verification.State
		if verification.RunID != "" {
			line += fmt.Sprintf(" (run %s, base HEAD %s, recorded %s)", verification.RunID,
				statusOr(shortCommit(verification.BaseHead), "none"), statusOr(verification.RecordedAt, "never"))
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

// statusExecutionLines shows every execution field the JSON carries.
func statusExecutionLines(execution *statusExecution) []string {
	attempt := fmt.Sprintf("attempt %d", execution.Attempt)
	if execution.ResumedFrom != "" {
		attempt += ", resumed from " + execution.ResumedFrom
	}
	platform := statusOr(execution.Platform, "unrecorded")
	if execution.ExecutionOwner != "" {
		platform += " (owner " + execution.ExecutionOwner + ")"
	}
	result := execution.Terminal
	if result == "" {
		result = "no terminal state (still running, killed, or recorded without a run receipt)"
	}
	if execution.StopReason != "" && execution.StopReason != execution.Terminal {
		result += " (" + execution.StopReason + ")"
	}
	phases := execution.Phases
	lines := []string{
		fmt.Sprintf("run: %s · task %s · %s · route %s · platform %s", statusOr(execution.RunID, "unrecorded"),
			statusOr(execution.TaskID, "unrecorded"), attempt, statusOr(execution.Route, "unrecorded"), platform),
		fmt.Sprintf("result: %s · finished %s · base HEAD %s · %s", result, statusOr(execution.FinishedAt, "not recorded"),
			statusOr(shortCommit(execution.BaseHead), "none"), execution.Source),
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
	if resume := execution.Resume; resume != nil {
		lines = append(lines, statusResumeLines(resume)...)
	}
	return lines
}

func statusResumeLines(resume *statusResume) []string {
	lines := []string{"resumable: no — " + resume.Reason}
	if resume.Admitted {
		lines = []string{"resumable: yes — " + statusResumeSummary(resume)}
		command := resume.Command
		if command == "" {
			command = "none — " + resume.CommandNote
		}
		lines = append(lines, "resume command: "+command)
	}
	return append(lines, "admission checked: "+strings.Join(resume.Checked, ", "))
}

func statusReviewLines(review *statusReview) []string {
	var lines []string
	switch review.State {
	case freshnessNone:
		lines = append(lines, "review: no review receipt")
	default:
		verdict := statusOr(review.Verdict, "no verdict")
		var facts []string
		for _, fact := range []struct{ name, value string }{
			{"gate", review.GateStatus}, {"loop", review.LoopStatus}, {"run", review.RunID},
		} {
			if fact.value != "" {
				facts = append(facts, fact.name+" "+fact.value)
			}
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
	if data.NextActions == nil {
		data.NextActions = []statusNextAction{}
	}
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

func statusOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
