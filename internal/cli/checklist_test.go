package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/checklist"
	"github.com/insajin/autopus-adk/pkg/learn"
)

func runAuto(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

// The loop through the real commands: record, propose, refute with a
// different provider, approve, then run in observe and enforce.
func TestChecklistCommands_EndToEnd(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	git("init", "-q")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "a.go"), []byte("ok\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "guard.sh"), []byte("#!/bin/sh\ngrep -q BUG src/a.go && exit 1; exit 0\n"), 0o755))
	git("add", "-A")
	git("commit", "-qm", "init")
	t.Chdir(dir)

	prevP, prevA := checklistProviders, checklistAsker
	checklistProviders = func() ([]string, error) { return []string{"claude", "codex"}, nil }
	checklistAsker = func(string) checklist.Asker {
		return func(context.Context, string, string) (string, error) {
			return "VERDICT: survived\nREASON: specific", nil
		}
	}
	t.Cleanup(func() { checklistProviders, checklistAsker = prevP, prevA })

	// The drafter is named so the result does not depend on whether the test
	// itself runs inside an agent session (CLAUDECODE and friends).
	out, err := runAuto(t, "checklist", "record", "--pattern", "guard regressed", "--missed-by", "unit tests",
		"--provider", "claude", "--when-path", "src/**", "--evidence-command", "./guard.sh", "--evidence-before-exit", "1", "--evidence-after-exit", "0")
	require.NoError(t, err, out)
	assert.Contains(t, out, "next: auto checklist propose")
	_, err = runAuto(t, "checklist", "record", "--pattern", "p", "--missed-by", "lint")
	assert.ErrorIs(t, err, learn.ErrEscapeIncomplete, "an escape needs a diff condition")
	_, err = runAuto(t, "learn", "record", "--type", "escape", "--pattern", "p")
	assert.Error(t, err, "the hidden learn entry checks the same fields")

	out, err = runAuto(t, "checklist", "propose")
	require.NoError(t, err, out)
	cands, err := checklist.LoadCandidates(dir)
	require.NoError(t, err)
	require.Len(t, cands, 1)
	id := cands[0].ID
	assert.Regexp(t, `^CHK-001-[0-9a-f]{6}$`, id, "a new candidate carries the branch-unique suffix")
	assert.Contains(t, out, id+" 반증 검토 대기")

	out, err = runAuto(t, "checklist", "refute", "--all")
	require.NoError(t, err, out)
	assert.Contains(t, out, id+" 승인 대기")
	assert.Contains(t, out, "codex: 반증 실패")
	assert.NotContains(t, out, "claude:")

	out, err = runAuto(t, "checklist", "approve", id)
	require.NoError(t, err, out)
	assert.Contains(t, out, "관찰 모드로 추가")
	git("add", "-A")
	git("commit", "-qm", "approve")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "a.go"), []byte("BUG\n"), 0o644))
	// M3: run from a subdirectory still finds the project's checks.
	t.Chdir(filepath.Join(dir, "src"))
	out, err = runAuto(t, "checklist", "run")
	t.Chdir(dir)
	require.NoError(t, err, out)
	assert.Contains(t, out, "관찰: 문제를 잡았습니다")

	out, err = runAuto(t, "checklist", "promote", id)
	require.NoError(t, err, out)
	git("add", ".autopus/checklist")
	git("commit", "-qm", "promote", "--", ".autopus/checklist")
	out, err = runAuto(t, "checklist", "run")
	assert.ErrorIs(t, err, errChecklistFailed, out)
	assert.Contains(t, out, id+" [강제 모드] 실패")

	out, err = runAuto(t, "checklist", "status")
	require.NoError(t, err, out)
	assert.Contains(t, out, "적용 2회, 잡음 2회")
	assert.Contains(t, out, "승인 대기열: 0개")
}

// M9: the drafter comes from the session environment; unknown stays empty.
func TestDetectAgentProvider(t *testing.T) {
	assert.Equal(t, "claude", detectAgentProvider([]string{"CLAUDECODE=1"}))
	assert.Equal(t, "codex", detectAgentProvider([]string{"CODEX_SANDBOX=seatbelt"}))
	assert.Equal(t, "gemini", detectAgentProvider([]string{"GEMINI_CLI=1"}))
	assert.Equal(t, "x", detectAgentProvider([]string{"AUTOPUS_PROVIDER=x", "CLAUDECODE=1"}))
	assert.Equal(t, "", detectAgentProvider([]string{"HOME=/h"}))
	assert.Equal(t, "", detectAgentProvider([]string{"CODEX_HOME=/h"}), "a user export is not a session")
	assert.Equal(t, "gemini", detectAgentProvider([]string{"CODEX_HOME=/h", "GEMINI_CLI=1"}))
}
