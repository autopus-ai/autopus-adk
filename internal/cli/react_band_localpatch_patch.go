package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/healthband"
	"github.com/insajin/autopus-adk/pkg/lore"
)

// The end of a flag-on diagnosis without a local_patch claim, and steps
// 3–6 of a local_patch claim: the result check of REQ-04, the branch check,
// the Lore message, and the patch request on the confined provider. Steps
// 7–12 follow in react_band_localpatch_apply.go.

// localPatchInput is the in-memory state of the diagnose claim that the
// patch stage reads; the BS file is never re-read.
type localPatchInput struct {
	outcome   healthband.ClaimOutcome // the diagnose claim's BS ID and diagnosis status
	diagnosis healthband.Evidence     // the sanitized text passed to brainstorm.Request.Diagnosis
	logs      []healthband.RunLog     // the sanitized failed-step logs
	reply     *bandConfinedReply      // the diagnosis's stream; nil when its request returned none
}

// finishDiagnosis ends a flag-on diagnosis without a local_patch claim:
// within the diagnosis-only cleanup deadline the Cleanup Rules remove its
// worktree, and its result, keyed by the diagnose claim id, is written.
func (p *bandLocalPatcher) finishDiagnosis(ctx context.Context, s *localPatchSetup, reply *bandConfinedReply) healthband.LocalPatchRecord {
	result := healthband.NewLocalPatchResult(s.target.diagnose.ID, s.code)
	result.BaseSHA, result.Kept = s.baseSHA, s.kept
	lpAddModel(&result, reply)
	if s.stopped {
		return result
	}
	if s.worktree != "" {
		lease := s.target.diagnose.LeaseUntil
		if !p.covers(lease, p.groups.diagnosisCleanup, true) {
			result.Status = healthband.ClaimFailedPrefix + healthband.LocalPatchCodeLeaseExhausted
		}
		result.Kept = append(result.Kept, p.cleanup(ctx, s, lease, p.groups.diagnosisCleanup)...)
	}
	return p.finish(ctx, s, result)
}

// finish appends the claim's write-once result and releases the key lock.
func (p *bandLocalPatcher) finish(ctx context.Context, s *localPatchSetup, result healthband.LocalPatchRecord) healthband.LocalPatchRecord {
	if _, err := p.ledger.AppendLocalPatchResult(ctx, result); err != nil && p.warn != nil {
		fmt.Fprintf(p.warn, "react band: local patch result of claim %s: %v\n", result.ClaimID, err)
	}
	p.release(s)
	return result
}

// lpAddModel records the models[] entry of a request that returned a stream.
func lpAddModel(r *healthband.LocalPatchRecord, reply *bandConfinedReply) {
	if reply == nil || !reply.streamed {
		return
	}
	r.Models = append(r.Models, reply.model)
	r.ModelSubstituted = r.ModelSubstituted || reply.substituted
}

// patchRun is one local_patch claim between steps 3 and 12.
type patchRun struct {
	p       *bandLocalPatcher
	s       *localPatchSetup
	in      localPatchInput
	result  healthband.LocalPatchRecord
	message string                // the -F file bytes (step 6)
	reply   healthband.PatchReply // the patch request's reply, in memory only
	diff    string                // the accepted diff (step 7)
	tree    string                // the expected tree (step 8)
	commit  string                // the recorded commit OID (step 9)
	temp    string                // band temp directory of steps 8–9
}

// patch runs steps 3–12 of the local_patch claim and writes its result; a
// claim that step 1 stopped gets no record and reports false.
func (p *bandLocalPatcher) patch(ctx context.Context, s *localPatchSetup, in localPatchInput) (healthband.LocalPatchRecord, bool) {
	if s.stopped {
		return healthband.LocalPatchRecord{}, false
	}
	run := &patchRun{p: p, s: s, in: in, result: healthband.NewLocalPatchResult(s.target.claimID, "")}
	run.result.BSID, run.result.BaseSHA, run.result.Kept = in.outcome.BSID, s.baseSHA, s.kept
	lpAddModel(&run.result, in.reply)
	code := run.steps(ctx)
	if run.temp != "" {
		_ = os.RemoveAll(run.temp)
	}
	if code != "" {
		run.result.Status = healthband.ClaimFailedPrefix + code
		if s.worktree != "" {
			run.result.Kept = append(run.result.Kept, p.cleanup(ctx, s, s.target.lease, p.groups.cleanup)...)
		}
		return p.finish(ctx, s, run.result), true
	}
	_ = s.dir.RemoveFile(s.key + ".diff")
	run.result.CommitSHA, run.result.Branch = run.commit, s.paths.Branch
	run.result.WorktreePath, run.result.PatchPath = s.worktree, s.paths.Patch
	return p.finish(ctx, s, run.result), true
}

// steps runs the step groups in order; each starts only while the lease
// covers it, and the first failing step gives the code.
func (r *patchRun) steps(ctx context.Context) string {
	if code := r.resultCheck(); code != "" {
		return code
	}
	groups, lease := []struct {
		deadline time.Duration
		steps    []func(gctx, ctx context.Context) string
	}{
		{r.p.groups.patchRequest, []func(gctx, ctx context.Context) string{r.branchCheck, r.draft, r.request}},
		{r.p.groups.applyCommit, []func(gctx, ctx context.Context) string{r.policy, r.apply, r.commitPatch}},
		{r.p.groups.branchPatch, []func(gctx, ctx context.Context) string{r.branch, r.patchFile}},
	}, r.s.target.lease
	for _, group := range groups {
		if !r.p.covers(lease, group.deadline, false) {
			return healthband.LocalPatchCodeLeaseExhausted
		}
		gctx, cancel := r.p.group(ctx, lease, group.deadline)
		for _, step := range group.steps {
			if code := step(gctx, ctx); code != "" {
				cancel()
				return code
			}
		}
		cancel()
	}
	return ""
}

// resultCheck is step 3 in the order of REQ-04.
func (r *patchRun) resultCheck() string {
	s := r.s
	switch {
	case s.unconfined && r.in.outcome.BSID == "":
		return lpCodeNoBS // a refused provider ran no step 1; REQ-04's BS and diagnosis checks remain
	case s.unconfined:
		return lpCodeDiagnosisUnavailable
	case !s.prepped && s.code != "":
		return s.code // record_unavailable or lease_exhausted before any prep
	case !s.prepped:
		return healthband.LocalPatchCodeRecordUnavailable
	case s.code != "":
		return s.code // the prep code, the worktree_failed code, or record_unavailable
	case s.worktree == "":
		return healthband.LocalPatchCodeRecordUnavailable
	case r.in.outcome.BSID == "":
		return lpCodeNoBS
	case r.in.outcome.DiagnosisStatus != bandDiagnosisOK:
		return lpCodeDiagnosisUnavailable
	}
	return ""
}

// branchCheck is step 4: the band branch is still absent, as a ref and as a
// symbolic ref; any answer but git's "absent" exit 1 counts as present.
func (r *patchRun) branchCheck(gctx, _ context.Context) string {
	run := r.p.git.In(r.s.worktree)
	if _, err := run.Run(gctx, "rev-parse", "--verify", "--quiet", r.s.paths.Ref); healthband.GitExitCode(err) != 1 {
		return lpCodeBranchExists
	}
	if _, err := run.Run(gctx, "symbolic-ref", "-q", "--no-recurse", r.s.paths.Ref); healthband.GitExitCode(err) != 1 {
		return lpCodeBranchExists
	}
	return ""
}

// draft is step 5: the Lore message without the Patch model line, so a
// configuration that cannot take the band commit stops before the provider.
func (r *patchRun) draft(context.Context, context.Context) string {
	_, code := r.commitMessage("")
	return code
}

func (r *patchRun) commitMessage(model string) (string, string) {
	d, h := r.s.target.diagnose, r.p.harness
	in := healthband.PatchCommitInput{
		Series: d.Series, EpisodeID: d.EpisodeID, Tier: d.Tier, Z: d.Event.Z, BSID: r.in.outcome.BSID, PatchModel: model,
	}
	if h != nil {
		in.Language = h.Language.Commits
		in.Lore = lore.LoreConfig{RequiredTrailers: h.Lore.RequiredTrailers, ForbiddenTrailers: h.Lore.EffectiveForbiddenTrailers()}
	}
	return healthband.PatchCommitMessage(in)
}

// request is step 6: the patch prompt on the provider that the contract
// resolves again, the model checks of Provider Contract item 7, the final
// message with the Patch model line, and its stage record.
func (r *patchRun) request(gctx, ctx context.Context) string {
	provider, reason := r.p.provider.resolve()
	if reason != "" {
		return lpCodePatchProviderUnconfined
	}
	d := r.s.target.diagnose
	rendered, err := healthband.PatchPrompt(healthband.PatchPromptInput{
		Event: d.Event, DiagnoseClaimID: d.ID, Diagnosis: r.in.diagnosis, Logs: r.in.logs,
		BaseSHA: r.s.baseSHA, TrackedPaths: r.trackedPaths(gctx),
	})
	if err != nil {
		return bandPromptInvalid
	}
	r.result.PromptManifest = rendered.Manifest.Entries
	reply, reason := r.p.provider.request(gctx, provider, r.s.worktree, rendered.Prompt, lpRequestPatch)
	lpAddModel(&r.result, &reply)
	if reason != "" {
		return reason
	}
	if code := reply.patchCode(); code != "" {
		return code
	}
	message, code := r.commitMessage(reply.initModel)
	if code != "" {
		return code
	}
	r.message, r.reply = message, reply.patchReply()
	stage := healthband.NewLocalPatchStage(r.s.target.claimID, healthband.StageMessage)
	stage.MessageSHA256 = lpSHA256([]byte(message))
	if err := r.p.appendStage(ctx, r.s, stage); err != nil {
		return healthband.LocalPatchCodeRecordUnavailable
	}
	return ""
}

// trackedPaths lists the base tree for the band.base layer; a listing that
// fails leaves the layer with no paths.
func (r *patchRun) trackedPaths(ctx context.Context) []string {
	out, err := r.p.git.In(r.s.worktree).Run(ctx, "ls-tree", "-r", "-z", "--name-only", r.s.baseSHA)
	if err != nil || len(out) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
}

func lpSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
