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

// countPipelinePhases reads the counts from a run receipt. A restored phase is
// done without having been dispatched by this run, so it is never executed.
func countPipelinePhases(receipt pipeline.OrchestrationRunReceipt) pipelinePhaseCounts {
	restored := map[pipeline.PhaseID]bool{}
	if receipt.Resume != nil {
		for _, phase := range receipt.Resume.RestoredPhases {
			restored[phase] = true
		}
	}
	var counts pipelinePhaseCounts
	for _, phase := range receipt.Phases {
		switch {
		case phase.Status == pipeline.CheckpointStatusDone && restored[phase.PhaseID]:
			counts.Restored++
		case phase.Status == pipeline.CheckpointStatusDone:
			counts.Executed++
		case phase.Status == pipeline.CheckpointStatusFailed:
			counts.Failed++
		default:
			counts.Pending++
		}
	}
	return counts
}

// pipelineCompletionLine is the result line of a completed run.
func pipelineCompletionLine(result *pipeline.PipelineResult) string {
	counts := countPipelinePhases(result.Receipt)
	return fmt.Sprintf("Pipeline complete: %d phases executed, %d restored from checkpoint, %d dispatches",
		counts.Executed, counts.Restored, result.Receipt.DispatchCount)
}
