package verifygate

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// goPackageStep is the whole-package step over a one-package module.
func goPackageStep(t *testing.T, files map[string]string) Step {
	t.Helper()
	dir := t.TempDir()
	files["go.mod"] = "module m\n\ngo 1.22\n"
	files["m.go"] = "package m\n"
	writeFiles(t, dir, files)
	return Step{Name: "go test", Argv: []string{"go", "test", "-json", "."}, Dir: dir,
		GoTestJSON: true, BudgetBound: true, GoPackages: []string{"m"}}
}

// P7: a parent that deadlocks in its own body after starting a parallel
// subtest (which only continues once the body returns) emits nothing more.
// The package stalls; that is a hang, not a budget skip.
func TestRun_ParentDeadlockedBeforeItsParallelSubtestIsHung(t *testing.T) {
	step := goPackageStep(t, map[string]string{"m_test.go": "package m\n\nimport (\n\t\"sync\"\n\t\"testing\"\n)\n\n" +
		"func TestOK(t *testing.T) {}\n\n" +
		"func TestParent(t *testing.T) {\n\tt.Run(\"a\", func(t *testing.T) { t.Parallel() })\n" +
		"\tvar mu sync.Mutex\n\tmu.Lock()\n\tmu.Lock()\n}\n"})

	failure, skipped := Run(context.Background(), []Step{step}, 15*time.Second)

	require.NotNil(t, failure, "skipped: %v", skipped)
	assert.True(t, failure.Hung)
	assert.Contains(t, failure.Reason, "m.TestParent")
}

// A TestMain that waits forever on something the runtime cannot see (a
// timer here; a socket or poll in practice) emits only the package start.
// The package stalls; it is not "still building".
func TestRun_TestMainThatNeverRunsTheTestsIsHung(t *testing.T) {
	step := goPackageStep(t, map[string]string{"m_test.go": "package m\n\nimport (\n\t\"testing\"\n\t\"time\"\n)\n\n" +
		"func TestMain(m *testing.M) { time.Sleep(time.Hour); m.Run() }\n\nfunc TestA(t *testing.T) {}\n"})

	failure, skipped := Run(context.Background(), []Step{step}, 15*time.Second)

	require.NotNil(t, failure, "skipped: %v", skipped)
	assert.True(t, failure.Hung)
	assert.Contains(t, failure.Reason, "package m ")
	assert.Contains(t, failure.Reason, "TestMain")
}

// A covered step leaves at least half the remaining time to the step that
// covers it, so that step runs even after a slow build.
func TestRun_CoveredStepLeavesTimeForTheStepThatCoversIt(t *testing.T) {
	dir := t.TempDir()
	covered := sleepyStep(dir, `{}`, true)
	covered.Share, covered.CoveredLater = 0.5, true
	steps := []Step{
		{Name: "go build", Argv: []string{"sleep", "6"}, Dir: dir},
		covered,
		{Name: "go test", Argv: []string{"sh", "-c", "echo broke; exit 1"}, Dir: dir, GoTestJSON: true, BudgetBound: true},
	}

	failure, _ := Run(context.Background(), steps, 10*time.Second)

	require.NotNil(t, failure, "the covering step ran")
	assert.Equal(t, "go test", failure.Step.Name)
}
