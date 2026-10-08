package healthband_test

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// Shared fixtures of the Patch Policy tests: a real temp repository at a base
// commit stands in for the band worktree (core.symlinks=false, as Git
// Execution Policy item 2 checks it out) and for the user's checkout, and a
// plain recording runner stands in for the hardened runner of T4.

// baseFile is one tracked entry of a fixture's base commit. A 120000 entry
// holds its link text and a 160000 entry its commit OID.
type baseFile struct {
	path, content, mode string
}

// policyBase is the S4 setup's base plus the tracked entries the S5 rows name.
var policyBase = []baseFile{
	{path: "pkg/foo/foo.go", content: "package foo\n\nfunc Foo() int { return 1 }\n"},
	{path: "pkg/foo/foo_test.go", content: "package foo\n"},
	{path: "pkg/foo/\u00e9t\u00e9.go", content: "package foo\n"},
	{path: "pkg/foo/model.go", content: "package foo\n"},
	{path: ".gitattributes", content: "pkg/foo/model.go filter=lfs diff=lfs merge=lfs -text\n"},
	{path: "scripts/build.py", content: "print(1)\n"},
	{path: "tools/gen.py", content: "print(2)\n", mode: "100755"},
	{path: "go.mod", content: "module example.com/x\n\ngo 1.26\n"},
	{path: "package.json", content: "{}\n"},
	{path: "link.go", content: "../target", mode: "120000"},
	{path: "pkg/sub", content: "1111111111111111111111111111111111111111", mode: "160000"},
}

// policyRepo is one fixture repository checked out at its base commit.
type policyRepo struct {
	dir, home, base string
}

// newPolicyRepo builds a repository whose only commit holds files, with the
// exact modes given, and checks it out with core.symlinks=false.
func newPolicyRepo(t *testing.T, files []baseFile) *policyRepo {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	repo := &policyRepo{dir: dir, home: t.TempDir()}
	repo.git(t, nil, "init", "-q", "-b", "main")
	for _, kv := range [][2]string{{"core.symlinks", "false"}, {"user.name", "fixture"}, {"user.email", "fixture@example.invalid"}} {
		repo.git(t, nil, "config", kv[0], kv[1])
	}
	// One hash-object call writes every blob from a scratch file per entry.
	scratch := t.TempDir()
	var blobs []string
	for i, file := range files {
		if file.mode != "160000" {
			blob := filepath.Join(scratch, fmt.Sprint(i))
			require.NoError(t, os.WriteFile(blob, []byte(file.content), 0o600))
			blobs = append(blobs, blob)
		}
	}
	oids := strings.Fields(repo.git(t, []byte(strings.Join(blobs, "\n")+"\n"), "hash-object", "-w", "--stdin-paths"))
	var index strings.Builder
	for _, file := range files {
		mode, oid := file.mode, file.content
		if mode == "" {
			mode = "100644"
		}
		if mode != "160000" {
			oid, oids = oids[0], oids[1:]
		}
		fmt.Fprintf(&index, "%s %s\t%s\n", mode, oid, file.path)
	}
	repo.git(t, []byte(index.String()), "update-index", "--add", "--index-info")
	tree := strings.TrimSpace(repo.git(t, nil, "write-tree"))
	repo.base = strings.TrimSpace(repo.git(t, []byte("base\n"), "commit-tree", tree))
	repo.git(t, nil, "update-ref", "refs/heads/main", repo.base)
	repo.git(t, nil, "reset", "-q", "--hard", "main")
	return repo
}

// git runs a fixture command that must succeed and returns its stdout.
func (r *policyRepo) git(t *testing.T, stdin []byte, args ...string) string {
	t.Helper()
	out, err := runTestGit(context.Background(), r.dir, r.home, stdin, args...)
	require.NoError(t, err)
	return string(out)
}

// objects lists every file under .git/objects with its size. Any object
// write changes it, so an unchanged listing implies an unchanged
// `git count-objects -v` output (S5) without a git process per check.
func (r *policyRepo) objects(t *testing.T) string {
	t.Helper()
	root := filepath.Join(r.dir, ".git", "objects")
	var listing strings.Builder
	require.NoError(t, filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		fmt.Fprintf(&listing, "%s %d\n", strings.TrimPrefix(path, root), info.Size())
		return nil
	}))
	return listing.String()
}

// runTestGit runs git with no inherited GIT_ variable, no system
// configuration, and an empty HOME, so the host's git-lfs or other global
// settings cannot reach a fixture.
func runTestGit(ctx context.Context, dir, home string, stdin []byte, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, "HOME="+home, "XDG_CONFIG_HOME="+home, "GIT_CONFIG_NOSYSTEM=1")
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, stderr.String())
	}
	return out, nil
}

// recordingGit is the plain GitRunner of the tests: it records every argv
// and lets a test rewrite one command's output (a test seam over git).
type recordingGit struct {
	home    string
	rewrite func(args []string, out []byte) []byte

	mu    sync.Mutex
	calls [][]string
}

func (g *recordingGit) Run(ctx context.Context, dir string, stdin []byte, args ...string) ([]byte, error) {
	g.mu.Lock()
	g.calls = append(g.calls, append([]string(nil), args...))
	g.mu.Unlock()
	out, err := runTestGit(ctx, dir, g.home, stdin, args...)
	if err == nil && g.rewrite != nil {
		out = g.rewrite(args, out)
	}
	return out, err
}

// invoked reports whether some recorded argv starts with prefix.
func (g *recordingGit) invoked(prefix ...string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, call := range g.calls {
		if len(call) >= len(prefix) && strings.Join(call[:len(prefix)], "\x00") == strings.Join(prefix, "\x00") {
			return true
		}
	}
	return false
}

// evaluate runs the Patch Policy over reply in repo, which is also the
// user's checkout, and asserts that no object was written.
func evaluate(t *testing.T, repo *policyRepo, policy healthband.PatchPolicy, reply healthband.PatchReply) healthband.PatchVerdict {
	t.Helper()
	before := repo.objects(t)
	if policy.Git == nil {
		policy.Git = &recordingGit{home: repo.home}
	}
	verdict, err := policy.Evaluate(context.Background(), healthband.PatchPolicyInput{
		Reply: reply, BaseSHA: repo.base, Worktree: repo.dir, Checkout: repo.dir,
	})
	require.NoError(t, err)
	require.Equal(t, before, repo.objects(t), "the Patch Policy wrote an object")
	return verdict
}

// replyWith wraps diff text in one diff fence inside a proposal reply.
func replyWith(diff string) healthband.PatchReply {
	return healthband.PatchReply{Text: "Proposed change:\n\n```diff\n" + diff + "```\n\nReview it before running anything.\n"}
}

// newFile is a diff that adds path with mode and one line per entry of lines.
func newFile(path, mode string, lines ...string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "diff --git a/%s b/%s\nnew file mode %s\n--- /dev/null\n+++ b/%s\n@@ -0,0 +1,%d @@\n", path, path, mode, path, len(lines))
	for _, line := range lines {
		b.WriteString("+" + line + "\n")
	}
	return b.String()
}

// modifyFoo is a diff that appends lines to the tracked pkg/foo/foo.go.
func modifyFoo(lines ...string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "diff --git a/pkg/foo/foo.go b/pkg/foo/foo.go\n--- a/pkg/foo/foo.go\n+++ b/pkg/foo/foo.go\n@@ -1,3 +1,%d @@\n package foo\n \n func Foo() int { return 1 }\n", 3+len(lines))
	for _, line := range lines {
		b.WriteString("+" + line + "\n")
	}
	return b.String()
}
