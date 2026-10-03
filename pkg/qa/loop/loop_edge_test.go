package loop

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/agentexec"
	"github.com/insajin/autopus-adk/pkg/qa/run"
	"github.com/insajin/autopus-adk/pkg/qa/triage"
)

// REQ-12: a detached start is restored as the same detached commit.
func TestQALoopRun_DetachedHeadIsRestored(t *testing.T) {
	t.Parallel()
	repo := newRepo(t, seed)
	sha := repo.git("rev-parse", "HEAD")
	repo.git("switch", "-q", "--detach", "HEAD")

	report, err := repo.loop(repo.deps(triage.ClassProductDefect, writing(repo, map[string]string{"fixed.txt": "ok\n"})), 3)

	require.NoError(t, err)
	assert.Equal(t, sha, report.OriginalRef)
	assert.True(t, report.Restored)
	assert.Equal(t, "HEAD", repo.git("rev-parse", "--abbrev-ref", "HEAD"), "HEAD is detached again")
	assert.Equal(t, sha, repo.git("rev-parse", "HEAD"))
}

// An agent that switches branches is rejected, and the loop resets nothing
// it does not own.
func TestQALoopRun_AgentLeavingTheLoopBranchIsRejected(t *testing.T) {
	t.Parallel()
	repo := newRepo(t, seed)
	agent := func(context.Context, agentexec.Request) (agentexec.Response, error) {
		repo.git("switch", "-q", "-c", "elsewhere")
		return agentexec.Response{}, nil
	}

	report, err := repo.loop(repo.deps(triage.ClassProductDefect, agent), 3)

	require.Error(t, err)
	assert.Equal(t, CodeGuardRejected, ErrorCode(err))
	assert.Contains(t, report.StopDetail, "agent left the loop branch for elsewhere")
	assert.Contains(t, report.StopDetail, "nothing was reverted")
	assert.Equal(t, "main", repo.git("rev-parse", "--abbrev-ref", "HEAD"))
}

// Triage reads the manifest when there is one, falls back to the adapter
// result otherwise, and re-runs only journeys that failed.
func TestQALoopRun_TriageInputPrefersManifestAndSkipsRerunForBlocked(t *testing.T) {
	t.Parallel()
	repo := newRepo(t, seed)
	runs := 0
	var inputs []triage.Input
	deps := repo.deps(triage.ClassEnvironment, neverCalled(t))
	deps.Run = func(run.Options) (run.Result, error) {
		runs++
		return run.Result{Status: "blocked", AdapterResults: []run.AdapterResult{
			{JourneyID: "login", Adapter: "playwright", Status: "blocked", SetupGap: &run.SetupGap{Adapter: "playwright", Reason: "browser_missing"}},
			{JourneyID: "checkout", Adapter: "playwright", Status: "failed", QAMESHManifestPath: "/runs/checkout/manifest.json", FailureSummary: "summary"},
		}}, errors.New("qa run blocked")
	}
	deps.LoadInput = func(_, manifestPath string) (triage.Input, error) {
		return triage.Input{JourneyID: "checkout", FailureText: "from " + manifestPath}, nil
	}
	deps.Classify = func(in triage.Input) triage.Verdict {
		inputs = append(inputs, in)
		return triage.Verdict{Class: triage.ClassEnvironment, Signal: "fake"}
	}

	report, err := repo.loop(deps, 3)

	require.Error(t, err)
	assert.Equal(t, StopBlockedEnv, report.StopReason)
	assert.Equal(t, 2, runs, "the lane once, then only the failed journey again")
	require.Len(t, inputs, 2)
	assert.Equal(t, "browser_missing", inputs[0].SetupGapCode)
	assert.Equal(t, "blocked", inputs[0].Status)
	assert.Nil(t, inputs[0].RerunPassed, "a blocked journey is not re-run")
	assert.Equal(t, "from /runs/checkout/manifest.json", inputs[1].FailureText)
	assert.Equal(t, repo.dir, inputs[1].ProjectDir)
	require.NotNil(t, inputs[1].RerunPassed)
	assert.False(t, *inputs[1].RerunPassed)
	assert.Equal(t, "checkout", report.Iterations[0].Failures[1].JourneyID, "an empty verdict id falls back to the journey")
}

// A project in a subdirectory may not reach outside it, and its reports stay
// excluded from git status.
func TestQALoopRun_SubdirectoryProjectRejectsEditsOutsideIt(t *testing.T) {
	t.Parallel()
	repo := newRepo(t, map[string]string{"web/index.ts": "export {}\n", "shared/lib.ts": "export {}\n"})
	deps := repo.deps(triage.ClassProductDefect, writing(repo, map[string]string{
		"web/src/app.ts": "export const x = 1\n",
		"shared/lib.ts":  "export const leaked = true\n",
	}))

	report, err := Run(context.Background(), Options{ProjectDir: filepath.Join(repo.dir, "web"), Agent: agentexec.TargetCodex}, deps)

	require.Error(t, err)
	assert.Equal(t, CodeGuardRejected, ErrorCode(err))
	assert.Equal(t, "../shared/lib.ts", report.Iterations[0].Guard.Path)
	assert.Contains(t, report.Iterations[0].Guard.Reason, "outside the project directory")
	assert.Equal(t, "export {}\n", repo.read("shared/lib.ts"))
	assert.Equal(t, "<missing>", repo.read("web/src/app.ts"))
	assert.FileExists(t, filepath.Join(repo.dir, "web", ".autopus", "qa", "loop", report.RunID, "report.json"))
	assert.Empty(t, repo.git("status", "--porcelain"))
}
