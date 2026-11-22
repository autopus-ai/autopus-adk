package verifygate

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A parallel parent whose body returned waits, with no events, for its
// t.Parallel subtests, which wait paused in the queue for a parallel slot.
// Neither is a hang: the parent has an unfinished subtest, and the subtest is
// paused, not running.
func TestGoTestProgress_ParentWaitingOnPausedSubtestsIsNotHung(t *testing.T) {
	t.Parallel()
	t0 := time.Now().Add(-12 * time.Second).UTC().Format(time.RFC3339Nano)
	ev := func(action, test string) string {
		return fmt.Sprintf(`{"Time":%q,"Action":%q,"Package":"m","Test":%q}`, t0, action, test)
	}
	recent := fmt.Sprintf(`{"Time":%q,"Action":"pass","Package":"m","Test":"TestQ1"}`, time.Now().UTC().Format(time.RFC3339Nano))
	out := strings.Join([]string{
		ev("run", "TestP0"), ev("pause", "TestP0"), ev("cont", "TestP0"),
		ev("run", "TestP0/sub"), ev("pause", "TestP0/sub"), recent,
	}, "\n")

	prog := goTestProgress([]byte(out), time.Now())

	assert.Empty(t, prog.stalled(time.Now(), 6*time.Second), "the package's other tests keep finishing")
	assert.Empty(t, prog.Running, "TestP0 waits on its paused subtest, which is paused itself")
}

// The P2' shape end to end: many parallel tests, and parents whose single
// parallel subtest is queued behind them. The run outlasts the budget with
// tests finishing; it is a budget skip, not a hang.
func TestRun_ParallelParentsQueuedBehindOtherTestsAreNotHung(t *testing.T) {
	dir := t.TempDir()
	var body strings.Builder
	body.WriteString("package m\n\nimport (\n\t\"testing\"\n\t\"time\"\n)\n\n")
	for i := 0; i < 960; i++ {
		fmt.Fprintf(&body, "func TestQ%d(t *testing.T) { t.Parallel(); time.Sleep(500 * time.Millisecond) }\n", i)
		if i == 100 {
			for p := 0; p < 10; p++ {
				fmt.Fprintf(&body, "func TestP%d(t *testing.T) {\n\tt.Parallel()\n\tt.Run(\"sub\", func(t *testing.T) { t.Parallel(); time.Sleep(10 * time.Millisecond) })\n}\n", p)
			}
		}
	}
	writeFiles(t, dir, map[string]string{"go.mod": "module m\n\ngo 1.22\n", "m.go": "package m\n", "m_test.go": body.String()})
	step := Step{Name: "go test", Argv: []string{"go", "test", "-json", "."}, Dir: dir, GoTestJSON: true, BudgetBound: true}

	failure, skipped := Run(context.Background(), []Step{step}, 14*time.Second)

	require.Nil(t, failure, "queued parallel parents are not hung")
	require.Len(t, skipped, 1)
	assert.True(t, skipped[0].Budget, "tests were finishing: %s", skipped[0].Reason)
}

// Only the step that runs a package last judges hangs: a changed test that
// is slow within the changed-tests step's half of the budget is run again,
// with the rest of the budget, by the whole-package step.
func TestRun_CoveredStepDoesNotJudgeHangs(t *testing.T) {
	dir := t.TempDir()
	first := sleepyStep(dir, eventsAt(`{"Action":"run","Package":"x","Test":"TestSlow"}`), true)
	first.Share, first.CoveredLater = 0.5, true
	whole := Step{Name: "go test", Argv: []string{"sh", "-c", "exit 0"}, Dir: dir}

	failure, skipped := Run(context.Background(), []Step{first, whole}, 4*time.Second)

	require.Nil(t, failure, "the whole-package step passed it")
	assert.Empty(t, skipped)
}

// A package that never finished a test (still compiling when time ran out)
// was not checked, whatever another package of the step did: the step is
// not a budget skip, and the reason names the package.
func TestRun_PackageWithNoFinishedTestIsNotRecorded(t *testing.T) {
	dir := t.TempDir()
	events := eventsAt(
		`{"Action":"start","Package":"x/a"}`,
		`{"Action":"pass","Package":"x/a","Test":"TestA"}`,
		`{"Action":"pass","Package":"x/a"}`,
	)
	step := sleepyStep(dir, events, true)
	step.GoPackages = []string{"x/a", "x/b"}

	failure, skipped := Run(context.Background(), []Step{step}, 2*time.Second)

	require.Nil(t, failure)
	require.Len(t, skipped, 1)
	assert.False(t, skipped[0].Budget, "x/b was not checked")
	assert.Contains(t, skipped[0].Reason, "x/b")
}

// A covered step that reservations leave no time for checks nothing that a
// later step does not check again: it leaves no skip behind.
func TestRun_CoveredStepWithNoWindowLeavesNoSkip(t *testing.T) {
	dir := t.TempDir()
	first := sleepyStep(dir, `{}`, true)
	first.Share, first.CoveredLater = 0.5, true
	steps := []Step{first}
	for i := 0; i < 10; i++ {
		steps = append(steps, Step{Name: fmt.Sprintf("check %d", i), Argv: []string{"true"}, Dir: dir})
	}

	failure, skipped := Run(context.Background(), steps, 2*time.Second)

	require.Nil(t, failure)
	assert.Empty(t, skipped)
}
