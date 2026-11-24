package verifygate

import (
	"context"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Review M1: RunCharged hands the cached failures to the step. TestAFails
// fails on HEAD (an earlier commit's file says so) and TestBSlow runs past
// two thirds of the window: given the known failure, the step is not cut
// early: it runs to its end and nothing is left owed.
func TestRunCharged_PassesKnownFailuresToTheStep(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not installed")
	}
	dir := useBaselineCache(t)
	slow := "package basegate\n\nimport (\n\t\"testing\"\n\t\"time\"\n)\n\n" +
		"func TestAFails(t *testing.T) { t.Fatal(\"broken\") }\n\n" +
		"func TestBSlow(t *testing.T) { time.Sleep(21 * time.Second) }\n"
	root := redRepo(t, map[string]string{"go.mod": baseModule, "add.go": baseAdd, "slow_test.go": slow},
		map[string]string{"add.go": baseAddFix})
	writeEntries(t, filepath.Join(dir, "baseline-0000-x.json"), map[string]baselineEntry{
		"basegate.TestAFails": {Runs: []testRun{{Failed: true}, {Failed: true}}, At: time.Now()}})
	warm := exec.Command("go", "test", "-count=1", "-run", "^$", ".")
	warm.Dir = root
	out, err := warm.CombinedOutput()
	require.NoError(t, err, string(out))
	step := Step{Name: "go test", Argv: []string{"go", "test", "-json", "."}, Dir: root, GoTestJSON: true, BudgetBound: true,
		GoPackages: importPaths(root, []string{"."})}

	res := RunCharged(context.Background(), root, []string{"add.go"}, []Step{step}, 30*time.Second)
	assert.False(t, slices.ContainsFunc(res.Skipped, func(s Skip) bool { return s.Baseline }),
		"the step was cut early for a failure the cache already knew: %+v", res.Skipped)
	// No time is left to ask HEAD, so the failure stays charged; what matters
	// is that the step ran to its end.
	require.NotNil(t, res.Failure)
	assert.False(t, res.Failure.Partial, "the step was cut before TestBSlow finished")
}
