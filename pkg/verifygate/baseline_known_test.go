package verifygate

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Review M1: RunCharged hands the cached failures to the step. The step
// is a script that prints go test -json events (no compile, so the timing is
// the script's own): TestAFails fails at once, which an earlier commit's
// cache file says HEAD fails too, and TestBSlow ends at 24s of a 30s window,
// past the 20s early cut. Given the known failure the step is not cut, so it
// ends on its own instead of Partial.
func TestRunCharged_PassesKnownFailuresToTheStep(t *testing.T) {
	dir := useBaselineCache(t)
	root := redRepo(t, map[string]string{"go.mod": baseModule, "add.go": baseAdd}, map[string]string{"add.go": baseAddFix})
	writeEntries(t, filepath.Join(dir, "baseline-0000-x.json"), map[string]baselineEntry{
		"basegate.TestAFails": {Runs: []testRun{{Failed: true}, {Failed: true}}, At: time.Now()}})
	script := `echo '{"Action":"run","Package":"basegate","Test":"TestAFails"}'
echo '{"Action":"output","Package":"basegate","Test":"TestAFails","Output":"    a_test.go:1: broken\n"}'
echo '{"Action":"fail","Package":"basegate","Test":"TestAFails"}'
echo '{"Action":"run","Package":"basegate","Test":"TestBSlow"}'
sleep 24
echo '{"Action":"pass","Package":"basegate","Test":"TestBSlow"}'
echo '{"Action":"fail","Package":"basegate"}'
exit 1
`
	step := Step{Name: "go test", Argv: []string{"sh", "-c", script}, Dir: root, GoTestJSON: true, BudgetBound: true,
		GoPackages: []string{"basegate"}}

	res := RunCharged(context.Background(), root, []string{"add.go"}, []Step{step}, 30*time.Second)
	assert.False(t, slices.ContainsFunc(res.Skipped, func(s Skip) bool { return s.Baseline }),
		"the step was cut early for a failure the cache already knew: %+v", res.Skipped)
	// No time is left to ask HEAD, so the failure stays charged; what matters
	// is that the step ran to its end.
	require.NotNil(t, res.Failure)
	assert.False(t, res.Failure.Partial, "the step was cut before TestBSlow finished")
}
