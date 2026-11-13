package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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

var (
	isolatedStopTests sync.Map
	followupOptIns    sync.Map
)

// isolateStop gives the test its own stop-state directory, once per test so
// several stops in one test share it, and turns the default-on verification
// round off unless the test opted in with optInFollowup. Leftover state in
// the real cache, or AUTOPUS_FOLLOWUP in the developer's shell, must not
// decide another gate's test.
func isolateStop(t *testing.T) {
	t.Helper()
	if _, loaded := isolatedStopTests.LoadOrStore(t.Name(), true); !loaded {
		useStopStateDir(t, t.TempDir())
		t.Cleanup(func() { isolatedStopTests.Delete(t.Name()) })
	}
	if _, on := followupOptIns.Load(t.Name()); !on {
		t.Setenv("AUTOPUS_FOLLOWUP", "off")
	}
}

// optInFollowup lets the test's own AUTOPUS_FOLLOWUP value reach the hook.
func optInFollowup(t *testing.T, env string) {
	t.Helper()
	followupOptIns.Store(t.Name(), true)
	t.Cleanup(func() { followupOptIns.Delete(t.Name()) })
	t.Setenv("AUTOPUS_FOLLOWUP", env)
}

func runStop(t *testing.T, dir, session string) stopHookOutput {
	t.Helper()
	isolateStop(t)
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
