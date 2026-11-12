package worker

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/insajin/autopus-adk/pkg/worker/a2a"
	"github.com/insajin/autopus-adk/pkg/worker/adapter"
	"github.com/insajin/autopus-adk/pkg/worker/controlplane"
	"github.com/insajin/autopus-adk/pkg/worker/parallel"
	"github.com/insajin/autopus-adk/pkg/worker/pidlock"
	"github.com/insajin/autopus-adk/pkg/worker/security"
	"github.com/insajin/autopus-adk/pkg/worker/tui"
)

func (wl *WorkerLoop) configureExecutionConcurrency() {
	concurrencyLimit := wl.config.MaxConcurrency
	if concurrencyLimit <= 0 {
		concurrencyLimit = parallel.DefaultWorktreeSlotCap
	} else if concurrencyLimit == 1 {
		concurrencyLimit = 1
	}
	wl.semaphore = parallel.NewTaskSemaphore(concurrencyLimit)
	if wl.config.WorktreeIsolation {
		wl.worktreeManager = parallel.NewWorktreeManager(wl.config.WorkDir)
	}
}

// Start connects to the backend and begins processing tasks.
// @AX:ANCHOR[AUTO]: public lifecycle entry point — Start/Close are the primary WorkerLoop API; callers (CLI, tests) depend on error contract
// @AX:REASON: Startup order wires PID lock, A2A server, services, semaphore, and worktree manager before task dispatch.
func (wl *WorkerLoop) Start(ctx context.Context) error {
	// REQ-004: refuse to connect to the a2a broker unless server-signed
	// control-plane/policy metadata can be trusted, or the operator has
	// explicitly opted into unsigned mode via AllowUnsignedControlPlaneEnv.
	if err := controlplane.EnforceSignedControlPlane(); err != nil {
		return err
	}

	wl.pidLock = pidlock.New(pidlock.DefaultPath())
	if err := wl.pidLock.Acquire(); err != nil {
		return fmt.Errorf("acquire PID lock: %w", err)
	}

	log.Printf("[worker] starting loop: provider=%s backend=%s", wl.config.Provider.Name(), wl.config.BackendURL)
	if err := wl.server.Start(ctx); err != nil {
		if releaseErr := wl.pidLock.Release(); releaseErr != nil {
			log.Printf("[worker] PID lock release failed on start error: %v", releaseErr)
		}
		return err
	}
	wl.startServices(ctx)
	wl.configureExecutionConcurrency()

	return nil
}

// Close shuts down the worker loop and its A2A server.
func (wl *WorkerLoop) Close() error {
	wl.stopServices()
	if wl.pidLock != nil {
		if err := wl.pidLock.Release(); err != nil {
			log.Printf("[worker] PID lock release failed: %v", err)
		}
	}
	return wl.server.Close()
}

// cleanupPolicy removes the cached SecurityPolicy file for the given task.
func cleanupPolicy(taskID string) {
	path := security.NewPolicyCache().PolicyPath(taskID)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		log.Printf("[worker] cleanup policy file: %v", err)
	}
}

// SetTUIProgram registers the bubbletea program for sending approval messages.
func (wl *WorkerLoop) SetTUIProgram(p *tea.Program) {
	wl.tuiProgram = p
	if p != nil {
		wl.AddHostObserver(newTUIObserver(p))
	}
}

func newTUIObserver(p *tea.Program) HostObserver {
	return HostObserverFunc(func(event HostEvent) {
		switch event.Type {
		case HostEventApprovalRequested:
			p.Send(tui.ApprovalRequestMsg{
				TaskID:    event.TaskID,
				Action:    event.Action,
				RiskLevel: event.RiskLevel,
				Context:   event.Context,
			})
		case HostEventTaskProgress:
			p.Send(tui.TaskProgressMsg{Phase: event.Phase})
		}
	})
}

// handleApproval forwards an approval request from A2A to the host observer bridge.
func (wl *WorkerLoop) handleApproval(params a2a.ApprovalRequestParams) {
	wl.storePendingApproval(params)
	wl.emitHostEvent(HostEvent{
		Type:       HostEventApprovalRequested,
		TaskID:     params.TaskID,
		ApprovalID: params.ApprovalID,
		TraceID:    params.TraceID,
		Action:     params.Action,
		RiskLevel:  params.RiskLevel,
		Context:    params.Context,
	})
	if !wl.hasHostObservers() {
		log.Printf("[worker] approval request but no host observer registered")
		return
	}
}

func (wl *WorkerLoop) handleDispatchIssue(issue a2a.DispatchIssue) {
	stage := strings.ReplaceAll(issue.Stage, "_", " ")
	wl.emitHostEvent(HostEvent{
		Type:    HostEventRuntimeDegraded,
		TaskID:  issue.TaskID,
		Phase:   issue.Stage,
		Message: fmt.Sprintf("platform reconciliation degraded during %s: %s", stage, issue.Message),
	})
}

// SetOnApprovalDecision returns a callback that sends approval decisions to the backend.
func (wl *WorkerLoop) SetOnApprovalDecision() func(taskID, decision string) {
	return func(taskID, decision string) {
		pending, _ := wl.pendingApproval(taskID)
		if err := wl.server.SendApprovalResponse(a2a.ApprovalResponseParams{
			TaskID:     taskID,
			ApprovalID: pending.ApprovalID,
			TraceID:    pending.TraceID,
			Decision:   decision,
		}); err != nil {
			log.Printf("[worker] send approval response error: %v", err)
			return
		}
		wl.clearPendingApproval(taskID)
		wl.emitHostEvent(HostEvent{
			Type:       HostEventApprovalResolved,
			TaskID:     taskID,
			ApprovalID: pending.ApprovalID,
			TraceID:    pending.TraceID,
			Message:    decision,
		})
	}
}

// convertArtifacts converts adapter artifacts to A2A artifacts.
func convertArtifacts(src []adapter.Artifact) []a2a.Artifact {
	if len(src) == 0 {
		return nil
	}
	out := make([]a2a.Artifact, len(src))
	for i, artifact := range src {
		out[i] = a2a.Artifact{
			Name:     artifact.Name,
			MimeType: artifact.MimeType,
			Data:     artifact.Data,
		}
	}
	return out
}

func ensureOutputArtifact(output string, artifacts []adapter.Artifact) []adapter.Artifact {
	if strings.TrimSpace(output) == "" {
		return artifacts
	}
	for _, artifact := range artifacts {
		if artifact.Name == "output" {
			return artifacts
		}
	}
	return append([]adapter.Artifact{{
		Name:     "output",
		MimeType: "text/plain",
		Data:     output,
	}}, artifacts...)
}
