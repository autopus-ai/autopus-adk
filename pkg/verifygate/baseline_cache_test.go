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

// cacheFile is the cache file RunCharged uses for root's HEAD.
func cacheFile(t *testing.T, dir, root string) string {
	t.Helper()
	head := strings.TrimSpace(gitRun(t, root, "rev-parse", "HEAD"))
	return filepath.Join(dir, "baseline-"+head+"-"+EnvDigest(context.Background(), root)+".json")
}

func readEntries(t *testing.T, file string) map[string]baselineEntry {
	t.Helper()
	data, err := os.ReadFile(file)
	require.NoError(t, err, "the HEAD result is cached per commit")
	var entries map[string]baselineEntry
	require.NoError(t, json.Unmarshal(data, &entries))
	return entries
}

func writeEntries(t *testing.T, file string, entries map[string]baselineEntry) {
	t.Helper()
	data, _ := json.Marshal(entries)
	require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o700))
	require.NoError(t, os.WriteFile(file, data, 0o600))
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

	file := cacheFile(t, dir, root)
	entries := readEntries(t, file)
	require.Len(t, entries["basegate.TestNeedsTool"].Runs, headCount)
	assert.True(t, entries["basegate.TestNeedsTool"].failed("basegate.TestNeedsTool"))

	writeEntries(t, file, map[string]baselineEntry{"basegate.TestNeedsTool": {At: time.Now()}})
	second := runCharged(t, root)
	require.NotNil(t, second.Failure, "the cached answer was used instead of a HEAD run")
	assert.Empty(t, second.Preexisting)
}

// Review B2: a "fails on HEAD" answer older than baselineFailTTL does not
// excuse a failure: HEAD is asked again (the entry is stored anew). A recent
// one is used as it is.
func TestRunCharged_AnOldFailsOnHEADAnswerIsAskedAgain(t *testing.T) {
	dir := useBaselineCache(t)
	root := redRepo(t, baselineBase(), map[string]string{"add.go": baseAddFix})
	require.Nil(t, runCharged(t, root).Failure)
	file := cacheFile(t, dir, root)
	entries := readEntries(t, file)
	const key = "basegate.TestNeedsTool"

	recent := entries[key]
	recent.At = time.Now().Add(-baselineFailTTL / 2).UTC().Truncate(time.Second)
	writeEntries(t, file, map[string]baselineEntry{key: recent})
	runCharged(t, root)
	assert.True(t, readEntries(t, file)[key].At.Equal(recent.At), "a recent answer is not asked again")

	other := entries[key]
	other.At, other.Session = time.Now().UTC().Truncate(time.Second), "another-session"
	writeEntries(t, file, map[string]baselineEntry{key: other})
	runCharged(t, root)
	assert.Empty(t, readEntries(t, file)[key].Session, "another session's answer is asked again and stored anew")

	old := entries[key]
	old.At = time.Now().Add(-2 * baselineFailTTL).UTC().Truncate(time.Second)
	writeEntries(t, file, map[string]baselineEntry{key: old})
	res := runCharged(t, root)
	assert.Equal(t, []string{key}, res.Preexisting)
	assert.True(t, readEntries(t, file)[key].At.After(old.At.Add(baselineFailTTL)), "HEAD was asked again")
}

// A cache of another commit says nothing about this HEAD.
func TestRunCharged_ANewHEADAsksAgain(t *testing.T) {
	dir := useBaselineCache(t)
	root := redRepo(t, baselineBase(), map[string]string{"add.go": baseAddFix})
	writeEntries(t, cacheFile(t, dir, root), map[string]baselineEntry{"basegate.TestNeedsTool": {At: time.Now()}})
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "next")
	writeFiles(t, root, map[string]string{"add.go": baseAdd})

	res := runCharged(t, root)
	assert.Nil(t, res.Failure)
	assert.Equal(t, []string{"basegate.TestNeedsTool"}, res.Preexisting)
}

// Review A3: a cache file holding null is an empty cache, not a panic.
func TestReadBaselineFile_NullIsEmpty(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "baseline-x.json"), []byte("null"), 0o600))
	entries := readBaselineFile(dir, "baseline-x.json")
	require.NotNil(t, entries)
	entries["k"] = baselineEntry{}
}

// Review M5: only keys HEAD settled are cached.
func TestHeadResult_OnlySettledKeysAreAnswered(t *testing.T) {
	two := []testRun{{Failed: true}, {Failed: true}}
	h := headResult{runs: map[string][]testRun{"m/a.TestA": two, "m/b.TestB": two[:1]}, brokenPkgs: map[string]bool{"m/c": true}}
	assert.True(t, h.answered("m/a.TestA"))
	assert.True(t, h.answered("m/b.TestB"), "a run that was not cut off settles every key")
	assert.False(t, h.answered("m/c.TestC"), "a package broken on HEAD answers nothing")
	h.partial = true
	assert.True(t, h.answered("m/a.TestA"))
	assert.False(t, h.answered("m/b.TestB"), "cut off before the key ran enough times")
	assert.False(t, h.answered("m/z.TestZ"))
}

// Review A2: a failure another commit's file knows (HEAD moved since) still
// moves the cut point.
func TestKnownFailing_ReadsEveryCommitsFile(t *testing.T) {
	dir := t.TempDir()
	writeEntries(t, filepath.Join(dir, "baseline-old-x.json"), map[string]baselineEntry{
		"m.TestA": {Runs: []testRun{{Failed: true}, {Failed: true}}, At: time.Now().Add(-48 * time.Hour)},
		"m.TestB": {Runs: []testRun{{Failed: false}}}})
	c := &baselineCache{dir: dir, name: "baseline-new-x.json", entries: map[string]baselineEntry{}}
	assert.Equal(t, map[string]bool{"m.TestA": true}, c.knownFailing())
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
