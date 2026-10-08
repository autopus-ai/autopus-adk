//go:build unix

package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/filelock"
	"github.com/insajin/autopus-adk/pkg/healthband"
)

// SPEC-SIGMABAND-002 S1 and S9 recovery wiring through the band command
// (plan task T8): the recovery step runs between the network step and
// phase A whenever the local patch log exists, whatever the flag, never
// under --dry-run, and a flag-off run otherwise adds nothing to 001.

// bandLPFlagOff is 001's integration autopus.yaml (orchestra.judge codex)
// with local_patch_provider claude but without the flag.
const bandLPFlagOff = bandITConfig + "health_band:\n  local_patch_provider: claude\n"

// interruptedClaim stores the decision and claim records of an S4 claim
// whose lease passed an hour before bandLPT0, with no artifact.
func (w *bandLPWorld) interruptedClaim() {
	w.t.Helper()
	paths := w.patcher.location.Paths(lpKey)
	locked, err := healthband.NewStore(w.repo).Lock(context.Background(), healthband.StoreLockWait)
	require.NoError(w.t, err)
	_, err = locked.AppendLocalPatch(
		healthband.LocalPatchRecord{Kind: healthband.LocalPatchKindDecision, Series: lpSeries, EpisodeID: "e1042",
			EvaluationSeq: 1, Decision: healthband.LocalPatchDecideClaim},
		healthband.LocalPatchRecord{Kind: healthband.LocalPatchKindClaim, ClaimID: lpPatchClaimID, Owner: healthband.NewOwner(),
			LeaseUntil: bandLPT0.Add(-time.Hour), Series: lpSeries, EpisodeID: "e1042", DependsOn: lpDiagnoseClaimID,
			Key: lpKey, WorktreePath: paths.Worktree, PatchPath: paths.Patch, Branch: paths.Branch},
	)
	require.NoError(w.t, errors.Join(err, locked.Unlock()))
}

// S9: a run with the flag off and the log present still recovers the claim.
func TestReactBandLocalPatch_Recovery_RunsWithTheFlagOff(t *testing.T) {
	w := newBandLPWorld(t, bandLPFlagOff)
	w.interruptedClaim()
	run := w.band("--no-fetch", "--format", "json")
	require.NoError(t, run.err, run.stdout)

	result := w.record(healthband.LocalPatchKindResult)
	assert.Equal(t, lpPatchClaimID, result.ClaimID)
	assert.Equal(t, healthband.ClaimFailedPrefix+healthband.LocalPatchCodeInterrupted, result.Status)
	assert.True(t, result.Recovered)
	_, err := os.Lstat(w.project.metrics(healthband.RecoveryLockFile))
	assert.NoError(t, err, "the recovery step took .recovery.lock")
	assert.Zero(t, w.enable.calls, "the flag is off: nothing is handed over")
}

// S9: a second process holding the recovery lock makes the run report
// recovery_locked in its JSON envelope and text output and recover nothing.
func TestReactBandLocalPatch_Recovery_ReportsALockedRecovery(t *testing.T) {
	w := newBandLPWorld(t, bandLPFlagOff)
	w.interruptedClaim()
	held, err := filelock.Acquire(context.Background(), w.project.metrics(healthband.RecoveryLockFile), 0)
	require.NoError(t, err)
	defer func() { require.NoError(t, held.Unlock()) }()

	run := w.band("--no-fetch", "--format", "json")
	require.NoError(t, run.err, run.stdout)
	assert.Contains(t, decodeBandLPEnvelope(t, run.stdout).Data.Reasons, healthband.ReasonRecoveryLocked)
	text := w.band("--no-fetch")
	require.NoError(t, text.err, text.stdout)
	assert.Contains(t, text.stdout, "reason: "+healthband.ReasonRecoveryLocked+"\n")
	assert.NotContains(t, w.trail(), "result", "no result while another process recovers")
}

// S9: a --dry-run run takes neither lock and leaves every file under
// .autopus/ and <lp> byte-identical.
func TestReactBandLocalPatch_Recovery_NeverRunsUnderDryRun(t *testing.T) {
	w := newBandLPWorld(t, bandLPConfig)
	w.storeO3()
	w.interruptedClaim()
	store, lp := bandTreeHashes(t, w.repo), bandLPTreeHashes(t, w.lp())
	run := w.band("--no-fetch", "--dry-run", "--format", "json")
	require.NoError(t, run.err, run.stdout)
	assert.Equal(t, store, bandTreeHashes(t, w.repo))
	assert.Equal(t, lp, bandLPTreeHashes(t, w.lp()))
	_, err := os.Lstat(w.project.metrics(healthband.RecoveryLockFile))
	assert.True(t, os.IsNotExist(err), "no .recovery.lock under --dry-run")
	assert.Zero(t, w.enable.calls)
}

// S1: with the flag off, local_patch_provider set or not, band creates no
// local patch file, no .recovery.lock, and no <lp>, runs no Git Execution
// Policy command, and its JSON data has no local_patches key.
func TestReactBandLocalPatch_FlagOff_AddsNothingTo001(t *testing.T) {
	for name, config := range map[string]string{"no health_band": bandITConfig, "provider key only": bandLPFlagOff} {
		t.Run(name, func(t *testing.T) {
			w := newBandLPWorld(t, config)
			w.storeO3()
			cache, gitLog := t.TempDir(), filepath.Join(t.TempDir(), "git.log")
			fakeGit := filepath.Join(t.TempDir(), "git")
			require.NoError(t, os.WriteFile(fakeGit, []byte("#!/bin/sh\necho \"$@\" >> "+gitLog+"\nexec git \"$@\"\n"), 0o755))
			w.deps = func(deps *bandLocalPatchDeps) { deps.cacheDir, deps.git.Binary = cache, fakeGit }
			run := w.band("--no-fetch", "--format", "json")
			require.NoError(t, run.err, run.stdout)

			for _, name := range []string{healthband.LocalPatchEventsFile, healthband.LocalPatchStateFile, healthband.RecoveryLockFile} {
				_, err := os.Lstat(w.project.metrics(name))
				assert.True(t, os.IsNotExist(err), name)
			}
			entries, err := os.ReadDir(cache)
			require.NoError(t, err)
			assert.Empty(t, entries, "no <lp> below the user cache directory")
			_, err = os.Lstat(gitLog)
			assert.True(t, os.IsNotExist(err), "no Git Execution Policy command ran")
			assert.Zero(t, w.enable.calls, "001's diagnosis runs")
			require.Equal(t, 1, w.provider.count())
			assert.Equal(t, []string{w.repo}, w.workDirs, "the provider runs in the project directory")
			for _, flag := range []string{"--restricted", "--verbose", "stream-json"} {
				assert.NotContains(t, w.provider.argv[0], flag, "001's projection of the selected codex")
			}
			_, results := bandITEvents(t, w.project)
			require.Len(t, results, 1)
			assert.Equal(t, bandDiagnosisOK, results[0].DiagnosisStatus)
			assert.NotContains(t, decodeBandLPEnvelope(t, run.stdout).raw["data"], "local_patches")
		})
	}
}

// bandLPTreeHashes hashes every regular file below root by relative path.
func bandLPTreeHashes(t *testing.T, root string) map[string]string {
	t.Helper()
	hashes := make(map[string]string)
	require.NoError(t, filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		sum := sha256.Sum256(data)
		rel, _ := filepath.Rel(root, path)
		hashes[rel] = hex.EncodeToString(sum[:])
		return err
	}))
	return hashes
}
