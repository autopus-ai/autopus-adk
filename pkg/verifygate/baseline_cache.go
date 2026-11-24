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

// BaselineCacheSession names the agent session the run belongs to. A
// "fails on HEAD" answer only excuses failures in the session that asked.
var BaselineCacheSession string

// baselineCacheMaxAge is how long a commit's file is kept after its last
// write; HEAD has usually moved on by then.
const baselineCacheMaxAge = 7 * 24 * time.Hour

// baselineFailTTL is how long, within the session that asked, a "fails on
// HEAD" answer may excuse a failure.
// A failure HEAD has can be the machine's (a service down, a tool missing)
// and go away; an old answer would then excuse a real regression with the
// same failure line. A "passes on HEAD" answer only ever charges the change,
// so it is kept as long as the file.
const baselineFailTTL = time.Hour

// baselineEntry is what one key did on HEAD and when: its runs there, paths
// normalized. No runs means HEAD ran the test and it did not fail.
type baselineEntry struct {
	Runs    []testRun `json:"runs"`
	At      time.Time `json:"at"`
	Session string    `json:"session,omitempty"`
}

// failed reports whether HEAD failed the key every time it ran it, enough
// times to compare.
func (e baselineEntry) failed(key string) bool {
	n := 0
	for _, r := range e.Runs {
		if r.Failed {
			n++
		}
	}
	return n > 0 && n == len(e.Runs) && n >= wantRuns(key)
}

// usable reports whether the entry may answer for HEAD now, in session.
func (e baselineEntry) usable(now time.Time, session string) bool {
	for _, r := range e.Runs {
		if r.Failed {
			return e.Session == session && now.Sub(e.At) <= baselineFailTTL
		}
	}
	return true
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
	c := &baselineCache{dir: BaselineCacheDir, name: "baseline-" + head + "-" + EnvDigest(ctx, root) + ".json"}
	c.entries = readBaselineFile(c.dir, c.name)
	return c
}

func readBaselineFile(dir, name string) map[string]baselineEntry {
	entries := map[string]baselineEntry{}
	if data, err := privatedir.ReadFile(dir, name); err == nil {
		_ = json.Unmarshal(data, &entries)
	}
	if entries == nil {
		// A file holding null.
		entries = map[string]baselineEntry{}
	}
	return entries
}

// knownFailing lists the keys a recent HEAD failed every time it ran them,
// from every cache file, whatever its commit or age: a step that sees one of
// them fail again need not stop early to check it. It only moves the cut
// point; whether the failure is charged is still judged on this HEAD. Without
// the other commits a HEAD that moves at every stop would cut every run early.
func (c *baselineCache) knownFailing() map[string]bool {
	if c == nil {
		return nil
	}
	out := map[string]bool{}
	add := func(entries map[string]baselineEntry) {
		for key, e := range entries {
			if e.failed(key) {
				out[key] = true
			}
		}
	}
	add(c.entries)
	files, _ := os.ReadDir(c.dir)
	for _, f := range files {
		if name := f.Name(); name != c.name && isBaselineFile(name) {
			add(readBaselineFile(c.dir, name))
		}
	}
	return out
}

func isBaselineFile(name string) bool {
	return strings.HasPrefix(name, "baseline-") && strings.HasSuffix(name, ".json")
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
	now := time.Now()
	for _, key := range keys {
		if e, ok := c.entries[key]; ok && e.usable(now, BaselineCacheSession) {
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
		if head.answered(key) {
			c.entries[key] = baselineEntry{Runs: head.runs[key], At: now, Session: BaselineCacheSession}
		}
	}
	c.save()
	return res
}

// answered reports whether the HEAD run settled key: its package built and
// ran there, and the run was not cut off before key ran enough times. Only a
// settled key may be cached; another is asked again next time.
func (h headResult) answered(key string) bool {
	pkg, _, isTest := splitTestKey(key)
	if isTest && h.brokenPkgs[pkg] {
		return false
	}
	return !h.partial || len(h.runs[key]) >= wantRuns(key)
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
		if name == c.name || !isBaselineFile(name) {
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
