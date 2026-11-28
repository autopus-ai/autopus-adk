package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func approveForEncode(t *testing.T) string {
	t.Helper()
	dir := encodeRepo(t)
	_, err := runAuto(t, "harness", "approve", "HL-001")
	require.NoError(t, err)
	return dir
}

// A proof runs in the worktree the agent works in, not the main checkout:
// the check exists only in the linked worktree.
func TestHarnessEncode_ProvesTheCallersWorktree(t *testing.T) {
	dir := approveForEncode(t)
	encodeGit(t, dir, "add", "lint.sh", "src/app.js")
	encodeGit(t, dir, "commit", "-q", "-m", "check")
	linked := filepath.Join(t.TempDir(), "feat")
	encodeGit(t, dir, "worktree", "add", "-q", "-b", "feat", linked)
	require.NoError(t, os.WriteFile(filepath.Join(linked, "only-here.sh"), []byte("! grep -rn setTimeout src\n"), 0o755))
	encodeGit(t, dir, "rm", "-q", "lint.sh")
	encodeGit(t, dir, "commit", "-q", "-m", "main has no check")
	t.Chdir(linked)

	out, err := runAuto(t, "harness", "encode", "HL-001", "--layer", "lint", "--check", "sh only-here.sh", "--repro", writePatch(t, reproPatch))
	require.NoError(t, err, out)
	assert.Contains(t, out, "HL-001 증명됨")
	status, err := runAuto(t, "harness", "status")
	require.NoError(t, err)
	assert.Contains(t, status, "증명한 브랜치: feat")
}

// Run from a git hook, auto inherits GIT_INDEX_FILE; the proof must not
// write the snapshot into the user's index.
func TestHarnessEncode_InheritedIndexIsLeftAlone(t *testing.T) {
	dir := approveForEncode(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "staged.txt"), []byte("s\n"), 0o644))
	encodeGit(t, dir, "add", "staged.txt")
	before := encodeGit(t, dir, "status", "--porcelain", "--", ".", ":!.claude")
	t.Setenv("GIT_INDEX_FILE", filepath.Join(dir, ".git", "index"))

	out, err := runAuto(t, "harness", "encode", "HL-001", "--layer", "lint", "--check", "sh lint.sh", "--repro", writePatch(t, reproPatch))
	require.NoError(t, err, out)
	require.NoError(t, os.Unsetenv("GIT_INDEX_FILE")) // t.Setenv restores it after the test
	assert.Equal(t, before, encodeGit(t, dir, "status", "--porcelain", "--", ".", ":!.claude"))
}

// The copy is not a worktree of the user's repository: no hook of theirs
// runs, nothing is added to their worktree list, and no copy is left.
func TestHarnessEncode_CopyRunsNoUserHookAndLeavesNothing(t *testing.T) {
	dir := approveForEncode(t)
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	marker := filepath.Join(t.TempDir(), "hook-ran")
	hook := "#!/bin/sh\ntouch " + marker + "\nexit 1\n"
	for _, h := range []string{"post-checkout", "pre-commit", "post-commit"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "hooks", h), []byte(hook), 0o755))
	}
	before := encodeGit(t, dir, "worktree", "list")

	out, err := runAuto(t, "harness", "encode", "HL-001", "--layer", "lint", "--check", "sh lint.sh", "--repro", writePatch(t, reproPatch))
	require.NoError(t, err, out)
	assert.NoFileExists(t, marker, "no hook of the user's ran")
	assert.Equal(t, before, encodeGit(t, dir, "worktree", "list"))
	left, _ := filepath.Glob(filepath.Join(tmp, "auto-harness-proof-*"))
	assert.Empty(t, left, "the copy is removed")
	assert.NotContains(t, out, "auto-harness-proof-", "the copy is not reported as a rule file")
}

func TestHarnessEncode_VacuousChecksAreRefused(t *testing.T) {
	cases := []struct{ name, check, patch, want string }{
		{"clean-tree check", "git diff --quiet", reproPatch, "재현 patch를 적용해도 검사가 통과합니다"},
		{"patch edits the check", "sh lint.sh", "", "검사 명령이 가리키는 파일을 고칩니다"},
		{"patch removes the check", "sh run.sh", "", "검사 명령 자체가 실행되지 않습니다"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := approveForEncode(t)
			require.NoError(t, os.MkdirAll(filepath.Join(dir, "tools"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "tools", "lint.sh"), []byte("true\n"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "run.sh"), []byte("exec ./tools/lint.sh\n"), 0o755))
			encodeGit(t, dir, "add", "-A", "src", "tools", "run.sh")
			encodeGit(t, dir, "commit", "-q", "-m", "clean")
			patch := tc.patch
			if tc.name == "patch edits the check" {
				patch = "--- a/lint.sh\n+++ b/lint.sh\n@@ -1 +1 @@\n-! grep -rn setTimeout src\n+exit 1\n"
			}
			if tc.name == "patch removes the check" {
				patch = "diff --git a/tools/lint.sh b/tools/lint.sh\ndeleted file mode 100755\n--- a/tools/lint.sh\n+++ /dev/null\n@@ -1 +0,0 @@\n-true\n"
			}
			out, err := runAuto(t, "harness", "encode", "HL-001", "--layer", "lint", "--check", tc.check, "--repro", writePatch(t, patch))
			require.Error(t, err, out)
			assert.Contains(t, out+err.Error(), tc.want)
			assert.Contains(t, ruleText(t, dir), "HL-001")
		})
	}
}

func TestHarnessEncode_BriefOnlyForApprovedLessons(t *testing.T) {
	encodeRepo(t)
	_, err := runAuto(t, "harness", "encode", "HL-001")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "승인된 교훈이 아닙니다")
	_, err = runAuto(t, "harness", "encode", "HL-001", "--check", "x")
	require.Error(t, err)
	_, err = runAuto(t, "harness", "approve", "HL-001")
	require.NoError(t, err)
	_, err = runAuto(t, "harness", "encode", "HL-001", "--layer", "bogus")
	require.Error(t, err)
	_, err = runAuto(t, "harness", "encode", "HL-001", "--layer", "text", "--check", "x", "--reason", "r")
	require.Error(t, err)
	_, err = runAuto(t, "harness", "encode", "HL-001", "--layer", "lint", "--reason", "r")
	require.Error(t, err)
	_, err = runAuto(t, "harness", "encode", "HL-001", "--layer", "lint", "--check", "sh lint.sh", "--repro", writePatch(t, " \n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "비어 있지 않고")
}
