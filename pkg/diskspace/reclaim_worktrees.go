package diskspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// WorktreeReclaimInput is what a worktree reclaim scan reads.
type WorktreeReclaimInput struct {
	Run         Runner
	ProjectDir  string
	Cwd         string
	Now         time.Time
	IdleMinutes int
	UID         int
	Ledger      []LedgerEntry
	// Project, when set, is a scan of the project's repository already
	// taken, so status does not walk every worktree twice.
	Project *WorktreeScan
	// Cwds lists process working directories; nil keeps every worktree a
	// rule needs them for. Alive tells a live agent lock from a dead one;
	// nil checks only that the pid exists.
	Cwds  CwdLister
	Alive AgentAliveFunc
}

// WorktreeReclaimScan is what a worktree reclaim scan selected. Owned names
// every owned linked worktree it saw, selected or not, for status marking.
type WorktreeReclaimScan struct {
	Candidates []ReclaimCandidate
	Skipped    map[string]int
	Owned      map[string]bool
	// Idle are the kept linked worktrees, owned by the uid, idle beyond the
	// threshold, and held by no listed process: where regenerable build
	// output may be reclaimed.
	Idle []IdleWorktree
}

// ScanWorktreeReclaim selects owned worktrees (REQ-DISK2-RECLAIM-02) in the
// repository holding the project directory and in every repository a live
// ledger entry names. A worktree that is neither a harness worktree nor
// registered is only counted as not_owned.
func ScanWorktreeReclaim(ctx context.Context, in WorktreeReclaimInput) WorktreeReclaimScan {
	out := WorktreeReclaimScan{Skipped: map[string]int{}, Owned: map[string]bool{}}
	registered := map[string]LedgerEntry{}
	dirs := []string{in.ProjectDir}
	for _, entry := range in.Ledger {
		if entry.Kind == LedgerKindWorktree && entry.Live() {
			registered[canonicalOrSelf(entry.Path)] = entry
			dirs = append(dirs, entry.Path)
		}
	}
	cwd := canonicalOrSelf(in.Cwd)
	var cwds []string
	cwdsKnown := false
	if in.Cwds != nil {
		if listed, err := in.Cwds(ctx); err == nil {
			cwds, cwdsKnown = canonicalCwds(listed), true
		}
	}
	seen := map[string]bool{}
	for i, dir := range dirs {
		var scan WorktreeScan
		if i == 0 && in.Project != nil {
			scan = *in.Project
		} else {
			scan = ScanWorktrees(ctx, in.Run, dir)
		}
		if !scan.Root.Available() || seen[canonicalOrSelf(scan.Root.Path)] {
			continue
		}
		seen[canonicalOrSelf(scan.Root.Path)] = true
		for _, item := range scan.Linked() {
			path := canonicalOrSelf(item.Path)
			entry, isRegistered := registered[path]
			wide := !item.Harness && !isRegistered
			if wide && (item.Merged != True || !remoteBase(scan.Base)) {
				out.Skipped[SkipNotOwned]++
				in.noteIdle(&out, path, cwd, cwds, cwdsKnown)
				continue
			}
			if !wide {
				out.Owned[path] = true
			}
			if owner := pathOwner(path); owner >= 0 && owner != in.UID {
				out.Skipped[SkipOwner]++
				continue
			}
			activity, crossed := worktreeActivity(path)
			if crossed {
				out.Skipped[SkipDevice]++
				continue
			}
			reason, unlock, skip, handled := in.wideVerdict(ctx, item, wide, path, cwd, activity, cwds, cwdsKnown)
			if !handled {
				reason, skip = worktreeVerdict(item, entry, isRegistered, path, cwd, in.Now, in.IdleMinutes, activity)
			}
			if skip != "" {
				out.Skipped[skip]++
				in.noteIdleAt(&out, path, cwd, cwds, cwdsKnown, activity)
				continue
			}
			c := ReclaimCandidate{
				Kind: ReclaimWorktree, Path: path, Reason: reason, Bytes: item.Bytes,
				IdleMinutes: int64(in.Now.Sub(activity) / time.Minute),
				MainPath:    scan.Root.Path, ID: pathID(path), Head: item.Head, BaseRef: baseRefOf(scan.Base),
				Cutoff: in.Now.Add(-time.Duration(in.IdleMinutes) * time.Minute),
				Unlock: unlock, LockReason: item.LockReason, cwds: in.Cwds, alive: in.Alive,
			}
			c.agentPID, c.agentStart, _ = parseAgentLock(item.LockReason)
			out.Candidates = append(out.Candidates, c)
		}
	}
	return out
}

// worktreeVerdict returns the selection reason of an owned worktree, or the
// skip that keeps it.
func worktreeVerdict(item Worktree, entry LedgerEntry, registered bool, path, cwd string, now time.Time, idleMinutes int, activity time.Time) (string, string) {
	switch {
	case item.Locked:
		return "", SkipLocked
	case item.Prunable:
		return "", SkipPrunable
	case item.Clean == Unknown:
		return "", SkipUnknown
	case item.Clean == False:
		return "", SkipDirty
	case cwd == path || strings.HasPrefix(cwd, path+string(filepath.Separator)):
		return "", SkipCwd
	case registered && entry.Released():
		// The branch survives removal, so a released worktree may hold
		// unmerged commits; a detached one would leave them unreachable.
		if item.Branch == "" && item.Merged != True {
			return "", SkipDetached
		}
		return ReasonReleased, ""
	case item.Merged == Unknown:
		return "", SkipUnknown
	case item.Merged == False:
		return "", SkipUnmerged
	case !isIdle(now, activity, idleMinutes):
		return "", SkipRecent
	}
	return ReasonMergedIdle, ""
}

// ApplyWorktrees removes each candidate with `git worktree remove` without
// --force after re-reading, right before removal, everything that selected
// it (REQ-DISK2-RECLAIM-03): identity, owner, HEAD, clean state, and, by
// reason, the merge into the base and the idle cutoff or the live released
// ledger entry. Anything that no longer holds is skipped_changed. Git
// refuses a dirty or locked worktree on its own; the branch is kept. A
// removed worktree's ledger entry is dropped.
func ApplyWorktrees(ctx context.Context, run Runner, candidates []ReclaimCandidate, state *StateDir, uid int, beforeRemove func(path string)) ReclaimCounters {
	var counters ReclaimCounters
	for _, c := range candidates {
		if beforeRemove != nil {
			beforeRemove(c.Path)
		}
		if o := recheckWorktree(ctx, run, c, state, uid); o != outcomeDeleted {
			counters.record(ReclaimWorktree, 0, o)
			continue
		}
		if err := removeCandidate(ctx, run, c); err != nil {
			counters.record(ReclaimWorktree, 0, outcomeFailed)
			counters.Failures = append(counters.Failures,
				fmt.Sprintf("worktree %s: git worktree remove exited %d", c.Path, exitCode(err)))
			continue
		}
		counters.record(ReclaimWorktree, c.Bytes, outcomeDeleted)
		if state != nil {
			_ = ForgetWorktree(ctx, *state, c.Path)
		}
	}
	return counters
}

// recheckWorktree returns outcomeDeleted when the candidate still qualifies.
func recheckWorktree(ctx context.Context, run Runner, c ReclaimCandidate, state *StateDir, uid int) outcome {
	info, err := os.Lstat(c.Path)
	switch {
	case err != nil && os.IsNotExist(err):
		return outcomeVanished
	case err != nil || !info.IsDir() || statOf(info).id != c.ID:
		return outcomeChanged
	case statOf(info).uid >= 0 && statOf(info).uid != uid:
		return outcomeOwner
	}
	head, err := git(ctx, run, c.Path, "rev-parse", "HEAD")
	if err != nil || strings.TrimSpace(string(head)) != c.Head {
		return outcomeChanged
	}
	status, err := git(ctx, run, c.Path, "status", "--porcelain=v1", "-z", "--untracked-files=normal")
	if err != nil || len(status) != 0 {
		return outcomeChanged
	}
	activity, crossed := worktreeActivity(c.Path)
	if crossed {
		return outcomeDevice
	}
	if o := recheckWide(ctx, run, c); o != outcomeDeleted {
		return o
	}
	switch c.Reason {
	case ReasonAgentDead:
		if c.BaseRef == "" {
			return outcomeChanged
		}
		if _, err := git(ctx, run, c.Path, "merge-base", "--is-ancestor", "HEAD", c.BaseRef); err != nil {
			return outcomeChanged
		}
	case ReasonMergedIdle, ReasonMergedLinked, ReasonAgentIdle:
		if c.BaseRef == "" || activity.After(c.Cutoff) {
			return outcomeChanged
		}
		if _, err := git(ctx, run, c.Path, "merge-base", "--is-ancestor", "HEAD", c.BaseRef); err != nil {
			return outcomeChanged
		}
	case ReasonReleased:
		if !stillReleased(state, c) {
			return outcomeChanged
		}
		// The branch is what keeps the commits reachable once the worktree's
		// own reflog is gone: still on a branch, or HEAD merged (review r2).
		_, onBranch := git(ctx, run, c.Path, "symbolic-ref", "-q", "HEAD")
		if onBranch != nil {
			if c.BaseRef == "" {
				return outcomeChanged
			}
			if _, err := git(ctx, run, c.Path, "merge-base", "--is-ancestor", "HEAD", c.BaseRef); err != nil {
				return outcomeChanged
			}
		}
	default:
		return outcomeFailed
	}
	return outcomeDeleted
}

// stillReleased re-reads the ledger: the entry must still be live, released,
// and name the same directory.
func stillReleased(state *StateDir, c ReclaimCandidate) bool {
	if state == nil {
		return false
	}
	entries, err := LoadLedger(*state)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if canonicalOrSelf(e.Path) == c.Path && e.Released() && e.Live() && (FileID{Dev: e.Dev, Ino: e.Ino}) == c.ID {
			return true
		}
	}
	return false
}

// IsOwned reports whether path is an owned worktree the scan saw.
func (s WorktreeReclaimScan) IsOwned(path string) bool { return s.Owned[canonicalOrSelf(path)] }
