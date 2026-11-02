package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/insajin/autopus-adk/pkg/pipeline"
)

// reportPipelineResume prints what a resumed run took over from its
// checkpoint and every phase it had to dispatch again, so a resume that could
// not restore an output never reads as a plain completion
// (SPEC-PIPELINE-RESUME-001 REQ-PRR-OUT-04).
func reportPipelineResume(w io.Writer, result *pipeline.PipelineResult) {
	if result == nil || result.Receipt.Resume == nil {
		return
	}
	resume := result.Receipt.Resume
	restored := "no phase output"
	if len(resume.RestoredPhases) > 0 {
		names := make([]string, len(resume.RestoredPhases))
		for i, phase := range resume.RestoredPhases {
			names[i] = string(phase)
		}
		restored = strings.Join(names, ", ")
	}
	from := result.Receipt.ResumedFrom
	if from == "" {
		from = "an unrecorded run"
	}
	fmt.Fprintf(w, "Pipeline resume: restored %s from %s (task %s)\n", restored, from, result.Receipt.TaskID)
	for _, note := range resume.Notes {
		fmt.Fprintf(w, "Pipeline resume: %s\n", note)
	}
}

// pipelinePhaseCounts separates the phases a run executed from the phases it
// restored from its checkpoint (SPEC-STATUSEVIDENCE-001 REQ-SEV-RUN-01).
type pipelinePhaseCounts struct {
	Executed int `json:"executed"`
	Restored int `json:"restored"`
	Failed   int `json:"failed"`
	Pending  int `json:"pending"`
}

// countPipelinePhases counts what this run did with each phase. A completed
// phase is executed when this run dispatched it and restored when it did not:
// a restored phase that a routed fix sends again counts as executed.
func countPipelinePhases(receipt pipeline.OrchestrationRunReceipt) pipelinePhaseCounts {
	dispatched := dispatchedPipelinePhases(receipt)
	var counts pipelinePhaseCounts
	for _, phase := range receipt.Phases {
		switch {
		case phase.Status == pipeline.CheckpointStatusDone && dispatched[phase.PhaseID]:
			counts.Executed++
		case phase.Status == pipeline.CheckpointStatusDone:
			counts.Restored++
		case phase.Status == pipeline.CheckpointStatusFailed:
			counts.Failed++
		default:
			counts.Pending++
		}
	}
	return counts
}

// dispatchedPipelinePhases are the phases this run sent to its backend. Every
// dispatch starts with an in-progress transition of attempt one or more; a
// restore records attempt zero.
func dispatchedPipelinePhases(receipt pipeline.OrchestrationRunReceipt) map[pipeline.PhaseID]bool {
	dispatched := map[pipeline.PhaseID]bool{}
	for _, transition := range receipt.Transitions {
		if transition.PhaseID != "" && transition.Status == pipeline.CheckpointStatusInProgress && transition.Attempt > 0 {
			dispatched[transition.PhaseID] = true
		}
	}
	return dispatched
}

// pipelineCompletionLine is the result line of a completed run.
func pipelineCompletionLine(result *pipeline.PipelineResult) string {
	counts := countPipelinePhases(result.Receipt)
	return fmt.Sprintf("Pipeline complete: %d phases executed, %d restored from checkpoint, %d dispatches",
		counts.Executed, counts.Restored, result.Receipt.DispatchCount)
}

// pipelineStopReason names what ended a stopped run: its last failure's
// decision when that decision ends a run in this terminal state, otherwise
// the terminal state. A remediate or transport-retry decision never ends a
// run, so a persistence failure right after one reads partial_preserved.
func pipelineStopReason(receipt pipeline.OrchestrationRunReceipt) string {
	if n := len(receipt.Failures); n > 0 {
		decision := receipt.Failures[n-1].Decision
		ends := map[pipeline.FailureDecision]pipeline.TerminalState{
			pipeline.DecisionNoProgress:      pipeline.TerminalBlocked,
			pipeline.DecisionBlocked:         pipeline.TerminalBlocked,
			pipeline.DecisionBudgetExhausted: pipeline.TerminalFailedBudget,
			pipeline.DecisionCancelled:       pipeline.TerminalCancelled,
		}
		if terminal, ok := ends[decision]; ok && terminal == receipt.Terminal {
			return string(decision)
		}
	}
	return string(receipt.Terminal)
}
