package learn

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gitProject is a repository whose .gitattributes union-merges the store,
// as this repository's does.
type gitProject struct {
	t   *testing.T
	dir string
}

func newGitProject(t *testing.T, seed ...LearningEntry) gitProject {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	p := gitProject{t: t, dir: t.TempDir()}
	p.git("init", "-q", "-b", "main")
	require.NoError(t, os.WriteFile(filepath.Join(p.dir, ".gitattributes"),
		[]byte(".autopus/learnings/pipeline.jsonl merge=union\n"), 0o644))
	for _, e := range seed {
		require.NoError(t, p.store().Append(e))
	}
	p.git("add", "-A")
	p.git("commit", "-qm", "base")
	return p
}

func (p gitProject) git(args ...string) string {
	p.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", p.dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t",
		"GIT_COMMITTER_EMAIL=t@t", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	require.NoError(p.t, err, "git %v: %s", args, out)
	return string(out)
}

func (p gitProject) store() *Store {
	s, err := NewStore(p.dir)
	require.NoError(p.t, err)
	return s
}

func (p gitProject) branch(name string, fn func(*Store)) {
	p.t.Helper()
	p.git("checkout", "-q", "-b", name, "main")
	fn(p.store())
	p.git("commit", "-qam", name)
}

func seedEntries() []LearningEntry {
	base := time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)
	var out []LearningEntry
	for i, id := range []string{"L-001", "L-002", "L-003"} {
		out = append(out, LearningEntry{ID: id, Timestamp: base.Add(time.Duration(i) * time.Hour), Type: EntryTypeGateFail, Pattern: "seed " + id})
	}
	return out
}

// One branch rewrites the last line (MarkReused) while the other appends: a
// real `git merge` under the union attribute merges cleanly, and the reader
// sees each lesson once with the rewritten reuse count.
func TestGitMerge_RewriteAndAppend(t *testing.T) {
	p := newGitProject(t, seedEntries()...)
	p.branch("a", func(s *Store) { require.NoError(t, s.MarkReused([]string{"L-003"})) })
	p.branch("b", func(s *Store) { require.NoError(t, s.AppendAtomic(EntryTypeEscape, escapeOpts("branch B defect"))) })
	p.git("checkout", "-q", "a")
	p.git("merge", "--no-edit", "b")

	data, err := os.ReadFile(p.store().path)
	require.NoError(t, err)
	assert.Equal(t, 5, strings.Count(string(data), "\n"), "the union keeps both versions of L-003")
	entries, err := p.store().Read()
	require.NoError(t, err)
	require.Len(t, entries, 4)
	assert.Equal(t, "L-003", entries[2].ID)
	assert.Equal(t, 1, entries[2].ReuseCount)
	assert.Regexp(t, `^L-004-[0-9a-f]{6}$`, entries[3].ID)
}

// Two writers that predate the suffix give different lessons the same id on
// two branches. Both lessons survive the merge, a rewrite, and MarkReused,
// and IDConflicts names the id.
func TestGitMerge_OldFormIDCollisionKeepsBoth(t *testing.T) {
	p := newGitProject(t, seedEntries()...)
	at := time.Date(2026, 11, 22, 9, 0, 0, 0, time.UTC)
	p.branch("a", func(s *Store) {
		require.NoError(t, s.Append(LearningEntry{ID: "L-004", Timestamp: at, Type: EntryTypeEscape, Pattern: "lesson A"}))
	})
	p.branch("b", func(s *Store) {
		require.NoError(t, s.Append(LearningEntry{ID: "L-004", Timestamp: at.Add(time.Second), Type: EntryTypeEscape, Pattern: "lesson B"}))
	})
	p.git("checkout", "-q", "a")
	p.git("merge", "--no-edit", "b")

	store := p.store()
	require.NoError(t, store.MarkReused([]string{"L-001"}))
	require.NoError(t, store.MarkReused([]string{"L-004"}))
	entries, err := store.Read()
	require.NoError(t, err)
	var patterns []string
	for _, e := range entries {
		if e.ID == "L-004" {
			patterns = append(patterns, e.Pattern)
			assert.Equal(t, 1, e.ReuseCount)
		}
	}
	assert.Equal(t, []string{"lesson A", "lesson B"}, patterns)
	conflicts := IDConflicts(entries)
	require.Len(t, conflicts, 1)
	assert.Equal(t, "L-004", conflicts[0].ID)
}

// The id suffix is a hash of what the store writes, never of the raw text a
// secret was redacted from.
func TestAppendAtomic_SuffixFromRedactedEntry(t *testing.T) {
	store, _ := branchStore(t, nil)
	raw := "deploy failed token=abc123 in ci"
	require.NoError(t, store.AppendAtomic(EntryTypeGateFail, RecordOpts{Phase: "ci", Pattern: raw}))
	entries, err := store.Read()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	stored := entries[0]
	require.NotContains(t, stored.Pattern, "abc123")

	_, suffix, ok := ParseEntryID(stored.ID)
	require.True(t, ok)
	unredacted := stored
	unredacted.Pattern = raw
	assert.Equal(t, entrySuffix(stored), suffix)
	assert.NotEqual(t, entrySuffix(unredacted), suffix)
}
