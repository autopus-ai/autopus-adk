package verifygate

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/privatedir"
)

// BaselineCacheDir is where RunCharged keeps what each failed test did on
// HEAD, one file per HEAD commit, so a test that fails before the change is
// run on HEAD at most once per commit and not again at every stop. "" turns
// the cache off. The hook points it at its private state directory.
var BaselineCacheDir string

// baselineCacheMaxAge is how long a commit's file is kept after its last
// write; HEAD has usually moved on by then.
const baselineCacheMaxAge = 7 * 24 * time.Hour

// baselineEntry is what one key did on HEAD: its runs there, paths
// normalized. No runs means HEAD ran the test and it did not fail.
type baselineEntry struct {
	Runs []testRun `json:"runs"`
}

// baselineCache is the cache file of one HEAD commit.
type baselineCache struct {
	dir, name string
	entries   map[string]baselineEntry
}

// loadBaselineCache reads the cache of root's HEAD; nil when the cache is
// off or HEAD has no commit. A file that cannot be read is an empty cache.
func loadBaselineCache(ctx context.Context, root string) *baselineCache {
	if BaselineCacheDir == "" {
		return nil
	}
	head := headCommit(ctx, root)
	if head == "" {
		return nil
	}
	c := &baselineCache{dir: BaselineCacheDir, name: "baseline-" + head + ".json", entries: map[string]baselineEntry{}}
	if data, err := privatedir.ReadFile(c.dir, c.name); err == nil {
		_ = json.Unmarshal(data, &c.entries)
	}
	return c
}

// knownFailing lists the keys HEAD failed every time it ran them: a step
// that sees one of them fail again need not stop early to check it.
func (c *baselineCache) knownFailing() map[string]bool {
	if c == nil {
		return nil
	}
	out := map[string]bool{}
	for key, e := range c.entries {
		failed := 0
		for _, r := range e.Runs {
			if r.Failed {
				failed++
			}
		}
		if failed > 0 && failed == len(e.Runs) && failed >= wantRuns(key) {
			out[key] = true
		}
	}
	return out
}

// wantRuns is how many HEAD runs a key needs to be compared.
func wantRuns(key string) int {
	if _, _, ok := splitTestKey(key); ok {
		return headCount
	}
	return 1
}

// headFailures is headFailures with the cache in front: keys the cache
// answers are not run again, the others run on HEAD and are stored. A key
// HEAD could not answer for (its package broken there, a run cut off before
// it) is not stored, so it is tried again next time.
func (c *baselineCache) headFailures(ctx context.Context, root string, keys []string, budget time.Duration) headResult {
	if c == nil {
		return headFailures(ctx, root, keys, budget)
	}
	res := headResult{runs: map[string][]testRun{}, brokenPkgs: map[string]bool{}, decided: map[string]bool{}}
	var missing []string
	for _, key := range keys {
		if e, ok := c.entries[key]; ok {
			res.runs[key], res.decided[key] = e.Runs, true
			continue
		}
		missing = append(missing, key)
	}
	if len(missing) == 0 {
		return res
	}
	head := headFailures(ctx, root, missing, budget)
	if head.unavailable != "" || head.interrupted {
		return head
	}
	for key, runs := range head.runs {
		if !res.decided[key] {
			res.runs[key] = runs
		}
	}
	for pkg := range head.brokenPkgs {
		res.brokenPkgs[pkg] = true
	}
	res.partial = head.partial
	for _, key := range missing {
		pkg, _, isTest := splitTestKey(key)
		if (isTest && head.brokenPkgs[pkg]) || (head.partial && len(head.runs[key]) < wantRuns(key)) {
			continue
		}
		c.entries[key] = baselineEntry{Runs: head.runs[key]}
	}
	c.save()
	return res
}

// save writes the cache and drops the files of commits not written for
// baselineCacheMaxAge. A cache that cannot be written only costs a rerun.
func (c *baselineCache) save() {
	data, err := json.Marshal(c.entries)
	if err != nil || privatedir.WriteFile(c.dir, c.name, data) != nil {
		return
	}
	files, _ := os.ReadDir(c.dir)
	for _, f := range files {
		name := f.Name()
		if name == c.name || !strings.HasPrefix(name, "baseline-") || !strings.HasSuffix(name, ".json") {
			continue
		}
		if info, err := f.Info(); err == nil && time.Since(info.ModTime()) > baselineCacheMaxAge {
			_ = os.Remove(filepath.Join(c.dir, name))
		}
	}
}

// withKnownFailing gives each whole-package Go test step the keys HEAD is
// known to fail, without changing the caller's steps.
func withKnownFailing(steps []Step, known map[string]bool) []Step {
	if len(known) == 0 {
		return steps
	}
	out := make([]Step, len(steps))
	for i, s := range steps {
		if s.GoTestJSON {
			s.KnownFailing = known
		}
		out[i] = s
	}
	return out
}
