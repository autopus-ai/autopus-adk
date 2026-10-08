//go:build unix

package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lpGitWrapper logs every band git argv to LP_GIT_LOG and, by LP_GIT_MODE,
// fails git worktree add or makes the checkout write part of the base and
// then hang ignoring SIGTERM, or exit 128; everything else runs real git.
const lpGitWrapper = `#!/bin/sh
echo "$*" >> "$LP_GIT_LOG"
fail=$(cat "$LP_GIT_FAIL" 2>/dev/null)
if [ -n "$fail" ]; then
	case " $* " in *"$fail"*) exit 128 ;; esac
fi
case " $* " in
*" worktree add "*) [ "$LP_GIT_MODE" = add128 ] && exit 128 ;;
*" reset --hard "*)
	case "$LP_GIT_MODE" in
	hang) echo partial > partial.txt; trap '' TERM; sleep 30; exit 0 ;;
	reset128) echo partial > partial.txt; exit 128 ;;
	esac ;;
esac
exec "$LP_REAL_GIT" "$@"
`

// useGitWrapper routes the fixture's band git through lpGitWrapper in mode
// and returns the argv log.
func (f *lpFixture) useGitWrapper(mode string) string {
	f.t.Helper()
	real, err := exec.LookPath("git")
	require.NoError(f.t, err)
	wrapper, log := filepath.Join(f.root, "gitwrap"), filepath.Join(f.root, "git.log")
	require.NoError(f.t, os.WriteFile(wrapper, []byte(lpGitWrapper), 0o700))
	base := f.patcher.git.Environ()
	env := append(slices.Clone(base), "LP_GIT_LOG="+log, "LP_GIT_MODE="+mode, "LP_REAL_GIT="+real,
		"LP_GIT_FAIL="+filepath.Join(f.root, "git.fail"))
	f.patcher.git.Binary, f.patcher.git.Environ = wrapper, func() []string { return slices.Clone(env) }
	return log
}

// failGit makes every later band git command whose argv holds part exit 128.
func (f *lpFixture) failGit(part string) {
	require.NoError(f.t, os.WriteFile(filepath.Join(f.root, "git.fail"), []byte(part), 0o600))
}

func lpGitLog(t *testing.T, log string) []string {
	t.Helper()
	data, err := os.ReadFile(log)
	require.NoError(t, err)
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

func lpCountArgv(lines []string, part string) int {
	count := 0
	for _, line := range lines {
		if strings.Contains(line, part) {
			count++
		}
	}
	return count
}

func TestLocalPatchPrepare_StoppedOrFailedCheckout_RemovedAtOnce(t *testing.T) {
	for _, mode := range []string{"hang", "reset128"} {
		t.Run(mode, func(t *testing.T) {
			f := newLPFixture(t, nil)
			log := f.useGitWrapper(mode)
			f.patcher.groups.setup, f.patcher.git.StopGrace = 8*time.Second, 2*time.Second
			started := time.Now()
			s := f.patcher.prepare(context.Background(), f.target())
			assert.Less(t, time.Since(started), 30*time.Second, "SIGKILL at the setup deadline")
			assert.Equal(t, lpCodeWorktreeFailed, s.code)
			assert.Equal(t, []string{"prep", "stage:worktree_intent", "stage:worktree_failed"}, f.ledger.trail())
			assert.Equal(t, lpCodeWorktreeFailed, f.ledger.stage(t, lpPhaseWorktreeFailed).Code)
			worktree := filepath.Join(f.lp(), lpKey, "worktree")
			lines := lpGitLog(t, log)
			assert.Equal(t, 1, lpCountArgv(lines, "worktree remove --force "+worktree), "one remove with a single --force")
			assert.Zero(t, lpCountArgv(lines, "--force --force"))
			assert.Zero(t, lpCountArgv(lines, "worktree unlock"))
			assert.Empty(t, s.kept)
			assert.NotContains(t, f.git(f.repo, "worktree", "list", "--porcelain"), worktree)
			_, err := os.Lstat(filepath.Join(f.lp(), lpKey))
			assert.True(t, os.IsNotExist(err), "no <lp>/<key>/ is left, so no retention slot is taken")
			assert.Empty(t, f.cleaner.calls, "the live removal replaces the Cleanup Rules")
			f.patcher.release(s)
			result, _ := f.patcher.patch(context.Background(), s, localPatchInput{})
			assert.Equal(t, "failed:worktree_failed", result.Status)
		})
	}
}

func TestLocalPatchPrepare_WorktreeAddFails_CleanupRules(t *testing.T) {
	f := newLPFixture(t, nil)
	f.useGitWrapper("add128")
	s := f.patcher.prepare(context.Background(), f.target())
	assert.Equal(t, lpCodeWorktreeFailed, s.code)
	assert.Equal(t, []string{"prep", "stage:worktree_intent", "stage:worktree_failed"}, f.ledger.trail())
	assert.Equal(t, []string{lpPatchClaimID}, f.cleaner.calls)
	_, err := os.Lstat(filepath.Join(f.lp(), lpKey))
	assert.True(t, os.IsNotExist(err), "the empty <lp>/<key>/ is removed")
	f.patcher.release(s)
}

func TestLocalPatchPrepare_IncludeIfInsideWorktree_UnsafeBeforeCheckout(t *testing.T) {
	f := newLPFixture(t, nil)
	log := f.useGitWrapper("pass")
	include := filepath.Join(f.root, "worktrees.gitconfig")
	require.NoError(t, os.WriteFile(include, []byte("[filter \"mark\"]\n\tsmudge = tools/smudge.sh\n"), 0o600))
	global := "[includeIf \"gitdir:**/worktrees/**\"]\n\tpath = " + include + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(f.root, "band-home", ".gitconfig"), []byte(global), 0o600))
	s := f.patcher.prepare(context.Background(), f.target())
	assert.Equal(t, "git_config_unsafe:filter.mark.smudge", s.code)
	assert.Equal(t, "git_config_unsafe:filter.mark.smudge", f.ledger.stage(t, lpPhaseWorktreeFailed).Code)
	assert.Equal(t, "ok", f.ledger.prep(t).Code, "the user's checkout passed step 1")
	assert.Zero(t, lpCountArgv(lpGitLog(t, log), "reset --hard"), "no file of the base is checked out")
	assert.Equal(t, []string{lpPatchClaimID}, f.cleaner.calls)
	f.patcher.release(s)
}

func TestLocalPatchPrepare_DiagnosisOnly_FinishWritesResult(t *testing.T) {
	f := newLPFixture(t, nil)
	target := f.target()
	target.claimID = ""
	s := f.patcher.prepare(context.Background(), target)
	require.True(t, s.ready(), "code %q", s.code)
	assert.Equal(t, bandLocalPatchKey(lpSeries, "e1042", lpDiagnoseClaimID), s.key)
	reply := parseBandStream(lpStream(lpInit55, lpAssistant55, lpResult("ok")), lpRequestDiagnosis, "claude-opus-5-5")
	result := f.patcher.finishDiagnosis(context.Background(), s, &reply)
	assert.Equal(t, lpDiagnoseClaimID, result.ClaimID)
	assert.Equal(t, lpStatusDone, result.Status)
	assert.Equal(t, []localPatchModel{reply.model}, result.Models)
	assert.Equal(t, []string{"prep", "stage:worktree_intent", "stage:worktree_done", "result"}, f.ledger.trail())
	assert.Equal(t, []string{lpDiagnoseClaimID}, f.cleaner.calls)
	_, err := os.Lstat(filepath.Join(f.lp(), s.key))
	assert.True(t, os.IsNotExist(err), "the diagnosis worktree is removed")
	_, err = os.Lstat(filepath.Join(f.lp(), s.key+".lock"))
	assert.True(t, os.IsNotExist(err))
}
