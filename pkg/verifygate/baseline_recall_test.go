package verifygate

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var brokenLines = []string{"a_test.go:1: broken"}

// failingStep prints go test -json events in which every test of names
// fails with the failure line "a_test.go:1: broken", and exits at once.
func failingStep(root string, names ...string) Step {
	script := ""
	for _, n := range names {
		script += `echo '{"Action":"run","Package":"basegate","Test":"` + n + `"}'
echo '{"Action":"output","Package":"basegate","Test":"` + n + `","Output":"    a_test.go:1: broken"}'
echo '{"Action":"fail","Package":"basegate","Test":"` + n + `"}'
`
	}
	script += `echo '{"Action":"fail","Package":"basegate"}'
exit 1
`
	return Step{Name: "go test", Argv: []string{"sh", "-c", script}, Dir: root, GoTestJSON: true, BudgetBound: true,
		GoPackages: []string{"basegate"}}
}

// The Pilot C overblock: an earlier stop found tests that fail on HEAD; a
// later stop on an edited tree sees them fail again the same way, but the
// whole-package step used the budget (it is not cut early for a known
// failure) and the "fails on HEAD" answer is past its 20 minutes, so HEAD
// cannot be asked again. That must not block the stop on failures HEAD was
// already seen to share; a different failure, or one HEAD was never seen
// failing, still does.
func TestRunCharged_RecallsHEADFailuresWhenHEADCannotBeAsked(t *testing.T) {
	failed := func(lines []string, at time.Time, session string) baselineEntry {
		return baselineEntry{Runs: []testRun{{Failed: true, Lines: lines}, {Failed: true, Lines: lines}}, At: at, Session: session}
	}
	old := time.Now().Add(-45 * time.Minute)
	cases := []struct {
		name       string
		tests      []string
		entries    map[string]baselineEntry
		otherFile  bool
		charged    bool
		recalled   bool
		noteSubstr string
	}{
		{name: "expired answer, same lines", tests: []string{"TestAFails"},
			entries:  map[string]baselineEntry{"basegate.TestAFails": failed(brokenLines, old, "s1")},
			recalled: true, noteSubstr: "an earlier run on HEAD failed them with the same failure lines"},
		{name: "another session's answer, same lines", tests: []string{"TestAFails"},
			entries:  map[string]baselineEntry{"basegate.TestAFails": failed(brokenLines, time.Now(), "other")},
			recalled: true},
		{name: "another commit's answer, same lines", tests: []string{"TestAFails"}, otherFile: true,
			entries:  map[string]baselineEntry{"basegate.TestAFails": failed(brokenLines, old, "s1")},
			recalled: true},
		{name: "a fresh answer needs no time on HEAD", tests: []string{"TestAFails"},
			entries: map[string]baselineEntry{"basegate.TestAFails": failed(brokenLines, time.Now(), "s1")}},
		{name: "different failure line", tests: []string{"TestAFails"},
			entries: map[string]baselineEntry{"basegate.TestAFails": failed([]string{"a_test.go:1: other"}, old, "s1")},
			charged: true, noteSubstr: "no time was left"},
		{name: "one failure HEAD was never seen failing", tests: []string{"TestAFails", "TestNew"},
			entries: map[string]baselineEntry{"basegate.TestAFails": failed(brokenLines, old, "s1")},
			charged: true, noteSubstr: "charged to this change"},
		{name: "HEAD passed it", tests: []string{"TestAFails"},
			entries: map[string]baselineEntry{"basegate.TestAFails": {Runs: []testRun{{}, {}}, At: old}},
			charged: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := useBaselineCache(t)
			saved := BaselineCacheSession
			BaselineCacheSession = "s1"
			t.Cleanup(func() { BaselineCacheSession = saved })
			root := redRepo(t, map[string]string{"go.mod": baseModule, "add.go": baseAdd}, map[string]string{"add.go": baseAddFix})
			file := cacheFile(t, dir, root)
			if tc.otherFile {
				file = filepath.Join(dir, "baseline-0000-x.json")
			}
			writeEntries(t, file, tc.entries)

			// 3s is less than minBaselineBudget: HEAD cannot be run.
			res := RunCharged(context.Background(), root, []string{"add.go"}, []Step{failingStep(root, tc.tests...)}, 3*time.Second)
			if tc.charged {
				require.NotNil(t, res.Failure)
				assert.Empty(t, res.Recalled)
			} else {
				assert.Nil(t, res.Failure, "note: %s", res.Note())
				assert.Contains(t, res.Preexisting, "basegate.TestAFails")
			}
			if tc.recalled {
				assert.Equal(t, []string{"basegate.TestAFails"}, res.Recalled)
			} else {
				assert.Empty(t, res.Recalled)
			}
			assert.Contains(t, res.Note(), tc.noteSubstr)
		})
	}
}

// A test the change edited is its failure even when HEAD was seen failing it.
func TestRunCharged_RecallNeverExcusesAnEditedTest(t *testing.T) {
	dir := useBaselineCache(t)
	base := map[string]string{"go.mod": baseModule, "add.go": baseAdd,
		"a_test.go": "package basegate\n\nimport \"testing\"\n\nfunc TestAFails(t *testing.T) { t.Fatal(1) }\n"}
	root := redRepo(t, base, map[string]string{"a_test.go": "package basegate\n\nimport \"testing\"\n\nfunc TestAFails(t *testing.T) { t.Fatal(2) }\n"})
	writeEntries(t, cacheFile(t, dir, root), map[string]baselineEntry{"basegate.TestAFails": {
		Runs: []testRun{{Failed: true, Lines: brokenLines}, {Failed: true, Lines: brokenLines}}, At: time.Now().Add(-time.Hour)}})

	res := RunCharged(context.Background(), root, []string{"a_test.go"}, []Step{failingStep(root, "TestAFails")}, 3*time.Second)
	require.NotNil(t, res.Failure)
	assert.Equal(t, []string{"basegate.TestAFails"}, res.Edited)
	assert.Empty(t, res.Recalled)
}
