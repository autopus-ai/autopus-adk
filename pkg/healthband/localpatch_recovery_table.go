package healthband

import (
	"context"
	"path/filepath"
	"time"
)

// lpFacts is what one claim's records say about its artifacts.
type lpFacts struct {
	claimID, key, series, episode, diagnoseID  string
	baseSHA, statusSHA, failedCode, messageSHA string
	diffSHA, expectedTree, commitOID, patchSHA string
	lease                                      time.Time
	has                                        map[string]bool // stage phases present
	last                                       string          // last stage phase; "" without one
	result                                     bool
	// Every key, series, episode, path, and branch that a record stores.
	keys, seriesSeen, episodesSeen []string
	worktrees, patches, branches   []string
}

// lpFactsOf folds a claim's records (one RecordClaimID) in seq order. The
// claim record's lease wins over the prep's, which is the diagnose claim's.
func lpFactsOf(records []LocalPatchRecord) lpFacts {
	facts := lpFacts{has: make(map[string]bool)}
	var claimLease, prepLease time.Time
	for _, record := range records {
		if facts.claimID == "" {
			facts.claimID = record.RecordClaimID()
		}
		switch record.Kind {
		case LocalPatchKindClaim:
			claimLease = record.LeaseUntil
			if facts.diagnoseID == "" {
				facts.diagnoseID = record.DependsOn
			}
			facts.note(record)
			facts.worktrees = append(facts.worktrees, record.WorktreePath)
			facts.patches = append(facts.patches, record.PatchPath)
			facts.branches = append(facts.branches, record.Branch)
		case LocalPatchKindPrep:
			prepLease, facts.diagnoseID = record.LeaseUntil, record.DiagnoseClaimID
			facts.baseSHA = record.BaseSHA
			facts.note(record)
		case LocalPatchKindStage:
			facts.stage(record)
		case LocalPatchKindResult:
			facts.result = true
		}
	}
	facts.lease = prepLease
	if !claimLease.IsZero() {
		facts.lease = claimLease
	}
	if len(facts.keys) > 0 {
		facts.key, facts.series, facts.episode = facts.keys[0], facts.seriesSeen[0], facts.episodesSeen[0]
	}
	return facts
}

func (f *lpFacts) note(record LocalPatchRecord) {
	f.keys = append(f.keys, record.Key)
	f.seriesSeen = append(f.seriesSeen, record.Series)
	f.episodesSeen = append(f.episodesSeen, record.EpisodeID)
}

func (f *lpFacts) stage(record LocalPatchRecord) {
	f.has[record.Phase], f.last = true, record.Phase
	switch record.Phase {
	case StageWorktreeIntent:
		f.worktrees = append(f.worktrees, record.Path)
	case StageWorktreeDone:
		f.statusSHA = record.StatusSHA256
	case StageWorktreeFailed:
		f.failedCode = record.Code
	case StageMessage:
		f.messageSHA = record.MessageSHA256
	case StageApplyIntent:
		f.diffSHA, f.expectedTree = record.DiffSHA256, record.Tree
	case StageCommitDone, StageBranchIntent:
		f.commitOID = record.CommitOID
	case StagePatchIntent:
		f.patches = append(f.patches, record.Path)
		f.patchSHA = record.PatchSHA256
	case StagePatchDone:
		if f.patchSHA == "" {
			f.patchSHA = record.PatchSHA256
		}
	}
}

// derived is the Derived paths check (CD-3 L3): the key matches
// ^[a-z0-9-]+$, ends in -<c8> of the record claim id, and equals the <key>
// rule of the record's series and episode; every record agrees on key,
// series, and episode; and every stored path and branch equals its value
// derived from loc. Any mismatch ends the claim failed:record_invalid.
func (f lpFacts) derived(loc LocalPatchLocation) bool {
	if len(f.keys) == 0 || !localPatchKeyPattern.MatchString(f.key) || len(f.claimID) < 8 ||
		f.key != LocalPatchKey(f.series, f.episode, f.claimID) {
		return false
	}
	for i := range f.keys {
		if f.keys[i] != f.key || f.seriesSeen[i] != f.series || f.episodesSeen[i] != f.episode {
			return false
		}
	}
	paths := loc.Paths(f.key)
	for _, stored := range f.worktrees {
		if filepath.Clean(stored) != paths.Worktree {
			return false
		}
	}
	for _, stored := range f.patches {
		if filepath.Clean(stored) != paths.Patch {
			return false
		}
	}
	for _, stored := range f.branches {
		if stored != paths.Branch {
			return false
		}
	}
	return true
}

// settle is the Recovery State Table for a claim whose fresh records passed
// the Derived paths check, by its last durable record:
//
//	decision, claim, or prep   no artifact can exist      failed:interrupted
//	worktree_failed            Cleanup Rules              failed:<its code>
//	patch_intent, complete     delete the diff file       done, recovered
//	patch_done                 keep, list what differs    done, recovered
//	any other stage            Cleanup Rules 3, 1, 2      failed:interrupted
//
// A complete patch_intent set is a patch file whose SHA-256 equals the
// patch_intent hash, the branch and the worktree HEAD at the claim commit,
// and a clean worktree. Every rule keeps and lists an artifact that a user
// changed, whatever the row.
func (r *lpRecovery) settle(ctx context.Context, facts lpFacts) LocalPatchRecord {
	c := newLPCleaner(r.opts.Git, r.dir, facts, durationOr(r.opts.GitTimeout, LocalPatchGitTimeout))
	code := LocalPatchCodeInterrupted
	switch {
	case facts.last == "":
	case facts.last == StageWorktreeFailed:
		c.rules(ctx)
		code = facts.failedCode
	case facts.last == StagePatchIntent && c.complete(ctx):
		c.removeIfHash(filepath.Base(c.paths.Diff), facts.diffSHA, ArtifactDiffFile)
		code = ""
	case facts.last == StagePatchDone:
		c.patchDone(ctx)
		code = ""
	default:
		c.rules(ctx)
	}
	result := NewLocalPatchResult(facts.claimID, code)
	result.Kept, result.BaseSHA = c.kept, facts.baseSHA
	result.Branch, result.WorktreePath, result.PatchPath = c.paths.Branch, c.paths.Worktree, c.paths.Patch
	if code == "" {
		result.CommitSHA, result.BSID = c.facts.commitOID, r.bsID(facts.diagnoseID)
	}
	return result
}

// bsID returns the BS ID that SPEC-SIGMABAND-001 recorded for the diagnose
// claim's result, if any.
func (r *lpRecovery) bsID(diagnoseID string) string {
	events, _, err := readEvents(r.store.Path(EventsFile))
	if err != nil {
		return ""
	}
	for _, event := range events {
		if event.Kind == EventKindActionResult && event.ClaimID == diagnoseID && bsIDPattern.MatchString(event.BSID) {
			return event.BSID
		}
	}
	return ""
}
