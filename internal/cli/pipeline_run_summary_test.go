package cli

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/pipeline"
)

// completedReceipt is a completed run that restored the given phases and
// dispatched the rest once each.
func completedReceipt(restored ...pipeline.PhaseID) pipeline.OrchestrationRunReceipt {
	receipt := pipeline.OrchestrationRunReceipt{Terminal: pipeline.TerminalCompleted}
	for _, phase := range pipeline.DefaultPhases() {
		receipt.Phases = append(receipt.Phases, pipeline.PhaseReceipt{PhaseID: phase.ID, Status: pipeline.CheckpointStatusDone})
	}
	receipt.DispatchCount = len(receipt.Phases) - len(restored)
	if len(restored) > 0 {
		receipt.Resume = &pipeline.ResumeRecord{CheckpointVersion: pipeline.CheckpointVersion, RestoredPhases: restored}
	}
	return receipt
}

// S6: a resumed run never counts the phases it restored as executed.
func TestPipelineRunSummary_SeparatesExecutedAndRestored(t *testing.T) {
	t.Parallel()
	resumed := &pipeline.PipelineResult{Receipt: completedReceipt(pipeline.PhasePlan, pipeline.PhaseTestScaffold)}
	assert.Equal(t, "Pipeline complete: 3 phases executed, 2 restored from checkpoint, 3 dispatches",
		pipelineCompletionLine(resumed))
	assert.Equal(t, pipelinePhaseCounts{Executed: 3, Restored: 2}, countPipelinePhases(resumed.Receipt))

	fresh := &pipeline.PipelineResult{Receipt: completedReceipt()}
	assert.Equal(t, "Pipeline complete: 5 phases executed, 0 restored from checkpoint, 5 dispatches",
		pipelineCompletionLine(fresh))
}

// S11: the pipeline says which platform it took and why.
func TestPipelinePlatformSelection_Reasons(t *testing.T) {
	t.Parallel()
	installed := func(names ...string) func(string) (string, error) {
		return func(name string) (string, error) {
			for _, candidate := range names {
				if candidate == name {
					return "/stub/" + name, nil
				}
			}
			return "", exec.ErrNotFound
		}
	}
	cases := []struct {
		flag, wantPlatform, wantReason string
		path                           []string
	}{
		{flag: "codex", path: []string{"claude"}, wantPlatform: "codex", wantReason: "--platform"},
		{path: []string{"codex", "agy"}, wantPlatform: "codex", wantReason: "first installed of claude → codex → agy"},
		{path: []string{"agy"}, wantPlatform: "gemini", wantReason: "first installed of claude → codex → agy"},
		{wantPlatform: "claude", wantReason: "no AI CLI installed; falls back to claude"},
	}
	for _, tc := range cases {
		platform, reason := resolvePlatformWith(tc.flag, installed(tc.path...))
		assert.Equal(t, tc.wantPlatform, platform)
		assert.Equal(t, tc.wantReason, reason)
	}
	assert.Equal(t, "gemini (agy)", pipelinePlatformLabel("gemini"))
	assert.Equal(t, "codex", pipelinePlatformLabel("codex"))
}

// The run reports its platform before anything dispatches, and a dry run
// records no verification baseline.
func TestPipelineRunCmd_ReportsPlatformAndDryRunRecordsNoBaseline(t *testing.T) {
	const specID = "SPEC-PLATFORMLINE-001"
	root := t.TempDir()
	chdirForTest(t, root)
	writePipelineOwnerSpec(t, root, specID)
	commitRouteProject(t, root)

	cmd := newPipelineRunCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{specID, "--dry-run", "--platform", "codex"})
	require.NoError(t, cmd.Execute(), errOut.String())

	assert.Contains(t, errOut.String(), "Pipeline platform: codex — --platform\n")
	assert.Contains(t, out.String(), "Pipeline dry run: 5 phases planned; no backend execution")
	assert.NoFileExists(t, filepath.Join(root, pipelineVerificationPath(pipelineStateDir, specID)))
}
