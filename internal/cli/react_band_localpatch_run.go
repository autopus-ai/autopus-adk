package cli

import (
	"context"
	"errors"
	"slices"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// The Local Patch Flow of one flag-on band run (SPEC-SIGMABAND-002 plan task
// T8, REQ-02, REQ-03, REQ-12): <lp> resolved and the executor built before
// phase A's store.Lock, the Decision Table and the lease budgets wired into
// 001's Plan, the decision and claim records appended after Commit and
// before Unlock, and the hand-over to the diagnose side in phase B
// (bandEnableLocalPatch), whose AfterRecord hook runs each local_patch claim
// right after phase C. The recovery step, which runs whatever the flag, is
// recoverLocalPatches.

// errBandConfinedUnwired refuses a flag-on run whose deps have no diagnose
// side, before anything runs: without it a flag-on diagnosis could only run
// unconfined (REQ-03), so the flag stays unusable instead.
var errBandConfinedUnwired = errors.New("react band: health_band.allow_local_patch is true, but this build has no confined diagnosis; keep the flag off")

// bandLocalPatchRun is the flag-on state of one band run.
type bandLocalPatchRun struct {
	deps     bandLocalPatchDeps
	noAgent  bool
	location *healthband.LocalPatchLocation // <lp>; nil when it could not be resolved
	patcher  *bandLocalPatcher
	plan     []healthband.LocalPatchRecord // Plan.LocalPatch once phase A recorded it
	claimIDs map[string]bool               // the local_patch claim ids of plan
	results  []bandLocalPatchReport        // local_patches[] of the run output
	// recordsErr is set when this SPEC's records could not be read or
	// appended in phase A: no diagnosis of the run then creates a worktree
	// or starts a provider, and the run exits non-zero after the report.
	recordsErr error
}

func newBandLocalPatchRun(deps bandLocalPatchDeps, noAgent bool) *bandLocalPatchRun {
	return &bandLocalPatchRun{deps: deps, noAgent: noAgent, claimIDs: make(map[string]bool)}
}

// start resolves <lp> before phase A's store.Lock (Decision Table) and
// builds the run's executor with the default branch of the network step.
// An <lp> that cannot be resolved leaves the location nil, so each flag-on
// diagnosis ends step 1 cache_unavailable.
func (lp *bandLocalPatchRun) start(ctx context.Context, r bandRun, store *healthband.Store, fetch bandCIFetch) {
	rctx, cancel := context.WithTimeout(ctx, healthband.LocalPatchGitTimeout)
	defer cancel()
	if location, err := healthband.ResolveLocalPatchLocation(rctx, lp.deps.git.In(r.projectDir), lp.deps.cacheDir); err == nil {
		lp.location = &location
	}
	p := newBandLocalPatcher(r.projectDir, r.harness, lp.location, store)
	p.git, p.defaultBranch, p.warn = lp.deps.git, fetch.DefaultBranch, r.stderr
	if r.deps.clock != nil {
		p.now = r.deps.clock // the clock of the leases phase A chained
	}
	if lp.deps.patcher != nil {
		lp.deps.patcher(p)
	}
	lp.patcher = p
}

// planOptions wires the flag into 001's Plan under the store lock: the
// 990 s diagnose budget and the Decision Table hook, which reads this SPEC's
// records and the retention count of <lp>. A record read that fails keeps
// the hook out, so the run decides nothing it could not record, and so does
// an <lp> that could not be resolved, so no claim names a zero location;
// each flag-on diagnosis then ends step 1 cache_unavailable.
func (lp *bandLocalPatchRun) planOptions(locked *healthband.Locked, checkpoint healthband.Checkpoint, opts *healthband.PlanOptions) {
	opts.DiagnoseBudget = healthband.LocalPatchDiagnoseBudget
	if lp.location == nil {
		return
	}
	log, err := locked.Store().ReadLocalPatchLog()
	if err != nil {
		lp.recordsErr = err
		return
	}
	decider := &healthband.LocalPatchDecider{
		NoAgent: lp.noAgent, Checkpoint: checkpoint, Log: log, Owner: opts.Owner, NewClaimID: lp.deps.newClaimID,
		Location: *lp.location,
	}
	// A directory read that fails counts no key: step 1 counts again and
	// refuses the claim with the code of what it finds.
	decider.Kept, _ = healthband.CountKeptKeys(lp.location.Path)
	opts.LocalPatch = decider.Decide
}

// record appends the plan's decision and claim records after wal.Commit and
// before Unlock, in one write (Decision Table, phase A).
func (lp *bandLocalPatchRun) record(locked *healthband.Locked, records []healthband.LocalPatchRecord) {
	if len(records) == 0 {
		return
	}
	appendLog := lp.deps.appendLog
	if appendLog == nil {
		appendLog = (*healthband.Locked).AppendLocalPatch
	}
	if _, err := appendLog(locked, records...); err != nil {
		lp.recordsErr = err
		return
	}
	lp.plan = records
	for _, record := range records {
		if record.Kind == healthband.LocalPatchKindClaim {
			lp.claimIDs[record.ClaimID] = true
		}
	}
}

// enable hands the run's executor and records to the diagnose side before
// phase B and returns its AfterRecord hook. Records that phase A could not
// keep hand over no executor, so no diagnosis creates a worktree or starts
// a provider; --no-agent runs no provider, so it hands over nothing and
// keeps 001's diagnosis.
func (lp *bandLocalPatchRun) enable(d *bandDiagnoser) func(context.Context, healthband.DueClaim, healthband.ClaimOutcome, healthband.Recorded) {
	if lp.noAgent {
		return nil
	}
	patcher := lp.patcher
	if lp.recordsErr != nil {
		patcher = nil
	}
	return lp.deps.enable(d, patcher, lp.plan, lp.report)
}

// report keeps the result of a local_patch claim of this run for
// local_patches[]; a diagnosis result, keyed by its diagnose claim id, is
// no local patch and stays out.
func (lp *bandLocalPatchRun) report(result healthband.LocalPatchRecord) {
	if lp.claimIDs[result.ClaimID] {
		lp.results = append(lp.results, newBandLocalPatchReport(result))
	}
}

// recoverLocalPatches is the recovery step (Data Contracts): after fetchCI
// and before phase A's store.Lock, whatever the flag, never under
// --dry-run. Without localpatch-events.jsonl it runs nothing and creates no
// lock; its run reasons (recovery_locked, recovery_key_locked,
// recovery_skipped) go to the report, and only a store or lock I/O failure
// is an error.
func (r bandRun) recoverLocalPatches(ctx context.Context, store *healthband.Store, report *bandReport) error {
	deps := r.deps.localPatch
	recovery, err := store.RecoverLocalPatches(ctx, healthband.RecoveryOptions{
		Git: deps.git.In(r.projectDir), CacheDir: deps.cacheDir, Now: r.deps.clock, LockWait: deps.recoveryWait,
	})
	for _, reason := range recovery.Reasons {
		if !slices.Contains(report.Reasons, reason) {
			report.Reasons = append(report.Reasons, reason)
		}
	}
	return err
}
