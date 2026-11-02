package worker

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/insajin/autopus-adk/pkg/worker/a2a"
	"github.com/insajin/autopus-adk/pkg/worker/adapter"
)

type taskRunMeta struct {
	TraceID       string
	CorrelationID string
	// Postconditions is the completion plan handleTask built for the task
	// (SPEC-WORKERPOST-001); nil falls back to inference from the prompt.
	Postconditions *taskPostconditions
}

func newTaskRunMeta(taskID, correlationID string) taskRunMeta {
	correlationID = strings.TrimSpace(correlationID)
	return taskRunMeta{
		TraceID:       defaultTraceID(taskID, correlationID),
		CorrelationID: correlationID,
	}
}

func defaultTraceID(taskID, correlationID string) string {
	if correlationID != "" {
		return correlationID
	}
	return taskID
}

func resolveTaskTraceID(taskID string, pending a2a.ApprovalRequestParams, meta taskRunMeta) string {
	if traceID := strings.TrimSpace(pending.TraceID); traceID != "" {
		return traceID
	}
	if meta.TraceID != "" {
		return meta.TraceID
	}
	return taskID
}

func buildExecutionContextSnapshot(
	cfg LoopConfig,
	requestedWorkDir string,
	activeWorkDir string,
	worktreePath string,
) *HostExecutionContext {
	rootWorkDir := strings.TrimSpace(requestedWorkDir)
	activeWorkDir = strings.TrimSpace(activeWorkDir)
	worktreePath = strings.TrimSpace(worktreePath)
	if activeWorkDir == "" {
		activeWorkDir = rootWorkDir
	}

	context := &HostExecutionContext{
		WorkspaceID:   strings.TrimSpace(cfg.WorkspaceID),
		RootWorkDir:   cleanPath(rootWorkDir),
		ActiveWorkDir: cleanPath(activeWorkDir),
		Mode:          "workspace",
		BoundaryHint:  "Desktop is observing the retained worker workspace rather than acting as a second filesystem executor.",
	}
	if worktreePath != "" {
		context.Mode = "temporary_worktree"
		context.WorktreePath = cleanPath(worktreePath)
		context.BoundaryHint = "Task execution moved into a temporary git worktree so local root-workspace edits stay isolated."
	}
	return context
}

func describeExecutionContext(context *HostExecutionContext) string {
	if context == nil {
		return "task execution context is not available yet"
	}
	if context.Mode == "temporary_worktree" && context.WorktreePath != "" {
		return fmt.Sprintf("task running inside temporary worktree %s", context.WorktreePath)
	}
	if context.ActiveWorkDir != "" {
		return fmt.Sprintf("task running in retained workspace %s", context.ActiveWorkDir)
	}
	return "task running in the retained workspace"
}

func buildHostResult(status, summary, errorMessage string, result adapter.TaskResult) *HostResult {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		summary = firstNonEmpty(
			previewText(result.Output),
			artifactSummary(result.Artifacts),
		)
	}
	if summary == "" {
		if status == "failed" {
			summary = "Task failed before returning a retained result."
		} else {
			summary = "Task completed and retained a desktop-visible result summary."
		}
	}

	hostResult := &HostResult{
		Status:       status,
		Summary:      summary,
		ErrorMessage: strings.TrimSpace(errorMessage),
		CostLabel:    formatCostLabel(result.CostUSD),
		DurationMS:   result.DurationMS,
		SessionID:    strings.TrimSpace(result.SessionID),
		Artifacts:    summarizeArtifacts(result.Artifacts),
	}
	return hostResult
}

func summarizeArtifacts(src []adapter.Artifact) []HostArtifact {
	if len(src) == 0 {
		return nil
	}
	artifacts := make([]HostArtifact, 0, len(src))
	for _, artifact := range src {
		artifacts = append(artifacts, HostArtifact{
			Name:     strings.TrimSpace(artifact.Name),
			MimeType: strings.TrimSpace(artifact.MimeType),
			Preview:  previewArtifact(artifact),
			Source:   "worker_result",
		})
	}
	return artifacts
}

func artifactSummary(src []adapter.Artifact) string {
	for _, artifact := range src {
		if preview := previewArtifact(artifact); preview != "" {
			return preview
		}
	}
	return ""
}

func previewArtifact(artifact adapter.Artifact) string {
	if !isTextualArtifact(artifact) {
		return ""
	}
	return previewText(artifact.Data)
}

func isTextualArtifact(artifact adapter.Artifact) bool {
	mime := strings.ToLower(strings.TrimSpace(artifact.MimeType))
	if mime == "" || strings.HasPrefix(mime, "text/") || mime == "application/json" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(artifact.Name), "output")
}

func previewText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= 160 {
		return value
	}
	return value[:157] + "..."
}

func formatCostLabel(cost float64) string {
	if cost <= 0 {
		return ""
	}
	return fmt.Sprintf("$%.4f", cost)
}

func cleanPath(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return filepath.Clean(value)
}

func worktreePath(activeWorkDir, requestedWorkDir string) string {
	activeWorkDir = cleanPath(activeWorkDir)
	requestedWorkDir = cleanPath(requestedWorkDir)
	if activeWorkDir == "" || activeWorkDir == requestedWorkDir {
		return ""
	}
	return activeWorkDir
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
