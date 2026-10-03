package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/agentexec"
	qaloop "github.com/insajin/autopus-adk/pkg/qa/loop"
	"github.com/insajin/autopus-adk/pkg/qa/run"
	"github.com/insajin/autopus-adk/pkg/qa/triage"
)

// qaLoopTestEnv isolates temp repositories from the developer's git config,
// hooks, and signing.
var qaLoopTestEnv = []string{
	"GIT_AUTHOR_NAME=QA Loop Test", "GIT_AUTHOR_EMAIL=qa-loop@example.invalid",
	"GIT_COMMITTER_NAME=QA Loop Test", "GIT_COMMITTER_EMAIL=qa-loop@example.invalid",
	"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull,
}

func qaLoopRepo(t *testing.T) string {
	t.Helper()
	dir, hooks := t.TempDir(), t.TempDir()
	git := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), qaLoopTestEnv...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	git("init", "-q", "-b", "main")
	git("config", "core.hooksPath", hooks)
	git("config", "commit.gpgsign", "false")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "fixed.txt"), []byte("broken\n"), 0o644))
	git("add", "-A")
	git("commit", "-q", "-m", "chore: seed")
	return dir
}

// qaLoopFakeDeps fails journey login until fixed.txt reads ok; the agent
// writes ok when fix is true.
func qaLoopFakeDeps(dir string, fix bool) func() qaloop.Deps {
	return func() qaloop.Deps {
		return qaloop.Deps{
			Run: func(run.Options) (run.Result, error) {
				body, _ := os.ReadFile(filepath.Join(dir, "fixed.txt"))
				if strings.TrimSpace(string(body)) == "ok" {
					return run.Result{Status: "passed", AdapterResults: []run.AdapterResult{{JourneyID: "login", Status: "passed"}}}, nil
				}
				return run.Result{Status: "failed", AdapterResults: []run.AdapterResult{{JourneyID: "login", Status: "failed", FailureSummary: "banner missing"}}}, errors.New("qa run failed")
			},
			Classify: func(in triage.Input) triage.Verdict {
				return triage.Verdict{JourneyID: in.JourneyID, Class: triage.ClassProductDefect, Signal: "fake"}
			},
			LoadInput: func(string, string) (triage.Input, error) { return triage.Input{}, nil },
			Agent: func(context.Context, agentexec.Request) (agentexec.Response, error) {
				if !fix {
					return agentexec.Response{}, nil
				}
				return agentexec.Response{}, os.WriteFile(filepath.Join(dir, "fixed.txt"), []byte("ok\n"), 0o644)
			},
			Compile: func(string) error { return nil },
			GitEnv:  qaLoopTestEnv,
		}
	}
}

func execQALoop(deps func() qaloop.Deps, args ...string) (string, error) {
	cmd := newQALoopCmdWith(deps)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

type qaLoopEnvelope struct {
	Status string        `json:"status"`
	Data   qaloop.Report `json:"data"`
	Error  struct {
		Code string `json:"code"`
	} `json:"error"`
}

// AC-QALOOP-012 through the command surface.
func TestQALoopCmd_JSONEndsPassedOnLoopBranch(t *testing.T) {
	t.Parallel()
	dir := qaLoopRepo(t)

	out, err := execQALoop(qaLoopFakeDeps(dir, true), "--lane", "fast", "--agent", "claude", "--max-iterations", "3", "--project-dir", dir, "--format", "json")

	require.NoError(t, err)
	var envelope qaLoopEnvelope
	require.NoError(t, json.Unmarshal([]byte(out), &envelope), out)
	assert.Equal(t, "ok", envelope.Status)
	assert.Equal(t, qaloop.StopPassed, envelope.Data.StopReason)
	assert.True(t, strings.HasPrefix(envelope.Data.Branch, qaloop.BranchPrefix+"qaloop-"), envelope.Data.Branch)
	assert.True(t, envelope.Data.Restored)
	require.Len(t, envelope.Data.Iterations, 2)
	assert.NotEmpty(t, envelope.Data.Iterations[0].Commit)
	assert.FileExists(t, envelope.Data.ReportPath)
}

// AC-QALOOP-013 through the command surface.
func TestQALoopCmd_DirtyTreeFailsWithCode(t *testing.T) {
	t.Parallel()
	dir := qaLoopRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "fixed.txt"), []byte("wip\n"), 0o644))

	out, err := execQALoop(qaLoopFakeDeps(dir, true), "--agent", "claude", "--project-dir", dir, "--format", "json")

	require.Error(t, err)
	var envelope qaLoopEnvelope
	require.NoError(t, json.Unmarshal([]byte(out), &envelope), out)
	assert.Equal(t, "error", envelope.Status)
	assert.Equal(t, qaloop.CodeDirtyWorktree, envelope.Error.Code)
	assert.Empty(t, envelope.Data.RunID, "no loop branch was created")
}

func TestQALoopCmd_TextNamesBranchAndReportOnNoProgress(t *testing.T) {
	t.Parallel()
	dir := qaLoopRepo(t)

	out, err := execQALoop(qaLoopFakeDeps(dir, false), "--agent", "codex", "--project-dir", dir)

	require.Error(t, err)
	assert.Contains(t, err.Error(), qaloop.CodeNoProgress)
	assert.Contains(t, out, ": no_progress")
	assert.Contains(t, out, "loop branch: none kept (no fix commit; "+qaloop.BranchPrefix)
	assert.Contains(t, out, "report: "+filepath.Join(dir, ".autopus", "qa", "loop"))
}

func TestQALoopCmd_ProductionFlagDefaults(t *testing.T) {
	t.Parallel()
	cmd := newQALoopCmd()
	assert.Equal(t, "loop", cmd.Name())
	for flag, want := range map[string]string{
		"lane": "fast", "max-iterations": "3", "agent-timeout": "15m0s", "project-dir": ".", "format": "text", "agent": "",
	} {
		require.NotNil(t, cmd.Flags().Lookup(flag), flag)
		assert.Equal(t, want, cmd.Flags().Lookup(flag).DefValue, flag)
	}
}

func TestQALoopCmd_RejectsBadFlags(t *testing.T) {
	t.Parallel()
	dir := qaLoopRepo(t)
	for _, args := range [][]string{
		{"--agent", "nope", "--project-dir", dir, "--format", "json"},
		{"--agent", "claude", "--max-iterations", "0", "--project-dir", dir, "--format", "json"},
	} {
		out, err := execQALoop(qaLoopFakeDeps(dir, true), args...)
		require.Error(t, err)
		var envelope qaLoopEnvelope
		require.NoError(t, json.Unmarshal([]byte(out), &envelope), out)
		assert.NotEmpty(t, envelope.Error.Code)
	}
}
