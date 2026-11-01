package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/diskspace"
)

func diskGitT(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t.test", "GIT_COMMITTER_NAME=t",
		"GIT_COMMITTER_EMAIL=t@t.test", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
}

// diskS7Repo builds acceptance S7 under base and returns the repository and
// its linked worktree paths by letter.
func diskS7Repo(t *testing.T, base string) (string, map[string]string) {
	t.Helper()
	repo := filepath.Join(base, "repo")
	a := diskAged{t: t, now: time.Now()}
	a.file(filepath.Join(repo, ".gitignore"), 0, 0)
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("target/\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("seed\n"), 0o644))
	diskGitT(t, repo, "init", "--initial-branch=main")
	diskGitT(t, repo, "add", ".")
	diskGitT(t, repo, "commit", "-m", "seed")
	wt := map[string]string{"a": "wt-a", "b": "wt-b", "c": "wt-c", "d": "wt-d", "e": "wt-e", "f": "wt a$(b)'c", "g": "wt\ng"}
	for name, rel := range wt {
		wt[name] = filepath.Join(repo, rel)
	}
	diskGitT(t, repo, "worktree", "add", "-b", "feat-a", wt["a"], "main")
	require.NoError(t, os.WriteFile(filepath.Join(wt["a"], "Cargo.toml"), []byte("[package]\n"), 0o644))
	diskGitT(t, wt["a"], "add", ".")
	diskGitT(t, wt["a"], "commit", "-m", "crate")
	diskGitT(t, repo, "merge", "--ff-only", "feat-a")
	a.file(filepath.Join(wt["a"], "target", "blob.bin"), 4*diskspace.MiB, 0)
	diskGitT(t, repo, "worktree", "add", "-b", "feat-b", wt["b"], "main")
	require.NoError(t, os.WriteFile(filepath.Join(wt["b"], "b.txt"), []byte("b\n"), 0o644))
	diskGitT(t, wt["b"], "add", ".")
	diskGitT(t, wt["b"], "commit", "-m", "b")
	for _, name := range []string{"c", "d", "e", "f", "g"} {
		diskGitT(t, repo, "worktree", "add", "-b", "feat-"+name, wt[name], "main")
	}
	require.NoError(t, os.WriteFile(filepath.Join(wt["c"], "tracked.txt"), []byte("modified\n"), 0o644))
	require.NoError(t, os.RemoveAll(wt["d"]))
	diskGitT(t, repo, "worktree", "lock", wt["e"])
	return repo, wt
}

// S4: from fixtures only, every consumer with exact bytes and counts, the
// floor verdict, and the dry-run preview of the trim; the text ends with the
// S5 dry-run block.
func TestDiskStatus_S4EveryConsumerFromFixtures(t *testing.T) {
	f := newDiskS5(t, false)
	withDiskSeams(t, f)
	withDiskFree(t, gib(100), diskDataMount)
	repo, _ := diskS7Repo(t, f.base)
	scratch := filepath.Join(f.base, "scratch", "claude-test")
	a := diskAged{t: t, now: f.now}
	a.file(filepath.Join(scratch, "session-a", "a.bin"), diskspace.MiB, 4320*time.Minute)
	a.stamp(filepath.Join(scratch, "session-a"), 4320*time.Minute)
	a.file(filepath.Join(scratch, "session-b", "b.bin"), 3*diskspace.MiB, 60*time.Minute)
	a.stamp(filepath.Join(scratch, "session-b"), 60*time.Minute)

	run := runDiskCLI(t, "disk", "status", "--dir", repo, "--ci", "idle", "--json")

	require.Equal(t, 0, run.code, run.stderr)
	var data diskStatusData
	require.NoError(t, json.Unmarshal(decodeDiskEnvelope(t, run.stdout).Data, &data))
	c := data.Consumers
	assert.Equal(t, 8, c.GoBuildCache.Entries)
	assert.InDelta(t, 324546560, c.GoBuildCache.Bytes, 16*4096)
	assert.Equal(t, []int{3, 1}, []int{c.GoBuildWorkDirs.Dirs, c.GoBuildWorkDirs.IdleDirs})
	assert.InDelta(t, 4296704, c.GoBuildWorkDirs.Bytes, 8*4096)
	assert.Len(t, c.Worktrees.Items, 8)
	assert.Equal(t, "main", c.Worktrees.Base)
	assert.InDelta(t, 4*diskspace.MiB, c.Worktrees.CargoTargetBytes, 2*4096)
	assert.Equal(t, []any{true, scratch, 2, int64(4320), int64(60)},
		[]any{c.Scratchpads.Available, c.Scratchpads.Root, c.Scratchpads.Dirs, c.Scratchpads.OldestAgeMinutes, c.Scratchpads.NewestAgeMinutes})
	assert.InDelta(t, 4194304, c.Scratchpads.Bytes, 6*4096)
	assert.Equal(t, int64(107374182400), data.FreeBytes)
	assert.True(t, data.Floor.OK)
	assert.Equal(t, "dry-run", data.TrimPreview.Run)
	assert.InDelta(t, 131072000, data.TrimPreview.TotalBytes, 8*4096)

	text := runDiskCLI(t, "disk", "status", "--dir", repo, "--ci", "idle")
	require.Equal(t, 0, text.code, text.stderr)
	assert.True(t, strings.HasPrefix(text.stdout, "disk status: 100.0 GiB free on /System/Volumes/Data; floor 10.0 GiB (disk.min_free_gb): ok\n"), text.stdout)
	rows, _, _ := strings.Cut(text.stdout, "disk trim: ")
	for _, row := range []string{"  go_build_cache: ", "  go_build_work_dirs: ", "  worktrees: ", "  scratchpads: "} {
		assert.Equal(t, 1, strings.Count(rows, "\n"+row), row)
	}
	assert.True(t, strings.HasSuffix(text.stdout, diskS5Block(f)), text.stdout)
}

// S7: through the real command, exactly three suggestion lines, two quoted
// commands and the withheld notice with the newline shown as ?.
func TestDiskStatus_S7SuggestionLines(t *testing.T) {
	f := newDiskS5(t, true)
	withDiskSeams(t, f)
	withDiskFree(t, gib(100), diskDataMount)
	t.Setenv("GOCACHE", "off")
	t.Setenv("GOTMPDIR", filepath.Join(f.base, "missing"))
	repo, wt := diskS7Repo(t, f.base)

	run := runDiskCLI(t, "disk", "status", "--dir", repo, "--ci", "idle")

	require.Equal(t, 0, run.code, run.stderr)
	var suggestions []string
	for _, line := range strings.Split(run.stdout, "\n") {
		if strings.Contains(line, "git worktree remove") || strings.Contains(line, "suggestion withheld") {
			suggestions = append(suggestions, strings.TrimSpace(line))
		}
	}
	require.Len(t, suggestions, 3, run.stdout)
	assert.Contains(t, suggestions, "candidate: git worktree remove -- '"+wt["a"]+"' (merged into main, clean, 4.0 MiB)")
	assert.Contains(t, run.stdout, "candidate: git worktree remove -- '"+filepath.Join(repo, `wt a$(b)'\''c`)+"' (merged into main, clean, ")
	assert.Contains(t, suggestions, "suggestion withheld: path contains control characters ("+filepath.Join(repo, "wt?g")+")")
	assert.Contains(t, run.stdout, "  go_build_cache: unavailable (gocache_off)\n")
	assert.Contains(t, run.stdout, "  go_build_work_dirs: unavailable (root_missing)\n")
}

// S4: outside a repository, with GOCACHE=off, and with a README-less cache,
// each consumer reports its reason.
func TestDiskStatus_UnavailableReasons(t *testing.T) {
	f := newDiskS5(t, true)
	withDiskSeams(t, f)
	withDiskFree(t, gib(100), diskDataMount)
	project := writeDiskProject(t, "")
	consumers := func() diskConsumers {
		run := runDiskCLI(t, "disk", "status", "--dir", project, "--ci", "idle", "--json")
		require.Equal(t, 0, run.code, run.stderr)
		var data diskStatusData
		require.NoError(t, json.Unmarshal(decodeDiskEnvelope(t, run.stdout).Data, &data))
		return data.Consumers
	}
	assert.Equal(t, []any{false, "not_a_git_repository"}, []any{consumers().Worktrees.Available, consumers().Worktrees.Reason})
	t.Setenv("GOCACHE", "off")
	assert.Equal(t, "gocache_off", consumers().GoBuildCache.Reason)
	t.Setenv("GOCACHE", f.cache)
	require.NoError(t, os.Remove(filepath.Join(f.cache, "README")))
	assert.Equal(t, "not_a_go_build_cache", consumers().GoBuildCache.Reason)
}

// S10: status and a dry-run trim leave the autopus.yaml that config.Load
// would rewrite byte-identical.
func TestDiskStatusAndTrim_NeverRewriteAutopusYAML(t *testing.T) {
	f := newDiskS5(t, true)
	withDiskSeams(t, f)
	withDiskFree(t, gib(100), diskDataMount)
	dir := t.TempDir()
	path := filepath.Join(dir, "autopus.yaml")
	require.NoError(t, os.WriteFile(path, []byte("mode: full\nproject_name: disk\nplatforms:\n  - claude\n"), 0o600))
	before := diskFileSHA(t, path)

	for _, sub := range []string{"status", "trim"} {
		run := runDiskCLI(t, "disk", sub, "--dir", dir, "--ci", "idle")
		require.Equal(t, 0, run.code, sub+": "+run.stderr)
	}

	assert.Equal(t, before, diskFileSHA(t, path))
}
