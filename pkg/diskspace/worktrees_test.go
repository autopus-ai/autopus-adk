package diskspace

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var hermeticGitEnv = []string{
	"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t.test", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t.test",
	"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
}

// hermeticRunner is ExecRunner without the developer's git configuration.
func hermeticRunner(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), hermeticGitEnv...), env...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	return stdout.Bytes(), err
}

func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := hermeticRunner(context.Background(), dir, nil, "git", args...)
	require.NoError(t, err, "git %v in %s", args, dir)
	return string(out)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// s7Repo builds acceptance S7 and returns the repository and the paths of
// wt-a through wt-g.
func s7Repo(t *testing.T) (string, map[string]string) {
	t.Helper()
	repo := filepath.Join(canonicalTempDir(t), "repo")
	require.NoError(t, os.MkdirAll(repo, 0o755))
	gitT(t, repo, "init", "--initial-branch=main")
	writeFile(t, filepath.Join(repo, ".gitignore"), "target/\n")
	writeFile(t, filepath.Join(repo, "tracked.txt"), "seed\n")
	gitT(t, repo, "add", ".")
	gitT(t, repo, "commit", "-m", "seed")
	wt := map[string]string{
		"a": filepath.Join(repo, "wt-a"), "b": filepath.Join(repo, "wt-b"), "c": filepath.Join(repo, "wt-c"),
		"d": filepath.Join(repo, "wt-d"), "e": filepath.Join(repo, "wt-e"),
		"f": filepath.Join(repo, "wt a$(b)'c"), "g": filepath.Join(repo, "wt\ng"),
	}
	gitT(t, repo, "worktree", "add", "-b", "feat-a", wt["a"], "main")
	writeFile(t, filepath.Join(wt["a"], "crate", "Cargo.toml"), "[package]\nname = \"crate\"\n")
	gitT(t, wt["a"], "add", ".")
	gitT(t, wt["a"], "commit", "-m", "crate")
	gitT(t, repo, "merge", "--ff-only", "feat-a")
	a := &aged{now: time.Now()}
	a.file(filepath.Join(wt["a"], "crate", "target", "blob.bin"), 4*MiB, 0)
	require.NoError(t, a.err)
	gitT(t, repo, "worktree", "add", "-b", "feat-b", wt["b"], "main")
	writeFile(t, filepath.Join(wt["b"], "b.txt"), "b\n")
	gitT(t, wt["b"], "add", ".")
	gitT(t, wt["b"], "commit", "-m", "b")
	for _, name := range []string{"c", "d", "e", "f", "g"} {
		gitT(t, repo, "worktree", "add", "-b", "feat-"+name, wt[name], "main")
	}
	writeFile(t, filepath.Join(wt["c"], "tracked.txt"), "modified\n")
	require.NoError(t, os.RemoveAll(wt["d"]))
	gitT(t, repo, "worktree", "lock", "--reason", "busy", wt["e"])
	return repo, wt
}

// S7: eight items classified with tri-state answers and reasons, exactly two
// commands and one withheld notice, and a repository left byte-identical.
func TestScanWorktrees_S7ClassifiesQuotesAndNeverTouches(t *testing.T) {
	t.Parallel()
	repo, wt := s7Repo(t)
	listBefore := gitT(t, repo, "worktree", "list", "--porcelain", "-z")
	before := snapshot(t, filepath.Dir(repo))

	scan := ScanWorktrees(context.Background(), hermeticRunner, repo)

	assert.Equal(t, Root{Path: repo}, scan.Root)
	assert.Equal(t, "main", scan.Base)
	require.Len(t, scan.Items, 8)
	byPath := map[string]Worktree{}
	for _, item := range scan.Items {
		byPath[item.Path] = item
	}
	main := scan.Items[0]
	assert.True(t, main.Main)
	assert.Equal(t, repo, main.Path)
	assert.Empty(t, main.Suggestion)

	wtA := byPath[wt["a"]]
	assert.Equal(t, []any{True, True, "main", false}, []any{wtA.Clean, wtA.Merged, wtA.Base, wtA.Harness})
	assert.InDelta(t, 4*MiB, wtA.CargoTargetBytes, 2*blockTolerance)
	assert.Equal(t, []Tristate{True, False}, []Tristate{byPath[wt["b"]].Clean, byPath[wt["b"]].Merged})
	assert.Equal(t, []Tristate{False, True}, []Tristate{byPath[wt["c"]].Clean, byPath[wt["c"]].Merged})
	wtD := byPath[wt["d"]]
	assert.Equal(t, []any{true, Unknown, Unknown, ReasonPathMissing}, []any{wtD.Prunable, wtD.Clean, wtD.Merged, wtD.Reason})
	assert.True(t, byPath[wt["e"]].Locked)
	for _, name := range []string{"f", "g"} {
		assert.Equal(t, []Tristate{True, True}, []Tristate{byPath[wt[name]].Clean, byPath[wt[name]].Merged}, name)
	}

	assert.Equal(t, "candidate: git worktree remove -- '"+wt["a"]+"' (merged into main, clean, 4.0 MiB)", wtA.Suggestion)
	assert.Equal(t, "candidate: git worktree remove -- '"+filepath.Join(repo, `wt a$(b)'\''c`)+"' (merged into main, clean, "+
		SizeString(byPath[wt["f"]].Bytes)+")", byPath[wt["f"]].Suggestion)
	assert.Equal(t, SuggestionWithheld, byPath[wt["g"]].Suggestion)
	assert.Equal(t, filepath.Join(repo, "wt?g"), DisplayPath(wt["g"]))
	for _, name := range []string{"b", "c", "d", "e"} {
		assert.Empty(t, byPath[wt[name]].Suggestion, name)
	}
	assert.Equal(t, listBefore, gitT(t, repo, "worktree", "list", "--porcelain", "-z"))
	assert.Equal(t, before, snapshot(t, filepath.Dir(repo)))
}

// S7: a worktree under .autopus/worktrees of the main worktree is a harness
// worktree; outside a repository the consumer is unavailable.
func TestScanWorktrees_HarnessAndOutsideARepository(t *testing.T) {
	t.Parallel()
	repo := filepath.Join(canonicalTempDir(t), "repo")
	require.NoError(t, os.MkdirAll(repo, 0o755))
	gitT(t, repo, "init", "--initial-branch=trunk")
	writeFile(t, filepath.Join(repo, "a.txt"), "a\n")
	gitT(t, repo, "add", ".")
	gitT(t, repo, "commit", "-m", "a")
	harness := filepath.Join(repo, ".autopus", "worktrees", "h1")
	gitT(t, repo, "worktree", "add", "-b", "feat-h", harness, "trunk")

	scan := ScanWorktrees(context.Background(), hermeticRunner, repo)

	require.Len(t, scan.Items, 2)
	assert.True(t, scan.Items[1].Harness)
	assert.Equal(t, BaseUnknown, scan.Base, "no ref of the ladder exists")
	assert.Equal(t, Unknown, scan.Items[1].Merged)
	assert.Equal(t, ReasonNoBase, scan.Items[1].Reason)
	assert.Empty(t, scan.Items[1].Suggestion)

	outside := ScanWorktrees(context.Background(), hermeticRunner, canonicalTempDir(t))
	assert.Equal(t, ReasonNotAGitRepository, outside.Root.Reason)
}

func TestParseWorktreePorcelain_ReadsNULRecords(t *testing.T) {
	t.Parallel()
	out := "worktree /r\x00bare\x00\x00worktree /a b\x00HEAD abc\x00detached\x00locked\x00\x00" +
		"worktree /n\nl\x00HEAD def\x00branch refs/heads/x/y\x00prunable gitdir file points to non-existent location\x00\x00"
	assert.Equal(t, []Worktree{
		{Path: "/r", Bare: true},
		{Path: "/a b", Head: "abc", Detached: true, Locked: true},
		{Path: "/n\nl", Head: "def", Branch: "x/y", Prunable: true},
	}, parseWorktreePorcelain([]byte(out)))
}

func TestShellQuoteAndTristateJSON(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "'/a b'", ShellQuote("/a b"))
	assert.Equal(t, `'/it'\''s'`, ShellQuote("/it's"))
	data, err := json.Marshal([]Tristate{True, False, Unknown})
	require.NoError(t, err)
	assert.Equal(t, `[true,false,"unknown"]`, string(data))
	var back []Tristate
	require.NoError(t, json.Unmarshal(data, &back))
	assert.Equal(t, []Tristate{True, False, Unknown}, back)
	assert.Error(t, json.Unmarshal([]byte(`["maybe"]`), &back))
	assert.Equal(t, "/a?b?c", DisplayPath("/a\tb\x7fc"))
}
