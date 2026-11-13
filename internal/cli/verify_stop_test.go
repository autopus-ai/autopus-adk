package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/verifygate"
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

// largeStopChange is a passing edit of more than followupFullLines code lines.
func largeStopChange() string {
	var body strings.Builder
	body.WriteString("package stopgate\n\n// Add sums.\nfunc Add(a, b int) int { return a + b }\n")
	for i := 0; i < 45; i++ {
		fmt.Fprintf(&body, "\nvar filler%d = %d\n", i, i)
	}
	return body.String()
}

// The verification round is on by default; AUTOPUS_FOLLOWUP=off turns it off.
func TestVerifyStop_FollowupIsOnByDefault(t *testing.T) {
	optInFollowup(t, "")
	session := "followup-default"
	t.Cleanup(func() { resetStopState(session); _ = os.Remove(stopStateFile(session, "followup")) })
	dir := newStopRepo(t, largeStopChange())

	assert.Equal(t, "block", runStop(t, dir, session).Decision)
}

func TestVerifyStop_FollowupEnabled(t *testing.T) {
	for _, tc := range []struct {
		env  string
		want bool
	}{
		{"", true}, {"on", true}, {"ON", true}, {"off", false}, {"Off", false}, {"0", false}, {"false", false}, {"maybe", true},
	} {
		t.Setenv("AUTOPUS_FOLLOWUP", tc.env)
		assert.Equal(t, tc.want, followupEnabled(false), "env=%q", tc.env)
	}
}

func TestVerifyStop_LargePassingChangeGetsTheFullRoundOnce(t *testing.T) {
	optInFollowup(t, "")
	session := "followup"
	t.Cleanup(func() { resetStopState(session); _ = os.Remove(stopStateFile(session, "followup")) })
	body := largeStopChange()
	dir := newStopRepo(t, body)

	first := runStop(t, dir, session)
	assert.Equal(t, "block", first.Decision)
	assert.Contains(t, first.Reason, "full verification round")
	assert.Contains(t, first.Reason, "qa-explorer")
	assert.Contains(t, first.Reason, "AskUserQuestion")
	assert.Contains(t, first.Reason, "auto qa intent", "user scenarios come before the hunt")
	assert.Contains(t, first.Reason, "--no-loop", "the tree is dirty when the hook fires, and the loop refuses a dirty tree")
	assert.Contains(t, first.Reason, "policy_mismatch")

	assert.Equal(t, stopHookOutput{}, runStop(t, dir, session), "the same tree is not sent round again")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "add.go"), []byte(body+"\nvar more = 1\n"), 0o644))
	assert.Equal(t, stopHookOutput{}, runStop(t, dir, session), "a fix after the round does not start another")
}

// A one-line logic change is exactly where a written policy breaks, so it is
// verified too, only less deeply.
func TestVerifyStop_SmallBehaviorChangeGetsTheLightRound(t *testing.T) {
	optInFollowup(t, "")
	session := "small"
	t.Cleanup(func() { resetStopState(session); _ = os.Remove(stopStateFile(session, "followup")) })
	dir := newStopRepo(t, "package stopgate\n\n// Add sums.\nfunc Add(a, b int) int { return b + a }\n")

	out := runStop(t, dir, session)
	assert.Equal(t, "block", out.Decision)
	assert.Contains(t, out.Reason, "light verification round")
	assert.Contains(t, out.Reason, "auto qa exec --all")
	assert.NotContains(t, out.Reason, "auto qa intent", "a small change does not generate new scenarios")
}

func TestVerifyStop_CommentOnlyChangeEndsAtGreen(t *testing.T) {
	optInFollowup(t, "")
	session := "comment-only"
	t.Cleanup(func() { resetStopState(session); _ = os.Remove(stopStateFile(session, "followup")) })
	dir := newStopRepo(t, "package stopgate\n\n// Add sums two numbers.\nfunc Add(a, b int) int { return a + b }\n")

	assert.Equal(t, stopHookOutput{}, runStop(t, dir, session), "a documented function behaves the same")
}

func TestVerifyStop_FollowupOffEndsAtGreen(t *testing.T) {
	optInFollowup(t, "off")
	session := "followup-off"
	t.Cleanup(func() { resetStopState(session); _ = os.Remove(stopStateFile(session, "followup")) })
	dir := newStopRepo(t, largeStopChange())

	assert.Equal(t, stopHookOutput{}, runStop(t, dir, session))
}

func TestVerifyStop_QAOutputAndLogsDoNotGrowTheChange(t *testing.T) {
	optInFollowup(t, "")
	session := "qa-output"
	t.Cleanup(func() { resetStopState(session); _ = os.Remove(stopStateFile(session, "followup")) })
	dir := newStopRepo(t, "package stopgate\n\n// Add sums.\nfunc Add(a, b int) int { return b + a }\n")
	big := strings.Repeat("line\n", 500)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".autopus", "qa", "hunt"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".autopus", "qa", "hunt", "memory.md"), []byte(big), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "run.log"), []byte(big), 0o644))

	assert.Contains(t, runStop(t, dir, session).Reason, "light verification round", "only product code counts toward the round")
}

func TestVerifyStop_FollowupRoundsAreBounded(t *testing.T) {
	useStopStateDir(t, t.TempDir())
	root := t.TempDir()
	assert.Equal(t, followupNone, nextFollowup("rounds", root, "fp1", followupNone), "no behavior change, no round, and no round used up")
	assert.Equal(t, followupLight, nextFollowup("rounds", root, "fp1", followupLight))
	assert.Equal(t, followupNone, nextFollowup("rounds", root, "fp2", followupFull), "a second round never starts")
}

// A tree one session already sent round is not sent again by the next
// session, so a session that only asks a question is not blocked by
// uncommitted work it did not make; a changed tree is.
func TestVerifyStop_FollowupSkipsATreeAnotherSessionVerified(t *testing.T) {
	useStopStateDir(t, t.TempDir())
	root := t.TempDir()
	assert.Equal(t, followupFull, nextFollowup("session-a", root, "fp1", followupFull))
	assert.Equal(t, followupNone, nextFollowup("session-b", root, "fp1", followupFull))
	assert.Equal(t, followupLight, nextFollowup("session-c", root, "fp2", followupLight))
}

// When the round cannot be recorded it would block every stop forever, so
// the hook fails open.
func TestVerifyStop_FollowupFailsOpenWhenStateCannotBeWritten(t *testing.T) {
	dir := t.TempDir()
	unsafe := filepath.Join(dir, "state")
	require.NoError(t, os.Symlink(t.TempDir(), unsafe))
	useStopStateDir(t, unsafe)
	assert.Equal(t, followupNone, nextFollowup("ro", t.TempDir(), "fp", followupFull))
}

func TestFollowupTierFor(t *testing.T) {
	t.Parallel()
	assert.Equal(t, followupNone, followupTierFor(verifygate.Behavior{}))
	assert.Equal(t, followupLight, followupTierFor(verifygate.Behavior{Lines: 1}))
	assert.Equal(t, followupFull, followupTierFor(verifygate.Behavior{Lines: followupFullLines}))
	assert.Equal(t, followupFull, followupTierFor(verifygate.Behavior{Lines: 2, NewFiles: []string{"cmd/new.go"}}))
	assert.Equal(t, followupFull, followupTierFor(verifygate.Behavior{Unmeasured: true}), "a diff git could not produce is not nothing")
}

// A shell script has no build toolchain, but its logic is still behavior.
func TestVerifyStop_UnbuiltLanguageStillGetsTheRound(t *testing.T) {
	optInFollowup(t, "")
	session := "unbuilt"
	t.Cleanup(func() { resetStopState(session); _ = os.Remove(stopStateFile(session, "followup")) })
	dir := newStopRepo(t, "")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "refund.sh"), []byte("#!/usr/bin/env bash\n[ \"$1\" -le 14 ] && echo approved\n"), 0o755))

	out := runStop(t, dir, session)
	assert.Equal(t, "block", out.Decision)
	assert.Contains(t, out.Reason, "full verification round", "a new product file")
	assert.Contains(t, out.Reason, "No build or test toolchain covers these files")
}

// The round tells the session to fix what it found, so the fixed tree is the
// verified one: a later session that only asks a question is not sent round
// on it again.
func TestVerifyStop_FixedTreeIsNotSentRoundAgainByTheNextSession(t *testing.T) {
	optInFollowup(t, "")
	body := largeStopChange()
	dir := newStopRepo(t, body)
	assert.Equal(t, "block", runStop(t, dir, "fix-a").Decision)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "add.go"), []byte(body+"\nvar fixed = 1\n"), 0o644))
	assert.Equal(t, stopHookOutput{}, runStop(t, dir, "fix-a"), "the fix does not start a second round")
	assert.Equal(t, stopHookOutput{}, runStop(t, dir, "fix-b"), "nor does the next session on the fixed tree")
}
