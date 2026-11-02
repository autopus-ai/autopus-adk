package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

type statusJSONData struct {
	Summary     statusJSONSummary     `json:"summary"`
	Specs       []statusJSONSpec      `json:"specs,omitempty"`
	Findings    []statusJSONFinding   `json:"findings,omitempty"`
	Environment *statusEnvironment    `json:"environment,omitempty"`
	NextActions []statusNextAction    `json:"next_actions,omitempty"`
	Hygiene     *statusHygienePayload `json:"hygiene,omitempty"`
}

type statusJSONSummary struct {
	Total      int `json:"total"`
	Done       int `json:"done"`
	Approved   int `json:"approved"`
	InProgress int `json:"in_progress"`
	Draft      int `json:"draft"`
	Other      int `json:"other"`
	Open       int `json:"open"`
	// Done counts declared statuses; Verified counts only SPECs whose last
	// completed run still matches the code (SPEC-STATUSEVIDENCE-001).
	Verified int            `json:"verified"`
	Evidence map[string]int `json:"evidence,omitempty"`
}

type statusJSONSpec struct {
	ID        string `json:"id"`
	DisplayID string `json:"display_id,omitempty"`
	Module    string `json:"module,omitempty"`
	Title     string `json:"title,omitempty"`
	Status    string `json:"status"`
	Completed bool   `json:"completed"`
	Source    string `json:"source,omitempty"`
	// Evidence keeps execution, verification, and review apart from Status.
	Evidence *statusEvidence `json:"evidence,omitempty"`
}

type statusJSONFinding struct {
	Code     string `json:"code"`
	SpecID   string `json:"spec_id"`
	Module   string `json:"module,omitempty"`
	Severity string `json:"severity"`
	Status   string `json:"status"`
	Message  string `json:"message"`
}

func normalizeSpecStatus(status string) string {
	return strings.ToLower(strings.TrimSpace(strings.ReplaceAll(status, "_", "-")))
}

func runStatusJSON(cmd *cobra.Command, dir string) error {
	specs := scanAllSpecs(dir)
	data, warnings, checks, status := buildStatusJSONPayload(dir, specs)
	applyStatusEvidenceJSON(&data, collectStatusReport(dir, specs), &warnings, &status)
	hygiene := collectStatusHygiene(dir)
	payload := hygiene.payload()
	data.Hygiene = &payload
	applyHygieneJSON("status", hygiene, &warnings, &checks, &status)
	return writeJSONResult(cmd, status, data, warnings, checks)
}

func buildStatusJSONPayload(
	baseDir string,
	specs []specEntry,
) (statusJSONData, []jsonMessage, []jsonCheck, jsonEnvelopeStatus) {
	data := statusJSONData{
		Summary: statusJSONSummary{Total: len(specs)},
	}
	if len(specs) == 0 {
		return data,
			[]jsonMessage{{Code: "no_specs", Message: "No SPEC directories found."}},
			[]jsonCheck{{
				ID:       "specs.scan",
				Severity: "warning",
				Status:   "warn",
				Detail:   "No SPEC directories found.",
			}},
			jsonStatusWarn
	}

	checks := make([]jsonCheck, 0, len(specs))
	findings := make([]statusJSONFinding, 0)
	for _, entry := range specs {
		normalized := normalizeSpecStatus(entry.status)
		switch normalized {
		case "done", "completed", "implemented":
			data.Summary.Done++
		case "approved":
			data.Summary.Approved++
		case "in-progress":
			data.Summary.InProgress++
		case "draft":
			data.Summary.Draft++
		default:
			data.Summary.Other++
		}

		specPayload := statusJSONSpec{
			ID:        entry.id,
			DisplayID: entry.displayID(),
			Module:    entry.module,
			Title:     entry.title,
			Status:    normalized,
			Completed: entry.isDone(),
			Source:    relativeStatusPath(baseDir, entry.path),
		}
		data.Specs = append(data.Specs, specPayload)

		check := jsonCheck{
			ID:       statusCheckID(entry),
			Severity: "info",
			Status:   "pass",
			Detail:   fmt.Sprintf("%s: %s", entry.displayID(), normalized),
		}
		if !entry.isDone() {
			data.Summary.Open++
			check.Severity = "warning"
			check.Status = "warn"
			check.Detail = fmt.Sprintf("%s remains open (%s)", entry.displayID(), normalized)
			findings = append(findings, statusJSONFinding{
				Code:     "open_spec",
				SpecID:   entry.id,
				Module:   entry.module,
				Severity: "warning",
				Status:   "open",
				Message:  fmt.Sprintf("%s remains open (%s)", entry.displayID(), normalized),
			})
		}
		checks = append(checks, check)
	}

	data.Findings = findings
	warnings := buildStatusWarnings(data.Summary, findings)
	if data.Summary.Open > 0 {
		return data, warnings, checks, jsonStatusWarn
	}
	return data, warnings, checks, jsonStatusOK
}

func buildStatusWarnings(summary statusJSONSummary, findings []statusJSONFinding) []jsonMessage {
	if summary.Open == 0 {
		return nil
	}

	messages := []jsonMessage{{
		Code:    "open_specs",
		Message: fmt.Sprintf("%d SPEC(s) remain open.", summary.Open),
	}}
	if summary.Draft > 0 {
		messages = append(messages, jsonMessage{
			Code:    "draft_specs",
			Message: fmt.Sprintf("%d SPEC(s) are still draft.", summary.Draft),
		})
	}
	if len(findings) == 0 {
		return messages
	}
	return messages
}

func relativeStatusPath(baseDir, path string) string {
	if path == "" {
		return ""
	}
	rel, err := filepath.Rel(baseDir, path)
	if err != nil {
		return filepath.Base(path)
	}
	return filepath.ToSlash(rel)
}

func statusCheckID(entry specEntry) string {
	if entry.module == "" {
		return "spec." + entry.id
	}
	return fmt.Sprintf("spec.%s.%s", entry.module, entry.id)
}
