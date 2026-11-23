package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// envBrokenTest stands for a test the machine cannot pass (no tool, no
// network): committed failing, it fails whatever the change is.
const envBrokenTest = "package stopgate\n\nimport \"testing\"\n\nfunc TestNeedsTool(t *testing.T) {\n\tt.Fatal(\"the tool is not installed\")\n}\n"

// newBrokenBaseRepo is newStopRepo whose committed base already has a
// failing test.
func newBrokenBaseRepo(t *testing.T, edit string) string {
	t.Helper()
	dir := newStopRepo(t, "")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "env_test.go"), []byte(envBrokenTest), 0o644))
	for _, args := range [][]string{{"add", "-A"}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "broken test"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "add.go"), []byte(edit), 0o644))
	return dir
}

// A test that fails before the change as well is not charged to it.
func TestVerifyStop_APreexistingFailureDoesNotBlock(t *testing.T) {
	session := "baseline-preexisting"
	t.Cleanup(func() { resetStopState(session) })
	dir := newBrokenBaseRepo(t, "package stopgate\n\n// Add sums two ints.\nfunc Add(a, b int) int { return a + b }\n")

	got := runStopReply(t, dir, session, "주석을 고쳤습니다.")
	assert.Empty(t, got.Decision, got.Reason)
	assert.Contains(t, got.SystemMessage, "also fail on HEAD")
	assert.Contains(t, got.SystemMessage, "stopgate.TestNeedsTool")
	assert.Empty(t, readStopState(session, "blocks"))
}

// A failure the change caused still blocks, and the pre-existing one is
// named as not charged.
func TestVerifyStop_AFailureTheChangeCausedStillBlocks(t *testing.T) {
	session := "baseline-caused"
	t.Cleanup(func() { resetStopState(session) })
	dir := newBrokenBaseRepo(t, "package stopgate\n\nfunc Add(a, b int) int { return a - b }\n")

	got := runStopReply(t, dir, session, "Add를 바꿨습니다.")
	assert.Equal(t, "block", got.Decision)
	assert.Contains(t, got.Reason, "TestAdd")
	assert.Contains(t, got.Reason, "not charged to it: stopgate.TestNeedsTool")
}

// A failing test the change edited is its failure even if it failed before.
func TestVerifyStop_AnEditedFailingTestBlocks(t *testing.T) {
	session := "baseline-edited"
	t.Cleanup(func() { resetStopState(session) })
	dir := newBrokenBaseRepo(t, "package stopgate\n\nfunc Add(a, b int) int { return a + b }\n")
	edited := "package stopgate\n\nimport \"testing\"\n\nfunc TestNeedsTool(t *testing.T) {\n\tt.Fatal(\"still not installed\")\n}\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "env_test.go"), []byte(edited), 0o644))

	got := runStopReply(t, dir, session, "테스트를 고쳤습니다.")
	assert.Equal(t, "block", got.Decision)
	assert.Contains(t, got.Reason, "were edited by this change")
}
