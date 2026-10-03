package loop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/agentexec"
	"github.com/insajin/autopus-adk/pkg/qa/run"
	"github.com/insajin/autopus-adk/pkg/qa/scenario"
	"github.com/insajin/autopus-adk/pkg/qa/triage"
)

func neverCalled(t *testing.T) AgentFunc {
	return func(context.Context, agentexec.Request) (agentexec.Response, error) {
		t.Error("the agent must not run")
		return agentexec.Response{}, nil
	}
}

func TestQALoopRun_UnfixableClassesStopWithoutAgent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		class  triage.Class
		reason StopReason
		code   string
	}{
		{triage.ClassUnknown, StopUnknownFailure, CodeUnknownFailure},
		{triage.ClassEnvironment, StopBlockedEnv, CodeBlockedEnv},
	}
	for _, tc := range cases {
		repo := newRepo(t, seed)
		report, err := repo.loop(repo.deps(tc.class, neverCalled(t)), 3)
		require.Error(t, err)
		assert.Equal(t, tc.code, ErrorCode(err))
		assert.Equal(t, tc.reason, report.StopReason)
		assert.Contains(t, report.StopDetail, "login")
		assert.True(t, report.Restored)
	}
}

func TestQALoopRun_FlakyFailureIsQuarantined(t *testing.T) {
	t.Parallel()
	repo := newRepo(t, seed)
	deps := repo.deps(triage.ClassProductDefect, neverCalled(t))
	deps.Run = func(opts run.Options) (run.Result, error) {
		if opts.JourneyID == "login" {
			return laneResult("passed")
		}
		return laneResult("failed")
	}
	deps.Classify = func(in triage.Input) triage.Verdict {
		if in.RerunPassed != nil && *in.RerunPassed {
			return triage.Verdict{JourneyID: in.JourneyID, Class: triage.ClassFlaky, Signal: "rerun:passed"}
		}
		return triage.Verdict{JourneyID: in.JourneyID, Class: triage.ClassUnknown}
	}

	report, err := repo.loop(deps, 3)

	require.NoError(t, err)
	assert.Equal(t, StopPassedWithFlaky, report.StopReason)
	assert.Equal(t, []string{"login"}, report.Quarantined)
}

func TestQALoopRun_EmptyLaneIsBlockedNotPassed(t *testing.T) {
	t.Parallel()
	repo := newRepo(t, seed)
	deps := repo.deps(triage.ClassProductDefect, neverCalled(t))
	deps.Run = func(run.Options) (run.Result, error) { return run.Result{Status: "passed"}, nil }

	report, err := repo.loop(deps, 3)

	require.Error(t, err)
	assert.Equal(t, StopBlockedEnv, report.StopReason)
	assert.Equal(t, "blocked", report.FinalStatus)
}

func TestQALoopRun_AgentErrorsStopAndRevert(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err    error
		reason StopReason
		code   string
	}{
		{&agentexec.SetupGapError{Code: agentexec.CodeCLIMissing, Binary: "claude"}, StopBlockedEnv, agentexec.CodeCLIMissing},
		{&agentexec.AgentError{Code: agentexec.CodeTimeout, ExitCode: 124, Err: errors.New("deadline")}, StopAgentFailed, agentexec.CodeTimeout},
	}
	for _, tc := range cases {
		repo := newRepo(t, seed)
		agent := func(context.Context, agentexec.Request) (agentexec.Response, error) {
			repo.write("src/app.ts", "half-written")
			return agentexec.Response{}, tc.err
		}
		report, err := repo.loop(repo.deps(triage.ClassProductDefect, agent), 3)
		require.Error(t, err)
		assert.Equal(t, tc.code, ErrorCode(err))
		assert.Equal(t, tc.reason, report.StopReason)
		assert.Equal(t, seed["src/app.ts"], repo.read("src/app.ts"), "a failed agent's partial edit is reverted")
	}
}

func TestQALoopRun_RejectedCommitHookStopsAndReverts(t *testing.T) {
	t.Parallel()
	repo := newRepo(t, seed)
	hook := filepath.Join(repo.hooks, "pre-commit")
	require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\necho 'lint failed' >&2\nexit 1\n"), 0o755))

	report, err := repo.loop(repo.deps(triage.ClassProductDefect, writing(repo, map[string]string{"fixed.txt": "ok\n"})), 3)

	require.Error(t, err)
	assert.Equal(t, CodeCommitRejected, ErrorCode(err))
	assert.Equal(t, StopCommitRejected, report.StopReason)
	assert.Contains(t, report.StopDetail, "lint failed")
	assert.True(t, report.Iterations[0].Guard.Accepted)
	assert.Equal(t, seed["fixed.txt"], repo.read("fixed.txt"))
	assert.Empty(t, repo.git("status", "--porcelain"))
}

func TestQALoopRun_MaxIterationsAfterLastFixIsVerified(t *testing.T) {
	t.Parallel()
	repo := newRepo(t, seed)
	calls := 0
	deps := repo.deps(triage.ClassProductDefect, nil)
	deps.Classify = func(in triage.Input) triage.Verdict {
		calls++
		step := &scenario.StepRef{Screen: fmt.Sprintf("screen-%d", calls), Kind: scenario.StepKindExpect}
		return triage.Verdict{JourneyID: in.JourneyID, Class: triage.ClassProductDefect, Step: step}
	}
	deps.Agent = func(_ context.Context, req agentexec.Request) (agentexec.Response, error) {
		repo.write("attempts.txt", req.Prompt[:10])
		return agentexec.Response{}, nil
	}

	report, err := repo.loop(deps, 1)

	require.Error(t, err)
	assert.Equal(t, CodeMaxIterations, ErrorCode(err))
	require.Len(t, report.Iterations, 2, "the run after the last fix only verifies")
	assert.Nil(t, report.Iterations[1].Agent)
	assert.Equal(t, "1", repo.git("rev-list", "--count", "main.."+report.Branch))
}

func TestQALoopRun_UnchangedFingerprintAfterFixIsNoProgress(t *testing.T) {
	t.Parallel()
	repo := newRepo(t, seed)
	deps := repo.deps(triage.ClassProductDefect, nil)
	deps.Agent = func(_ context.Context, req agentexec.Request) (agentexec.Response, error) {
		repo.write("src/app.ts", "export const greeting = 'try "+fmt.Sprint(len(req.Prompt))+"'\n")
		return agentexec.Response{}, nil
	}

	report, err := repo.loop(deps, 3)

	require.Error(t, err)
	assert.Equal(t, CodeNoProgress, ErrorCode(err))
	assert.Len(t, report.Iterations, 2)
	assert.Contains(t, report.StopDetail, "login|product_defect|")
}

func TestQALoopRun_AgentCommitIsUndoneAndRejected(t *testing.T) {
	t.Parallel()
	repo := newRepo(t, seed)
	agent := func(context.Context, agentexec.Request) (agentexec.Response, error) {
		repo.write("fixed.txt", "ok\n")
		repo.git("commit", "-q", "-am", "sneaky")
		return agentexec.Response{}, nil
	}

	report, err := repo.loop(repo.deps(triage.ClassProductDefect, agent), 3)

	require.Error(t, err)
	assert.Equal(t, CodeGuardRejected, ErrorCode(err))
	assert.Contains(t, report.StopDetail, "only the loop may commit")
	assert.Equal(t, "0", repo.git("rev-list", "--count", "main.."+report.Branch))
	assert.Equal(t, seed["fixed.txt"], repo.read("fixed.txt"))
}

func TestQALoopRun_HealIsRecompiledAndCommitted(t *testing.T) {
	t.Parallel()
	repo := newRepo(t, map[string]string{"fixed.txt": "x\n", scenarioRel: loginScenario})
	healed := strings.Replace(loginScenario, "name: Sign in", "name: Log in", 1)
	deps := repo.deps(triage.ClassTestDrift, writing(repo, map[string]string{scenarioRel: healed}))
	deps.Run = func(run.Options) (run.Result, error) {
		if strings.Contains(repo.read(scenarioRel), "Log in") {
			return laneResult("passed")
		}
		return laneResult("failed")
	}
	deps.Compile = func(string) error {
		repo.write("e2e/autopus-generated/login.spec.ts", "// generated from the healed scenario\n")
		return nil
	}

	report, err := repo.loop(deps, 3)

	require.NoError(t, err)
	assert.Equal(t, StopPassed, report.StopReason)
	files := repo.git("show", "--name-only", "--format=", report.Branch)
	assert.Contains(t, files, scenarioRel)
	assert.Contains(t, files, "e2e/autopus-generated/login.spec.ts")
	assert.Equal(t, "fix(qa): test_drift login", repo.git("log", "-1", "--format=%s", report.Branch))
}

func TestQALoopRun_HealMovingAnOracleIsRevertedToHead(t *testing.T) {
	t.Parallel()
	repo := newRepo(t, map[string]string{"fixed.txt": "x\n", scenarioRel: loginScenario})
	moved := strings.Replace(loginScenario, "Welcome back", "Anything", 1)

	report, err := repo.loop(repo.deps(triage.ClassTestDrift, writing(repo, map[string]string{scenarioRel: moved})), 3)

	require.Error(t, err)
	assert.Equal(t, CodeGuardRejected, ErrorCode(err))
	assert.Contains(t, report.StopDetail, scenarioRel+": screen \"sign-in\": a heal changed expect step 1")
	assert.Equal(t, loginScenario, repo.read(scenarioRel))
}
