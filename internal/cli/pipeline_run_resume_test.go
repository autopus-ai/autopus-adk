package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/pipeline"
)

// S9: a resume that re-dispatched a phase says so on its own line.
func TestReportPipelineResume_PrintsEveryNote(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	reportPipelineResume(&out, &pipeline.PipelineResult{Receipt: pipeline.OrchestrationRunReceipt{
		TaskID: "task-1", ResumedFrom: "pipeline-a",
		Resume: &pipeline.ResumeRecord{
			RestoredPhases: []pipeline.PhaseID{},
			RerunPhases:    []pipeline.PhaseID{pipeline.PhasePlan},
			Notes: []string{"phase plan output cannot be restored (checkpoint pipeline_checkpoint.v1 " +
				"predates persisted phase outputs); plan writes nothing outside the pipeline and is dispatched again"},
		},
	}})

	assert.Equal(t, "Pipeline resume: restored no phase output from pipeline-a (task task-1)\n"+
		"Pipeline resume: phase plan output cannot be restored (checkpoint pipeline_checkpoint.v1 "+
		"predates persisted phase outputs); plan writes nothing outside the pipeline and is dispatched again\n",
		out.String())
}

func TestReportPipelineResume_SilentWithoutAResume(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	reportPipelineResume(&out, nil)
	reportPipelineResume(&out, &pipeline.PipelineResult{})

	assert.Empty(t, out.String())
}

func TestReportPipelineResume_NamesRestoredPhases(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	reportPipelineResume(&out, &pipeline.PipelineResult{Receipt: pipeline.OrchestrationRunReceipt{
		TaskID: "task-2", Resume: &pipeline.ResumeRecord{
			RestoredPhases: []pipeline.PhaseID{pipeline.PhasePlan, pipeline.PhaseTestScaffold},
		},
	}})

	assert.Equal(t, "Pipeline resume: restored plan, test_scaffold from an unrecorded run (task task-2)\n", out.String())
}
