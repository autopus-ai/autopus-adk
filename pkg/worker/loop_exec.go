package worker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/worker/adapter"
	"github.com/insajin/autopus-adk/pkg/worker/budget"
	"github.com/insajin/autopus-adk/pkg/worker/security"
)

func (wl *WorkerLoop) detachedTaskContext(parent context.Context) (context.Context, context.CancelFunc) {
	base := context.Background()
	if parent != nil {
		base = context.WithoutCancel(parent)
	}
	ctx, cancel := context.WithCancel(base)

	if wl.lifecycleCtx != nil {
		go func() {
			select {
			case <-wl.lifecycleCtx.Done():
				cancel()
			case <-ctx.Done():
			}
		}()
	}
	if parent != nil {
		go func() {
			select {
			case <-parent.Done():
				if errors.Is(parent.Err(), context.Canceled) {
					cancel()
				}
			case <-ctx.Done():
			}
		}()
	}

	return ctx, cancel
}

func (wl *WorkerLoop) executionContext(parent context.Context, taskID string) (context.Context, context.CancelFunc) {
	baseCtx, baseCancel := wl.detachedTaskContext(parent)
	deadline, ok := wl.taskExecutionDeadline(taskID)
	if !ok {
		return baseCtx, baseCancel
	}

	execCtx, timeoutCancel := context.WithDeadline(baseCtx, deadline)
	return execCtx, func() {
		timeoutCancel()
		baseCancel()
	}
}

func (wl *WorkerLoop) taskExecutionTimeout(taskID string) time.Duration {
	if strings.TrimSpace(taskID) == "" {
		return 0
	}

	policy, err := security.NewPolicyCache().Read(taskID)
	if err != nil || policy == nil || policy.TimeoutSec <= 0 {
		return 0
	}
	return time.Duration(policy.TimeoutSec) * time.Second
}

// BudgetConfig holds optional budget configuration for subprocess execution.
type BudgetConfig struct {
	Budget        budget.IterationBudget
	EmergencyStop *security.EmergencyStop
}

// executeWithParallel wraps executeSubprocess with semaphore gating, worktree
// isolation, and audit event recording. It is the primary execution entry point
// called from handleTask.
func (wl *WorkerLoop) executeWithParallel(
	ctx context.Context,
	taskCfg adapter.TaskConfig,
	bc *BudgetConfig,
	meta taskRunMeta,
) (adapter.TaskResult, error) {
	taskID := taskCfg.TaskID
	startTime := time.Now()
	requestedWorkDir := taskCfg.WorkDir

	// Record task start in the audit log.
	if wl.auditWriter != nil {
		recordAuditEvent(wl.auditWriter, newAuditStartedEvent(taskID, taskCfg.ComputerUse), wl.auditLogger)
	}

	// Acquire a semaphore slot when parallel execution is configured.
	// This blocks until a slot is available or ctx is cancelled.
	if wl.semaphore != nil {
		acquireCtx, cancelAcquire := wl.executionContext(ctx, taskID)
		defer cancelAcquire()
		if err := wl.semaphore.Acquire(acquireCtx); err != nil {
			return adapter.TaskResult{}, fmt.Errorf("acquire semaphore: %w", err)
		}
		defer wl.semaphore.Release()
	}

	cleanupWorktree, err := wl.assignTaskWorktree(taskID, &taskCfg)
	if err != nil {
		return adapter.TaskResult{}, err
	}
	defer cleanupWorktree()
	if err := wl.resolveTaskRequiredContext(&taskCfg, taskCfg.WorkDir); err != nil {
		return adapter.TaskResult{}, fmt.Errorf("required context delivery: %w", err)
	}
	execution := buildExecutionContextSnapshot(
		wl.config,
		requestedWorkDir,
		taskCfg.WorkDir,
		worktreePath(taskCfg.WorkDir, requestedWorkDir),
	)
	wl.emitHostEvent(HostEvent{
		Type:          HostEventTaskProgress,
		TaskID:        taskID,
		TraceID:       meta.TraceID,
		CorrelationID: meta.CorrelationID,
		Phase:         "execution_context",
		Message:       describeExecutionContext(execution),
		Execution:     execution,
	})

	// The baseline is read from the directory the provider will run in, right
	// before it runs, so a worktree or a queue wait cannot shift it.
	plan := meta.postconditionPlan(taskCfg.Prompt)
	baseline := wl.postconditionGit.captureBaseline(taskCfg.WorkDir, plan)

	// Delegate to the core subprocess executor.
	execCtx, cancelExec := wl.executionContext(ctx, taskID)
	defer cancelExec()
	result, err := wl.executeWithBudget(execCtx, taskCfg, bc)
	durationMS := time.Since(startTime).Milliseconds()

	// Record completion or failure in the audit log.
	if err != nil {
		if wl.auditWriter != nil {
			recordAuditEvent(wl.auditWriter, newAuditFailedEvent(taskID, durationMS, taskCfg.ComputerUse), wl.auditLogger)
		}
		return result, err
	}
	artifact, verifyErr := wl.postconditionGit.verify(taskCfg.WorkDir, plan, baseline)
	if artifact.Name != "" {
		result.Artifacts = append(result.Artifacts, artifact)
	}
	if verifyErr != nil {
		if wl.auditWriter != nil {
			recordAuditEvent(wl.auditWriter, newAuditFailedEvent(taskID, durationMS, taskCfg.ComputerUse), wl.auditLogger)
		}
		return result, verifyErr
	}
	if wl.auditWriter != nil {
		recordAuditEvent(wl.auditWriter, newAuditCompletedEvent(taskID, durationMS, result.CostUSD, taskCfg.ComputerUse), wl.auditLogger)
	}

	return result, nil
}

func (wl *WorkerLoop) executePipelineWithParallel(
	ctx context.Context,
	taskCfg adapter.TaskConfig,
	phases []Phase,
	instructions map[Phase]string,
	promptTemplates map[Phase]string,
	bc *BudgetConfig,
	meta taskRunMeta,
) (adapter.TaskResult, error) {
	startTime := time.Now()
	taskID, prompt, model := taskCfg.TaskID, taskCfg.Prompt, taskCfg.Model

	if wl.auditWriter != nil {
		recordAuditEvent(wl.auditWriter, newAuditStartedEvent(taskID, false), wl.auditLogger)
	}

	if wl.semaphore != nil {
		acquireCtx, cancelAcquire := wl.executionContext(ctx, taskID)
		defer cancelAcquire()
		if err := wl.semaphore.Acquire(acquireCtx); err != nil {
			return adapter.TaskResult{}, fmt.Errorf("acquire semaphore: %w", err)
		}
		defer wl.semaphore.Release()
	}

	requestedWorkDir := wl.config.WorkDir
	workDir, envVars, cleanupWorktree, err := wl.assignPipelineWorktree(taskID, prompt)
	if err != nil {
		return adapter.TaskResult{}, err
	}
	defer cleanupWorktree()
	if err := wl.resolveTaskRequiredContext(&taskCfg, workDir); err != nil {
		return adapter.TaskResult{}, fmt.Errorf("required context delivery: %w", err)
	}
	execution := buildExecutionContextSnapshot(
		wl.config,
		requestedWorkDir,
		workDir,
		worktreePath(workDir, requestedWorkDir),
	)
	wl.emitHostEvent(HostEvent{
		Type:          HostEventTaskProgress,
		TaskID:        taskID,
		TraceID:       meta.TraceID,
		CorrelationID: meta.CorrelationID,
		Phase:         "execution_context",
		Message:       describeExecutionContext(execution),
		Execution:     execution,
	})

	pe := NewPipelineExecutor(wl.config.Provider, wl.config.MCPConfig, workDir)
	pe.SetRequiredContext(taskCfg.RequiredContext)
	pe.SetPersistentTask(taskCfg.PersistentTask)
	pe.SetUsageIdentity(taskCfg.RunID, taskCfg.Attempt, taskCfg.Effort, taskCfg.Role)
	pe.SetUsageProvenance(taskCfg.ProviderVersion, taskCfg.ModelVersion, taskCfg.RiskPolicy, taskCfg.CacheStratum, taskCfg.ConfigHash)
	plan := meta.postconditionPlan(prompt)
	baseline := wl.postconditionGit.captureBaseline(workDir, plan)
	pe.SetEnvVars(envVars)
	pe.SetInterruptRecorder(func(evt AuditEvent) {
		recordWorkerSafetyEvent(wl, evt)
	})
	pe.SetPhaseInstructions(instructions)
	pe.SetPhasePromptTemplates(promptTemplates)
	if bc != nil && bc.Budget.Limit > 0 {
		pe.SetIterationBudget(bc.Budget)
	}
	execCtx, cancelExec := wl.executionContext(ctx, taskID)
	defer cancelExec()
	result, err := pe.ExecuteWithPlan(execCtx, taskID, prompt, model, phases)
	durationMS := time.Since(startTime).Milliseconds()

	if err != nil {
		if wl.auditWriter != nil {
			recordAuditEvent(wl.auditWriter, newAuditFailedEvent(taskID, durationMS, false), wl.auditLogger)
		}
		return result, err
	}
	artifact, verifyErr := wl.postconditionGit.verify(workDir, plan, baseline)
	if artifact.Name != "" {
		result.Artifacts = append(result.Artifacts, artifact)
	}
	if verifyErr != nil {
		if wl.auditWriter != nil {
			recordAuditEvent(wl.auditWriter, newAuditFailedEvent(taskID, durationMS, false), wl.auditLogger)
		}
		return result, verifyErr
	}
	if wl.auditWriter != nil {
		recordAuditEvent(wl.auditWriter, newAuditCompletedEvent(taskID, durationMS, result.CostUSD, false), wl.auditLogger)
	}

	return result, nil
}
