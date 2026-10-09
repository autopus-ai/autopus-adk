//go:build unix

package healthband

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// gpHostile is a repository in which every source that the Git Execution
// Policy neutralizes writes a marker when git runs it: executable hooks of
// every type in the common hooks directory, a local fsmonitor, an
// allowlisted git-lfs driver
// with filter.lfs.required=true, a local textconv driver, a global
// attributes file that selects a global filter, a global diff.external, a
// replace ref of the base commit, and inherited GIT_* variables aimed at the
// user's checkout. Probe A3 wrote 15 distinct markers from such sources.
type gpHostile struct {
	*gpFixture
	repo, base, realTree string
	bandEnv, controlEnv  []string
	bandTrace            string
	controlTrace         string
}

var gpHookNames = []string{
	"post-checkout", "pre-commit", "prepare-commit-msg", "commit-msg", "post-commit",
	"reference-transaction", "post-index-change", "post-rewrite", "pre-auto-gc",
}

// gpCanaryLink is the link text of the tracked docs/canary.md symlink; from
// <root>/lp/key/worktree/docs/ it resolves to <root>/lp/canary.txt.
const gpCanaryLink = "../../../canary.txt"

// gpLFSPointer is a git-lfs pointer file; band checks it out as is.
var gpLFSPointer = "version https://git-lfs.github.com/spec/v1\noid sha256:" + strings.Repeat("a", 64) + "\nsize 12\n"

// gpPatch modifies a Go file, a textconv file, and the LFS pointer, and adds a
// Go file that the global attributes file would filter.
const gpPatch = `diff --git a/pkg/foo/foo.go b/pkg/foo/foo.go
--- a/pkg/foo/foo.go
+++ b/pkg/foo/foo.go
@@ -1,3 +1,3 @@
 package foo

-func A() int { return 1 }
+func A() int { return 2 }
diff --git a/notes.txt b/notes.txt
--- a/notes.txt
+++ b/notes.txt
@@ -1 +1 @@
-one
+two
diff --git a/data.bin b/data.bin
--- a/data.bin
+++ b/data.bin
@@ -1,3 +1,3 @@
 version https://git-lfs.github.com/spec/v1
-oid sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
-size 12
+oid sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
+size 13
diff --git a/pkg/foo/new.go b/pkg/foo/new.go
new file mode 100644
--- /dev/null
+++ b/pkg/foo/new.go
@@ -0,0 +1 @@
+package foo
`

// newGPHostile builds the repository; runner is the environment through which
// a CI runner's git reaches raw git (setup and control), such as
// GIT_CONFIG_SYSTEM. Band strips every GIT_* variable, so none reaches it.
func newGPHostile(t *testing.T, runner ...string) *gpHostile {
	t.Helper()
	f := newGPFixture(t)
	f.setupEnv = append(f.setupEnv, runner...)
	h := &gpHostile{gpFixture: f, bandTrace: filepath.Join(f.root, "trace-band.json"),
		controlTrace: filepath.Join(f.root, "trace-control.json")}
	h.repo, h.base = f.repo("repo", map[string]string{
		".gitattributes": "*.bin filter=lfs\n*.txt diff=tc\n",
		"data.bin":       gpLFSPointer, "notes.txt": "one\n",
		"pkg/foo/foo.go": "package foo\n\nfunc A() int { return 1 }\n",
		"docs/canary.md": "->" + gpCanaryLink,
	})
	h.realTree = strings.TrimSpace(f.git(f.setupEnv, h.repo, "rev-parse", h.base+"^{tree}"))
	h.replaceBase()
	// One marker script behind symlinks: macOS assesses each new executable
	// on its first run, which made a script per source slow.
	f.script("mark", "M='"+f.markers+"'\ncase \"${0##*/}\" in\n"+
		"git-lfs) touch \"$M/lfs-$1\"; exec cat ;;\ngmark) touch \"$M/gmark-$1\"; exec cat ;;\n"+
		"tc) touch \"$M/textconv\"; exec cat \"$1\" ;;\nfsmon) touch \"$M/fsmonitor-local\" ;;\n"+
		"envmon) touch \"$M/fsmonitor-env\" ;;\nextdiff) touch \"$M/diff-external\" ;;\n"+
		"envdiff) touch \"$M/diff-env\" ;;\n*) touch \"$M/hook-${0##*/}\" ;;\nesac\n")
	for _, name := range []string{"git-lfs", "gmark", "tc", "fsmon", "envmon", "extdiff", "envdiff"} {
		require.NoError(t, os.Symlink(filepath.Join(f.bin, "mark"), filepath.Join(f.bin, name)))
	}
	for _, name := range gpHookNames {
		require.NoError(t, os.Symlink(filepath.Join(f.bin, "mark"), filepath.Join(h.repo, ".git", "hooks", name)))
	}
	for key, value := range map[string]string{
		"core.fsmonitor":    filepath.Join(f.bin, "fsmon"),
		"filter.lfs.smudge": filepath.Join(f.bin, "git-lfs") + " smudge -- %f",
		"filter.lfs.clean":  filepath.Join(f.bin, "git-lfs") + " clean -- %f", "filter.lfs.required": "true",
		"diff.tc.textconv": filepath.Join(f.bin, "tc"),
		"author.name":      "User", "author.email": "user@example.invalid", "committer.email": "user@example.invalid",
	} {
		f.git(f.setupEnv, h.repo, "config", key, value)
	}
	attrs := filepath.Join(f.root, "global-attrs")
	require.NoError(t, os.WriteFile(attrs, []byte("*.go filter=gmark\n"), 0o600))
	global := "[core]\n\tattributesFile = " + attrs + "\n[filter \"gmark\"]\n\tclean = " +
		filepath.Join(f.bin, "gmark") + " clean\n\tsmudge = " + filepath.Join(f.bin, "gmark") +
		" smudge\n[diff]\n\texternal = " + filepath.Join(f.bin, "extdiff") +
		"\n[author]\n\temail = global@example.invalid\n"
	h.controlEnv = append(f.home("control-home", global, h.controlTrace), runner...)
	h.bandEnv = append(f.home("band-home", global, h.bandTrace),
		"GIT_DIR="+filepath.Join(h.repo, ".git"), "GIT_WORK_TREE="+h.repo,
		"GIT_INDEX_FILE="+filepath.Join(h.repo, ".git", "index"),
		"GIT_CONFIG_PARAMETERS='core.fsmonitor'='"+filepath.Join(f.bin, "envmon")+"'",
		"GIT_EXTERNAL_DIFF="+filepath.Join(f.bin, "envdiff"),
		"GIT_TRACE2_EVENT="+filepath.Join(f.root, "leak.json"))
	require.Empty(t, f.markerNames(), "setup must not run a marker source")
	return h
}

// replaceBase maps the base commit to a commit whose tree adds a workflow,
// through a temp index, before any hostile local setting exists.
func (h *gpHostile) replaceBase() {
	env := append(slices.Clone(h.setupEnv), "GIT_INDEX_FILE="+filepath.Join(h.root, "replace.index"))
	h.git(env, h.repo, "read-tree", h.base)
	h.writeFiles(h.repo, map[string]string{".github/workflows/x.yaml": "on: push\n"})
	h.git(env, h.repo, "add", "-f", ".github/workflows/x.yaml")
	tree := strings.TrimSpace(h.git(env, h.repo, "write-tree"))
	replacement := strings.TrimSpace(h.git(h.setupEnv, h.repo, "commit-tree", tree, "-m", "replaced"))
	h.git(h.setupEnv, h.repo, "replace", h.base, replacement)
	require.NoError(h.t, os.RemoveAll(filepath.Join(h.repo, ".github")))
}

// snapshot hashes every file of the user's checkout outside .git/ plus its
// index, HEAD, and packed and loose refs other than refs/heads/autopus/.
func (h *gpHostile) snapshot() map[string]string {
	hashes := map[string]string{}
	err := filepath.WalkDir(h.repo, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(h.repo, path)
		keep := !strings.HasPrefix(rel, ".git"+string(filepath.Separator)) ||
			slices.Contains([]string{".git/index", ".git/HEAD", ".git/config", ".git/packed-refs"}, filepath.ToSlash(rel)) ||
			strings.HasPrefix(filepath.ToSlash(rel), ".git/refs/") && !strings.Contains(rel, "autopus")
		if keep {
			data, readErr := os.ReadFile(path)
			if entry.Type()&fs.ModeSymlink != 0 {
				var target string
				target, readErr = os.Readlink(path)
				data = []byte("link:" + target)
			}
			if readErr != nil {
				return readErr
			}
			sum := sha256.Sum256(data)
			hashes[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
		}
		return nil
	})
	require.NoError(h.t, err)
	return hashes
}
