package pipeline

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type parallelCompletion struct {
	index  int
	result PhaseResult
	err    error
}

func (r *ParallelRunner) runDependencyPhases(ctx context.Context, phases []Phase, cfg RunConfig) ([]PhaseResult, error) {
	indices, err := parallelPhaseGraph(phases)
	if err != nil {
		return nil, err
	}
	if err := cfg.preflightWorkflowAuthenticity(); err != nil {
		return nil, err
	}
	// Preflight delegation before any backend call; append its evidence after the
	// first admission snapshot to preserve existing safety event ordering.
	var delegation []DegradedEvidence
	preflight := cfg
	preflight.SafetyEvents = &delegation
	for _, phase := range phases {
		if err := preflight.checkDelegationSafety(phase.ID); err != nil {
			for _, evidence := range delegation {
				cfg.recordSafetyEvidence(evidence)
			}
			return nil, err
		}
	}
	n := len(phases)
	results := make([]PhaseResult, n)
	states := make([]string, n)
	ordered := make([]int, n)
	for i := range phases {
		ordered[i] = i
	}
	sort.Slice(ordered, func(i, j int) bool { return taskIDLess(string(phases[ordered[i]].ID), string(phases[ordered[j]].ID)) })
	cap := cfg.effectiveWorktreeSlotCap()
	completions := make(chan parallelCompletion, n)
	active, finished := 0, 0
	var runErr error
	firstSnapshot := true
	for finished < n {
		if err := ctx.Err(); err != nil && runErr == nil {
			runErr = err
		}
		admitted := false
		if runErr == nil {
			for _, i := range ordered {
				if active >= cap {
					break
				}
				if states[i] != "" {
					continue
				}
				ready := true
				for _, dep := range phases[i].DependsOn {
					if states[indices[dep]] != "pass" {
						ready = false
						break
					}
				}
				if !ready {
					continue
				}
				if err := ctx.Err(); err != nil {
					runErr = err
					break
				}
				states[i] = "active"
				active++
				admitted = true
				prompt := parallelDependencyPrompt(phases[i], indices, results)
				go func(index int, prompt string) {
					phase := phases[index]
					if err := ctx.Err(); err != nil {
						completions <- parallelCompletion{index: index, err: err}
						return
					}
					resp, err := r.backend.Execute(ctx, PhaseRequest{PhaseID: phase.ID, Prompt: prompt})
					if err != nil {
						completions <- parallelCompletion{index: index, err: fmt.Errorf("phase %s: %w", phase.ID, err)}
						return
					}
					if resp == nil {
						completions <- parallelCompletion{index: index, err: fmt.Errorf("phase %s: nil response", phase.ID)}
						return
					}
					verdict := EvaluateGate(phase.Gate, resp.Output)
					completions <- parallelCompletion{index: index, result: PhaseResult{PhaseID: phase.ID, Output: resp.Output, Verdict: verdict}}
				}(i, prompt)
			}
		}
		if admitted || firstSnapshot {
			cfg.recordParallelAdmission(phases, states, cap)
			if firstSnapshot {
				for _, e := range delegation {
					cfg.recordSafetyEvidence(e)
				}
				firstSnapshot = false
			}
		}
		if active == 0 {
			if runErr != nil {
				return nil, runErr
			}
			blocked := []string{}
			for _, i := range ordered {
				if states[i] == "" {
					blocked = append(blocked, string(phases[i].ID))
				}
			}
			if len(blocked) > 0 {
				return nil, fmt.Errorf("parallel dependencies failed; blocked phases: %s", strings.Join(blocked, ", "))
			}
			break
		}
		var completed parallelCompletion
		if runErr != nil {
			completed = <-completions
		} else {
			select {
			case completed = <-completions:
			case <-ctx.Done():
				runErr = ctx.Err()
				continue
			}
		}
		active--
		finished++
		if completed.err != nil {
			states[completed.index] = "fail"
			learnHookExecutorError(cfg.LearnStore, phases[completed.index].ID, completed.err)
			if runErr == nil {
				runErr = completed.err
			}
		} else {
			results[completed.index] = completed.result
			states[completed.index] = "pass"
			phase := phases[completed.index]
			if phase.Gate != GateNone && completed.result.Verdict != VerdictPass {
				states[completed.index] = "fail"
				learnHookGateFail(cfg.LearnStore, phase.ID, phase.Gate, completed.result.Output, 0)
			}
		}
	}
	if runErr != nil {
		return nil, runErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

// Snapshots describe admitted slots and all pending work (including tasks
// waiting on dependencies), not worktree allocation or execution-start order.
func (cfg RunConfig) recordParallelAdmission(phases []Phase, states []string, cap int) {
	active, pending := []string{}, []string{}
	for i, phase := range phases {
		switch states[i] {
		case "active":
			active = append(active, string(phase.ID))
		case "":
			pending = append(pending, string(phase.ID))
		}
	}
	cfg.recordSafetyEvidence(DegradedEvidence{Reason: ReasonWorktreeSlotCap, ActiveTaskIDs: orderedTaskIDs(active), QueuedTaskIDs: orderedTaskIDs(pending), SlotCount: len(active), Cap: cap, QueueDiscipline: "dependency_ready_task_id_admission"})
}
