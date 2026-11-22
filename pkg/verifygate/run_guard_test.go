package verifygate

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// eventsAt stamps go test -json events with the current time, the way
// test2json does when each event happens.
func eventsAt(lines ...string) string {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for i, l := range lines {
		lines[i] = strings.Replace(l, "{", `{"Time":"`+now+`",`, 1)
	}
	return strings.Join(lines, "\n")
}

// A test that started and never returned, holding the step for most of its
// window, is a hang the gate could see: it fails, naming the test, instead
// of becoming a budget skip. Its output so far (an earlier t.Error) is shown.
func TestRun_TestRunningMostOfTheWindowIsAHang(t *testing.T) {
	dir := t.TempDir()
	events := eventsAt(
		`{"Action":"run","Package":"x","Test":"TestOK"}`,
		`{"Action":"pass","Package":"x","Test":"TestOK"}`,
		`{"Action":"run","Package":"x","Test":"TestHang"}`,
		`{"Action":"output","Package":"x","Test":"TestHang","Output":"    hang_test.go:9: half done\n"}`,
	)

	failure, _ := Run(context.Background(), []Step{sleepyStep(dir, events, true)}, 3*time.Second)

	require.NotNil(t, failure, "a deadlocked test must not pass as a budget skip")
	assert.Contains(t, failure.Reason, "x.TestHang")
	assert.Contains(t, failure.Output, "half done")
}

// A budget skip needs evidence the suite was making progress: a step cut off
// before any test finished (still compiling) tells nothing about the change,
// so it is an ordinary skip, and its reason names the window it had.
func TestRun_BudgetTimeoutWithNoFinishedTestIsNotABudgetSkip(t *testing.T) {
	dir := t.TempDir()
	failure, skipped := Run(context.Background(), []Step{sleepyStep(dir, `{}`, true)}, 2*time.Second)

	require.Nil(t, failure)
	require.Len(t, skipped, 1)
	assert.False(t, skipped[0].Budget, "no test finished, so nothing may be recorded")
	assert.Contains(t, skipped[0].Reason, "within the 2s it had")
}

// The budget skip says what was still running when time ran out.
func TestRun_BudgetSkipNamesTheTestsStillRunning(t *testing.T) {
	dir := t.TempDir()
	step := Step{Name: "slow", Dir: dir, GoTestJSON: true, BudgetBound: true, Argv: []string{"sh", "-c",
		"printf '%s\\n' '" + eventsAt(`{"Action":"pass","Package":"x","Test":"TestOK"}`) + "'; sleep 5.5; " +
			`printf '{"Time":"%s","Action":"run","Package":"x","Test":"TestLate"}\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"; exec sleep 60`}}

	// date has whole seconds, so TestLate looks up to 1s older than it is:
	// 2.5s to 3.5s of an 8s window, under the 4s that would make it a hang.
	failure, skipped := Run(context.Background(), []Step{step}, 8*time.Second)

	require.Nil(t, failure, "TestLate ran for under half the window")
	require.Len(t, skipped, 1)
	assert.True(t, skipped[0].Budget)
	assert.Contains(t, skipped[0].Reason, "x.TestLate")
}

// A slow build must not let a BudgetBound step spend the time the unbound
// steps after it need: they still run, and their failure counts.
func TestRun_BoundStepLeavesTimeForTheUnboundStepsAfterIt(t *testing.T) {
	dir := t.TempDir()
	bound := sleepyStep(dir, eventsAt(`{"Action":"pass","Package":"x","Test":"TestOK"}`), true)
	bound.Share, bound.CoveredLater = 0.5, true
	steps := []Step{
		{Name: "go build", Argv: []string{"sleep", "6"}, Dir: dir},
		bound,
		{Name: "npm run test", Argv: []string{"sh", "-c", "echo npm broke; exit 1"}, Dir: dir},
		sleepyStep(dir, eventsAt(`{"Action":"pass","Package":"x","Test":"TestOK"}`), true),
		{Name: "qa exec --all", Argv: []string{"sh", "-c", "exit 1"}, Dir: dir},
	}

	failure, _ := Run(context.Background(), steps, 10*time.Second)

	require.NotNil(t, failure, "npm run test ran and failed")
	assert.Equal(t, "npm run test", failure.Step.Name)
}

// A Ctrl-C or SIGTERM (a canceled context) is not a verdict on the change:
// no failure, and the skip says the run was interrupted.
func TestRun_InterruptIsNotAFailure(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(time.Second, cancel)

	failure, skipped := Run(ctx, []Step{sleepyStep(dir, `{}`, false)}, time.Minute)

	require.Nil(t, failure)
	require.NotEmpty(t, skipped)
	assert.Contains(t, skipped[0].Reason, "interrupted")
	assertGrandchildGone(t, dir)
}

// A before-the-change run that is interrupted or runs out of time has not
// shown the tests fail: test-first is unchecked, not proven.
func TestCheckRed_InterruptOrTimeoutIsUnavailable(t *testing.T) {
	hang := "package redgate\n\nimport (\n\t\"testing\"\n\t\"time\"\n)\n\nfunc TestAdd(t *testing.T) { time.Sleep(time.Minute) }\n"
	root := redRepo(t, map[string]string{"go.mod": redModule, "add.go": redAddBase},
		map[string]string{"add.go": redAddFix, "add_test.go": hang})
	_, changed, ok := ChangedFiles(context.Background(), root)
	require.True(t, ok)

	assert.Equal(t, RedUnavailable, CheckRed(context.Background(), root, changed, 3*time.Second).Verdict, "timeout")

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(1500*time.Millisecond, cancel)
	assert.Equal(t, RedUnavailable, CheckRed(ctx, root, changed, time.Minute).Verdict, "interrupt")
}

// The before-the-change run runs only the changed tests, not the whole
// package at HEAD.
func TestRedSteps_RunOnlyTheChangedGoTests(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"go.mod": redModule, "b/b.go": "package b\n",
		"b/b_test.go": "package b\nimport \"testing\"\nfunc TestB(t *testing.T) {}\n"})

	steps := redSteps(root, []string{filepath.ToSlash("b/b_test.go")})

	require.Len(t, steps, 1)
	assert.Equal(t, []string{"go", "test", "-run", "^(TestB)$", "./b"}, steps[0].Argv)
}
