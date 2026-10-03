package loop

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/agentexec"
	"github.com/insajin/autopus-adk/pkg/qa/run"
	"github.com/insajin/autopus-adk/pkg/qa/triage"
)

// seed is the tracked content of every test repository.
var seed = map[string]string{
	"fixed.txt":         "broken\n",
	"src/app.ts":        "export const greeting = ''\n",
	"e2e/login.spec.ts": "test('login', () => {})\n",
}

// repo is a throwaway git repository isolated from the developer's global
// and system git config, hooks, and signing.
type repo struct {
	t     *testing.T
	dir   string
	hooks string
	env   []string
}

func newRepo(t *testing.T, files map[string]string) *repo {
	t.Helper()
	r := &repo{t: t, dir: t.TempDir(), hooks: t.TempDir(), env: []string{
		"GIT_AUTHOR_NAME=QA Loop Test", "GIT_AUTHOR_EMAIL=qa-loop@example.invalid",
		"GIT_COMMITTER_NAME=QA Loop Test", "GIT_COMMITTER_EMAIL=qa-loop@example.invalid",
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull,
	}}
	r.git("init", "-q", "-b", "main")
	r.git("config", "core.hooksPath", r.hooks)
	r.git("config", "commit.gpgsign", "false")
	for rel, body := range files {
		r.write(rel, body)
	}
	r.git("add", "-A")
	r.git("commit", "-q", "-m", "chore: seed")
	return r
}

func (r *repo) git(args ...string) string {
	r.t.Helper()
	out, err := runGit(r.dir, r.env, args...)
	require.NoError(r.t, err)
	return strings.TrimSpace(out)
}

func (r *repo) write(rel, body string) {
	path := filepath.Join(r.dir, filepath.FromSlash(rel))
	require.NoError(r.t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(r.t, os.WriteFile(path, []byte(body), 0o644))
}

func (r *repo) read(rel string) string {
	body, err := os.ReadFile(filepath.Join(r.dir, filepath.FromSlash(rel)))
	if err != nil {
		return "<missing>"
	}
	return string(body)
}

func laneResult(status string) (run.Result, error) {
	result := run.Result{Status: status, AdapterResults: []run.AdapterResult{{JourneyID: "login", Adapter: "playwright", Status: status}}}
	if status == "passed" {
		return result, nil
	}
	result.AdapterResults[0].FailureSummary = "expected the welcome banner"
	return result, errors.New("qa run " + status)
}

// deps fails journey login until fixed.txt reads ok, and classifies every
// failure as class.
func (r *repo) deps(class triage.Class, agent AgentFunc) Deps {
	return Deps{
		Run: func(run.Options) (run.Result, error) {
			if strings.TrimSpace(r.read("fixed.txt")) == "ok" {
				return laneResult("passed")
			}
			return laneResult("failed")
		},
		Classify: func(in triage.Input) triage.Verdict {
			return triage.Verdict{JourneyID: in.JourneyID, Class: class, Signal: "fake:" + string(class)}
		},
		LoadInput: func(string, string) (triage.Input, error) { return triage.Input{}, nil },
		Agent:     agent,
		Compile:   func(string) error { return nil },
		Now:       func() time.Time { return time.Date(2026, 10, 3, 9, 30, 0, 0, time.UTC) },
		RunSuffix: func() string { return "beef" },
		GitEnv:    r.env,
	}
}

func (r *repo) loop(deps Deps, maxIterations int) (Report, error) {
	return Run(context.Background(), Options{ProjectDir: r.dir, Lane: "fast", Agent: agentexec.TargetClaude, MaxIterations: maxIterations}, deps)
}

// writing is a fake agent that writes files and reports success.
func writing(r *repo, files map[string]string) AgentFunc {
	return func(context.Context, agentexec.Request) (agentexec.Response, error) {
		for rel, body := range files {
			r.write(rel, body)
		}
		return agentexec.Response{Duration: time.Second}, nil
	}
}

// AC-QALOOP-012: one product fix, committed on the loop branch, then passed.
func TestQALoopRun_ProductFixPassesOnLoopBranch(t *testing.T) {
	t.Parallel()
	repo := newRepo(t, seed)
	var requests []agentexec.Request
	var inputs []triage.Input
	deps := repo.deps(triage.ClassProductDefect, func(ctx context.Context, req agentexec.Request) (agentexec.Response, error) {
		requests = append(requests, req)
		return writing(repo, map[string]string{"fixed.txt": "ok\n"})(ctx, req)
	})
	classify := deps.Classify
	deps.Classify = func(in triage.Input) triage.Verdict { inputs = append(inputs, in); return classify(in) }

	report, err := repo.loop(deps, 3)

	require.NoError(t, err)
	assert.Equal(t, StopPassed, report.StopReason)
	assert.Equal(t, "passed", report.FinalStatus)
	assert.Equal(t, "autopus/qa-loop/qaloop-20261003093000-beef", report.Branch)
	assert.Equal(t, "main", repo.git("rev-parse", "--abbrev-ref", "HEAD"), "the original branch is checked out again")
	assert.True(t, report.Restored)
	assert.Equal(t, "1", repo.git("rev-list", "--count", "main.."+report.Branch))
	assert.False(t, report.BranchDeleted)
	assert.Equal(t, "fix(qa): product_defect login", repo.git("log", "-1", "--format=%s", report.Branch))
	body := repo.git("log", "-1", "--format=%B", report.Branch)
	assert.Contains(t, body, "Constraint: QA loop iteration 1\nConfidence: medium\nRelated: "+report.RunID)
	assert.True(t, strings.HasSuffix(body, signOff), body)
	assert.Equal(t, "broken\n", repo.read("fixed.txt"), "the fix lives on the loop branch only")
	assert.Empty(t, repo.git("status", "--porcelain"), "reports never dirty the tree")

	require.Len(t, requests, 1)
	assert.Equal(t, agentexec.ModeEdit, requests[0].Mode)
	assert.Equal(t, repo.dir, requests[0].WorkDir)
	assert.Contains(t, requests[0].Prompt, "Do not change expected values")
	assert.Contains(t, requests[0].Prompt, "expected the welcome banner")
	require.NotEmpty(t, inputs)
	require.NotNil(t, inputs[0].RerunPassed, "every failure is re-run once before triage")
	assert.False(t, *inputs[0].RerunPassed)
	assert.Equal(t, "expected the welcome banner", inputs[0].FailureText)

	raw, err := os.ReadFile(report.ReportPath)
	require.NoError(t, err)
	var onDisk Report
	require.NoError(t, json.Unmarshal(raw, &onDisk))
	assert.Equal(t, ReportSchema, onDisk.Schema)
	require.Len(t, onDisk.Iterations, 2)
	first := onDisk.Iterations[0]
	require.Len(t, first.Failures, 1)
	assert.Equal(t, triage.ClassProductDefect, first.Failures[0].Class)
	require.NotNil(t, first.Guard)
	assert.True(t, first.Guard.Accepted)
	assert.Equal(t, []string{"fixed.txt"}, first.ChangedPaths)
	assert.Equal(t, repo.git("rev-parse", report.Branch), first.Commit)
	require.NotNil(t, first.Agent)
	assert.Equal(t, triage.ClassProductDefect, first.Agent.Class)
	assert.Empty(t, onDisk.Iterations[1].Failures)
	md, err := os.ReadFile(report.ReportMDPath)
	require.NoError(t, err)
	assert.Contains(t, string(md), "- Stop reason: `passed`")
	assert.Contains(t, string(md), "- Guard: accepted")
}

// AC-QALOOP-012: an agent that changes nothing ends the loop.
func TestQALoopRun_AgentThatChangesNothingStopsNoProgress(t *testing.T) {
	t.Parallel()
	repo := newRepo(t, seed)

	report, err := repo.loop(repo.deps(triage.ClassProductDefect, writing(repo, nil)), 3)

	require.Error(t, err)
	assert.Equal(t, CodeNoProgress, ErrorCode(err))
	assert.Equal(t, StopNoProgress, report.StopReason)
	assertLoopBranchRemoved(t, repo, report)
	assert.Equal(t, "main", repo.git("rev-parse", "--abbrev-ref", "HEAD"))
	require.Len(t, report.Iterations, 1)
	assert.False(t, report.Iterations[0].Guard.Accepted)
	assert.FileExists(t, report.ReportMDPath)
}

// AC-QALOOP-011: a product fix touching a test is rejected and reverted
// precisely.
func TestQALoopRun_ProductFixTouchingTestsIsRevertedPrecisely(t *testing.T) {
	t.Parallel()
	repo := newRepo(t, seed)
	repo.write("notes.txt", "mine\n")
	agent := writing(repo, map[string]string{
		"src/app.ts":        "export const greeting = 'hi'\n",
		"e2e/login.spec.ts": "test.skip('login', () => {})\n",
		"src/helper.ts":     "export {}\n",
	})

	report, err := repo.loop(repo.deps(triage.ClassProductDefect, agent), 3)

	require.Error(t, err)
	assert.Equal(t, CodeGuardRejected, ErrorCode(err))
	assert.Equal(t, StopGuardRejected, report.StopReason)
	assert.Contains(t, err.Error(), "e2e/login.spec.ts")
	guard := report.Iterations[0].Guard
	require.NotNil(t, guard)
	assert.False(t, guard.Accepted)
	assert.Equal(t, "e2e/login.spec.ts", guard.Path)
	assert.Equal(t, seed["e2e/login.spec.ts"], repo.read("e2e/login.spec.ts"), "the spec is restored")
	assert.Equal(t, seed["src/app.ts"], repo.read("src/app.ts"))
	assert.Equal(t, "<missing>", repo.read("src/helper.ts"), "files the agent created are removed")
	assert.Equal(t, "mine\n", repo.read("notes.txt"), "pre-existing untracked files are kept")
	assertLoopBranchRemoved(t, repo, report)
}

// AC-QALOOP-013: a dirty tracked tree is refused before any branch exists.
func TestQALoopRun_DirtyTreeIsRefusedWithoutBranch(t *testing.T) {
	t.Parallel()
	repo := newRepo(t, seed)
	repo.write("src/app.ts", "export const greeting = 'wip'\n")
	ran := false
	deps := repo.deps(triage.ClassProductDefect, writing(repo, nil))
	deps.Run = func(run.Options) (run.Result, error) { ran = true; return run.Result{}, nil }

	report, err := repo.loop(deps, 3)

	require.Error(t, err)
	assert.Equal(t, CodeDirtyWorktree, ErrorCode(err))
	assert.Empty(t, report.RunID)
	assert.False(t, ran)
	assert.Empty(t, repo.git("branch", "--list", "autopus/qa-loop/*"))
	assert.Equal(t, "export const greeting = 'wip'\n", repo.read("src/app.ts"))
}

func TestQALoopRun_OutsideGitIsRefused(t *testing.T) {
	t.Parallel()
	_, err := Run(context.Background(), Options{ProjectDir: t.TempDir(), Agent: agentexec.TargetClaude}, Deps{})
	assert.Equal(t, CodeNotGitRepo, ErrorCode(err))
	_, err = Run(context.Background(), Options{ProjectDir: t.TempDir()}, Deps{})
	assert.Equal(t, CodeInvalidOptions, ErrorCode(err))
}

// assertLoopBranchRemoved checks that a loop with no fix commit leaves no
// branch behind: there is nothing on it to review.
func assertLoopBranchRemoved(t *testing.T, r *repo, report Report) {
	t.Helper()
	assert.True(t, report.BranchDeleted)
	_, err := runGit(r.dir, r.env, "rev-parse", "--verify", "--quiet", report.Branch)
	assert.Error(t, err, "loop branch %s should be gone", report.Branch)
}
