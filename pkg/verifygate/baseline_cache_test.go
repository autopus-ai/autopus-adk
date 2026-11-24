package verifygate

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func useBaselineCache(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "cache")
	saved := BaselineCacheDir
	BaselineCacheDir = dir
	t.Cleanup(func() { BaselineCacheDir = saved })
	return dir
}

// A test that fails on HEAD is run there once per HEAD commit: the next
// stop answers it from the cache. The cache is planted with a lie (HEAD
// passed it) to show the second run did not ask HEAD again.
func TestRunCharged_HEADIsAskedOncePerCommit(t *testing.T) {
	dir := useBaselineCache(t)
	root := redRepo(t, baselineBase(), map[string]string{"add.go": baseAddFix})
	first := runCharged(t, root)
	require.Nil(t, first.Failure)
	require.Equal(t, []string{"basegate.TestNeedsTool"}, first.Preexisting)

	head := strings.TrimSpace(gitRun(t, root, "rev-parse", "HEAD"))
	file := filepath.Join(dir, "baseline-"+head+".json")
	var entries map[string]baselineEntry
	data, err := os.ReadFile(file)
	require.NoError(t, err, "the HEAD result is cached per commit")
	require.NoError(t, json.Unmarshal(data, &entries))
	require.Len(t, entries["basegate.TestNeedsTool"].Runs, headCount)
	assert.True(t, cachedFailing(t, dir, head)["basegate.TestNeedsTool"])

	lie, _ := json.Marshal(map[string]baselineEntry{"basegate.TestNeedsTool": {}})
	require.NoError(t, os.WriteFile(file, lie, 0o600))
	second := runCharged(t, root)
	require.NotNil(t, second.Failure, "the cached answer was used instead of a HEAD run")
	assert.Empty(t, second.Preexisting)
}

// A cache of another commit says nothing about this HEAD.
func TestRunCharged_ANewHEADAsksAgain(t *testing.T) {
	dir := useBaselineCache(t)
	root := redRepo(t, baselineBase(), map[string]string{"add.go": baseAddFix})
	old := strings.TrimSpace(gitRun(t, root, "rev-parse", "HEAD"))
	lie, _ := json.Marshal(map[string]baselineEntry{"basegate.TestNeedsTool": {}})
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "baseline-"+old+".json"), lie, 0o600))
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "next")
	writeFiles(t, root, map[string]string{"add.go": baseAdd})

	res := runCharged(t, root)
	assert.Nil(t, res.Failure)
	assert.Equal(t, []string{"basegate.TestNeedsTool"}, res.Preexisting)
}

func cachedFailing(t *testing.T, dir, head string) map[string]bool {
	t.Helper()
	saved := BaselineCacheDir
	BaselineCacheDir = dir
	defer func() { BaselineCacheDir = saved }()
	c := &baselineCache{dir: dir, name: "baseline-" + head + ".json", entries: map[string]baselineEntry{}}
	data, err := os.ReadFile(filepath.Join(dir, c.name))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &c.entries))
	return c.knownFailing()
}

// A failure HEAD is known to share does not stop the step early: the HEAD
// check costs nothing, so the tests after it keep running.
func TestRun_AKnownFailureDoesNotCutTheStepEarly(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not installed")
	}
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"go.mod": baseModule,
		"slow_test.go": "package basegate\n\nimport (\n\t\"testing\"\n\t\"time\"\n)\n\n" +
			"func TestAFails(t *testing.T) { t.Fatal(\"broken\") }\n\n" +
			"func TestBSlow(t *testing.T) { time.Sleep(12 * time.Second) }\n",
	})
	cmd := exec.Command("go", "test", "-count=1", "-run", "^$", ".")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	step := Step{Name: "go test", Argv: []string{"go", "test", "-json", "."}, Dir: root, GoTestJSON: true, BudgetBound: true,
		GoPackages: importPaths(root, []string{"."}), KnownFailing: map[string]bool{"basegate.TestAFails": true}}
	failure, _ := Run(context.Background(), []Step{step}, 30*time.Second)
	require.NotNil(t, failure)
	assert.False(t, failure.Partial, "the step ran to the end instead of stopping at two thirds")
	assert.False(t, failure.EarlyCut)
	assert.Equal(t, []string{"basegate.TestAFails"}, failure.FailedTests)
}

// Cut early for the HEAD check, the tests after the failures are still
// owed: the skip says so (Baseline), which keeps the tree unrecorded.
func TestPartialSkip_AnEarlyCutIsABaselineSkip(t *testing.T) {
	early := partialSkip(&Failure{Step: Step{Name: "go test"}, FailedTests: []string{"m.TestA"}, Partial: true, EarlyCut: true})
	assert.True(t, early.Budget && early.Baseline)
	assert.Contains(t, early.Reason, "run at the next stop")
	late := partialSkip(&Failure{Step: Step{Name: "go test"}, FailedTests: []string{"m.TestA"}, Partial: true})
	assert.True(t, late.Budget)
	assert.False(t, late.Baseline, "cut at the window's end is the size of the suite")
}
