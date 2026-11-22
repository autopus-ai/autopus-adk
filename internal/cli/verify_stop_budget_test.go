package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runStopWithin(t *testing.T, dir, session string, budget time.Duration) (stopHookOutput, time.Duration) {
	t.Helper()
	return runStopContext(t, context.Background(), dir, session, budget)
}

func runStopContext(t *testing.T, ctx context.Context, dir, session string, budget time.Duration) (stopHookOutput, time.Duration) {
	t.Helper()
	isolateStop(t)
	cmd := newVerifyStopCmd()
	payload, _ := json.Marshal(stopHookInput{SessionID: session, Cwd: dir})
	cmd.SetIn(bytes.NewReader(payload))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--budget", budget.String()})
	start := time.Now()
	require.NoError(t, cmd.ExecuteContext(ctx))
	var got stopHookOutput
	require.NoError(t, json.Unmarshal(out.Bytes(), &got), out.String())
	return got, time.Since(start)
}

// longSuite is a test file whose tests each finish quickly but together
// outlast the budget: a large package, not a hung test.
func longSuite() string {
	var b strings.Builder
	b.WriteString("package stopgate\n\nimport (\n\t\"testing\"\n\t\"time\"\n)\n\n")
	for i := 0; i < 600; i++ {
		fmt.Fprintf(&b, "func TestShort%d(t *testing.T) { time.Sleep(100 * time.Millisecond) }\n", i)
	}
	return b.String()
}

func commitAll(t *testing.T, dir, msg string) {
	t.Helper()
	for _, args := range [][]string{{"add", "-A"}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", msg}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
}

const addSwapped = "package stopgate\n\n// Add sums.\nfunc Add(a, b int) int { return b + a }\n"

// A package whose suite outlasts the budget, with nothing failing, is not a
// failing change: the stop goes through, and the same tree is not run again
// at the next stop, which repeats the note instead of claiming a pass.
func TestVerifyStop_SuiteLongerThanTheBudgetDoesNotBlock(t *testing.T) {
	session := "budget-long"
	t.Cleanup(func() { resetStopState(session) })
	dir := newStopRepo(t, addSwapped)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "slow_test.go"), []byte(longSuite()), 0o644))

	got, _ := runStopWithin(t, dir, session, 30*time.Second)
	assert.Empty(t, got.Decision, got.Reason)
	assert.Contains(t, got.SystemMessage, "nothing failed within the budget")

	again, took := runStopWithin(t, dir, session, 30*time.Second)
	assert.Empty(t, again.Decision)
	assert.Contains(t, again.SystemMessage, "did not finish", "the unfinished suite is still reported")
	assert.Less(t, took, 10*time.Second, "the same tree is not run again")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "add.go"), []byte("package stopgate\n\nfunc Add(a, b int) int { return a - b }\n"), 0o644))
	edited, _ := runStopWithin(t, dir, session, 30*time.Second)
	assert.Equal(t, "block", edited.Decision, "an edit is verified again, and TestAdd fails inside the budget")
	assert.Contains(t, edited.Reason, "Add(1, 2) != 3")
}

// A deadlocked test (one that already reported an error and never returns)
// emits no fail event before the budget runs out. It must block, naming the
// test, and the tree must not be recorded.
func TestVerifyStop_DeadlockedTestBlocks(t *testing.T) {
	session := "budget-deadlock"
	t.Cleanup(func() { resetStopState(session) })
	dir := newStopRepo(t, "")
	deadlock := "package stopgate\n\nimport (\n\t\"sync\"\n\t\"testing\"\n)\n\n" +
		"func TestDeadlock(t *testing.T) {\n\tt.Error(\"half done\")\n\tvar mu sync.Mutex\n\tmu.Lock()\n\tmu.Lock()\n}\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "deadlock_test.go"), []byte(deadlock), 0o644))

	got, _ := runStopWithin(t, dir, session, 40*time.Second)
	assert.Equal(t, "block", got.Decision, got.SystemMessage)
	assert.Contains(t, got.Reason, "TestDeadlock")
	assert.Contains(t, got.Reason, "hooks.verify_stop_budget", "a legitimately slow test needs a budget change that lasts")
	assert.Contains(t, got.Reason, "next session")
	assert.Empty(t, readStopState(session, "budget"), "a hung tree is not recorded")
	assert.Empty(t, readStopState(session, "pass"))
	assertSuspicionBlocksOnce(t, dir, session, 40*time.Second)
}

// A suspected hang blocks once per tree: stopping again on the same tree
// lets the session end with the note, runs nothing, and records no pass.
func assertSuspicionBlocksOnce(t *testing.T, dir, session string, budget time.Duration) {
	t.Helper()
	again, took := runStopWithin(t, dir, session, budget)
	assert.Empty(t, again.Decision, again.Reason)
	assert.Contains(t, again.SystemMessage, "not verified")
	assert.Less(t, took, 10*time.Second, "the same tree is not run again")
	assert.Empty(t, readStopState(session, "pass"))
	assert.Empty(t, readStopState(session, "budget"))
}

// RG1d/P1: a legitimately slow test looks like a hang from outside. It
// blocks once; the next stop lets it through; an edit re-arms the block.
func TestVerifyStop_SlowTestBlocksOncePerTree(t *testing.T) {
	session := "budget-slow-once"
	t.Cleanup(func() { resetStopState(session) })
	dir := newStopRepo(t, "")
	slow := "package stopgate\n\nimport (\n\t\"testing\"\n\t\"time\"\n)\n\nfunc TestSlow(t *testing.T) { time.Sleep(2 * time.Minute) }\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "slow_test.go"), []byte(slow), 0o644))

	first, _ := runStopWithin(t, dir, session, 30*time.Second)
	assert.Equal(t, "block", first.Decision, first.SystemMessage)
	assert.Contains(t, first.Reason, "TestSlow")
	assertSuspicionBlocksOnce(t, dir, session, 30*time.Second)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "add.go"), []byte(addSwapped), 0o644))
	edited, _ := runStopWithin(t, dir, session, 30*time.Second)
	assert.Equal(t, "block", edited.Decision, "a changed tree re-arms the block")
}

// B1: a product change whose slow test was suspected hung still gets the
// verification round, once, on the stop that lets the tree through; the
// round says the tests were not verified, never that they pass.
func TestVerifyStop_HangSuspectedTreeStillGetsTheRoundOnce(t *testing.T) {
	optInFollowup(t, "")
	session := "budget-hang-round"
	t.Cleanup(func() { resetStopState(session); _ = os.Remove(stopStateFile(session, "followup")) })
	dir := newStopRepo(t, largeStopChange())
	slow := "package stopgate\n\nimport (\n\t\"testing\"\n\t\"time\"\n)\n\nfunc TestSlow(t *testing.T) { time.Sleep(2 * time.Minute) }\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "slow_test.go"), []byte(slow), 0o644))

	first, _ := runStopWithin(t, dir, session, 30*time.Second)
	require.Equal(t, "block", first.Decision)
	assert.Contains(t, first.Reason, "TestSlow")

	second, took := runStopWithin(t, dir, session, 30*time.Second)
	assert.Equal(t, "block", second.Decision, "the verification round: %s", second.SystemMessage)
	assert.Contains(t, second.Reason, "verification round")
	assert.Contains(t, second.Reason, "not verified")
	assert.NotContains(t, second.Reason, "The build and tests pass")
	assert.Less(t, took, 10*time.Second, "the tree is not run again")

	third, _ := runStopWithin(t, dir, session, 30*time.Second)
	assert.Empty(t, third.Decision, "the round fires once per session: %s", third.Reason)
	assert.Contains(t, third.SystemMessage, "not verified")
	assert.Empty(t, readStopState(session, "pass"))
}

// RG3 end to end: a deadlocked parallel test in a busy package is named
// first, blocks once, and is never recorded as a pass.
func TestVerifyStop_DeadlockInABusyPackageBlocksOnce(t *testing.T) {
	session := "budget-busy-deadlock"
	t.Cleanup(func() { resetStopState(session) })
	dir := newStopRepo(t, "")
	var b strings.Builder
	b.WriteString("package stopgate\n\nimport (\n\t\"sync\"\n\t\"testing\"\n\t\"time\"\n)\n\n" +
		"func TestDead(t *testing.T) {\n\tt.Parallel()\n\tvar mu sync.Mutex\n\tmu.Lock()\n\tmu.Lock()\n}\n\n")
	// Every test gets a slot at once, so TestDead's start does not depend on
	// which paused test the scheduler resumes first (with -parallel 2 it often
	// never started inside the window, a correct budget skip but not this
	// scenario). The sleeps spread pass events over 20s, keeping the package
	// busy while TestDead sits idle from the start.
	t.Setenv("GOFLAGS", "-parallel=4096")
	for i := 0; i < 2000; i++ {
		fmt.Fprintf(&b, "func TestQ%d(t *testing.T) { t.Parallel(); time.Sleep(%d * time.Millisecond) }\n", i, (i%400)*50)
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "busy_test.go"), []byte(b.String()), 0o644))
	warmStopBuild(t, dir)

	got, _ := runStopWithin(t, dir, session, 30*time.Second)
	assert.Equal(t, "block", got.Decision, got.SystemMessage)
	evidence, _, _ := strings.Cut(got.Reason, "\n")
	assert.Contains(t, evidence, "(stopgate.TestDead ", "the deadlocked test is named first")
	assertSuspicionBlocksOnce(t, dir, session, 30*time.Second)
}

// warmStopBuild compiles the repo's tests once, so the hook's window is spent
// running them rather than compiling 2000 test functions on a loaded runner.
func warmStopBuild(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("go", "test", "-count=1", "-run", "^$", "./...")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

// The verification round must not tell the agent the tests passed when they
// did not finish.
func TestVerifyStop_BudgetSkipRoundDoesNotClaimTestsPassed(t *testing.T) {
	optInFollowup(t, "")
	session := "budget-round"
	t.Cleanup(func() { resetStopState(session); _ = os.Remove(stopStateFile(session, "followup")) })
	dir := newStopRepo(t, largeStopChange())
	require.NoError(t, os.WriteFile(filepath.Join(dir, "slow_test.go"), []byte(longSuite()), 0o644))

	got, _ := runStopWithin(t, dir, session, 30*time.Second)
	assert.Equal(t, "block", got.Decision)
	assert.NotContains(t, got.Reason, "The build and tests pass")
	assert.Contains(t, got.Reason, "did not finish within the budget")
}

// An interrupted stop (Ctrl-C, SIGTERM) is not a verdict: no block, no block
// count, and nothing recorded.
func TestVerifyStop_InterruptIsNotAVerdict(t *testing.T) {
	session := "budget-interrupt"
	t.Cleanup(func() { resetStopState(session) })
	dir := newStopRepo(t, addSwapped)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "slow_test.go"), []byte(longSuite()), 0o644))
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(2*time.Second, cancel)

	got, _ := runStopContext(t, ctx, dir, session, time.Minute)
	assert.Empty(t, got.Decision, got.Reason)
	assert.Contains(t, got.SystemMessage, "interrupted")
	assert.Empty(t, readStopState(session, "blocks"))
	assert.Empty(t, readStopState(session, "pass"))
	assert.Empty(t, readStopState(session, "budget"))
}

// Under methodology: tdd, a source change with no test change is sent back
// even when the package's suite outlasts the budget: that verdict needs no
// test run.
func TestVerifyStop_BudgetSkipStillAppliesTestFirst(t *testing.T) {
	session := "budget-tdd"
	t.Cleanup(func() { resetStopState(session) })
	dir := newStopRepo(t, "")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "slow_test.go"), []byte(longSuite()), 0o644))
	commitAll(t, dir, "slow")
	tddProject(t, dir, "tdd", true)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "add.go"), []byte(addSwapped), 0o644))

	got, _ := runStopWithin(t, dir, session, 20*time.Second)
	assert.Equal(t, "block", got.Decision)
	assert.Contains(t, got.Reason, "no test changed")
	assert.Empty(t, readStopState(session, "budget"), "a blocked tree is not recorded")
}

// When test-first could not be checked (no time was left to run the tests
// against HEAD), the budget skip is not recorded: the next stop checks again.
func TestVerifyStop_BudgetSkipWithUncheckedTestFirstIsNotRecorded(t *testing.T) {
	session := "budget-red-unchecked"
	t.Cleanup(func() { resetStopState(session) })
	dir := newStopRepo(t, "")
	tddProject(t, dir, "tdd", true)
	commitAll(t, dir, "tdd")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "add.go"), []byte(addSwapped), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "slow_test.go"), []byte(longSuite()), 0o644))

	got, _ := runStopWithin(t, dir, session, 20*time.Second)
	assert.Empty(t, got.Decision, got.Reason)
	assert.Contains(t, got.SystemMessage, "test-first was not checked")
	assert.Empty(t, readStopState(session, "budget"), "an unchecked tree is checked again next stop")
}
