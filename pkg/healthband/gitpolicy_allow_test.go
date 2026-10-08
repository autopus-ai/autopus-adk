package healthband

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	gpOID    = "0123456789abcdef0123456789abcdef01234567"
	gpOID2   = "fedcba9876543210fedcba9876543210fedcba98"
	gpBand   = "refs/heads/autopus/band/ci-failure-rate-ci-c6d37d0a-e1042-a1b2c3d4"
	gpWT     = "/cache/autopus/local-patches/0123456789ab/ci-failure-rate-ci-c6d37d0a-e1042-a1b2c3d4/worktree"
	gpIndex  = "/tmp/band-0001/index"
	gpOID256 = gpOID + "0123456789abcdef01234567"
)

// gpArgs splits a command line on spaces; "”" stands for an empty argument.
func gpArgs(line string) []string {
	args := strings.Fields(line)
	for i, arg := range args {
		if arg == "''" {
			args[i] = ""
		}
	}
	return args
}

// SPEC-SIGMABAND-002 Git Execution Policy item 6: the allowlist admits every
// git command this SPEC names, in its exact form.
func TestCheckGitCommand_AdmitsEveryCommandTheSPECNames(t *testing.T) {
	t.Parallel()

	admitted := []string{
		"version",
		"config --list --show-scope --show-origin -z",
		"rev-parse --path-format=absolute --git-common-dir",
		"rev-parse --path-format=absolute --git-dir",
		"rev-parse --path-format=absolute --show-toplevel",
		"rev-parse HEAD",
		"rev-parse --verify --quiet refs/remotes/origin/main^{commit}",
		"rev-parse --verify --quiet refs/heads/release/2.0^{commit}",
		"rev-parse --verify refs/remotes/origin/main^{commit}",
		"rev-parse --verify --quiet " + gpOID + "^{commit}",
		"rev-parse --verify --quiet " + gpOID256 + "^{tree}",
		"rev-parse --verify --quiet " + gpBand,
		"symbolic-ref --short refs/remotes/origin/HEAD",
		"symbolic-ref -q --short refs/remotes/origin/HEAD",
		"symbolic-ref -q --no-recurse " + gpBand,
		"check-ref-format --branch main",
		"check-ref-format --branch feature/한글",
		"ls-tree -r -l -z " + gpOID,
		"ls-tree -r -z --name-only " + gpOID,
		"ls-tree -r -z " + gpOID,
		"ls-tree -z " + gpOID + " -- :(literal)pkg/foo",
		"check-attr -z --source=" + gpOID + " filter -- pkg/foo/foo.go pkg/foo/new.go",
		"apply --numstat --summary -z --check",
		"apply --index",
		"apply --index /cache/lp/key.diff",
		"write-tree",
		"worktree add --no-checkout --detach " + gpWT + " " + gpOID,
		"worktree add --no-checkout --detach " + gpWT + "/ " + gpOID,
		"worktree list --porcelain -z",
		"worktree remove --force " + gpWT,
		"reset --hard --no-recurse-submodules --quiet",
		"status --porcelain -z --untracked-files=all --ignored",
		"diff --no-ext-diff --no-textconv --binary",
		"ls-files -v -z",
		"ls-files -s -z",
		"commit --no-verify --cleanup=verbatim -F /tmp/band-0001/msg",
		"cat-file commit " + gpOID,
		"update-ref --no-deref " + gpBand + " " + gpOID + " ''",
		"update-ref --no-deref -d " + gpBand + " " + gpOID,
		strings.Join(GitFormatPatchArgs(gpOID, gpOID2), " "),
	}
	for _, line := range admitted {
		assert.NoError(t, CheckGitCommand(gpArgs(line), ""), line)
	}
	for _, line := range []string{"read-tree " + gpOID, "apply --cached", "write-tree"} {
		assert.NoError(t, CheckGitCommand(gpArgs(line), gpIndex), "%s with the temp index", line)
	}
	assert.Equal(t, gpBand, BandBranchRef("ci-failure-rate-ci-c6d37d0a-e1042-a1b2c3d4"))
}

// The allowlist refuses network commands, worktree prune, lock, unlock, and
// a second --force, ref writes outside refs/heads/autopus/band/, global
// options, malformed values, other flags, and GIT_INDEX_FILE outside the
// three temp-index commands.
func TestCheckGitCommand_RefusesEverythingElse(t *testing.T) {
	t.Parallel()

	refused := []string{
		"", "push origin main", "fetch origin", "ls-remote origin", "remote -v", "clone /x /y",
		"gc", "maintenance run --auto", "lfs pull", "submodule update", "hook run pre-commit",
		"worktree prune", "worktree unlock " + gpWT, "worktree lock " + gpWT, "worktree move " + gpWT + " /x",
		"worktree remove --force --force " + gpWT, "worktree remove -f -f " + gpWT, "worktree remove " + gpWT,
		"worktree add --detach " + gpWT + " " + gpOID,
		"worktree add --no-checkout --detach relative/worktree " + gpOID,
		"worktree add --no-checkout --detach /cache/x/../y " + gpOID,
		"worktree add --no-checkout --detach " + gpWT + " main",
		"update-ref " + gpBand + " " + gpOID + " ''",
		"update-ref --no-deref refs/heads/main " + gpOID + " ''",
		"update-ref --no-deref refs/heads/autopus/band/Upper_Case " + gpOID + " ''",
		"update-ref --no-deref -d " + gpBand,
		"update-ref --no-deref " + gpBand + " " + gpOID,
		"symbolic-ref " + gpBand + " refs/heads/main",
		"commit -F /tmp/msg", "commit --no-verify --cleanup=verbatim -m x",
		"commit --no-verify --cleanup=verbatim -F relative/msg", "commit --no-verify --cleanup=strip -F /tmp/msg",
		"format-patch --stdout HEAD~1..HEAD",
		strings.Join(GitFormatPatchArgs("HEAD", gpOID2), " "),
		strings.Join(GitFormatPatchArgs(gpOID[:12], gpOID2), " "),
		"config core.hooksPath x", "config --list", "config --global --list --show-scope --show-origin -z",
		"read-tree " + gpOID, "apply --cached", "apply", "apply --3way", "apply --index relative.diff",
		"-c core.hooksPath=x version", "--git-dir=/x version", "-C /x version",
		"rev-parse --verify --quiet main", "rev-parse --verify --quiet refs/remotes/origin/a..b^{commit}",
		"rev-parse --verify --quiet refs/heads/x.lock^{commit}", "rev-parse --verify --quiet refs/heads/.x^{commit}",
		"rev-parse --verify --quiet refs/heads/a@{1}^{commit}", "rev-parse --verify --quiet " + gpOID[:7] + "^{commit}",
		"rev-parse --verify --quiet --output=x", "rev-parse --verify --quiet refs/heads/main^{tree}",
		"rev-parse HEAD~1",
		"ls-tree -z " + gpOID + " -- pkg/foo", "ls-tree -z " + gpOID + " -- :(literal)", "ls-tree -z HEAD -- :(literal)x",
		"ls-tree -r " + gpOID, "ls-tree -r -z HEAD", "ls-tree -r -z " + gpOID + " pkg", "ls-tree -r -z --full-tree " + gpOID,
		"check-attr -z --source=HEAD filter -- a.go", "check-attr -z --source=" + gpOID + " filter --",
		"check-attr -z --source=" + gpOID + " diff -- a.go", "check-attr -z filter -- a.go",
		"check-ref-format --branch -x", "check-ref-format --branch", "check-ref-format main",
		"cat-file -p " + gpOID, "cat-file commit HEAD",
		"status --porcelain", "diff --binary", "ls-files -z", "reset --hard",
	}
	for _, line := range refused {
		assert.ErrorIs(t, CheckGitCommand(gpArgs(line), ""), ErrGitCommandNotAllowed, line)
	}
	for _, args := range [][]string{
		{"check-ref-format", "--branch", ""}, {"check-ref-format", "--branch", "a\x1bb"},
		{"check-ref-format", "--branch", strings.Repeat("a", 1025)},
		{"ls-tree", "-z", gpOID, "--", ":(literal)a\x00b"}, {"check-attr", "-z", "--source=" + gpOID, "filter", "--", ""},
		{"commit", "--no-verify", "--cleanup=verbatim", "-F", "/tmp/a\nb"},
	} {
		assert.ErrorIs(t, CheckGitCommand(args, ""), ErrGitCommandNotAllowed, "%q", args)
	}
	for _, line := range []string{"status --porcelain -z --untracked-files=all --ignored", "worktree list --porcelain -z", "version"} {
		assert.ErrorIs(t, CheckGitCommand(gpArgs(line), gpIndex), ErrGitCommandNotAllowed, "%s takes no temp index", line)
	}
	assert.ErrorIs(t, CheckGitCommand(gpArgs("read-tree "+gpOID), "relative/index"), ErrGitCommandNotAllowed)
	assert.ErrorContains(t, CheckGitCommand(gpArgs("push origin"), ""), `git "push"`)
}
