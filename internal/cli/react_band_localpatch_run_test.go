//go:build unix

package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// SPEC-SIGMABAND-002 S2, S3, S4 wiring through the band command (plan task
// T8): phase A appends the decision and claim records after Commit with the
// chained leases, phase B hands the executor and Plan.LocalPatch to
// production's diagnose side before the prepare hook, which runs the
// confined diagnosis in place of 001's provider call, and the hook it
// returns runs as AfterRecord right after phase C, its results going to
// local_patches[].
func TestReactBandLocalPatch_FlagOn_HandsTheFlowToTheDiagnoseSide(t *testing.T) {
	w := newBandLPWorld(t, bandLPConfig)
	w.storeO3()
	worktree, patchPath := filepath.Join(w.lp(), lpKey, "worktree"), filepath.Join(w.lp(), lpKey+".patch")
	w.enable.during = func(healthband.DueClaim) {
		_, results := bandITEvents(t, w.project)
		assert.Len(t, results, 1, "phase C recorded the diagnose claim before AfterRecord")
	}
	run := w.band("--no-fetch", "--format", "json")
	require.NoError(t, run.err, run.stdout)

	assert.Equal(t, []string{"enable", "prepare"}, w.order, "the hand-over precedes the prepare hook")
	require.Equal(t, 1, w.enable.calls)
	p := w.enable.patcher
	require.NotNil(t, p)
	assert.Equal(t, w.repo, p.checkout)
	require.NotNil(t, p.location)
	assert.Equal(t, w.lp(), p.location.Path, "<lp> resolved below the user cache directory")
	assert.Equal(t, bandLPT0, p.now(), "the executor runs on the clock of the leases")
	assert.Equal(t, w.patcher.git.Environ(), p.git.Environ())
	assert.True(t, p.harness.HealthBand.AllowLocalPatch)

	diagnose := w.diagnoseClaim(lpSeries)
	assert.Equal(t, bandLPT0.Add(990*time.Second), diagnose.LeaseUntil.UTC(), "a flag-on diagnose claim gets 990 s")
	evaluations, results := bandITEvents(t, w.project)
	claim := healthband.LocalPatchRecord{
		Kind: healthband.LocalPatchKindClaim, ClaimID: lpPatchClaimID, Owner: diagnose.Owner,
		LeaseUntil: bandLPT0.Add(1800 * time.Second), Series: lpSeries, EpisodeID: "e1042", DependsOn: diagnose.ID,
		Key: lpKey, WorktreePath: worktree, PatchPath: patchPath, Branch: "autopus/band/" + lpKey,
	}
	require.Len(t, w.enable.plan, 2, "Plan.LocalPatch: the decision, then the row-5 claim")
	decision := w.enable.plan[0]
	assert.Equal(t, healthband.LocalPatchKindDecision, decision.Kind)
	assert.Equal(t, healthband.LocalPatchDecideClaim, decision.Decision)
	assert.Equal(t, evaluations[len(evaluations)-1].Seq, decision.EvaluationSeq, "the decision names the key-1042 event")
	plan := w.enable.plan[1]
	plan.LeaseUntil = plan.LeaseUntil.UTC()
	assert.Equal(t, claim, plan)
	stored := w.record(healthband.LocalPatchKindClaim)
	claim.Schema, claim.Seq = healthband.SchemaLocalPatch, stored.Seq
	assert.Equal(t, claim, stored, "the claim record was appended under the store lock")

	require.Len(t, results, 1)
	assert.Equal(t, []string{diagnose.ID + " " + results[0].BSID}, w.enable.after, "AfterRecord got the outcome phase C recorded")
	assert.Equal(t, bandDiagnosisOK, results[0].DiagnosisStatus, "the confined diagnosis answered")
	assert.Zero(t, w.provider.count(), "001's unconfined provider call never ran")
	assert.Equal(t, "call\ncall\n", w.fake.record(t, "calls"), "the diagnosis and the patch request")
	bs, err := os.ReadFile(filepath.Join(w.repo, ".autopus", "brainstorms", results[0].BSID+".md"))
	require.NoError(t, err)
	assert.Contains(t, string(bs), "Local patch (3σ, local only, if produced): branch autopus/band/"+lpKey+",")
	assert.Equal(t, []string{"decision", "claim", "prep", "stage:worktree_intent", "stage:worktree_done", "stage:message",
		"stage:apply_intent", "stage:apply_done", "stage:commit_done", "stage:branch_intent", "stage:branch_done",
		"stage:patch_intent", "stage:patch_done", "result"}, w.trail(), "the handed-over executor ran the whole flow")
	assert.Equal(t, w.base, w.record(healthband.LocalPatchKindPrep).BaseSHA)
	result := w.record(healthband.LocalPatchKindResult)
	assert.Equal(t, healthband.ClaimDone, result.Status)
	assert.Equal(t, results[0].BSID, result.BSID)
	assert.Equal(t, result.CommitSHA+"\n", w.git(w.repo, "rev-parse", "refs/heads/autopus/band/"+lpKey))

	envelope := decodeBandLPEnvelope(t, run.stdout)
	assert.Equal(t, []bandLocalPatchReport{{
		ClaimID: lpPatchClaimID, Status: healthband.ClaimDone, PatchPath: patchPath,
		Files:          []healthband.PatchFile{{Path: "pkg/foo/foo.go", Added: 1, Removed: 1}},
		RequestedModel: "claude-opus-5-5", ActualModel: "claude-opus-5-5", Warning: bandLocalPatchWarning,
	}}, envelope.Data.LocalPatches)
}

// local_patches[] lists local_patch claims only: a result keyed by a
// diagnose claim id, which a diagnosis without a local_patch claim writes,
// stays out.
func TestReactBandLocalPatch_Report_KeepsOnlyLocalPatchClaims(t *testing.T) {
	w := newBandLPWorld(t, bandLPConfig)
	w.storeO3()
	w.enable.reports = func(report func(healthband.LocalPatchRecord), claim healthband.DueClaim) {
		report(healthband.NewLocalPatchResult(claim.ID, healthband.LocalPatchCodeCapReached))
		report(healthband.NewLocalPatchResult(lpPatchClaimID, "diagnosis_unavailable"))
	}
	run := w.band("--no-fetch", "--format", "json")
	require.NoError(t, run.err, run.stdout)
	assert.Equal(t, []bandLocalPatchReport{{
		ClaimID: lpPatchClaimID, Status: "failed:diagnosis_unavailable", Files: []healthband.PatchFile{}, Warning: bandLocalPatchWarning,
	}}, decodeBandLPEnvelope(t, run.stdout).Data.LocalPatches)
}
