//go:build unix

package healthband

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// must fails the test on err and returns out, so a runner call reads as one
// step: f.must(r.Run(ctx, ...)).
func (f *gpFixture) must(out []byte, err error) []byte {
	f.t.Helper()
	require.NoError(f.t, err)
	return out
}

// SPEC-SIGMABAND-002 S4 and S6 (REQ-05, REQ-10): the whole band git sequence
// of Local Patch Flow steps 2–12 and the Cleanup Rules, run through the
// policy runner in the hostile repository, writes 0 markers, starts no hook,
// git-lfs, maintenance, gc, fetch, or upload-pack child (trace2 child_start),
// and leaves the user's checkout, index, HEAD, and refs unchanged. The flow
// bypasses item 3, as probe A3 did, to test items 1–2 themselves; the
// control test shows each source is live.
func TestGitPolicyRunner_HostileRepositoryRunsNoRepositoryCommand(t *testing.T) {
	t.Parallel()
	h := newGPHostile(t)
	ctx := gpContext(t)
	before := h.snapshot()
	r := h.runner(h.bandEnv, h.repo)

	code, err := r.CheckConfig(ctx)
	require.NoError(t, err)
	assert.Equal(t, "git_config_unsafe:filter.gmark.clean", code, "a driver of any scope is refused")

	lp := filepath.Join(h.root, "lp")
	wt := filepath.Join(lp, "key", "worktree")
	require.NoError(t, os.MkdirAll(lp, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(lp, "canary.txt"), []byte("CANARY-0123456789abcdef"), 0o600))
	h.must(r.Run(ctx, "worktree", "add", "--no-checkout", "--detach", wt, h.base))
	w := r.In(wt)
	h.must(w.Run(ctx, "reset", "--hard", "--no-recurse-submodules", "--quiet"))
	h.assertBandCheckout(wt)
	h.must(w.Run(ctx, "status", "--porcelain", "-z", "--untracked-files=all", "--ignored"))
	h.must(w.Run(ctx, "diff", "--no-ext-diff", "--no-textconv", "--binary"))
	numstat := h.must(w.RunInput(ctx, []byte(gpPatch), "apply", "--numstat", "--summary", "-z", "--check"))
	assert.Contains(t, string(numstat), "1\t1\tpkg/foo/foo.go\x00")

	temp := filepath.Join(h.root, "band-tmp")
	require.NoError(t, os.MkdirAll(temp, 0o700))
	indexed := w.WithIndexFile(filepath.Join(temp, "index"))
	h.must(indexed.Run(ctx, "read-tree", h.base))
	h.must(indexed.RunInput(ctx, []byte(gpPatch), "apply", "--cached"))
	expected := gpTrim(h.must(indexed.Run(ctx, "write-tree")))
	h.must(w.RunInput(ctx, []byte(gpPatch), "apply", "--index"))
	assert.Equal(t, expected, gpTrim(h.must(w.Run(ctx, "write-tree"))), "temp-index tree equals the apply --index tree")

	message := "fix(band): ci anomaly local patch (e1042)\n\nPatch model: claude-opus-5-5\n"
	messageFile := filepath.Join(temp, "msg")
	require.NoError(t, os.WriteFile(messageFile, []byte(message), 0o600))
	h.must(w.Run(ctx, "commit", "--no-verify", "--cleanup=verbatim", "-F", messageFile))
	head := gpTrim(h.must(w.Run(ctx, "rev-parse", "HEAD")))
	header, body, _ := strings.Cut(string(h.must(w.Run(ctx, "cat-file", "commit", head))), "\n\n")
	assert.Equal(t, message, body, "the commit message equals the -F bytes")
	assert.True(t, strings.HasPrefix(header, "tree "+expected+"\nparent "+h.base+"\n"), "one parent, the real base: %s", header)
	assert.Contains(t, header, "\nauthor autopus-band <band@autopus.invalid> ")
	assert.Contains(t, header, "\ncommitter autopus-band <band@autopus.invalid> ")

	ref := BandBranchRef("ci-failure-rate-ci-c6d37d0a-e1042-a1b2c3d4")
	h.must(r.Run(ctx, "update-ref", "--no-deref", ref, head, ""))
	_, err = r.Run(ctx, "update-ref", "--no-deref", ref, head, "")
	assert.Error(t, err, "the create-only update-ref refuses an existing branch")
	_, err = r.Run(ctx, "symbolic-ref", "-q", "--no-recurse", ref)
	assert.Equal(t, 1, GitExitCode(err), "a plain branch is no symbolic ref")
	patch := string(h.must(r.Run(ctx, GitFormatPatchArgs(h.base, head)...)))
	assert.Contains(t, patch, "Subject: [PATCH] fix(band): ci anomaly local patch (e1042)")
	assert.Contains(t, patch, "+func A() int { return 2 }")
	assert.NotContains(t, patch, "workflows/x.yaml", "the replace ref changes nothing that band reads")

	assert.Equal(t, h.realTree, gpTrim(h.must(r.Run(ctx, "rev-parse", "--verify", "--quiet", h.base+"^{tree}"))))
	h.must(r.Run(ctx, "ls-tree", "-r", "-l", "-z", h.base))
	link := string(h.must(r.Run(ctx, "ls-tree", "-z", h.base, "--", ":(literal)docs/canary.md")))
	assert.True(t, strings.HasPrefix(link, "120000 blob "), "base entry %q", link)
	attrs := string(h.must(r.Run(ctx, "check-attr", "-z", "--source="+h.base, "filter", "--", "data.bin", "pkg/foo/foo.go")))
	assert.Equal(t, "data.bin\x00filter\x00lfs\x00pkg/foo/foo.go\x00filter\x00unspecified\x00", attrs,
		"core.attributesFile=/dev/null hides the global attributes file")
	h.must(w.Run(ctx, "ls-files", "-v", "-z"))
	h.must(w.Run(ctx, "ls-files", "-s", "-z"))
	assert.Contains(t, string(h.must(r.Run(ctx, "worktree", "list", "--porcelain", "-z"))), "worktree "+wt+"\x00")
	h.must(r.Run(ctx, "update-ref", "--no-deref", "-d", ref, head))
	remover := r
	remover.RemoveRoot = lp // the removal names a path below <lp> (L4)
	h.must(remover.Run(ctx, "worktree", "remove", "--force", wt))

	assert.Empty(t, h.markerNames(), "no repository-selected command ran")
	children := gpChildren(t, h.bandTrace)
	info, err := os.Stat(h.bandTrace)
	require.NoError(t, err, "trace2 reaches band git through the global configuration")
	assert.Positive(t, info.Size())
	assert.Zero(t, gpCount(children, "hook", nil), "hook children: %v", children)
	words := append([]string{"maintenance", "gc"}, gpNetwork...)
	assert.Zero(t, gpCount(children, "", words, "git-lfs", "git-remote-", h.bin), "children: %v", children)
	assert.NoFileExists(t, filepath.Join(h.root, "leak.json"), "the inherited GIT_TRACE2_EVENT was removed")
	assert.NoDirExists(t, filepath.Join(h.repo, ".git", "lfs"))
	assert.Equal(t, before, h.snapshot(), "the user's checkout, index, HEAD, and refs stay unchanged")
}

// assertBandCheckout checks the checkout of a band worktree: the LFS pointer
// stays a pointer, a tracked symlink is a regular file holding its link text
// (so no Read through it reaches the canary), and the replace ref is ignored.
func (h *gpHostile) assertBandCheckout(wt string) {
	data, err := os.ReadFile(filepath.Join(wt, "data.bin"))
	require.NoError(h.t, err)
	assert.Equal(h.t, gpLFSPointer, string(data))
	info, err := os.Lstat(filepath.Join(wt, "docs", "canary.md"))
	require.NoError(h.t, err)
	assert.True(h.t, info.Mode().IsRegular(), "core.symlinks=false checks a tracked symlink out as a file")
	link, err := os.ReadFile(filepath.Join(wt, "docs", "canary.md"))
	require.NoError(h.t, err)
	assert.Equal(h.t, gpCanaryLink, string(link))
	assert.NoDirExists(h.t, filepath.Join(wt, ".github"), "GIT_NO_REPLACE_OBJECTS keeps the real base tree")
}
