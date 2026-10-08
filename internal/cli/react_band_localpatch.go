package cli

import (
	"context"
	"io"
	"time"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/editguard"
	"github.com/insajin/autopus-adk/pkg/healthband"
)

// Local Patch Flow executor of SPEC-SIGMABAND-002 (plan task T7, REQ-03,
// REQ-04, REQ-06, REQ-07, REQ-09–REQ-12, REQ-14). Steps 1–2 run inside a
// flag-on diagnose claim (prepare), a diagnosis without a local_patch claim
// ends with finishDiagnosis, and steps 3–12 run as the local_patch claim
// right after phase C recorded its diagnose claim (patch). Every git command
// goes through the Git Execution Policy runner (healthband.GitPolicyRunner):
// its own allowlist, a scrubbed environment, and a process group that gets
// SIGTERM 5 s before the step group's deadline and SIGKILL at it. <lp>, the
// key, the derived paths, the key lock, the status hash, the write-ahead
// log, and the Cleanup Rules are plan task T2's (pkg/healthband), so the
// live run and recovery derive and judge every artifact the same way.

// Codes of the Local Patch Flow steps that the executor owns; the step-1
// codes of <lp>, artifacts, and retention are healthband.LocalPatchCode*.
// A failure ends a claim failed:<code>.
const (
	lpCodeWorktreeTooLarge        = "worktree_too_large"
	lpCodeDiskInsufficient        = "disk_insufficient"
	lpCodeNoBS                    = "no_bs"
	lpCodeDiagnosisUnavailable    = "diagnosis_unavailable"
	lpCodeBranchExists            = "branch_exists"
	lpCodePatchProviderUnconfined = "patch_provider_unconfined"
	lpCodePatchModelRefused       = "patch_model_refused"
	lpCodePatchModelUnverified    = "patch_model_unverified"
	lpCodeCommitFailed            = "commit_failed"
	lpCodeCommitTreeMismatch      = "commit_tree_mismatch"
	lpCodeCommitMessageAltered    = "commit_message_altered"
	lpCodeBranchFailed            = "branch_failed"
	lpCodePatchFileFailed         = "patch_file_failed"
	// Unavailable reasons a flag-on diagnosis adds to 001's REQ-12 list.
	bandProviderUnconfined  = "provider_unconfined"
	bandWorktreeUnavailable = "worktree_unavailable"
)

// bandLocalPatchWarning is the run output's warning for every local_patch
// claim (BS Record, run output).
const bandLocalPatchWarning = "This patch was derived by an AI model from untrusted CI logs; read the whole patch file before running anything."

// localPatchModel is one models[] entry (Provider Contract item 7).
type localPatchModel = healthband.LocalPatchModel

// Request kinds of a models[] entry.
const (
	lpRequestDiagnosis = healthband.ModelRequestDiagnosis
	lpRequestPatch     = healthband.ModelRequestPatch
)

// bandLocalPatchReport is one local_patches[] entry of the run output: the
// result status, the patch file, the changed paths with their git apply
// --numstat counts, the patch request's requested and actual model, and the
// warning (BS Record, run output).
type bandLocalPatchReport struct {
	ClaimID        string                 `json:"claim_id"`
	Status         string                 `json:"status"`
	PatchPath      string                 `json:"patch_path,omitempty"`
	Files          []healthband.PatchFile `json:"files"`
	RequestedModel string                 `json:"requested_model,omitempty"`
	ActualModel    string                 `json:"actual_model,omitempty"`
	Warning        string                 `json:"warning"`
}

func newBandLocalPatchReport(result healthband.LocalPatchRecord) bandLocalPatchReport {
	report := bandLocalPatchReport{
		ClaimID: result.ClaimID, Status: result.Status, PatchPath: result.PatchPath,
		Files: append([]healthband.PatchFile{}, result.Files...), Warning: bandLocalPatchWarning,
	}
	for _, model := range result.Models {
		if model.Request == lpRequestPatch {
			report.RequestedModel, report.ActualModel = model.Requested, model.Actual
		}
	}
	return report
}

// localPatchLedger is the part of this SPEC's write-ahead log
// (.autopus/metrics/localpatch-events.jsonl) that the live flow writes;
// *healthband.Store implements it. Each append runs under SPEC-SIGMABAND-
// 001's store lock: prep re-reads the records and appends only while the
// claim has no result, and a result is write-once (appended false: the
// claim had already ended).
type localPatchLedger interface {
	AppendLocalPatchPrep(ctx context.Context, prep healthband.LocalPatchRecord) (bool, error)
	AppendLocalPatchStage(ctx context.Context, stage healthband.LocalPatchRecord) error
	AppendLocalPatchResult(ctx context.Context, result healthband.LocalPatchRecord) (bool, error)
}

// localPatchGroups are the step-group deadlines of Step Timeouts and Lease.
type localPatchGroups struct {
	setup, diagnosisCleanup, patchRequest, applyCommit, branchPatch, cleanup, margin time.Duration
}

var defaultLocalPatchGroups = localPatchGroups{
	setup: healthband.LocalPatchSetupDeadline, diagnosisCleanup: healthband.LocalPatchDiagnosisCleanupDeadline,
	patchRequest: healthband.LocalPatchRequestDeadline, applyCommit: healthband.LocalPatchApplyDeadline,
	branchPatch: healthband.LocalPatchBranchDeadline, cleanup: healthband.LocalPatchCleanupDeadline,
	margin: healthband.LocalPatchMarginDeadline,
}

// bandLocalPatcher executes the Local Patch Flow of one flag-on band run.
type bandLocalPatcher struct {
	checkout      string // the user's checkout, absolute; git runs there
	harness       *config.HarnessConfig
	location      *healthband.LocalPatchLocation // <lp>, resolved before the store lock; nil when unresolved
	ledger        localPatchLedger
	git           healthband.GitPolicyRunner // Dir is set per command
	provider      bandConfinedProvider
	defaultBranch string // the default branch 001's fetchCI resolved, or ""
	now           func() time.Time
	groups        localPatchGroups
	// Seams: the free space of <lp>, the edit guard of Patch Policy item 9,
	// and a hook between apply_done and git commit.
	statfs       func(path string) (lpDiskSpace, error)
	decide       func(editguard.Call, editguard.Options) editguard.Decision
	beforeCommit func(worktree string)
	warn         io.Writer // record faults; nil discards them
}

// newBandLocalPatcher returns the executor of a run; location comes from
// healthband.ResolveLocalPatchLocation before the store lock.
func newBandLocalPatcher(checkout string, harness *config.HarnessConfig, location *healthband.LocalPatchLocation, ledger localPatchLedger) *bandLocalPatcher {
	return &bandLocalPatcher{
		checkout: checkout, harness: harness, location: location, ledger: ledger,
		provider: newBandConfinedProvider(harness), now: time.Now, groups: defaultLocalPatchGroups, statfs: lpStatfs,
	}
}

// bandPolicyGit adapts the Git Execution Policy runner to the Patch Policy's
// GitRunner: each command runs in dir, with stdin when one is given.
func bandPolicyGit(runner healthband.GitPolicyRunner) healthband.GitRunner {
	return healthband.GitRunnerFunc(func(ctx context.Context, dir string, stdin []byte, args ...string) ([]byte, error) {
		r := runner.In(dir)
		if stdin != nil {
			return r.RunInput(ctx, stdin, args...)
		}
		return r.Run(ctx, args...)
	})
}

// covers reports that the claim's remaining lease covers a step group's
// deadline plus the cleanup and margin deadlines; a cleanup group reserves
// only the margin.
func (p *bandLocalPatcher) covers(lease time.Time, group time.Duration, cleanupGroup bool) bool {
	reserve := p.groups.margin
	if !cleanupGroup {
		reserve += p.groups.cleanup
	}
	return !p.now().Add(group + reserve).After(lease)
}

// group is the context of one step group: its deadline, never past the
// claim's lease, so a live claim never acts after lease_until.
func (p *bandLocalPatcher) group(ctx context.Context, lease time.Time, deadline time.Duration) (context.Context, context.CancelFunc) {
	if remaining := lease.Sub(p.now()); remaining < deadline {
		deadline = max(remaining, 0)
	}
	return context.WithTimeout(ctx, deadline)
}
