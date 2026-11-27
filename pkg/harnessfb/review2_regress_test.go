package harnessfb

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlockKind_BudgetFlag(t *testing.T) {
	assert.Equal(t, "verify", blockKind("auto verify stop --budget 15m0s", "Verification failed: x"))
	assert.Equal(t, "followup", blockKind("/opt/bin/auto verify stop --budget 15m0s", "pass; light verification round."))
	assert.Equal(t, "language", blockKind("auto verify language --x", "y"))
}

// Reviews run side by side count their wall time once.
func TestReadClaudeTranscript_ParallelReviewTimeCountsOnce(t *testing.T) {
	x := &tx{}
	x.prompt(0, "go")
	for _, id := range []string{"r1", "r2", "r3"} {
		x.use(1, id, "Agent", map[string]any{"subagent_type": "reviewer"})
	}
	x.result(201, "r1", "ok").result(201, "r2", "ok").result(201, "r3", "ok")
	x.prompt(1100, "next")
	path := filepath.Join(t.TempDir(), "s.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(x.text()), 0o600))
	s := Session{}
	require.NoError(t, ReadClaudeTranscript(path, &s))
	assert.Equal(t, 3, s.Reviews)
	assert.InDelta(t, 200, s.ReviewSeconds, 0.01)
	assert.Empty(t, Detect([]Metrics{s.Metrics}), "18% of the session is not review-heavy")
}

// A rule file a team committed, or one a person wrote, is not this
// machine's to rewrite or delete.
func TestSyncRules_LeavesFilesItDoesNotOwn(t *testing.T) {
	root := gitRepo(t)
	rule := filepath.Join(root, RuleFile)
	require.NoError(t, os.MkdirAll(filepath.Dir(rule), 0o755))

	require.NoError(t, os.WriteFile(rule, []byte("my own notes\n"), 0o644))
	require.NoError(t, SyncRules(root, root))
	got, _ := os.ReadFile(rule)
	assert.Equal(t, "my own notes\n", string(got))

	require.NoError(t, os.WriteFile(rule, []byte(ruleHeader+"- HL-009 (x, 3 sessions): team rule\n"), 0o644))
	out, err := exec.Command("git", "-C", root, "add", "-f", RuleFile).CombinedOutput()
	require.NoError(t, err, string(out))
	require.NoError(t, SyncRules(root, root))
	assert.FileExists(t, rule, "a tracked rule file is not deleted on a machine with no approvals")

	out, err = exec.Command("git", "-C", root, "rm", "-q", "--cached", RuleFile).CombinedOutput()
	require.NoError(t, err, string(out))
	require.NoError(t, SyncRules(root, root))
	assert.NoFileExists(t, rule, "an untracked file this loop wrote follows the approvals")
}

// A draft that could not be saved is not paid for again at every stop.
func TestPropose_UnsavableDraftIsNotRepaid(t *testing.T) {
	root := gitRepo(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".autopus", "harness"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "cands"), 0o755))
	require.NoError(t, os.Symlink("../../cands", filepath.Join(root, CandidatesDir)))
	var calls []string
	for i := 0; i < 3; i++ {
		_, errs := Propose(context.Background(), root, repeated(), []string{"claude"}, fakeAsker("survived", &calls), t0, Options{Auto: true})
		assert.NotEmpty(t, errs)
	}
	assert.Empty(t, calls, "the unwritable candidates directory is found before any model is asked")
}

func TestFailures_CorruptCountDoesNotPanic(t *testing.T) {
	f := failures{"x": {Count: 0, Last: t0}}
	assert.True(t, f.due("x", t0))
	f = failures{"x": {Count: -3, Last: t0}}
	assert.True(t, f.due("x", t0))
}

// A foreground test inside a longer background suite does not cut the
// suite's time short, and a test longer than the idle gap still counts.
func TestReadClaudeTranscript_NestedAndLongTests(t *testing.T) {
	x := &tx{}
	x.prompt(0, "go")
	x.use(0, "bg", "Bash", map[string]any{"command": "go test ./...", "run_in_background": true}).
		result(1, "bg", "Command running in background with ID: b1.")
	x.use(300, "fg", "Bash", map[string]any{"command": "go test ./pkg/x"}).result(360, "fg", "ok")
	x.notify(600, "bg")
	x.use(610, "long", "Bash", map[string]any{"command": "go test ./slow"}).result(610+45*60, "long", "ok")
	x.prompt(610+45*60+5, "next")
	path := filepath.Join(t.TempDir(), "s.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(x.text()), 0o600))
	s := Session{}
	require.NoError(t, ReadClaudeTranscript(path, &s))
	assert.InDelta(t, 600+45*60, s.TestWaitSeconds, 0.01)
	assert.InDelta(t, 610+45*60+5, s.ActiveSeconds, 0.01)
}

// The redo of a block starts when the blocked stop's last hook finished.
func TestReadClaudeTranscript_RedoStartsAfterTheBlockedStop(t *testing.T) {
	x := &tx{}
	x.prompt(0, "go")
	x.stopBlock(10, "language.ai_responses is ko")
	x.stopOK(490, "stop1") // the verification gate of the same stop ran 8 minutes
	x.stopOK(520, "stop2")
	path := filepath.Join(t.TempDir(), "s.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(x.text()), 0o600))
	s := Session{}
	require.NoError(t, ReadClaudeTranscript(path, &s))
	assert.InDelta(t, 30, s.StopBlockSeconds["language"], 0.01)
}
