package loop

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/qa/triage"
)

// Report is report.json, schema qamesh.qaloop.report.v1.
type Report struct {
	Schema string `json:"schema"`
	RunID  string `json:"run_id"`
	Branch string `json:"branch"`
	// BranchDeleted is set when the loop made no fix commit: an empty branch
	// carries nothing to review, so it is removed instead of left behind.
	BranchDeleted bool `json:"branch_deleted,omitempty"`
	// SeedCommit is the commit that added the QA files `auto qa go` generated.
	SeedCommit    string      `json:"seed_commit,omitempty"`
	OriginalRef   string      `json:"original_ref"`
	Restored      bool        `json:"original_ref_restored"`
	Lane          string      `json:"lane"`
	Agent         string      `json:"agent"`
	MaxIterations int         `json:"max_iterations"`
	StopReason    StopReason  `json:"stop_reason"`
	StopCode      string      `json:"stop_code,omitempty"`
	StopDetail    string      `json:"stop_detail,omitempty"`
	FinalStatus   string      `json:"final_status"`
	Quarantined   []string    `json:"quarantined,omitempty"`
	Iterations    []Iteration `json:"iterations"`
	StartedAt     time.Time   `json:"started_at"`
	FinishedAt    time.Time   `json:"finished_at"`
	ReportPath    string      `json:"report_path"`
	ReportMDPath  string      `json:"report_md_path"`
}

// Iteration records one lane run, its triage, and the fix attempt if any.
type Iteration struct {
	N            int              `json:"n"`
	QARunID      string           `json:"qa_run_id,omitempty"`
	RunStatus    string           `json:"run_status,omitempty"`
	Failures     []triage.Verdict `json:"failures"`
	Agent        *AgentRecord     `json:"agent,omitempty"`
	ChangedPaths []string         `json:"changed_paths,omitempty"`
	Guard        *GuardVerdict    `json:"guard,omitempty"`
	Commit       string           `json:"commit,omitempty"`
}

// AgentRecord is one repair agent call.
type AgentRecord struct {
	Class    triage.Class `json:"class"`
	Journeys []string     `json:"journeys"`
	ExitCode int          `json:"exit_code"`
	Duration string       `json:"duration"`
	Error    string       `json:"error,omitempty"`
}

func finalStatus(reason StopReason) string {
	switch reason {
	case StopPassed, StopPassedWithFlaky:
		return "passed"
	case StopBlockedEnv:
		return "blocked"
	}
	return "failed"
}

func writeReports(dir string, report Report) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "report.json"), append(body, '\n'), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "report.md"), []byte(renderMarkdown(report)), 0o644)
}

func renderMarkdown(report Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# QA loop %s\n\n", report.RunID)
	fmt.Fprintf(&b, "- Stop reason: `%s`", report.StopReason)
	if report.StopCode != "" {
		fmt.Fprintf(&b, " (`%s`)", report.StopCode)
	}
	b.WriteString("\n")
	if report.StopDetail != "" {
		fmt.Fprintf(&b, "- Detail: %s\n", oneLine(report.StopDetail))
	}
	fmt.Fprintf(&b, "- Final status: %s\n", report.FinalStatus)
	fmt.Fprintf(&b, "- Loop branch: `%s`\n", report.Branch)
	fmt.Fprintf(&b, "- Original ref: `%s` (restored: %t)\n", report.OriginalRef, report.Restored)
	fmt.Fprintf(&b, "- Lane: %s, agent: %s, max iterations: %d\n", report.Lane, report.Agent, report.MaxIterations)
	if len(report.Quarantined) > 0 {
		fmt.Fprintf(&b, "- Quarantined flaky journeys: %s\n", strings.Join(report.Quarantined, ", "))
	}
	for _, it := range report.Iterations {
		writeIterationMarkdown(&b, it)
	}
	return b.String()
}

func writeIterationMarkdown(b *strings.Builder, it Iteration) {
	fmt.Fprintf(b, "\n## Iteration %d\n\n", it.N)
	fmt.Fprintf(b, "- Run status: %s\n", firstNonEmpty(it.RunStatus, "unknown"))
	if len(it.Failures) == 0 {
		b.WriteString("- Failures: none\n")
	}
	for _, v := range it.Failures {
		fmt.Fprintf(b, "- Failure `%s`: %s (%s)\n", v.JourneyID, v.Class, oneLine(v.Signal))
	}
	if it.Agent != nil {
		fmt.Fprintf(b, "- Agent: %s for %s, exit %d, %s\n", it.Agent.Class, strings.Join(it.Agent.Journeys, ", "), it.Agent.ExitCode, it.Agent.Duration)
		if it.Agent.Error != "" {
			fmt.Fprintf(b, "- Agent error: %s\n", oneLine(it.Agent.Error))
		}
	}
	if len(it.ChangedPaths) > 0 {
		fmt.Fprintf(b, "- Changed paths: %s\n", strings.Join(it.ChangedPaths, ", "))
	}
	if it.Guard != nil {
		verdict := "accepted"
		if !it.Guard.Accepted {
			verdict = "rejected: " + strings.TrimPrefix(it.Guard.Path+": "+it.Guard.Reason, ": ")
		}
		fmt.Fprintf(b, "- Guard: %s\n", verdict)
	}
	if it.Commit != "" {
		fmt.Fprintf(b, "- Commit: `%s`\n", it.Commit)
	}
}
