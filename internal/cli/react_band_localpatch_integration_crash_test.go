//go:build unix

package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// S9 recovery after a real crash: a child process (this test binary again)
// runs the production band command, and the git wrapper kills it with
// SIGKILL right after git commit returns, before commit_done; the next run
// recovers the claim once its lease has passed.

// lpitCrashChildEnv names the user checkout of the crash child's run.
const lpitCrashChildEnv = "LPIT_CRASH_CHILD_REPO"

// TestReactBandLocalPatchIT_CrashChild is the child half: one production
// band run that must not return, because the git wrapper kills it.
func TestReactBandLocalPatchIT_CrashChild(t *testing.T) {
	repo := os.Getenv(lpitCrashChildEnv)
	if repo == "" {
		t.Skip("runs only as the child of TestReactBandLocalPatchIT_S9_RecoveryAfterACrashAfterCommit")
	}
	cmd := newReactBandCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--project-dir", repo, "--format", "json"})
	_ = cmd.ExecuteContext(context.Background())
	t.Fatal("band returned, but the git wrapper should have killed it after git commit")
}

// S9: the crash leaves the records up to apply_done, the claim commit in
// the worktree, the diff file, and the key lock file of a dead process; the
// next run, past the lease, finds the claim commit by its parent, message
// hash, and expected tree, removes the clean worktree and the diff file,
// unlinks the lock, and ends the claim failed:interrupted, recovered.
func TestReactBandLocalPatchIT_S9_RecoveryAfterACrashAfterCommit(t *testing.T) {
	w := newLPITWorld(t, lpitConfig)
	w.answerS4()
	before := w.snapshot()
	child := exec.Command(os.Args[0], "-test.run=^TestReactBandLocalPatchIT_CrashChild$", "-test.count=1")
	child.Env = append(os.Environ(), lpitCrashChildEnv+"="+w.repo, "LPIT_GIT_FAULT=crash-after-commit", "TMPDIR="+w.tmp)
	out, err := child.CombinedOutput()
	var exit *exec.ExitError
	require.ErrorAs(t, err, &exit, string(out))
	status, ok := exit.Sys().(syscall.WaitStatus)
	require.True(t, ok && status.Signaled() && status.Signal() == syscall.SIGKILL, "the child died of SIGKILL: %v\n%s", err, out)

	claim := w.record(healthband.LocalPatchKindClaim, "")
	key, lp := claim.Key, w.lp()
	worktree := filepath.Join(lp, key, "worktree")
	trail := w.trail()
	require.Equal(t, "stage:apply_done", trail[len(trail)-1], "the crash came after git commit and before commit_done")
	head := strings.TrimSpace(w.git(worktree, "rev-parse", "HEAD"))
	tree, parents, message, ok := lpParseCommit(w.git(worktree, "cat-file", "commit", head))
	require.True(t, ok)
	assert.Equal(t, []string{w.base}, parents)
	assert.Equal(t, w.record(healthband.LocalPatchKindStage, healthband.StageApplyIntent).Tree, tree)
	sum := sha256.Sum256([]byte(message))
	assert.Equal(t, hex.EncodeToString(sum[:]), w.record(healthband.LocalPatchKindStage, healthband.StageMessage).MessageSHA256)
	for _, name := range []string{key + ".diff", key + ".lock"} {
		_, err := os.Lstat(filepath.Join(lp, name))
		require.NoError(t, err, "the crash left <lp>/%s", name)
	}

	deps := lpitDeps(nil)
	deps.clock = fixedAt(time.Now().Add(2 * time.Hour))
	run := w.run(&deps, "--format", "json")
	require.NoError(t, run.err, run.stdout+run.stderr)
	result := w.record(healthband.LocalPatchKindResult, "")
	assert.Equal(t, claim.ClaimID, result.ClaimID)
	assert.Equal(t, "failed:interrupted", result.Status)
	assert.True(t, result.Recovered)
	assert.Empty(t, result.Kept)
	assert.Equal(t, "result", w.trail()[len(w.trail())-1])
	w.assertClaimLeftNothing(t, key)
	w.assertRepoUnchanged(t, before, "")
	w.assertNoHookNorNetwork(t)
	assert.NotContains(t, decodeBandLPEnvelope(t, run.stdout).Data.Reasons, healthband.ReasonRecoveryKeyLocked)
}
