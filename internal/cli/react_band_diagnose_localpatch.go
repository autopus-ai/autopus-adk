package cli

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/brainstorm"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/healthband"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// Flag-on diagnose claims of auto react band (SPEC-SIGMABAND-002 plan task
// T8; REQ-02, REQ-03, REQ-13, REQ-15). While health_band.allow_local_patch is
// true, every diagnosis runs Local Patch Flow steps 1–2 inside its diagnose
// claim, then the confined claude of the Local Patch Provider Contract with
// the band worktree as its only working directory, in place of 001's
// selectBandProvider and resolveProvider. The BS of a local_patch claim
// carries the pointer lines, and the BS of every request that returned a
// stream carries the Diagnosis model line. A diagnosis without a
// local_patch claim then removes its worktree and writes its result; a
// local_patch claim runs steps 3–12 right after phase C has recorded its
// diagnose claim, with the diagnosis handed over in memory.

// bandLocalPatchClaim is a local_patch claim of the run's plan (Local Patch
// Decision Table row 5).
type bandLocalPatchClaim struct {
	id    string
	lease time.Time
}

// bandHeldPatch is what a diagnose claim leaves for its local_patch claim:
// steps 1–2, with the key lock still held, and the in-memory diagnosis.
type bandHeldPatch struct {
	setup *localPatchSetup
	input localPatchInput
}

// bandDiagnoseLocalPatch is the flag-on state of one run's diagnoser.
type bandDiagnoseLocalPatch struct {
	d       *bandDiagnoser
	patcher *bandLocalPatcher              // nil: the phase-A records were not appended
	claims  map[string]bandLocalPatchClaim // by the diagnose claim each depends on
	held    map[string]bandHeldPatch       // by diagnose claim id, from Run to AfterRecord
	report  func(healthband.LocalPatchRecord)
}

// enableLocalPatch switches d to the flag-on flow of one band run and
// returns the hook for healthband.ExecuteOptions.AfterRecord; the caller
// calls it only while health_band.allow_local_patch is true. patcher is the
// run's Local Patch Flow executor, which then runs on d's clock; nil means
// that the phase-A decision and claim records could not be appended, so
// every diagnosis reports unavailable(worktree_unavailable) without a
// worktree or a provider, and no record follows. plan is the plan's
// LocalPatch records. report, when set, receives the result record of every
// local_patch claim that wrote one, in run order (local_patches[]).
func (d *bandDiagnoser) enableLocalPatch(patcher *bandLocalPatcher, plan []healthband.LocalPatchRecord,
	report func(healthband.LocalPatchRecord)) func(context.Context, healthband.DueClaim, healthband.ClaimOutcome, healthband.Recorded) {
	lp := &bandDiagnoseLocalPatch{
		d: d, patcher: patcher, claims: map[string]bandLocalPatchClaim{}, held: map[string]bandHeldPatch{}, report: report,
	}
	for _, record := range plan {
		if record.Kind == healthband.LocalPatchKindClaim {
			lp.claims[record.DependsOn] = bandLocalPatchClaim{id: record.ClaimID, lease: record.LeaseUntil}
		}
	}
	if patcher != nil {
		patcher.now = func() time.Time { return d.now() }
	}
	d.localPatch = lp
	return lp.afterRecord
}

// run is Run of a flag-on claim after its evidence: the provider of the
// contract, steps 1–2, the confined diagnosis, the BS, and, for a diagnosis
// without a local_patch claim, its cleanup and result. The provider is
// resolved before step 1, so a refused one (provider_unconfined,
// provider_policy_rejected) leaves no record, key lock, or worktree; its
// local_patch claim then ends no_bs or diagnosis_unavailable at step 3.
func (lp *bandDiagnoseLocalPatch) run(ctx context.Context, claim healthband.DueClaim, logs []healthband.RunLog, reports []healthband.ReactReport) healthband.ClaimOutcome {
	d := lp.d
	if lp.patcher == nil {
		diagnosis := bandDiagnosis{provider: bandConfinedName(d.harness), status: bandUnavailable(bandWorktreeUnavailable)}
		return d.writeBS(ctx, d.bsRequest(claim, diagnosis, logs, reports), nil)
	}
	patch, hasPatch := lp.claims[claim.ID]
	target := localPatchTarget{diagnose: claim, claimID: patch.id, lease: patch.lease}
	provider, refused := lp.patcher.provider.resolve()
	var setup *localPatchSetup
	var diagnosis bandDiagnosis
	var reply *bandConfinedReply
	if refused != "" {
		setup = &localPatchSetup{target: target, key: target.key(), unconfined: true}
		diagnosis = bandDiagnosis{provider: bandConfinedName(d.harness), status: bandUnavailable(refused)}
	} else {
		setup = lp.patcher.prepare(ctx, target)
		diagnosis, reply = lp.diagnose(ctx, setup, provider, claim.Event, logs, reports)
	}
	req := d.bsRequest(claim, diagnosis, logs, reports)
	if location := lp.patcher.location; hasPatch && location != nil {
		req.LocalPatch = &brainstorm.LocalPatch{Key: setup.key, Dir: location.Path, ClaimID: patch.id}
	}
	if reply != nil && reply.streamed {
		model := reply.model
		req.DiagnosisModel = &brainstorm.DiagnosisModel{
			Requested: model.Requested, Actual: model.Actual, RefusalCategory: model.RefusalCategory, Substituted: reply.substituted,
		}
	}
	outcome := d.writeBS(ctx, req, diagnosis.manifest)
	if !hasPatch {
		if !setup.unconfined {
			lp.patcher.finishDiagnosis(ctx, setup, reply)
		}
		return outcome
	}
	lp.held[claim.ID] = bandHeldPatch{setup: setup, input: localPatchInput{diagnosis: diagnosis.output, logs: logs, reply: reply}}
	return outcome
}

// diagnose is the confined diagnosis: no provider without a ready worktree,
// then the contract's provider, resolved before step 1, in that worktree
// with 001's prompt. The reply is returned whenever the request returned
// one, for the models[] record.
func (lp *bandDiagnoseLocalPatch) diagnose(ctx context.Context, setup *localPatchSetup, provider orchestra.ProviderConfig, event healthband.Event, logs []healthband.RunLog, reports []healthband.ReactReport) (bandDiagnosis, *bandConfinedReply) {
	diagnosis := bandDiagnosis{provider: bandConfinedName(lp.d.harness)}
	if !setup.ready() {
		diagnosis.status = bandUnavailable(bandWorktreeUnavailable)
		return diagnosis, nil
	}
	confined := lp.patcher.provider
	rendered, err := healthband.DiagnosisPrompt(event, logs, reports)
	if err != nil {
		diagnosis.status = bandUnavailable(bandPromptInvalid)
		return diagnosis, nil
	}
	diagnosis.manifest = rendered.Manifest.Entries
	reply, reason := confined.request(ctx, provider, setup.worktree, rendered.Prompt, lpRequestDiagnosis)
	if reason != "" {
		diagnosis.status = bandUnavailable(reason)
	} else {
		diagnosis.status, diagnosis.output = lp.output(reply, setup.worktree)
	}
	return diagnosis, &reply
}

// output is the diagnosis of a stream's result text, through 001's capture
// and sanitizer: the user's checkout and then the band worktree, the
// provider's working directory, are redacted as <project>.
func (lp *bandDiagnoseLocalPatch) output(reply bandConfinedReply, worktree string) (string, healthband.Evidence) {
	text, dropped, reason := reply.diagnosisText()
	if reason != "" {
		return bandUnavailable(reason), healthband.Evidence{}
	}
	output := healthband.SanitizeProviderOutput(bandRedactDir(text, lp.d.projectDir), dropped, worktree)
	if strings.TrimSpace(output.Text) == "" {
		return bandUnavailable(bandProviderEmptyOutput), healthband.Evidence{}
	}
	return bandDiagnosisOK, output
}

// afterRecord is the ExecuteOptions.AfterRecord hook: once phase C has
// recorded a diagnose claim, its local_patch claim runs steps 3–12 under the
// same owner with the diagnose outcome, so the BS ID is bound before the
// patch stage. A local_patch claim whose diagnosis ran no steps 1–2 ends at
// step 3 with record_unavailable, so it still gets its one result.
func (lp *bandDiagnoseLocalPatch) afterRecord(ctx context.Context, claim healthband.DueClaim, outcome healthband.ClaimOutcome, _ healthband.Recorded) {
	patch, ok := lp.claims[claim.ID]
	if !ok || lp.patcher == nil {
		return
	}
	held, ran := lp.held[claim.ID]
	delete(lp.held, claim.ID)
	if !ran {
		held.setup = &localPatchSetup{target: localPatchTarget{diagnose: claim, claimID: patch.id, lease: patch.lease}}
	}
	held.input.outcome = outcome
	if result, written := lp.patcher.patch(ctx, held.setup, held.input); written && lp.report != nil {
		lp.report(result)
	}
}

// bandConfinedName is the provider name that the Local Patch Provider
// Contract selects, for the BS: health_band.local_patch_provider, trimmed,
// when set, else 001's selection; "" for none.
func bandConfinedName(harness *config.HarnessConfig) string {
	if harness == nil {
		return ""
	}
	if name := strings.TrimSpace(harness.HealthBand.LocalPatchProvider); name != "" {
		return name
	}
	return selectBandProvider(harness)
}

// bandRedactDir replaces dir, in its given and real spellings, longest
// first, with <project>.
func bandRedactDir(text, dir string) string {
	forms := []string{dir}
	if real, err := filepath.EvalSymlinks(dir); err == nil && real != dir {
		forms = append(forms, real)
	}
	sort.Slice(forms, func(i, j int) bool { return len(forms[i]) > len(forms[j]) })
	for _, form := range forms {
		if len(form) > 1 {
			text = strings.ReplaceAll(text, form, "<project>")
		}
	}
	return text
}
