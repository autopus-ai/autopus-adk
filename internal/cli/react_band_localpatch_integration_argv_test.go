//go:build unix

package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Argv oracles of the integration tests: the Git Execution Policy flags and
// variables on every band git command (S6), the create-only update-ref and
// the canonical format-patch range (S4), and the confined provider calls
// (S7, S13), all spelled from spec.md and acceptance.md.

// lpitPolicyConfig are the -c values of Git Execution Policy item 2.
var lpitPolicyConfig = []string{
	"core.hooksPath=/dev/null", "core.attributesFile=/dev/null", "core.fsmonitor=false", "core.untrackedCache=false",
	"core.symlinks=false", "core.ignoreStat=false", "core.sparseCheckout=false", "core.useReplaceRefs=false",
	"commit.gpgSign=false", "user.name=autopus-band", "user.email=band@autopus.invalid", "author.name=autopus-band",
	"author.email=band@autopus.invalid", "committer.name=autopus-band", "committer.email=band@autopus.invalid",
	"gc.auto=0", "maintenance.auto=false", "filter.lfs.process=", "filter.lfs.clean=", "filter.lfs.smudge=",
	"filter.lfs.required=false",
}

// lpitPolicyEnv are the seven variables of Git Execution Policy item 1.
var lpitPolicyEnv = []string{
	"GIT_ATTR_NOSYSTEM=1", "GIT_EDITOR=:", "GIT_LFS_SKIP_SMUDGE=1", "GIT_NO_LAZY_FETCH=1", "GIT_NO_REPLACE_OBJECTS=1",
	"GIT_PAGER=cat", "GIT_TERMINAL_PROMPT=0",
}

// assertPolicyArgv: every band git command carries every item-2 flag and no
// GIT_* variable but the seven, plus GIT_INDEX_FILE naming a band temp file
// on exactly the three temp-index commands of step 8; the branch is created
// with update-ref --no-deref, and format-patch takes the canonical flags and
// <base-sha>..<commit-oid>, never HEAD.
func (w *lpitWorld) assertPolicyArgv(t *testing.T, key, commit string) {
	t.Helper()
	var indexed []string
	policy := 0
	for _, call := range w.calls() {
		config, args, ok := call.policy()
		if !ok {
			continue
		}
		policy++
		for _, value := range lpitPolicyConfig {
			assert.Contains(t, config, value, "%v", args)
		}
		env := slices.DeleteFunc(slices.Clone(call.env), func(line string) bool {
			index, isIndex := strings.CutPrefix(line, "GIT_INDEX_FILE=")
			if isIndex {
				indexed = append(indexed, strings.Join(args, " "))
				assert.NotEqual(t, filepath.Join(w.repo, ".git", "index"), index, "never the user's index")
				assert.Equal(t, "index", filepath.Base(index), "a band temp index")
				assert.True(t, strings.HasPrefix(filepath.Base(filepath.Dir(index)), "autopus-band-"), index)
			}
			return isIndex
		})
		assert.Equal(t, lpitPolicyEnv, env, "%v", args)
	}
	assert.Greater(t, policy, 30, "band ran its git commands through the policy")
	require.Len(t, indexed, 3, "exactly the three temp-index commands")
	assert.True(t, strings.HasPrefix(indexed[0], "read-tree "+w.base), indexed[0])
	assert.Equal(t, "apply --cached", indexed[1])
	assert.Equal(t, "write-tree", indexed[2])
	updates := w.policyCalls("update-ref")
	require.Len(t, updates, 1)
	_, args, _ := updates[0].policy()
	assert.Equal(t, []string{"update-ref", "--no-deref", "refs/heads/autopus/band/" + key, commit, ""}, args)
	patches := w.policyCalls("format-patch")
	require.Len(t, patches, 1)
	_, args, _ = patches[0].policy()
	assert.Equal(t, append(slices.Clone(lpitFormatPatch), w.base+".."+commit), args)
	assert.NotContains(t, strings.Join(args, " "), "HEAD")
}

// assertConfinedCalls: the run made calls provider calls, each the
// confined subprocess claude in the band worktree without credentials or
// GIT_* variables, and the worktree held no .env and checked the tracked
// symlink out as a file of its link text, so no read reached the canary.
func (w *lpitWorld) assertConfinedCalls(t *testing.T, worktree string, calls int) {
	t.Helper()
	require.Equal(t, calls, w.claudeCalls(), "provider calls")
	for n := 1; n <= calls; n++ {
		argv := strings.Split(strings.TrimSuffix(w.claudeRecord(t, "argv", n), "\n"), "\n")
		assert.Equal(t, []string{"--print", "--model", "claude-opus-5-5"}, argv[:3])
		for _, flag := range []string{"--restricted", "--verbose", "--strict-mcp-config", "--safe-mode"} {
			assert.Contains(t, argv, flag)
		}
		assert.Equal(t, "stream-json", argv[slices.Index(argv, "--output-format")+1])
		assert.Equal(t, "plan", argv[slices.Index(argv, "--permission-mode")+1])
		assert.Equal(t, "--tools=Read,Grep,Glob", argv[len(argv)-1])
		assert.NotContains(t, argv, "--add-dir")
		assert.NotContains(t, argv, "--bare")
		assert.Equal(t, worktree+"\n", w.claudeRecord(t, "cwd", n))
		for _, line := range strings.Split(w.claudeRecord(t, "env", n), "\n") {
			name, _, _ := strings.Cut(line, "=")
			assert.False(t, strings.HasPrefix(name, "GIT_"), "call %d inherited %s", n, name)
			assert.NotContains(t, []string{"GH_TOKEN", "ACTIONS_ID_TOKEN_REQUEST_TOKEN", "AWS_CONTAINER_CREDENTIALS_FULL_URI"}, name)
		}
		for _, kind := range []string{"files", "stdin"} {
			record := w.claudeRecord(t, kind, n)
			assert.NotContains(t, record, lpitCanary, "call %d %s", n, kind)
			assert.NotContains(t, record, lpitGhToken, "call %d %s", n, kind)
		}
		assert.Contains(t, w.claudeRecord(t, "files", n), lpitLink, "the read covered the checked-out link text")
	}
	_, err := os.Lstat(filepath.Join(worktree, ".env"))
	assert.True(t, os.IsNotExist(err), "the band worktree holds no .env")
	info, err := os.Lstat(filepath.Join(worktree, "docs", "canary.md"))
	require.NoError(t, err)
	assert.True(t, info.Mode().IsRegular(), "core.symlinks=false checks the link out as a file")
	data, err := os.ReadFile(filepath.Join(worktree, "docs", "canary.md"))
	require.NoError(t, err)
	assert.Equal(t, lpitLink, string(data))
}

// claudeCalls is the number of fake claude calls.
func (w *lpitWorld) claudeCalls() int {
	w.t.Helper()
	data, err := os.ReadFile(filepath.Join(w.claude, "count"))
	if os.IsNotExist(err) {
		return 0
	}
	require.NoError(w.t, err)
	n, err := strconv.Atoi(strings.TrimSpace(string(data)))
	require.NoError(w.t, err)
	return n
}

func (w *lpitWorld) claudeRecord(t *testing.T, kind string, n int) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(w.claude, kind+"."+strconv.Itoa(n)))
	require.NoError(t, err)
	return string(data)
}

// assertNoObjectWrite: count-objects is unchanged, no object holds marker,
// and no git command of the claim could write an object or a ref: no
// apply but the --check of Patch Policy item 2, and no read-tree,
// write-tree, commit, update-ref, or format-patch (rows of steps 4-8).
func (w *lpitWorld) assertNoObjectWrite(t *testing.T, before lpitSnapshot, marker string) {
	t.Helper()
	assert.Equal(t, before.objects, w.inspect("count-objects", "-v"))
	if marker != "" {
		assert.NotContains(t, w.inspect("cat-file", "--batch-all-objects", "--batch"), marker)
	}
	for _, sub := range []string{"read-tree", "write-tree", "commit", "update-ref", "format-patch"} {
		assert.Empty(t, w.policyCalls(sub), "0 %s invocations", sub)
	}
	for _, call := range w.policyCalls("apply") {
		_, args, _ := call.policy()
		assert.Contains(t, args, "--check", "only the object-free apply check: %v", args)
	}
}
