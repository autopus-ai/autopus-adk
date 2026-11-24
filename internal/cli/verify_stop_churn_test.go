package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/verifygate"
)

// Review of #374: QA commands write untracked files under .autopus/qa/ at
// every stop. A hang-blocked tree must stay "not run again, not blocked
// again" after one is written.
func TestVerifyStop_UntrackedQAStateDoesNotReblockAHang(t *testing.T) {
	session := "hang-qa-churn"
	t.Cleanup(func() { resetStopState(session) })
	dir := newStopRepo(t, addSwapped)
	slow := "package stopgate\n\nimport (\n\t\"testing\"\n\t\"time\"\n)\n\nfunc TestSlow(t *testing.T) { time.Sleep(2 * time.Minute) }\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "slow_test.go"), []byte(slow), 0o644))
	first, _ := runStopWithin(t, dir, session, 30*time.Second)
	require.Equal(t, "block", first.Decision, first.SystemMessage)

	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".autopus", "qa", "scenarios"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".autopus", "qa", "scenarios", "round.yaml"), []byte("id: round\n"), 0o644))
	again, took := runStopWithin(t, dir, session, 30*time.Second)
	assert.Empty(t, again.Decision, again.Reason)
	assert.Contains(t, again.SystemMessage, "not run again")
	assert.Less(t, took, 10*time.Second, "the tests were not run again")
}

// Review of #374 (M24): another session's TDD-exempt line is not judged on
// the recorded run; that session runs the tree itself.
func TestVerifyStop_AnotherSessionsExemptLineRunsTheTree(t *testing.T) {
	t.Cleanup(func() { resetStopState("tdd-a"); resetStopState("tdd-b") })
	dir := newStopRepo(t, addSwapped)
	tddProject(t, dir, "tdd", true)
	require.Equal(t, "block", runStopReply(t, dir, "tdd-a", "바꿨습니다.").Decision)

	withoutGo(t)
	got := runStopReply(t, dir, "tdd-b", "정리했습니다.\n\nTDD-exempt: 동작이 같은 순서 변경")
	assert.Equal(t, "block", got.Decision, "the stub go fails, so the tree was run again")
	assert.Contains(t, got.Reason, "not installed")
}

// Review of #374 (M22): a tree that changed while it ran is not recorded,
// since the result is not about the tree now on disk. The failing test
// itself writes a file into the repository.
func TestVerifyStop_ATreeEditedDuringTheRunIsNotRecorded(t *testing.T) {
	dir := newStopRepo(t, "package stopgate\n\nfunc Add(a, b int) int { return a - b }\n")
	writer := "package stopgate\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\n" +
		"func TestWrites(t *testing.T) {\n\tif err := os.WriteFile(\"during.txt\", []byte(\"x\"), 0o644); err != nil {\n\t\tt.Fatal(err)\n\t}\n}\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "writes_test.go"), []byte(writer), 0o644))
	require.Equal(t, "block", runStop(t, dir, "edited-during").Decision)

	root, _, ok := verifygate.ChangedFiles(context.Background(), dir)
	require.True(t, ok)
	_, found := readLastRun(root)
	assert.False(t, found, "the result of a tree edited while it ran is not kept")
}
