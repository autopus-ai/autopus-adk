package verifygate

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Review F (#376): this tree recorded "fails on HEAD" an hour ago, and this
// stop asks HEAD again; HEAD passes TestK once (40s per run) and the run is
// cut off before the second. That pass is newer evidence than the stored
// answer, so recall must not excuse the failure: the stop blocks.
func TestRunCharged_APassInThisHEADRunRulesOutRecall(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not installed")
	}
	const (
		slowHead = "package basegate\n\nimport \"time\"\n\nfunc Val() int { time.Sleep(40 * time.Second); return 3 }\n"
		slowBad  = "package basegate\n\nfunc Val() int { return 4 }\n"
		kTest    = "package basegate\n\nimport \"testing\"\n\nfunc TestK(t *testing.T) {\n\tif Val() != 3 {\n\t\tt.Fatal(\"broken\")\n\t}\n}\n"
	)
	dir := useBaselineCache(t)
	saved := BaselineCacheSession
	BaselineCacheSession = "s2"
	t.Cleanup(func() { BaselineCacheSession = saved })
	root := redRepo(t, map[string]string{"go.mod": baseModule, "slow.go": slowHead, "k_test.go": kTest},
		map[string]string{"slow.go": slowBad})
	line := []string{"k_test.go:7: broken"}
	writeEntries(t, cacheFile(t, dir, root), map[string]baselineEntry{"basegate.TestK": {
		Runs: []testRun{{Failed: true, Lines: line}, {Failed: true, Lines: line}}, At: time.Now().Add(-time.Hour),
		Session: "old", Root: root}})
	_, changed, ok := ChangedFiles(context.Background(), root)
	require.True(t, ok)

	// The tree's own steps take seconds, leaving HEAD some 60-75s: one 40s
	// run of TestK ends there, the second (at about 85s) does not.
	res := RunCharged(context.Background(), root, changed, Plan(root, changed), 80*time.Second)
	require.NotNil(t, res.Failure, "excused although HEAD passed TestK in this stop's run; note: %s", res.Note())
	assert.Empty(t, res.Recalled)
	assert.Contains(t, res.Unavailable, "on HEAD", "HEAD was not asked, so this did not test a pass in its run")
}
