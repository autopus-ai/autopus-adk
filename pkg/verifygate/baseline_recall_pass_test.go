package verifygate

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Review F and F3 (#376): this tree recorded "fails on HEAD" with "got 4" an
// hour ago, and this stop asks HEAD again. HEAD runs TestK once (40s per
// run) and is cut off before the second run: it passes it, or fails it with
// another line. Either is newer evidence than the stored answer, so recall
// must not excuse the tree's "got 4": the stop blocks.
func TestRunCharged_ThisHEADRunOverridesRecall(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not installed")
	}
	const (
		kTest = "package basegate\n\nimport \"testing\"\n\nfunc TestK(t *testing.T) {\n\tif v := Val(); v != 3 {\n\t\tt.Fatalf(\"got %d\", v)\n\t}\n}\n"
		bad   = "package basegate\n\nfunc Val() int { return 4 }\n"
	)
	slowHead := func(v string) string {
		return "package basegate\n\nimport \"time\"\n\nfunc Val() int { time.Sleep(40 * time.Second); return " + v + " }\n"
	}
	for name, head := range map[string]string{"HEAD passes it": slowHead("3"), "HEAD fails it differently": slowHead("5")} {
		t.Run(name, func(t *testing.T) {
			dir := useBaselineCache(t)
			saved := BaselineCacheSession
			BaselineCacheSession = "s2"
			t.Cleanup(func() { BaselineCacheSession = saved })
			root := redRepo(t, map[string]string{"go.mod": baseModule, "val.go": head, "k_test.go": kTest},
				map[string]string{"val.go": bad})
			line := []string{"k_test.go:7: got 4"}
			writeEntries(t, cacheFile(t, dir, root), map[string]baselineEntry{"basegate.TestK": {
				Runs: []testRun{{Failed: true, Lines: line}, {Failed: true, Lines: line}}, At: time.Now().Add(-time.Hour),
				Session: "old", Root: root}})
			_, changed, ok := ChangedFiles(context.Background(), root)
			require.True(t, ok)

			// The tree's own steps take seconds, leaving HEAD some 60-75s: one
			// 40s run of TestK ends there, the second (at about 85s) does not.
			res := RunCharged(context.Background(), root, changed, Plan(root, changed), 80*time.Second)
			require.NotNil(t, res.Failure, "excused against this stop's HEAD run; note: %s", res.Note())
			assert.Empty(t, res.Recalled)
			assert.Contains(t, res.Unavailable, "on HEAD", "HEAD was not asked, so this did not test its run")
		})
	}
}
