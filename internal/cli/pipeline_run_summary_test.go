package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/pipeline"
)

// S6: counts follow what the run dispatched. A resume restores plan,
// test_scaffold, and implement; validation fails, the fix dispatches
// implement again, and validation and review pass: three phases executed,
// two restored, four dispatches — implement counts as executed, not restored.
func TestPipelineRunSummary_SeparatesExecutedAndRestored(t *testing.T) {
	t.Parallel()
	const specID = "SPEC-COUNT-001"
	f := newStatusFixture(t)
	f.writeSpec(specID, "approved")
	f.commit("base")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	interrupted := &statusEngineBackend{hooks: map[pipeline.PhaseID]func(int) (*pipeline.PhaseResponse, error){
		pipeline.PhaseValidate: func(int) (*pipeline.PhaseResponse, error) { cancel(); return nil, context.Canceled },
	}}
	_, err := f.runEngine(ctx, specID, interrupted, nil)
	require.True(t, errors.Is(err, context.Canceled), err)

	resumed := &statusEngineBackend{outputs: map[pipeline.PhaseID][]string{
		pipeline.PhaseValidate: {failingValidation, "VERDICT: PASS"},
	}}
	result, err := f.runEngine(context.Background(), specID, resumed, f.loadCheckpoint(specID))
	require.NoError(t, err)

	assert.Equal(t, []pipeline.PhaseID{pipeline.PhasePlan, pipeline.PhaseTestScaffold, pipeline.PhaseImplement},
		result.Receipt.Resume.RestoredPhases, "the resume restored implement")
	assert.Equal(t, []pipeline.PhaseID{pipeline.PhaseValidate, pipeline.PhaseImplement, pipeline.PhaseValidate,
		pipeline.PhaseReview}, resumed.dispatched(), "the fix dispatched implement again")
	assert.Equal(t, "Pipeline complete: 3 phases executed, 2 restored from checkpoint, 4 dispatches",
		pipelineCompletionLine(result))

	fresh := f.completedRun("SPEC-COUNT-001")
	assert.Equal(t, "Pipeline complete: 5 phases executed, 0 restored from checkpoint, 5 dispatches",
		pipelineCompletionLine(fresh))
}

// The stop reason is the decision that ended the run. When a checkpoint
// write fails right after a routed fix, the run ends partial_preserved, not
// with the remediate decision recorded before it.
func TestPipelineRunSummary_StopReasonIsTheEndingDecision(t *testing.T) {
	t.Parallel()
	const specID = "SPEC-STOP-001"
	f := newStatusFixture(t)
	f.writeSpec(specID, "approved")
	f.commit("base")
	stateDir := filepath.Join(f.root, pipelineStateDir)
	breaking := &statusEngineBackend{hooks: map[pipeline.PhaseID]func(int) (*pipeline.PhaseResponse, error){
		pipeline.PhaseValidate: func(int) (*pipeline.PhaseResponse, error) {
			if err := os.RemoveAll(stateDir); err != nil {
				return nil, err
			}
			if err := os.WriteFile(stateDir, []byte("occupied"), 0o600); err != nil {
				return nil, err
			}
			return &pipeline.PhaseResponse{Output: failingValidation}, nil
		},
	}}
	result, err := f.runEngine(context.Background(), specID, breaking, nil)
	require.Error(t, err)
	require.NotEmpty(t, result.Receipt.Failures)
	assert.Equal(t, pipeline.DecisionRemediate, result.Receipt.Failures[len(result.Receipt.Failures)-1].Decision)
	assert.Equal(t, pipeline.TerminalPartialPreserved, result.Receipt.Terminal)
	assert.Equal(t, "partial_preserved", pipelineStopReason(result.Receipt))

	for decision, terminal := range map[pipeline.FailureDecision]pipeline.TerminalState{
		pipeline.DecisionNoProgress: pipeline.TerminalBlocked, pipeline.DecisionBudgetExhausted: pipeline.TerminalFailedBudget,
		pipeline.DecisionCancelled: pipeline.TerminalCancelled, pipeline.DecisionBlocked: pipeline.TerminalBlocked,
	} {
		receipt := pipeline.OrchestrationRunReceipt{Terminal: terminal, Failures: []pipeline.PhaseFailure{{Decision: decision}}}
		assert.Equal(t, string(decision), pipelineStopReason(receipt))
	}
	retried := pipeline.OrchestrationRunReceipt{Terminal: pipeline.TerminalBlocked,
		Failures: []pipeline.PhaseFailure{{Decision: pipeline.DecisionRetryTransport}}}
	assert.Equal(t, "blocked", pipelineStopReason(retried))
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
// keeps the last real run's checkpoint and baseline byte for byte, so status
// still reads that run (REQ-SEV-EXEC-04).
func TestPipelineRunCmd_DryRunKeepsTheLastRealRun(t *testing.T) {
	const specID = "SPEC-DRYRUN-001"
	f := newStatusFixture(t)
	f.writeSpec(specID, "approved")
	f.commit("base")
	result := f.completedRun(specID)
	f.recordBaseline(specID, result)
	canonical := filepath.Join(f.root, pipelineStateDir, specID+".yaml")
	baseline := pipelineVerificationPath(filepath.Join(f.root, pipelineStateDir), specID)
	before := [2][]byte{}
	for i, path := range []string{canonical, baseline} {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		before[i] = data
	}

	chdirForTest(t, f.root)
	cmd := newPipelineRunCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{specID, "--dry-run", "--platform", "codex"})
	require.NoError(t, cmd.Execute(), errOut.String())

	assert.Contains(t, errOut.String(), "Pipeline platform: codex — --platform\n")
	assert.Contains(t, out.String(), "Pipeline dry run: 5 phases planned; no backend execution")
	for i, path := range []string{canonical, baseline} {
		after, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, string(before[i]), string(after), path)
	}
	assert.FileExists(t, filepath.Join(f.root, pipelineDryRunStateDir, specID+".yaml"))
	evidence := f.evidence(specID)
	assert.Equal(t, "verified", evidence.Label)
	assert.Equal(t, result.Receipt.RunID, evidence.Execution.RunID)
}
