package cli

import (
	"context"
	"io"
	"time"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/editguard"
	"github.com/insajin/autopus-adk/pkg/healthband"
	"github.com/insajin/autopus-adk/pkg/promptlayer"
)

// Local Patch Flow executor of SPEC-SIGMABAND-002 (plan task T7, REQ-03,
// REQ-04, REQ-06, REQ-07, REQ-09–REQ-12, REQ-14). Steps 1–2 run inside a
// flag-on diagnose claim (prepare), a diagnosis without a local_patch claim
// ends with finishDiagnosis, and steps 3–12 run as the local_patch claim
// right after phase C recorded its diagnose claim (patch). Every git command
// goes through the Git Execution Policy runner (healthband.GitPolicyRunner):
// its own allowlist, a scrubbed environment, and a process group that gets
// SIGTERM 5 s before the step group's deadline and SIGKILL at it. The
// write-ahead log and the Cleanup Rules are plan task T2's; the executor
// reaches them through localPatchLedger and localPatchCleaner.

// Codes of the Local Patch Flow steps; a failure ends a claim failed:<code>.
const (
	lpStatusDone                  = "done"
	lpCodeOK                      = "ok"
	lpCodeRecordUnavailable       = "record_unavailable"
	lpCodeCacheUnavailable        = "cache_unavailable"
	lpCodeArtifactExists          = "artifact_exists"
	lpCodeCapReached              = "cap_reached"
	lpCodeWorktreeTooLarge        = "worktree_too_large"
	lpCodeDiskInsufficient        = "disk_insufficient"
	lpCodeWorktreeFailed          = "worktree_failed"
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
	lpCodeLeaseExhausted          = "lease_exhausted"
	lpKeptWorktreeIncomplete      = "worktree_incomplete"
	// Unavailable reasons a flag-on diagnosis adds to 001's REQ-12 list.
	bandProviderUnconfined  = "provider_unconfined"
	bandWorktreeUnavailable = "worktree_unavailable"
)

// Stage phases of the stage record (Data Contracts).
const (
	lpPhaseWorktreeIntent = "worktree_intent"
	lpPhaseWorktreeDone   = "worktree_done"
	lpPhaseWorktreeFailed = "worktree_failed"
	lpPhaseMessage        = "message"
	lpPhaseApplyIntent    = "apply_intent"
	lpPhaseApplyDone      = "apply_done"
	lpPhaseCommitDone     = "commit_done"
	lpPhaseBranchIntent   = "branch_intent"
	lpPhaseBranchDone     = "branch_done"
	lpPhasePatchIntent    = "patch_intent"
	lpPhasePatchDone      = "patch_done"
)

// bandLocalPatchWarning is the run output's warning for every local_patch
// claim (BS Record, run output).
const bandLocalPatchWarning = "This patch was derived by an AI model from untrusted CI logs; read the whole patch file before running anything."

// localPatchPrep is the prep record of one flag-on diagnosis.
type localPatchPrep struct {
	Series          string    `json:"series"`
	EpisodeID       string    `json:"episode_id"`
	DiagnoseClaimID string    `json:"diagnose_claim_id"`
	LeaseUntil      time.Time `json:"lease_until"`
	ClaimID         string    `json:"claim_id,omitempty"`
	Key             string    `json:"key"`
	BaseSHA         string    `json:"base_sha,omitempty"`
	Code            string    `json:"code"`
}

// localPatchStage is one stage record; each phase sets its own fields.
type localPatchStage struct {
	ClaimID       string `json:"claim_id"`
	Phase         string `json:"phase"`
	Path          string `json:"path,omitempty"`
	StatusSHA256  string `json:"status_sha256,omitempty"`
	Code          string `json:"code,omitempty"`
	MessageSHA256 string `json:"message_sha256,omitempty"`
	DiffSHA256    string `json:"diff_sha256,omitempty"`
	Tree          string `json:"tree,omitempty"`
	CommitOID     string `json:"commit_oid,omitempty"`
	PatchSHA256   string `json:"patch_sha256,omitempty"`
}

// localPatchKept is one artifact a rule kept, with its reason.
type localPatchKept struct {
	Artifact string `json:"artifact"`
	Reason   string `json:"reason"`
}

// localPatchModel is one models[] entry (Provider Contract item 7).
type localPatchModel struct {
	Request         string `json:"request"`
	Requested       string `json:"requested"`
	Actual          string `json:"actual"`
	RefusalCategory string `json:"refusal_category"`
}

// localPatchResult is the write-once result record of one claim.
type localPatchResult struct {
	ClaimID          string                      `json:"claim_id"`
	Status           string                      `json:"status"`
	BSID             string                      `json:"bs_id,omitempty"`
	BaseSHA          string                      `json:"base_sha,omitempty"`
	CommitSHA        string                      `json:"commit_sha,omitempty"`
	Branch           string                      `json:"branch,omitempty"`
	WorktreePath     string                      `json:"worktree_path,omitempty"`
	PatchPath        string                      `json:"patch_path,omitempty"`
	PromptManifest   []promptlayer.ManifestEntry `json:"prompt_manifest,omitempty"`
	Recovered        bool                        `json:"recovered"`
	Kept             []localPatchKept            `json:"kept,omitempty"`
	Models           []localPatchModel           `json:"models,omitempty"`
	ModelSubstituted bool                        `json:"model_substituted"`
	Files            []healthband.PatchFile      `json:"files,omitempty"`
}

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

func newBandLocalPatchReport(result localPatchResult) bandLocalPatchReport {
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
// (.autopus/metrics/localpatch-events.jsonl, plan task T2) that the live
// flow writes. Each append runs under SPEC-SIGMABAND-001's store lock and
// waits at most StoreLockWait (a result: ResultLockWait).
type localPatchLedger interface {
	// AppendPrep re-reads the records under the store lock and appends prep
	// only when they hold no result for the claim of prep.Key (ClaimID, else
	// DiagnoseClaimID); appended is false when such a result exists.
	AppendPrep(ctx context.Context, prep localPatchPrep) (appended bool, err error)
	// AppendStage appends one stage record.
	AppendStage(ctx context.Context, stage localPatchStage) error
	// AppendResult appends the write-once result of result.ClaimID;
	// appended is false when the claim already has one.
	AppendResult(ctx context.Context, result localPatchResult) (appended bool, err error)
}

// localPatchCleaner applies the Cleanup Rules (Data Contracts, plan task
// T2), in the order 3, 1, 2, to the artifacts that the intent records of
// claimID name at their derived paths, and returns what a rule kept.
type localPatchCleaner interface {
	Cleanup(ctx context.Context, claimID string) ([]localPatchKept, error)
}

// localPatchGroups are the step-group deadlines of Step Timeouts and Lease.
type localPatchGroups struct {
	setup, diagnosisCleanup, patchRequest, applyCommit, branchPatch, cleanup, margin time.Duration
}

var defaultLocalPatchGroups = localPatchGroups{
	setup: 30 * time.Second, diagnosisCleanup: 30 * time.Second, patchRequest: healthband.ProviderTimeout,
	applyCommit: 60 * time.Second, branchPatch: 30 * time.Second, cleanup: 60 * time.Second,
	margin: healthband.ResultLockWait,
}

// bandLocalPatcher executes the Local Patch Flow of one flag-on band run.
type bandLocalPatcher struct {
	checkout      string // top level of the user's checkout, absolute
	harness       *config.HarnessConfig
	cache         *localPatchCache // <lp>; nil when it could not be resolved
	ledger        localPatchLedger
	cleaner       localPatchCleaner
	git           healthband.GitPolicyRunner // Dir is set per command
	provider      bandConfinedProvider
	defaultBranch string // the default branch 001's fetchCI resolved, or ""
	now           func() time.Time
	groups        localPatchGroups
	// Seams: the free space of <lp>, the Patch Policy's git runner (nil is
	// bandPolicyGit over git), the edit guard of Patch Policy item 9, and a
	// hook between apply_done and git commit.
	statfs       func(path string) (lpDiskSpace, error)
	policyGit    healthband.GitRunner
	decide       func(editguard.Call, editguard.Options) editguard.Decision
	beforeCommit func(worktree string)
	warn         io.Writer // cleanup faults; nil discards them
}

// newBandLocalPatcher returns the executor of a run; cache comes from
// resolveLocalPatchCache before the store lock.
func newBandLocalPatcher(checkout string, harness *config.HarnessConfig, cache *localPatchCache, ledger localPatchLedger, cleaner localPatchCleaner) *bandLocalPatcher {
	return &bandLocalPatcher{
		checkout: checkout, harness: harness, cache: cache, ledger: ledger, cleaner: cleaner,
		provider: newBandConfinedProvider(harness), now: time.Now, groups: defaultLocalPatchGroups, statfs: lpStatfs,
	}
}

// bandLocalPatchKey is the <key> of a claim: <series-slug>-<h8>-<episode-id>-<c8>,
// where <c8> is the first 8 hex digits of the claim id (the local_patch
// claim's, else the diagnose claim's).
func bandLocalPatchKey(series, episodeID, claimID string) string {
	return healthband.BandSlug(series) + "-" + healthband.H8(series) + "-" + healthband.BandSlug(episodeID) + "-" + claimID[:min(8, len(claimID))]
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
