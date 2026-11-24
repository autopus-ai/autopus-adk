package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/verifygate"
)

const addMinus = "package stopgate\n\nfunc Add(a, b int) int { return a - b }\n"

// withoutGo puts a go first on PATH that answers `go env` (part of the tree
// hash) and nothing else: any other go command, a rerun of the tests, prints
// "not installed" and fails.
func withoutGo(t *testing.T) {
	t.Helper()
	realGo, err := exec.LookPath("go")
	require.NoError(t, err)
	bin := t.TempDir()
	script := "#!/bin/sh\nif [ \"$1\" = env ]; then exec " + realGo + " \"$@\"; fi\necho 'go is not installed (test stub)'\nexit 1\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "go"), []byte(script), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// Pilot C c01: three blocks of about 5.5 minutes each on a tree nothing
// edited between stops. The same tree is blocked once; the next stop on it
// goes through with the result as a note, without running the tests again.
func TestVerifyStop_AnUnchangedTreeIsBlockedOnce(t *testing.T) {
	dir := newStopRepo(t, addMinus)
	session := "block-once"
	first := runStop(t, dir, session)
	require.Equal(t, "block", first.Decision)
	assert.Contains(t, first.Reason, "auto verify stop --last")

	withoutGo(t)
	again := runStop(t, dir, session)
	assert.Empty(t, again.Decision, "the same tree is not blocked twice")
	assert.Contains(t, again.SystemMessage, "not blocked again")
	assert.Contains(t, again.SystemMessage, "not verified")
	assert.Contains(t, again.SystemMessage, "Verification failed", "the earlier result is repeated")
	assert.NotContains(t, again.SystemMessage, "not installed", "the tests were not run again")
}

// Fail closed: an edit that still fails is a new tree and blocks once more.
func TestVerifyStop_AnEditThatStillFailsBlocksAgain(t *testing.T) {
	dir := newStopRepo(t, addMinus)
	session := "block-edit"
	require.Equal(t, "block", runStop(t, dir, session).Decision)
	require.Empty(t, runStop(t, dir, session).Decision)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "add.go"), []byte("package stopgate\n\nfunc Add(a, b int) int { return a * b }\n"), 0o644))
	assert.Equal(t, "block", runStop(t, dir, session).Decision)
}

// Another session runs a passed tree again: the hash cannot see ignored
// files or environment variables, which may differ for it (review of #371).
func TestVerifyStop_AnotherSessionRunsAPassedTreeAgain(t *testing.T) {
	dir := newStopRepo(t, addSwapped)
	require.Equal(t, stopHookOutput{}, runStop(t, dir, "first"))
	withoutGo(t)
	second := runStop(t, dir, "second")
	assert.Equal(t, "block", second.Decision, "the stub go fails, so the tests were run again")
	assert.Contains(t, second.Reason, "not installed")
}

// The agent reads the hook's result instead of rerunning the package.
func TestVerifyStop_LastPrintsTheResult(t *testing.T) {
	dir := newStopRepo(t, addMinus)
	require.Equal(t, "block", runStop(t, dir, "last").Decision)
	wd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(wd) })

	cmd := newVerifyStopCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--last"})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, out.String(), "block")
	assert.Contains(t, out.String(), "Add(1, 2) != 3")
	assert.Contains(t, out.String(), "unchanged since")
}

// Pilot C c03/c04/c06: the budget went to checking failures on HEAD, all
// pre-existing, and the run was then reported as not finished. That does not
// block, is not recorded (the next stop runs the rest), and is never the
// tree's one block.
func TestVerifyStop_TestsOwedAfterABaselineCheckAreRunNextStop(t *testing.T) {
	isolateStop(t)
	dir := newStopRepo(t, addSwapped)
	root, changed, ok := verifygate.ChangedFiles(context.Background(), dir)
	require.True(t, ok)
	session := "owed"
	tree := verifygate.TreeHash(context.Background(), root, "1m0s")
	run := verifygate.ChargedRun{Preexisting: []string{"stopgate.TestEnv"}, Skipped: []verifygate.Skip{{Name: "go test", Budget: true,
		Baseline: true, Reason: "stopped early to check its 1 failed test(s) against HEAD"}}}
	cmd := newVerifyStopCmd()
	cmd.SetContext(context.Background())
	in := stopHookInput{SessionID: session, Cwd: dir}

	out := judgeRun(cmd, in, root, changed, tree, run, time.Now(), time.Minute, false)
	assert.Empty(t, out.Decision)
	assert.Contains(t, out.SystemMessage, "run at the next stop")
	assert.NotContains(t, out.SystemMessage, "did not finish within the budget")
	assert.Empty(t, readStopState(session, "budget"), "the tree is not recorded, so the next stop runs the rest")

	recordLastRun(session, root, tree, time.Minute, run, stopHookOutput{Decision: "block", Reason: "round"}, true)
	last, found := readLastRun(root)
	require.True(t, found)
	assert.Equal(t, lastPartial, last.Kind, "a round block here is not the tree's one block")
}

// Review B1: deleting a tracked file the change used breaks the build; that
// is a new tree, not the one blocked before.
func TestVerifyStop_DeletingATrackedFileIsANewTree(t *testing.T) {
	dir := newStopRepo(t, "")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "helper.go"), []byte("package stopgate\n\nfunc Helper() int { return 0 }\n"), 0o644))
	commitAll(t, dir, "helper")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "add.go"), []byte("package stopgate\n\nfunc Add(a, b int) int { return a - b + Helper() }\n"), 0o644))
	require.Equal(t, "block", runStop(t, dir, "del").Decision)
	require.NoError(t, os.Remove(filepath.Join(dir, "helper.go")))
	again := runStop(t, dir, "del")
	assert.Equal(t, "block", again.Decision, again.SystemMessage)
	assert.Contains(t, again.Reason, "Helper")
}
