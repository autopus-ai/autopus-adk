package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/orchestra"
)

type orchestraFailureReport struct {
	Timestamp        time.Time                          `json:"timestamp"`
	Command          string                             `json:"command"`
	Strategy         string                             `json:"strategy"`
	Providers        []string                           `json:"providers"`
	RunID            string                             `json:"run_id,omitempty"`
	Duration         string                             `json:"duration,omitempty"`
	Error            string                             `json:"error,omitempty"`
	Summary          string                             `json:"summary,omitempty"`
	EffectiveTimeout ResolvedOrchestraTimeout           `json:"effective_timeout"`
	FailedProviders  []orchestraFailureProvider         `json:"failed_providers,omitempty"`
	RetryHints       []string                           `json:"retry_hints,omitempty"`
	RunReceipt       *orchestra.OrchestrationRunReceipt `json:"run_receipt,omitempty"` // typed receipt incl. workspace evidence and provider provenance
}

type orchestraFailureProvider struct {
	Name             string `json:"name"`
	FailureClass     string `json:"failure_class,omitempty"`
	Error            string `json:"error"`
	NextRemediation  string `json:"next_remediation,omitempty"`
	StderrPreview    string `json:"stderr_preview,omitempty"`
	OutputPreview    string `json:"output_preview,omitempty"`
	CorrelationRunID string `json:"correlation_run_id,omitempty"`
}

func saveOrchestraFailureReport(command, strategy string, providers []string, timeout ResolvedOrchestraTimeout, result *orchestra.OrchestraResult, runErr error) (string, error) {
	return saveOrchestraDiagnosticsReport("failed", command, strategy, providers, timeout, result, runErr)
}

func saveOrchestraDegradedReport(command, strategy string, providers []string, timeout ResolvedOrchestraTimeout, result *orchestra.OrchestraResult) (string, error) {
	return saveOrchestraDiagnosticsReport("degraded", command, strategy, providers, timeout, result, nil)
}

// reportOrchestraFailure persists the diagnostics artifact for a failed run,
// prints the summary, and returns the surfaced error.
func reportOrchestraFailure(command, strategy string, providers []string, timeout ResolvedOrchestraTimeout, result *orchestra.OrchestraResult, runErr error, noPersist bool) error {
	if result != nil && !noPersist {
		reportPath, reportErr := saveOrchestraFailureReport(command, strategy, providers, timeout, result, runErr)
		if reportErr != nil {
			fmt.Fprintf(os.Stderr, "실패 보고서 저장 실패: %v\n", reportErr)
		}
		fmt.Fprint(os.Stderr, renderOrchestraFailureSummary(timeout, result, reportPath))
	}
	return fmt.Errorf("오케스트레이션 실패: %w", runErr)
}

func saveOrchestraDiagnosticsReport(prefix, command, strategy string, providers []string, timeout ResolvedOrchestraTimeout, result *orchestra.OrchestraResult, runErr error) (string, error) {
	dir := ".autopus/orchestra"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	report := orchestraFailureReport{
		Timestamp:        time.Now().UTC(),
		Command:          command,
		Strategy:         strategy,
		Providers:        append([]string(nil), providers...),
		EffectiveTimeout: timeout,
	}
	if runErr != nil {
		report.Error = runErr.Error()
	}
	if result != nil {
		report.RunID = result.RunID
		report.Duration = result.Duration.Round(time.Millisecond).String()
		report.Summary = result.Summary
		for _, fp := range result.FailedProviders {
			report.FailedProviders = append(report.FailedProviders, orchestraFailureProvider{
				Name:             fp.Name,
				FailureClass:     fp.FailureClass,
				Error:            fp.Error,
				NextRemediation:  fp.NextRemediation,
				StderrPreview:    fp.StderrPreview,
				OutputPreview:    fp.OutputPreview,
				CorrelationRunID: fp.CorrelationRunID,
			})
		}
		report.RetryHints = collectRetryHints(result.FailedProviders)
		report.RunReceipt = result.RunReceipt
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}

	ts := time.Now().Format("20060102-150405")
	filename := filepath.Join(dir, fmt.Sprintf("%s-%s-%s-%s.json", prefix, command, strategy, ts))
	if err := os.WriteFile(filename, data, 0o644); err != nil {
		return "", err
	}
	return filepath.Abs(filename)
}

func renderOrchestraFailureSummary(timeout ResolvedOrchestraTimeout, result *orchestra.OrchestraResult, reportPath string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "오케스트레이션 진단:\n")
	fmt.Fprintf(&sb, "- effective timeout: %ds (%s)\n", timeout.Seconds, timeout.Source)
	for _, detail := range timeout.Providers {
		fmt.Fprintf(&sb, "- provider timeout %s: %s (%s)\n", detail.Provider, detail.Duration, detail.Source)
	}
	if result != nil {
		if result.Workspace != nil && result.Workspace.MutationDetected {
			fmt.Fprintf(&sb, "- workspace mutation (no rollback): %s\n", strings.Join(result.Workspace.ChangedFiles, ", "))
		}
		for _, fp := range result.FailedProviders {
			fmt.Fprintf(&sb, "- failure %s [%s]: %s\n", fp.Name, fp.FailureClass, fp.Error)
			if fp.NextRemediation != "" {
				fmt.Fprintf(&sb, "  next: %s\n", fp.NextRemediation)
			}
			if fp.StderrPreview != "" {
				fmt.Fprintf(&sb, "  stderr: %s\n", fp.StderrPreview)
			}
		}
		for _, hint := range collectRetryHints(result.FailedProviders) {
			fmt.Fprintf(&sb, "- hint: %s\n", hint)
		}
	}
	if reportPath != "" {
		fmt.Fprintf(&sb, "- diagnostics report: %s\n", reportPath)
	}
	return sb.String()
}

func shouldTreatOrchestraResultAsFailure(result *orchestra.OrchestraResult) bool {
	if result == nil {
		return false
	}
	if result.TerminalState == orchestra.TerminalBlocked || result.GateStatus == "blocked" || result.JudgeStatus == orchestra.JudgeFailed {
		return true
	}
	if len(result.FailedProviders) == 0 {
		return false
	}
	if len(result.Responses) == 0 {
		return true
	}
	failed := make(map[string]struct{}, len(result.FailedProviders))
	for _, fp := range result.FailedProviders {
		failed[fp.Name] = struct{}{}
	}
	for _, resp := range result.Responses {
		if _, ok := failed[resp.Provider]; ok {
			continue
		}
		if resp.TimedOut || resp.EmptyOutput || resp.ExitCode != 0 {
			continue
		}
		return false
	}
	return true
}

func synthesizeOrchestraFailureError(result *orchestra.OrchestraResult) error {
	if result == nil {
		return fmt.Errorf("모든 프로바이더가 실패했습니다")
	}
	if result.Veto {
		return fmt.Errorf("해결되지 않은 Critical finding이 gate를 차단했습니다")
	}
	if result.JudgeStatus == orchestra.JudgeFailed {
		return fmt.Errorf("필수 judge가 실패했습니다: %s", summarizeFailedProviders(result.FailedProviders))
	}
	if containsDegradedReason(result.DegradedReasons, "provider_quorum") {
		return fmt.Errorf("프로바이더 quorum 미충족: %s", result.QuorumSummary())
	}
	if result.TerminalState == orchestra.TerminalBlocked && len(result.FailedProviders) == 0 {
		return fmt.Errorf("오케스트레이션 gate가 차단되었습니다")
	}
	return fmt.Errorf("모든 프로바이더가 실패했습니다: %s", summarizeFailedProviders(result.FailedProviders))
}

func containsDegradedReason(reasons []string, target string) bool {
	for _, reason := range reasons {
		if reason == target {
			return true
		}
	}
	return false
}

func summarizeFailedProviders(failed []orchestra.FailedProvider) string {
	details := make([]string, 0, len(failed))
	for _, fp := range failed {
		if fp.FailureClass != "" {
			details = append(details, fmt.Sprintf("%s(%s)", fp.Name, fp.FailureClass))
			continue
		}
		details = append(details, fmt.Sprintf("%s(%s)", fp.Name, fp.Error))
	}
	return strings.Join(details, ", ")
}

func collectRetryHints(failed []orchestra.FailedProvider) []string {
	seen := make(map[string]struct{}, len(failed))
	hints := make([]string, 0, len(failed))
	for _, fp := range failed {
		hint := strings.TrimSpace(fp.NextRemediation)
		if hint == "" {
			continue
		}
		if _, ok := seen[hint]; ok {
			continue
		}
		seen[hint] = struct{}{}
		hints = append(hints, hint)
	}
	return hints
}
