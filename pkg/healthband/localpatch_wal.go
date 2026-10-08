package healthband

import (
	"time"

	"github.com/insajin/autopus-adk/pkg/promptlayer"
)

// Local Patch write-ahead log of SPEC-SIGMABAND-002 (REQ-02, REQ-11, REQ-12,
// REQ-14). This block is the stable API that the Local Patch Flow executor
// (plan task T7) and the SPEC-SIGMABAND-001 integration (T8) call.
//
// Files under .autopus/metrics/, written only under 001's store lock through
// its store helpers (appendStoreFile, writeStoreFileAtomic, readStoreLines,
// openStoreFile) and never read by a SPEC-SIGMABAND-001 binary:
//
//	LocalPatchEventsFile  localpatch-events.jsonl, one LocalPatchRecord per line
//	LocalPatchStateFile   localpatch-state.json, LocalPatchState (claims per series)
//	RecoveryLockFile      .recovery.lock, held only by the recovery step
//
// Record kinds (LocalPatchRecord.Kind; Schema and Seq are set at append):
//
//	decision  phase A, one per evaluated tier-3 event (Decision Table)
//	claim     phase A, one per row-5 decision (kind ClaimKindLocalPatch)
//	prep      end of Local Patch Flow step 1, at most one per flag-on diagnosis
//	stage     Stage* phases; every *_intent is appended before the command that
//	          creates its artifact, so no artifact exists without a record
//	result    the write-once end of a claim
//
// RecordClaimID addresses a claim's records: the ClaimID of a claim, stage,
// or result record, and of a prep record whose diagnosis has a local_patch
// claim, else the prep's DiagnoseClaimID. <key> ends in its first 8 hex.
//
// Phase A (T8), with <lp> resolved before store.Lock and every call below
// made under the store lock after wal.Commit and before Unlock:
//
//	loc, err := ResolveLocalPatchLocation(ctx, git, cacheDir)
//	log, err := locked.Store().ReadLocalPatchLog()
//	kept, err := CountKeptKeys(loc.Path)
//	d := &LocalPatchDecider{NoAgent: …, Checkpoint: <001 checkpoint before the plan>,
//		Log: log, Location: loc, Kept: kept, Owner: owner}
//	decision, ok := d.Decide(seriesPlan.Events, i) // every event; ok for tier 3
//	decision.Claim.LeaseUntil = <phase A time + chained budgets>   // row 5 only
//	appended, err := locked.AppendLocalPatch(records...) // decisions and claims
//
// Live claim (T7), Local Patch Flow steps 1-12:
//
//	dir, code, err := loc.Open(ctx, git)          // code: cache_unavailable
//	key := LocalPatchKey(series, episodeID, recordClaimID)
//	code, err = dir.CheckNoArtifacts(ctx, git, key) // artifact_exists
//	n, err := dir.KeptKeys()                      // n >= LocalPatchRetentionCap: cap_reached
//	// git_version_unsupported and the three codes above append their prep
//	// without the key lock; every later step holds it.
//	lock, err := dir.AcquireKeyLock(ctx, key)     // ErrLocalPatchKeyLocked: stop, no record
//	ok, err := store.AppendLocalPatchPrep(ctx, prep) // store-locked re-read first;
//	                                              // !ok: a result exists, stop, no record
//	err = store.AppendLocalPatchStage(ctx, stage) // error: failed:record_unavailable
//	sum, err := WorktreeStatusSHA256(ctx, git.In(paths.Worktree)) // worktree_done
//	kept := RemoveStoppedCheckout(ctx, git, dir, key) // checkout stopped or exit != 0
//	kept = CleanupLocalPatch(ctx, git, dir, records)  // every other failure: Rules 3, 1, 2
//	ok, err = store.AppendLocalPatchResult(ctx, result) // write-once; !ok: already ended
//	err = lock.Release()                          // unlink, then unlock
//
// Before each step group the claim checks LocalPatchLeaseCovers(now, lease,
// <group deadline>) and otherwise ends failed:lease_exhausted through
// CleanupLocalPatch; the Local Patch*Deadline constants are the Step Timeouts
// table, and LocalPatchDiagnoseBudget (990 s) and LocalPatchClaimBudget
// (810 s) are what T8's lease chain counts while the flag is true.
//
// Recovery (T8, after fetchCI and before phase A's store.Lock, never under
// --dry-run, whatever the flag): store.RecoverLocalPatches(ctx, opts). It
// takes RecoveryLockFile, ends every claim whose lease passed without a
// result by the Recovery State Table (re-reading through
// ReadLocalPatchLocked after the key lock), and reports recovery_locked and
// recovery_key_locked as run reasons.
//
// Stage values: status_sha256 is WorktreeStatusSHA256 (git status --porcelain
// -z --untracked-files=all --ignored, then git diff --no-ext-diff
// --no-textconv --binary); message_sha256 is the SHA-256 of the -F file
// bytes, which the commit object's message equals; diff_sha256 and
// patch_sha256 hash the diff and the canonical format-patch bytes. A stored
// path or branch is only compared with its derived value (LocalPatchPaths)
// and never used; a record's Branch is the short name autopus/band/<key>, and
// its paths are the clean absolute paths that LocalPatchPaths derives.

// Files, schemas, and the claim kind of the local patch flow.
const (
	LocalPatchEventsFile  = "localpatch-events.jsonl"
	LocalPatchStateFile   = "localpatch-state.json"
	RecoveryLockFile      = ".recovery.lock"
	SchemaLocalPatch      = "autopus.band_localpatch.v1"
	SchemaLocalPatchState = "autopus.band_localpatch_state.v1"
	ClaimKindLocalPatch   = "local_patch"
)

// Record kinds and decision values.
const (
	LocalPatchKindDecision = "decision"
	LocalPatchKindClaim    = "claim"
	LocalPatchKindPrep     = "prep"
	LocalPatchKindStage    = "stage"
	LocalPatchKindResult   = "result"
	LocalPatchDecideClaim  = "claim"
	LocalPatchDecideSkip   = "skipped"
)

// Stage phases in Local Patch Flow order.
const (
	StageWorktreeIntent = "worktree_intent"
	StageWorktreeDone   = "worktree_done"
	StageWorktreeFailed = "worktree_failed"
	StageMessage        = "message"
	StageApplyIntent    = "apply_intent"
	StageApplyDone      = "apply_done"
	StageCommitDone     = "commit_done"
	StageBranchIntent   = "branch_intent"
	StageBranchDone     = "branch_done"
	StagePatchIntent    = "patch_intent"
	StagePatchDone      = "patch_done"
)

// Codes this file set owns. A prep's code is LocalPatchCodeOK or a step-1
// code; a result's status is ClaimDone or ClaimFailedPrefix plus a code.
const (
	LocalPatchCodeOK                = "ok"
	LocalPatchCodeCacheUnavailable  = "cache_unavailable"
	LocalPatchCodeArtifactExists    = "artifact_exists"
	LocalPatchCodeCapReached        = "cap_reached"
	LocalPatchCodeRecordUnavailable = "record_unavailable"
	LocalPatchCodeRecordInvalid     = "record_invalid"
	LocalPatchCodeInterrupted       = "interrupted"
	LocalPatchCodeLeaseExhausted    = "lease_exhausted"
	LocalPatchCodeWorktreeFailed    = "worktree_failed"
)

// Decision Table skip reasons (rows 1-4, 6, 7).
const (
	LocalPatchSkippedNoAgent        = "local_patch_skipped:no_agent"
	LocalPatchSkippedSuperseded     = "local_patch_skipped:superseded_in_batch"
	LocalPatchSkippedAlreadyPatched = "local_patch_skipped:episode_already_patched"
	LocalPatchSkippedCapReached     = "local_patch_skipped:cap_reached"
	LocalPatchSkippedBSNotTier3     = "local_patch_skipped:bs_not_tier3"
	LocalPatchSkippedNoOpeningClaim = "local_patch_skipped:no_opening_claim"
)

// Kept artifacts and the reasons a Cleanup Rule keeps them (result kept[]).
const (
	ArtifactWorktree       = "worktree"
	ArtifactBranch         = "branch"
	ArtifactPatchFile      = "patch_file"
	ArtifactPatchTemp      = "patch_temp"
	ArtifactDiffFile       = "diff_file"
	KeptWorktreeIncomplete = "worktree_incomplete"
	KeptWorktreeModified   = "worktree_modified"
	KeptHeadUnrecognized   = "head_unrecognized"
	KeptGitConfigUnsafe    = "git_config_unsafe"
	KeptBranchMoved        = "branch_moved"
	KeptPatchModified      = "patch_modified"
)

// Run reasons of the recovery step (JSON envelope and text output).
const (
	ReasonRecoveryLocked    = "recovery_locked"
	ReasonRecoveryKeyLocked = "recovery_key_locked"
)

// Step Timeouts and Lease (REQ-12): step-group deadlines and the budgets
// that the lease chain counts while the flag is true.
const (
	LocalPatchSetupDeadline            = 30 * time.Second  // steps 1-2 up to worktree_done
	LocalPatchDiagnosisCleanupDeadline = 30 * time.Second  // diagnosis-only Cleanup Rule 3 and result
	LocalPatchRequestDeadline          = 600 * time.Second // steps 3-6
	LocalPatchApplyDeadline            = 60 * time.Second  // steps 7-9
	LocalPatchBranchDeadline           = 30 * time.Second  // steps 10-11
	LocalPatchCleanupDeadline          = 60 * time.Second  // Cleanup Rules or a stopped checkout's removal
	LocalPatchMarginDeadline           = 60 * time.Second  // the result record
	// LocalPatchDiagnoseBudget is DiagnoseBudget + 30 + 30 = 990 s.
	LocalPatchDiagnoseBudget = DiagnoseBudget + LocalPatchSetupDeadline + LocalPatchDiagnosisCleanupDeadline
	// LocalPatchClaimBudget is 600 + 60 + 30 + 60 + 60 = 810 s.
	LocalPatchClaimBudget = LocalPatchRequestDeadline + LocalPatchApplyDeadline + LocalPatchBranchDeadline +
		LocalPatchCleanupDeadline + LocalPatchMarginDeadline
	// LocalPatchGitTimeout bounds every git call of the Cleanup Rules.
	LocalPatchGitTimeout = 30 * time.Second
	// LocalPatchRecoveryBudget bounds the recovery of one claim.
	LocalPatchRecoveryBudget = 120 * time.Second
	// RecoveryLockWait bounds the wait for RecoveryLockFile.
	RecoveryLockWait = 5 * time.Second
)

// LocalPatchRetentionCap is the number of kept keys under <lp> at which a
// new claim is skipped and every other flag-on worktree refused.
const LocalPatchRetentionCap = 5

// LocalPatchLeaseCovers reports whether a claim whose lease ends at
// leaseUntil may start, at now, a step group with deadline group: the lease
// must cover the group plus the cleanup and margin deadlines, else the claim
// ends failed:lease_exhausted through the Cleanup Rules.
func LocalPatchLeaseCovers(now, leaseUntil time.Time, group time.Duration) bool {
	return !now.Add(group + LocalPatchCleanupDeadline + LocalPatchMarginDeadline).After(leaseUntil)
}

// LocalPatchRecord is one localpatch-events.jsonl line. Kind selects the
// fields that apply (Data Contracts); every other field stays empty.
type LocalPatchRecord struct {
	Schema string `json:"schema"`
	Seq    int64  `json:"seq"`
	Kind   string `json:"kind"`
	// decision, claim, and prep
	ClaimID         string    `json:"claim_id,omitempty"`
	Series          string    `json:"series,omitempty"`
	EpisodeID       string    `json:"episode_id,omitempty"`
	EvaluationSeq   int64     `json:"evaluation_seq,omitempty"`
	Decision        string    `json:"decision,omitempty"`
	Reason          string    `json:"reason,omitempty"`
	Owner           string    `json:"owner,omitempty"`
	LeaseUntil      time.Time `json:"lease_until,omitzero"`
	DependsOn       string    `json:"depends_on,omitempty"`
	DiagnoseClaimID string    `json:"diagnose_claim_id,omitempty"`
	Key             string    `json:"key,omitempty"`
	BaseSHA         string    `json:"base_sha,omitempty"`
	Code            string    `json:"code,omitempty"` // prep code, or the worktree_failed code
	// stage
	Phase         string `json:"phase,omitempty"`
	Path          string `json:"path,omitempty"`
	StatusSHA256  string `json:"status_sha256,omitempty"`
	MessageSHA256 string `json:"message_sha256,omitempty"`
	DiffSHA256    string `json:"diff_sha256,omitempty"`
	Tree          string `json:"tree,omitempty"`
	CommitOID     string `json:"commit_oid,omitempty"`
	PatchSHA256   string `json:"patch_sha256,omitempty"`
	// result (and the claim's derived paths)
	Status           string                      `json:"status,omitempty"`
	BSID             string                      `json:"bs_id,omitempty"`
	CommitSHA        string                      `json:"commit_sha,omitempty"`
	Branch           string                      `json:"branch,omitempty"`
	WorktreePath     string                      `json:"worktree_path,omitempty"`
	PatchPath        string                      `json:"patch_path,omitempty"`
	PromptManifest   []promptlayer.ManifestEntry `json:"prompt_manifest,omitempty"`
	Recovered        bool                        `json:"recovered,omitempty"`
	Kept             []LocalPatchKept            `json:"kept,omitempty"`
	Models           []LocalPatchModel           `json:"models,omitempty"`
	ModelSubstituted bool                        `json:"model_substituted,omitempty"`
	Files            []PatchFile                 `json:"files,omitempty"`
}

// LocalPatchKept is one artifact that a Cleanup Rule kept, with its reason.
type LocalPatchKept struct {
	Artifact string `json:"artifact"`
	Reason   string `json:"reason"`
}

// LocalPatchModel is one confined request's model record (Local Patch
// Provider Contract item 7): request is diagnosis or patch.
type LocalPatchModel struct {
	Request         string `json:"request"`
	Requested       string `json:"requested"`
	Actual          string `json:"actual"`
	RefusalCategory string `json:"refusal_category"`
}

// Model request kinds.
const (
	ModelRequestDiagnosis = "diagnosis"
	ModelRequestPatch     = "patch"
)

// RecordClaimID returns the claim id that addresses the record's claim.
func (r LocalPatchRecord) RecordClaimID() string {
	if r.Kind == LocalPatchKindPrep && r.ClaimID == "" {
		return r.DiagnoseClaimID
	}
	if r.Kind == LocalPatchKindDecision {
		return ""
	}
	return r.ClaimID
}

// NewLocalPatchPrep returns the prep record of a flag-on diagnosis: claim is
// its diagnose DueClaim, lpClaimID the local_patch claim id or "", baseSHA
// the resolved base or "", and code LocalPatchCodeOK or a step-1 code.
func NewLocalPatchPrep(claim DueClaim, lpClaimID, key, baseSHA, code string) LocalPatchRecord {
	return LocalPatchRecord{
		Kind: LocalPatchKindPrep, Series: claim.Series, EpisodeID: claim.EpisodeID, DiagnoseClaimID: claim.ID,
		LeaseUntil: claim.LeaseUntil, ClaimID: lpClaimID, Key: key, BaseSHA: baseSHA, Code: code,
	}
}

// NewLocalPatchStage returns a stage record of phase for a record claim id;
// the caller sets the phase's field (Data Contracts).
func NewLocalPatchStage(claimID, phase string) LocalPatchRecord {
	return LocalPatchRecord{Kind: LocalPatchKindStage, ClaimID: claimID, Phase: phase}
}

// NewLocalPatchResult returns a result record: status done for an empty
// code, else failed:<code>.
func NewLocalPatchResult(claimID, code string) LocalPatchRecord {
	status := ClaimDone
	if code != "" {
		status = ClaimFailedPrefix + code
	}
	return LocalPatchRecord{Kind: LocalPatchKindResult, ClaimID: claimID, Status: status}
}
