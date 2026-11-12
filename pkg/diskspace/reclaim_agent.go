package diskspace

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Reasons and skips of the wider worktree reclaim.
const (
	// ReasonMergedLinked selects a linked worktree outside the harness
	// directories whose HEAD is in the default remote branch, clean, idle,
	// and held by no process.
	ReasonMergedLinked = "merged_linked"
	// ReasonAgentDead selects a Claude Code agent worktree whose locking
	// process is gone, once it is clean and merged.
	ReasonAgentDead = "agent_dead"
	// ReasonAgentIdle selects a Claude Code agent worktree whose locking
	// process still runs, once it is clean, merged, idle, and held by no
	// process.
	ReasonAgentIdle = "agent_idle"

	// SkipCwdUnknown keeps a worktree whose rule needs the process working
	// directories when they could not be listed.
	SkipCwdUnknown = "cwd_unknown"
)

// agentLock is the lock reason Claude Code writes on an agent worktree:
// "claude agent agent-<id> (pid <n> start <time>)".
var agentLock = regexp.MustCompile(`^claude agent agent-[0-9A-Za-z_-]+ \(pid ([0-9]+) start ([^)]*)\)$`)

// parseAgentLock returns the pid and start time of an agent lock reason.
func parseAgentLock(reason string) (pid int, start string, ok bool) {
	m := agentLock.FindStringSubmatch(strings.TrimSpace(reason))
	if m == nil {
		return 0, "", false
	}
	pid, err := strconv.Atoi(m[1])
	if err != nil || pid <= 0 {
		return 0, "", false
	}
	return pid, m[2], true
}

// AgentAliveFunc reports whether the process that locked an agent worktree
// still runs.
type AgentAliveFunc func(ctx context.Context, pid int, start string) bool

// SystemAgentAlive treats a pid as the locking process while it exists and,
// when ps can read its start time, that time matches the lock's: a reused
// pid with another start time is not the agent. Anything unreadable counts as
// alive, the side that keeps the worktree.
func SystemAgentAlive(run Runner) AgentAliveFunc {
	return func(ctx context.Context, pid int, start string) bool {
		if !processAlive(pid) {
			return false
		}
		out, err := run(ctx, "", []string{"LC_ALL=C"}, "ps", "-o", "lstart=", "-p", strconv.Itoa(pid))
		got := strings.Join(strings.Fields(string(out)), " ")
		if err != nil || got == "" {
			return true
		}
		return got == strings.Join(strings.Fields(start), " ")
	}
}

// wideVerdict is the selection of the wider rules: a locked Claude Code
// agent worktree, or a linked worktree outside the harness directories. It
// returns the reason, whether the worktree must be unlocked first, and the
// skip that keeps it. handled false leaves the worktree to worktreeVerdict.
func (in WorktreeReclaimInput) wideVerdict(ctx context.Context, item Worktree, wide bool, path, cwd string, activity time.Time,
	cwds []string, cwdsKnown bool) (reason string, unlock bool, skip string, handled bool) {
	if cwdsKnown && holdsCwd(path, cwds) {
		return "", false, SkipCwd, true
	}
	pid, start, agent := parseAgentLock(item.LockReason)
	if !wide && !(item.Locked && agent) {
		return "", false, "", false
	}
	switch {
	case item.Locked && !agent:
		return "", false, SkipLocked, true
	case item.Prunable:
		return "", false, SkipPrunable, true
	case item.Clean == Unknown || item.Merged == Unknown:
		return "", false, SkipUnknown, true
	case item.Clean == False:
		return "", false, SkipDirty, true
	case item.Merged == False:
		return "", false, SkipUnmerged, true
	case cwd == path || strings.HasPrefix(cwd, path+"/"):
		return "", false, SkipCwd, true
	}
	if wide {
		switch ignoredFiles(ctx, in.Run, path) {
		case Unknown:
			return "", false, SkipUnknown, true
		case True:
			return "", false, SkipIgnored, true
		}
	}
	if item.Locked && !in.alive(ctx, pid, start) {
		return ReasonAgentDead, true, "", true
	}
	switch {
	case !cwdsKnown:
		return "", false, SkipCwdUnknown, true
	case !isIdle(in.Now, activity, in.IdleMinutes):
		return "", false, SkipRecent, true
	case item.Locked:
		return ReasonAgentIdle, true, "", true
	}
	return ReasonMergedLinked, false, "", true
}

func (in WorktreeReclaimInput) alive(ctx context.Context, pid int, start string) bool {
	if in.Alive != nil {
		return in.Alive(ctx, pid, start)
	}
	return processAlive(pid)
}

// remoteBase reports whether a merge-base ladder name is the default remote
// branch, the only base the wider linked-worktree rule trusts.
func remoteBase(base string) bool { return base == "origin/HEAD" || base == "origin/main" }

// lockStill reports whether the main worktree still lists path locked with
// reason.
func lockStill(ctx context.Context, run Runner, mainPath, path, reason string) bool {
	out, err := git(ctx, run, mainPath, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return false
	}
	for _, item := range parseWorktreePorcelain(out) {
		if canonicalOrSelf(item.Path) == path {
			return item.Locked && item.LockReason == reason
		}
	}
	return false
}

// removeUnlocked unlocks an agent worktree and removes it without --force,
// locking it again with the same reason when git refuses the removal.
func removeUnlocked(ctx context.Context, run Runner, c ReclaimCandidate) error {
	if _, err := git(ctx, run, c.MainPath, "worktree", "unlock", c.Path); err != nil {
		return err
	}
	_, err := git(ctx, run, c.MainPath, "worktree", "remove", c.Path)
	if err != nil {
		_, _ = git(ctx, run, c.MainPath, "worktree", "lock", "--reason", c.LockReason, c.Path)
	}
	return err
}

// removeCandidate removes one selected worktree without --force.
func removeCandidate(ctx context.Context, run Runner, c ReclaimCandidate) error {
	if c.Unlock {
		return removeUnlocked(ctx, run, c)
	}
	_, err := git(ctx, run, c.MainPath, "worktree", "remove", c.Path)
	return err
}

// recheckWide re-reads what the wider rules selected on: a worktree outside
// the harness directories must still hold no ignored files beyond build
// output, no process may have entered the worktree since the scan, and an agent worktree must still
// carry the same lock, whose process must still be gone for agent_dead.
func recheckWide(ctx context.Context, run Runner, c ReclaimCandidate) outcome {
	if c.guardIgnored && ignoredFiles(ctx, run, c.Path) != False {
		return outcomeChanged
	}
	needsCwds := c.Reason == ReasonMergedLinked || c.Reason == ReasonAgentIdle
	if c.cwds != nil {
		listed, err := c.cwds(ctx)
		switch {
		case err != nil && needsCwds:
			return outcomeChanged
		case err == nil && holdsCwd(c.Path, canonicalCwds(listed)):
			return outcomeChanged
		}
	} else if needsCwds {
		return outcomeChanged
	}
	if !c.Unlock {
		return outcomeDeleted
	}
	if !lockStill(ctx, run, c.MainPath, c.Path, c.LockReason) {
		return outcomeChanged
	}
	in := WorktreeReclaimInput{Alive: c.alive}
	if c.Reason == ReasonAgentDead && in.alive(ctx, c.agentPID, c.agentStart) {
		return outcomeChanged
	}
	return outcomeDeleted
}
