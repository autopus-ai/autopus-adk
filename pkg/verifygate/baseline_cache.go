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
const baselineFailTTL = 20 * time.Minute

// baselineEntry is what one key did on HEAD and when: its runs there, paths
// normalized. No runs means HEAD ran the test and it did not fail.
type baselineEntry struct {
	Runs    []testRun `json:"runs"`
	At      time.Time `json:"at"`
	Session string    `json:"session,omitempty"`
	// Root is the work tree that asked; another tree at the same commit
	// shares the file but not its fallback (see recallable).
	Root string `json:"root,omitempty"`
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
	dir, name, root string
	entries         map[string]baselineEntry
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
	c := &baselineCache{dir: BaselineCacheDir, root: root, name: "baseline-" + head + "-" + EnvDigest(ctx, root) + ".json"}
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

// knownFailing lists the keys this tree's HEAD (same commit and Go
// environment) failed every time it ran them, within recallMaxAge: a step
// that sees one of them fail again need not stop early to check it, since
// the cache or recall answers it without time on HEAD. It only moves the
// cut point; whether the failure is charged is still judged on this HEAD.
func (c *baselineCache) knownFailing() map[string]bool {
	if c == nil {
		return nil
	}
	out := map[string]bool{}
	now := time.Now()
	for key := range c.entries {
		if c.recallable(key, now) {
			out[key] = true
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
			c.entries[key] = baselineEntry{Runs: head.runs[key], At: now, Session: BaselineCacheSession, Root: c.root}
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

// unanswered lists the keys the cache cannot answer for HEAD now: only they
// need time on HEAD.
func (c *baselineCache) unanswered(keys []string) []string {
	if c == nil {
		return keys
	}
	now := time.Now()
	var out []string
	for _, key := range keys {
		if e, ok := c.entries[key]; !ok || !e.usable(now, BaselineCacheSession) {
			out = append(out, key)
		}
	}
	return out
}

// recallMaxAge bounds how old a "fails on HEAD" answer recall may use.
const recallMaxAge = 4 * time.Hour

// recallable reports whether key's entry, the latest HEAD answer for this
// commit and Go environment, is a failure this work tree recorded within
// recallMaxAge. A later "passes on HEAD" answer replaces the entry, so it
// wins; an answer for another commit, environment or tree never counts.
func (c *baselineCache) recallable(key string, now time.Time) bool {
	e, ok := c.entries[key]
	return ok && e.failed(key) && e.Root == c.root && now.Sub(e.At) <= recallMaxAge
}

// recall is the fallback for a stop that ran out of time to ask HEAD (no
// time was left, or the run there was cut off). Each key counts as
// pre-existing only when it is recallable and this tree fails it with the
// same failure lines; it reports true only when every key is. The session
// and baselineFailTTL limits are what it relaxes: the same tree's HEAD
// answer for the same commit, a few hours old, is weaker evidence than a
// fresh run, but charging an unchanged failure HEAD was seen to share only
// because the budget ran out blocks on the tree's breakage, not the change.
// A stop that can ask HEAD always does.
func (c *baselineCache) recall(keys []string, current map[string][]testRun, moves map[string]string) bool {
	if c == nil || len(keys) == 0 {
		return false
	}
	now := time.Now()
	runs := map[string][]testRun{}
	for _, key := range keys {
		if !c.recallable(key, now) {
			return false
		}
		runs[key] = c.entries[key].Runs
	}
	runs = movedRuns(runs, moves)
	for _, key := range keys {
		if !sameFailure(current[key], runs[key], wantRuns(key)) {
			return false
		}
	}
	return true
}
