package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newStopRepo makes a committed Go module, then applies the uncommitted edit.
func newStopRepo(t *testing.T, edit string) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name, body string) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
	}
	write("go.mod", "module stopgate\n\ngo 1.22\n")
	write("add.go", "package stopgate\n\nfunc Add(a, b int) int { return a + b }\n")
	write("add_test.go", "package stopgate\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fatal(\"Add(1, 2) != 3\")\n\t}\n}\n")
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "base"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	if edit != "" {
		write("add.go", edit)
	}
	return dir
}

func runStop(t *testing.T, dir, session string) stopHookOutput {
	t.Helper()
	cmd := newVerifyStopCmd()
	payload, _ := json.Marshal(stopHookInput{SessionID: session, Cwd: dir})
	cmd.SetIn(bytes.NewReader(payload))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--budget", (3 * time.Minute).String()})
	require.NoError(t, cmd.Execute())
	var got stopHookOutput
	require.NoError(t, json.Unmarshal(out.Bytes(), &got), out.String())
	return got
}

func TestVerifyStop_BlocksOnFailingTestThenGivesUp(t *testing.T) {
	dir := newStopRepo(t, "package stopgate\n\nfunc Add(a, b int) int { return a - b }\n")
	session := "test-" + strings.ReplaceAll(t.Name(), "/", "_")
	t.Cleanup(func() { resetStopState(session) })

	for i := 0; i < verifyStopMaxBlocks; i++ {
		got := runStop(t, dir, session)
		assert.Equal(t, "block", got.Decision)
		assert.Contains(t, got.Reason, "go test .")
		assert.Contains(t, got.Reason, "Add(1, 2) != 3")
	}
	last := runStop(t, dir, session)
	assert.Empty(t, last.Decision, "the session may end after the block limit")
	assert.Contains(t, last.SystemMessage, "still failing")
}

func TestVerifyStop_AllowsPassingAndUnchangedTrees(t *testing.T) {
	t.Cleanup(func() { resetStopState("pass"); resetStopState("clean") })
	passing := newStopRepo(t, "package stopgate\n\n// Add sums.\nfunc Add(a, b int) int { return b + a }\n")
	assert.Equal(t, stopHookOutput{}, runStop(t, passing, "pass"))

	assert.Equal(t, stopHookOutput{}, runStop(t, newStopRepo(t, ""), "clean"))
}

func TestVerifyStop_SkipsATreeThatAlreadyPassedButRechecksAnEdit(t *testing.T) {
	session := "recheck"
	t.Cleanup(func() { resetStopState(session) })
	dir := newStopRepo(t, "package stopgate\n\n// Add sums.\nfunc Add(a, b int) int { return b + a }\n")
	require.Equal(t, stopHookOutput{}, runStop(t, dir, session))
	require.NotEmpty(t, readStopState(session, "pass"))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "add.go"), []byte("package stopgate\n\nfunc Add(a, b int) int { return a * b }\n"), 0o644))
	assert.Equal(t, "block", runStop(t, dir, session).Decision, "an edit after the pass is verified again")
}

func TestVerifyStop_OffSwitch(t *testing.T) {
	t.Setenv("AUTOPUS_VERIFY_STOP", "off")
	dir := newStopRepo(t, "package stopgate\n\nfunc Add(a, b int) int { return a - b }\n")

	assert.Equal(t, stopHookOutput{}, runStop(t, dir, "off"))
}

func TestVerifyStop_LargePassingChangeGoesOnToReviewAndQAOnce(t *testing.T) {
	session := "followup"
	t.Cleanup(func() { resetStopState(session); _ = os.Remove(stopStateFile(session, "followup")) })
	var body strings.Builder
	body.WriteString("package stopgate\n\n// Add sums.\nfunc Add(a, b int) int { return a + b }\n")
	for i := 0; i < 45; i++ {
		body.WriteString("\n// Filler documents nothing in particular.\n")
	}
	dir := newStopRepo(t, body.String())

	first := runStop(t, dir, session)
	assert.Equal(t, "block", first.Decision)
	assert.Contains(t, first.Reason, "qa-explorer")
	assert.Contains(t, first.Reason, "AskUserQuestion")

	assert.Equal(t, stopHookOutput{}, runStop(t, dir, session), "the same tree is not sent round again")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "add.go"), []byte(body.String()+"\n// one more line\n"), 0o644))
	assert.Equal(t, stopHookOutput{}, runStop(t, dir, session), "a small fix after the review does not start another round")
}

func TestVerifyStop_SmallPassingChangeEndsAtGreen(t *testing.T) {
	session := "small"
	t.Cleanup(func() { resetStopState(session); _ = os.Remove(stopStateFile(session, "followup")) })
	dir := newStopRepo(t, "package stopgate\n\n// Add sums.\nfunc Add(a, b int) int { return b + a }\n")

	assert.Equal(t, stopHookOutput{}, runStop(t, dir, session))
}
