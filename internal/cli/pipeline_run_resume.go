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
