package verifygate

import (
	"context"
	"fmt"
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
// cannot be asked again. That must not block the stop on failures this
// tree's HEAD was already seen to share. A different failure, one HEAD was
// never seen failing, an answer for another commit or tree, a later "passes
// on HEAD" answer, or a refusal that is not about time still blocks.
func TestRunCharged_RecallsHEADFailuresWhenHEADCannotBeAsked(t *testing.T) {
	const here, elsewhere = "<root>", "/another/worktree"
	failed := func(lines []string, at time.Time, session, root string) baselineEntry {
		return baselineEntry{Runs: []testRun{{Failed: true, Lines: lines}, {Failed: true, Lines: lines}},
			At: at, Session: session, Root: root}
	}
	passed := baselineEntry{Runs: []testRun{{}, {}}, At: time.Now(), Root: here}
	old := time.Now().Add(-45 * time.Minute)
	many := make([]string, 0, maxBaselineTests+1)
	manyEntries := map[string]baselineEntry{}
	for i := 0; i <= maxBaselineTests; i++ {
		name := fmt.Sprintf("TestMany%02d", i)
		many = append(many, name)
		manyEntries["basegate."+name] = failed(brokenLines, old, "s1", here)
	}
	cases := []struct {
		name       string
		tests      []string
		entries    map[string]baselineEntry // this HEAD's file
		other      map[string]baselineEntry // another commit's file
		budget     time.Duration
		charged    bool
		recalled   bool
		noteSubstr string
	}{
		{name: "expired answer, same lines", tests: []string{"TestAFails"},
			entries:  map[string]baselineEntry{"basegate.TestAFails": failed(brokenLines, old, "s1", here)},
			recalled: true, noteSubstr: "an earlier run on HEAD failed them with the same failure lines"},
		{name: "another session's answer, same tree", tests: []string{"TestAFails"},
			entries:  map[string]baselineEntry{"basegate.TestAFails": failed(brokenLines, time.Now(), "other", here)},
			recalled: true},
		{name: "a fresh answer needs no time on HEAD", tests: []string{"TestAFails"},
			entries: map[string]baselineEntry{"basegate.TestAFails": failed(brokenLines, time.Now(), "s1", here)}},
		// Review A: an older commit failed it, this HEAD fixed it, the change brings it back.
		{name: "another commit's answer", tests: []string{"TestAFails"},
			other:   map[string]baselineEntry{"basegate.TestAFails": failed(brokenLines, old, "s1", here)},
			charged: true, noteSubstr: "no time was left"},
		// Review B: this HEAD passed TestAFails; another commit's failure must not win.
		{name: "this HEAD passed it, another commit failed it", tests: []string{"TestAFails", "TestBFails"},
			entries: map[string]baselineEntry{"basegate.TestAFails": passed, "basegate.TestBFails": failed(brokenLines, old, "s1", here)},
			other:   map[string]baselineEntry{"basegate.TestAFails": failed(brokenLines, old, "s1", here)},
			charged: true},
		// Review C: another worktree at the same commit is not this tree.
		{name: "another tree's answer", tests: []string{"TestAFails"},
			entries: map[string]baselineEntry{"basegate.TestAFails": failed(brokenLines, old, "s1", elsewhere)},
			charged: true},
		{name: "answer older than the recall cap", tests: []string{"TestAFails"},
			entries: map[string]baselineEntry{"basegate.TestAFails": failed(brokenLines, time.Now().Add(-recallMaxAge-time.Minute), "s1", here)},
			charged: true},
		{name: "different failure line", tests: []string{"TestAFails"},
			entries: map[string]baselineEntry{"basegate.TestAFails": failed([]string{"a_test.go:1: other"}, old, "s1", here)},
			charged: true},
		{name: "one failure HEAD was never seen failing", tests: []string{"TestAFails", "TestNew"},
			entries: map[string]baselineEntry{"basegate.TestAFails": failed(brokenLines, old, "s1", here)},
			charged: true, noteSubstr: "charged to this change"},
		{name: "HEAD passed it", tests: []string{"TestAFails"},
			entries: map[string]baselineEntry{"basegate.TestAFails": passed}, charged: true},
		// Review 2: the 50-test cap is policy, not time; recall does not answer it.
		{name: "more failures than are checked", tests: many, entries: manyEntries, budget: 3 * time.Minute,
			charged: true, noteSubstr: "more than the"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := useBaselineCache(t)
			saved := BaselineCacheSession
			BaselineCacheSession = "s1"
			t.Cleanup(func() { BaselineCacheSession = saved })
			root := redRepo(t, map[string]string{"go.mod": baseModule, "add.go": baseAdd}, map[string]string{"add.go": baseAddFix})
			rooted := func(in map[string]baselineEntry) map[string]baselineEntry {
				out := map[string]baselineEntry{}
				for k, e := range in {
					if e.Root == here {
						e.Root = root
					}
					out[k] = e
				}
				return out
			}
			writeEntries(t, cacheFile(t, dir, root), rooted(tc.entries))
			if tc.other != nil {
				writeEntries(t, filepath.Join(dir, "baseline-0000-x.json"), rooted(tc.other))
			}
			budget := tc.budget
			if budget == 0 {
				// Less than minBaselineBudget: HEAD cannot be run.
				budget = 3 * time.Second
			}
			res := RunCharged(context.Background(), root, []string{"add.go"}, []Step{failingStep(root, tc.tests...)}, budget)
			if tc.charged {
				require.NotNil(t, res.Failure)
				assert.Empty(t, res.Recalled)
				assert.NotContains(t, res.Note(), "also fail on HEAD")
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
		Runs: []testRun{{Failed: true, Lines: brokenLines}, {Failed: true, Lines: brokenLines}}, At: time.Now().Add(-time.Hour), Root: root}})

	res := RunCharged(context.Background(), root, []string{"a_test.go"}, []Step{failingStep(root, "TestAFails")}, 3*time.Second)
	require.NotNil(t, res.Failure)
	assert.Equal(t, []string{"basegate.TestAFails"}, res.Edited)
	assert.Empty(t, res.Recalled)
}
