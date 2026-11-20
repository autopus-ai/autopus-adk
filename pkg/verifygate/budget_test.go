package verifygate

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlan_ChangedTestsRunFirstAndBothTestStepsAreBudgetBound(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"go.mod": "module x\n", "pkg/b/b.go": "package b\n",
		"pkg/b/b_test.go": "package b\nimport \"testing\"\nfunc TestB(t *testing.T) {}\nfunc TestB_Two(t *testing.T) {}\n" +
			"func Testable() {}\nfunc helper(t *testing.T) {}\n",
		"c/c.go": "package c\n",
	})

	steps := Plan(root, []string{"pkg/b/b_test.go", "pkg/b/b.go", "c/c.go"})

	require.Len(t, steps, 3)
	assert.Equal(t, []string{"go", "test", "-json", "-run", "^(TestB|TestB_Two)$", "./pkg/b"}, steps[1].Argv,
		"Testable and helper are not tests")
	assert.Equal(t, []string{"go", "test", "-json", "./c", "./pkg/b"}, steps[2].Argv)
	for _, s := range steps[1:] {
		assert.True(t, s.GoTestJSON && s.BudgetBound, s.Name)
	}
	assert.False(t, steps[0].BudgetBound, "the build is never budget bound")
}

func TestPlan_NoChangedTestFileRunsOnlyWholePackages(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"go.mod": "module x\n", "a.go": "package x\n"})

	steps := Plan(root, []string{"a.go"})

	require.Len(t, steps, 2)
	assert.Equal(t, []string{"go", "test", "-json", "."}, steps[1].Argv)
}

func TestRenderGoTestJSON_KeepsFailuresDropsPassingChatter(t *testing.T) {
	t.Parallel()
	out := strings.Join([]string{
		`# x/pkg [build note]`,
		`{"Action":"run","Package":"x","Test":"TestOK"}`,
		`{"Action":"output","Package":"x","Test":"TestOK","Output":"=== RUN   TestOK\n"}`,
		`{"Action":"pass","Package":"x","Test":"TestOK"}`,
		`{"Action":"output","Package":"x","Test":"TestBad","Output":"    bad_test.go:9: boom\n"}`,
		`{"Action":"fail","Package":"x","Test":"TestBad"}`,
		`{"Action":"output","Package":"x","Output":"FAIL\tx\t0.1s\n"}`,
	}, "\n")

	text, failed := renderGoTestJSON([]byte(out))

	assert.Equal(t, []string{"x.TestBad"}, failed)
	assert.Contains(t, text, "boom")
	assert.Contains(t, text, "FAIL\tx")
	assert.Contains(t, text, "[build note]")
	assert.NotContains(t, text, "TestOK")
}

// sleepyStep prints JSON events, then outlives the budget in a grandchild,
// the way go test's test binary does, and writes that grandchild's pid.
func sleepyStep(dir, events string, bound bool) Step {
	script := "printf '%s\\n' '" + events + "'; sh -c 'echo $$ > child.pid; exec sleep 60' & wait"
	return Step{Name: "slow", Argv: []string{"sh", "-c", script}, Dir: dir, GoTestJSON: true, BudgetBound: bound}
}

func TestRun_BudgetBoundTimeoutWithoutFailureSkipsTheRest(t *testing.T) {
	dir := t.TempDir()
	steps := []Step{
		sleepyStep(dir, `{"Action":"pass","Package":"x","Test":"TestOK"}`, true),
		{Name: "after", Argv: []string{"sh", "-c", "exit 1"}, Dir: dir},
	}
	start := time.Now()
	failure, skipped := Run(context.Background(), steps, 2*time.Second)

	require.Nil(t, failure)
	require.Len(t, skipped, 2)
	assert.True(t, skipped[0].Budget && skipped[1].Budget)
	assert.Contains(t, skipped[0].Reason, "no test had failed")
	assert.Less(t, time.Since(start), 20*time.Second, "a lingering grandchild must not hold Run")
	assertGrandchildGone(t, dir)
}

func TestRun_FailureSeenBeforeTheBudgetRanOutStillFails(t *testing.T) {
	dir := t.TempDir()
	failure, _ := Run(context.Background(),
		[]Step{sleepyStep(dir, `{"Action":"fail","Package":"x","Test":"TestBad"}`, true)}, 2*time.Second)

	require.NotNil(t, failure)
	assert.Contains(t, failure.Reason, "x.TestBad failed before")
}

func TestRun_TimeoutOfAnUnboundStepStillFails(t *testing.T) {
	dir := t.TempDir()
	failure, _ := Run(context.Background(), []Step{sleepyStep(dir, `{}`, false)}, 2*time.Second)

	require.NotNil(t, failure)
	assert.Contains(t, failure.Reason, "did not finish within")
}

func assertGrandchildGone(t *testing.T, dir string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "child.pid"))
	require.NoError(t, err)
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	require.NoError(t, err)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if exec.Command("kill", "-0", strconv.Itoa(pid)).Run() != nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("grandchild %d still running after the budget", pid)
}

func TestRun_StepShareRunsOutButLaterStepsStillRun(t *testing.T) {
	dir := t.TempDir()
	first := sleepyStep(dir, `{}`, true)
	first.Share = 0.25
	steps := []Step{first, {Name: "after", Argv: []string{"sh", "-c", "echo broke; exit 1"}, Dir: dir}}

	failure, skipped := Run(context.Background(), steps, 8*time.Second)

	require.NotNil(t, failure, "the later step ran and its failure counts")
	assert.Equal(t, "after", failure.Step.Name)
	require.Len(t, skipped, 1)
	assert.Contains(t, skipped[0].Reason, "its share")
}
