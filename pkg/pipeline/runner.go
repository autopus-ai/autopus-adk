// Package pipeline provides pipeline state management types and persistence.
package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/insajin/autopus-adk/pkg/learn"
	"gopkg.in/yaml.v3"
)

// @AX:ANCHOR: [AUTO] runner configuration contract shared by CLI, subprocess engine, sequential runner, and parallel runner.
// @AX:REASON: Delegation safety, worktree slot cap, and evidence sink fields coordinate runtime safety gates across runners.
// RunConfig holds optional configuration for a runner execution.
type RunConfig struct {
	// SpecID is used to name the checkpoint file when CheckpointDir is set.
	SpecID string
	// CheckpointDir is the directory where checkpoint files are written.
	// If empty, no checkpoint is saved.
	CheckpointDir string
	// LearnStore is the optional learning store for recording gate failures.
	// When nil, learning hooks are silently skipped.
	LearnStore *learn.Store
	// CoverageThreshold is the minimum coverage percentage for the coverage
	// gap hook. Zero means the project set no threshold, and no coverage gap
	// is recorded: the hook reports against an explicit policy or not at all.
	CoverageThreshold float64
	// DelegationSafety configures runtime depth and authenticity checks.
	DelegationSafety DelegationContext
	// WorktreeSlotCap limits concurrent isolated worktree tasks. Defaults to 5.
	WorktreeSlotCap int
	// SafetyEvents receives structured safety decisions when provided.
	SafetyEvents *[]DegradedEvidence
}

// SequentialRunner executes pipeline phases one at a time in order.
type SequentialRunner struct {
	backend PhaseBackend
}

// NewSequentialRunner creates a SequentialRunner backed by the given backend.
func NewSequentialRunner(backend PhaseBackend) *SequentialRunner {
	return &SequentialRunner{backend: backend}
}

// RunPhases executes the given phases sequentially and returns their results.
// When a phase gate fails it retries up to Phase.MaxRetries times.
// An error is returned if max retries are exceeded.
func (r *SequentialRunner) RunPhases(ctx context.Context, phases []Phase, cfg RunConfig) ([]PhaseResult, error) {
	if err := cfg.preflightWorkflowAuthenticity(); err != nil {
		return nil, err
	}
	results := make([]PhaseResult, 0, len(phases))
	var previousOutput string

	for _, phase := range phases {
		result, err := r.runPhaseWithRetry(ctx, phase, previousOutput, cfg)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
		previousOutput = result.Output

		if cfg.CheckpointDir != "" && cfg.SpecID != "" {
			if saveErr := saveRunCheckpoint(cfg, phase.ID, results); saveErr != nil {
				// Checkpoint save failure is non-fatal — log and continue.
				_ = saveErr
			}
		}
	}

	return results, nil
}

// defaultMaxRetries is the safety cap when Phase.MaxRetries is 0.
const defaultMaxRetries = 10

// runPhaseWithRetry executes a single phase, retrying on gate failure.
// When MaxRetries is 0, a safety cap of defaultMaxRetries is applied.
func (r *SequentialRunner) runPhaseWithRetry(ctx context.Context, phase Phase, previousOutput string, cfg RunConfig) (PhaseResult, error) {
	if err := cfg.checkDelegationSafety(phase.ID); err != nil {
		return PhaseResult{}, err
	}
	prompt := buildRunnerPrompt(phase.ID, previousOutput)

	maxRetries := phase.MaxRetries
	if maxRetries <= 0 {
		maxRetries = defaultMaxRetries
	}

	for attempt := 0; ; attempt++ {
		resp, err := r.backend.Execute(ctx, PhaseRequest{Prompt: prompt, PhaseID: phase.ID})
		if err != nil {
			learnHookExecutorError(cfg.LearnStore, phase.ID, err)
			return PhaseResult{}, fmt.Errorf("phase %s: %w", phase.ID, err)
		}

		verdict := EvaluateGate(phase.Gate, resp.Output)
		if verdict == VerdictPass || phase.Gate == GateNone {
			return PhaseResult{PhaseID: phase.ID, Output: resp.Output, Verdict: verdict}, nil
		}

		learnHookGateFail(cfg.LearnStore, phase.ID, phase.Gate, resp.Output, attempt)

		if cfg.CoverageThreshold > 0 {
			learnHookCoverageGap(cfg.LearnStore, resp.Output, cfg.CoverageThreshold)
		}

		if phase.Gate == GateReview {
			learnHookReviewIssue(cfg.LearnStore, resp.Output, cfg.SpecID)
		}

		if attempt >= maxRetries {
			learnHookGateFail(cfg.LearnStore, phase.ID, phase.Gate, resp.Output, attempt)
			return PhaseResult{}, fmt.Errorf("phase %s: max retries (%d) exceeded", phase.ID, maxRetries)
		}
	}
}

// ParallelRunner executes pipeline phases concurrently.
type ParallelRunner struct {
	backend PhaseBackend
}

// NewParallelRunner creates a ParallelRunner backed by the given backend.
func NewParallelRunner(backend PhaseBackend) *ParallelRunner {
	return &ParallelRunner{backend: backend}
}

// @AX:WARN: [AUTO] parallel worker goroutines depend on slot acquisition and backend Execute honoring ctx after dispatch.
// @AX:REASON: Cancellation after a slot is acquired still relies on backend context handling; future worker changes must preserve bounded shutdown.
// RunPhases executes all given phases in parallel and returns their results.
// Results are returned in the same order as the input phases.
func (r *ParallelRunner) RunPhases(ctx context.Context, phases []Phase, cfg RunConfig) ([]PhaseResult, error) {
	if err := cfg.preflightWorkflowAuthenticity(); err != nil {
		return nil, err
	}
	n := len(phases)
	results := make([]PhaseResult, n)
	errs := make([]error, n)
	slotCap := cfg.effectiveWorktreeSlotCap()
	if n > 0 {
		schedule := ScheduleWorktreeTasksWithCap(phaseTaskIDs(phases), slotCap)
		cfg.recordSafetyEvidence(schedule.Evidence)
	}
	for _, phase := range phases {
		if err := cfg.checkDelegationSafety(phase.ID); err != nil {
			return nil, err
		}
	}
	slots := make(chan struct{}, slotCap)

	// @AX:NOTE: [AUTO] start-gun pattern — gate channel releases all goroutines simultaneously; maximizes concurrency burst
	// gate is closed after all goroutines are launched, releasing them
	// simultaneously to maximise observable concurrency.
	gate := make(chan struct{})

	var wg sync.WaitGroup
	for i, phase := range phases {
		wg.Add(1)
		go func(idx int, ph Phase) {
			defer wg.Done()
			<-gate
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			case <-ctx.Done():
				errs[idx] = ctx.Err()
				return
			}
			resp, err := r.backend.Execute(ctx, PhaseRequest{PhaseID: ph.ID})
			if err != nil {
				learnHookExecutorError(cfg.LearnStore, ph.ID, err)
				errs[idx] = fmt.Errorf("phase %s: %w", ph.ID, err)
				return
			}
			verdict := EvaluateGate(ph.Gate, resp.Output)
			if verdict != VerdictPass && ph.Gate != GateNone {
				learnHookGateFail(cfg.LearnStore, ph.ID, ph.Gate, resp.Output, 0)
			}
			results[idx] = PhaseResult{PhaseID: ph.ID, Output: resp.Output, Verdict: verdict}
		}(i, phase)
	}
	close(gate) // release all goroutines at once
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return results, nil
}

// buildRunnerPrompt constructs a phase prompt injecting the previous output.
func buildRunnerPrompt(phaseID PhaseID, previousOutput string) string {
	if previousOutput == "" {
		return fmt.Sprintf("Phase: %s", phaseID)
	}
	return fmt.Sprintf("Phase: %s\n\nPrevious output:\n%s", phaseID, previousOutput)
}

// saveRunCheckpoint writes a checkpoint file to cfg.CheckpointDir/{SpecID}.yaml.
func saveRunCheckpoint(cfg RunConfig, lastPhase PhaseID, results []PhaseResult) error {
	taskStatus := make(map[string]string, len(results))
	for _, r := range results {
		taskStatus[string(r.PhaseID)] = string(CheckpointStatusDone)
	}

	data, err := yaml.Marshal(map[string]interface{}{
		"phase":       string(lastPhase),
		"task_status": taskStatus,
	})
	if err != nil {
		return err
	}

	path := filepath.Join(cfg.CheckpointDir, cfg.SpecID+".yaml")
	return os.WriteFile(path, data, 0o644)
}
