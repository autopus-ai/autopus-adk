package cli

import (
	"context"
	"time"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// The seam between a flag-on band run (SPEC-SIGMABAND-002 plan task T8,
// react_band_localpatch_run.go) and the flag-on diagnose side
// (react_band_diagnose*.go). The run resolves <lp>, builds the executor,
// wires the Decision Table and the lease budgets into phase A, appends the
// decision and claim records, and then, in phase B and before the prepare
// hook, hands the executor and the plan's records to the diagnose side. The
// diagnose side runs Local Patch Flow steps 1–2 and the confined diagnosis
// inside bandDiagnoser.Run, ends a diagnosis without a local_patch claim,
// and returns the ExecuteOptions.AfterRecord hook that runs each
// local_patch claim right after phase C. With the flag off, and under
// --no-agent, the run hands over nothing and 001's diagnosis runs.

// bandEnableLocalPatch is the diagnose side's entry point, production's
// (*bandDiagnoser).enableLocalPatch. patcher is the run's executor, or nil
// when this SPEC's records were unavailable in phase A, so every diagnosis
// ends unavailable(worktree_unavailable); plan is Plan.LocalPatch as phase
// A recorded it, nil when it recorded none; report receives every result
// that a local_patch claim ends with, for local_patches[]. The returned
// func is the run's ExecuteOptions.AfterRecord.
type bandEnableLocalPatch func(
	d *bandDiagnoser, patcher *bandLocalPatcher, plan []healthband.LocalPatchRecord, report func(healthband.LocalPatchRecord),
) func(context.Context, healthband.DueClaim, healthband.ClaimOutcome, healthband.Recorded)

// bandLocalPatchDeps are the seams of a flag-on run; the zero value with
// enable set is production.
type bandLocalPatchDeps struct {
	// enable is the diagnose side; a flag-on run without it refuses the flag
	// before anything runs (errBandConfinedUnwired).
	enable bandEnableLocalPatch
	// git runs every Git Execution Policy command (Dir is set per command);
	// the zero value is git from PATH with os.Environ.
	git healthband.GitPolicyRunner
	// cacheDir is the user cache directory of <lp>; "" is os.UserCacheDir.
	cacheDir string
	// newClaimID draws local_patch claim ids; nil is healthband.NewClaimID.
	newClaimID func() string
	// recoveryWait bounds the .recovery.lock wait; 0 is RecoveryLockWait.
	recoveryWait time.Duration
	// appendLog appends phase A's records; nil is Locked.AppendLocalPatch.
	appendLog func(*healthband.Locked, ...healthband.LocalPatchRecord) ([]healthband.LocalPatchRecord, error)
	// patcher adjusts the run's executor once it is built; nil keeps it.
	patcher func(*bandLocalPatcher)
}
