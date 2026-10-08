//go:build unix

package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// SPEC-SIGMABAND-002 S3 decision paths and the phase-A record failure
// through the band command (plan task T8).

// S3: a failed phase-A append of the decision and claim records hands the
// diagnose side no executor and no plan, so no diagnosis creates a worktree
// or starts a provider, and the run exits non-zero after the report.
func TestReactBandLocalPatch_PhaseAAppendFails_HandsOverNoExecutor(t *testing.T) {
	w := newBandLPWorld(t, bandLPConfig)
	w.storeO3()
	errAppend := errors.New("injected append failure")
	w.deps = func(deps *bandLocalPatchDeps) {
		deps.appendLog = func(*healthband.Locked, ...healthband.LocalPatchRecord) ([]healthband.LocalPatchRecord, error) {
			return nil, errAppend
		}
	}
	run := w.band("--no-fetch", "--format", "json")
	require.ErrorIs(t, run.err, errAppend)

	require.Equal(t, 1, w.enable.calls, "the diagnose side still runs every diagnosis")
	assert.Nil(t, w.enable.patcher, "no executor: each diagnosis ends unavailable(worktree_unavailable)")
	assert.Nil(t, w.enable.plan, "no claim was recorded, so none is handed over")
	assert.Empty(t, w.trail(), "no decision, claim, prep, stage, or result record")
	assert.NotContains(t, w.worktrees(), w.lp(), "0 worktree invocations")
	assert.Empty(t, w.fake.record(t, "calls"), "no patch request")
	_, results := bandITEvents(t, w.project)
	require.Len(t, results, 1, "phase C still records the diagnose claim")
	assert.Equal(t, bandUnavailable(bandWorktreeUnavailable), results[0].DiagnosisStatus)
	envelope := decodeBandLPEnvelope(t, run.stdout)
	assert.Equal(t, string(jsonStatusError), envelope.Status, "the report is written before the non-zero exit")
}

// F6: an <lp> that cannot be resolved before phase A installs no Decision
// Table hook, so no decision or claim record names a zero location; the
// tier-3 diagnosis still runs flag-on and step 1 ends it cache_unavailable.
func TestReactBandLocalPatch_UnresolvedLocation_DecidesNoClaim(t *testing.T) {
	w := newBandLPWorld(t, bandLPConfig)
	w.storeO3()
	broken := filepath.Join(t.TempDir(), "git")
	require.NoError(t, os.WriteFile(broken, []byte("#!/bin/sh\ncase \" $* \" in *\" --git-common-dir \"*) exit 128;; esac\nexec git \"$@\"\n"), 0o755))
	w.deps = func(deps *bandLocalPatchDeps) { deps.git.Binary = broken }
	run := w.band("--no-fetch", "--format", "json")
	require.NoError(t, run.err, run.stdout)

	assert.Equal(t, []string{"prep", "result"}, w.trail(), "no decision or claim record")
	assert.Empty(t, w.enable.plan)
	assert.Equal(t, healthband.LocalPatchCodeCacheUnavailable, w.record(healthband.LocalPatchKindPrep).Code)
	assert.Equal(t, w.diagnoseClaim(lpSeries).ID, w.record(healthband.LocalPatchKindResult).ClaimID)
	assert.NotContains(t, w.worktrees(), w.lp(), "0 worktree invocations")
	assert.Empty(t, w.fake.record(t, "calls"), "no provider call")
	assert.Empty(t, decodeBandLPEnvelope(t, run.stdout).Data.LocalPatches)
}

// S3 row 1: --no-agent wins over row 5, and the run hands nothing to the
// diagnose side, so 001's diagnosis runs no provider and needs no worktree.
func TestReactBandLocalPatch_NoAgent_RecordsTheSkipAndKeeps001(t *testing.T) {
	w := newBandLPWorld(t, bandLPConfig)
	w.storeO3()
	run := w.band("--no-fetch", "--no-agent", "--format", "json")
	require.NoError(t, run.err, run.stdout)

	assert.Equal(t, []string{"decision"}, w.trail())
	decision := w.record(healthband.LocalPatchKindDecision)
	assert.Equal(t, healthband.LocalPatchDecideSkip, decision.Decision)
	assert.Equal(t, healthband.LocalPatchSkippedNoAgent, decision.Reason)
	assert.Zero(t, w.enable.calls, "nothing is handed over under --no-agent")
	assert.Equal(t, []string{"prepare"}, w.order)
	_, results := bandITEvents(t, w.project)
	require.Len(t, results, 1)
	assert.Equal(t, bandDiagnosisSkippedNoAgent, results[0].DiagnosisStatus)
	assert.Len(t, bandITHeads(t, w.repo), 1, "001 writes the evidence-only BS")
}

// S3: an <lp> that holds 5 kept keys gives the tier-3 opening
// local_patch_skipped:cap_reached with no claim, so Plan.LocalPatch holds the
// decision alone; the diagnosis still runs flag-on, and step 1 refuses its
// worktree with a cap_reached prep and result keyed by the diagnose claim,
// with 0 worktree invocations and the kept files byte-identical.
func TestReactBandLocalPatch_RetentionCap_SkipsTheOpening(t *testing.T) {
	w := newBandLPWorld(t, bandLPConfig)
	w.storeO3()
	kept := func(i int) string { return filepath.Join(w.lp(), "kept-"+strconv.Itoa(i)+".patch") }
	for i := 1; i <= healthband.LocalPatchRetentionCap; i++ {
		require.NoError(t, os.WriteFile(kept(i), []byte("kept "+strconv.Itoa(i)+"\n"), 0o600))
	}
	run := w.band("--no-fetch", "--format", "json")
	require.NoError(t, run.err, run.stdout)

	assert.Equal(t, []string{"decision", "prep", "result"}, w.trail())
	assert.Equal(t, healthband.LocalPatchSkippedCapReached, w.record(healthband.LocalPatchKindDecision).Reason)
	require.Len(t, w.enable.plan, 1)
	assert.Equal(t, healthband.LocalPatchKindDecision, w.enable.plan[0].Kind)
	assert.NotNil(t, w.enable.patcher, "the tier-3 diagnosis still runs flag-on, step 1 refusing its worktree")
	diagnose := w.diagnoseClaim(lpSeries)
	assert.Equal(t, healthband.LocalPatchCodeCapReached, w.record(healthband.LocalPatchKindPrep).Code)
	result := w.record(healthband.LocalPatchKindResult)
	assert.Equal(t, diagnose.ID, result.ClaimID, "a diagnosis without a local_patch claim keys its result by its claim")
	assert.Equal(t, healthband.ClaimFailedPrefix+healthband.LocalPatchCodeCapReached, result.Status)
	assert.NotContains(t, w.worktrees(), w.lp(), "0 worktree invocations")
	assert.Empty(t, w.fake.record(t, "calls"), "no worktree, no provider")
	_, results := bandITEvents(t, w.project)
	require.Len(t, results, 1)
	assert.Equal(t, bandUnavailable(bandWorktreeUnavailable), results[0].DiagnosisStatus)
	for i := 1; i <= healthband.LocalPatchRetentionCap; i++ {
		data, err := os.ReadFile(kept(i))
		require.NoError(t, err)
		assert.Equal(t, "kept "+strconv.Itoa(i)+"\n", string(data))
	}
	assert.Empty(t, decodeBandLPEnvelope(t, run.stdout).Data.LocalPatches)
}
