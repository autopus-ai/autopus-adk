package cli

import (
	"context"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/diskspace"
)

// Seams of `auto disk reclaim`: the working directory a candidate must not
// hold, and the moments between the scan and each removal.
var (
	diskGetwd                 = os.Getwd
	diskReclaimBeforeWorktree func(path string)
	diskReclaimBeforeScratch  func(rule, rel string)
)

// diskReclaimData is the JSON data of `auto disk reclaim` and the
// reclaim_preview of `auto disk status` (REQ-DISK2-RECLAIM-01).
type diskReclaimData struct {
	Run         string          `json:"run"`
	Worktrees   diskReclaimKind `json:"worktrees"`
	Scratchpads diskReclaimKind `json:"scratchpads"`
	// Regenerable and LargeFiles are the build output in idle worktrees and
	// the large scratchpad files; large files count toward TotalBytes only
	// with --include-large-files.
	Regenerable diskReclaimKind            `json:"regenerable"`
	LargeFiles  diskReclaimKind            `json:"large_files"`
	ScratchRoot diskRootData               `json:"scratchpad_root"`
	Refused     *diskRefusal               `json:"refused"`
	TotalBytes  int64                      `json:"total_bytes"`
	Result      *diskspace.ReclaimCounters `json:"result,omitempty"`
}

type diskReclaimKind struct {
	IdleMinutes int                          `json:"idle_minutes"`
	Candidates  []diskspace.ReclaimCandidate `json:"candidates"`
	Bytes       int64                        `json:"bytes"`
	Skipped     map[string]int               `json:"skipped"`
}

// reclaimPlan is one reclaim scan: what apply acts on and status previews.
type reclaimPlan struct {
	worktrees diskspace.WorktreeReclaimScan
	scratch   diskspace.ScratchScan
	regen     diskspace.RegenScan
	large     []diskspace.ReclaimCandidate
	state     *diskspace.StateDir
	uid       int
	// includeLarge lets apply remove large files; auto keeps only what the
	// automatic reclaim may remove.
	includeLarge bool
}

// reclaimOptions are the flags of `auto disk reclaim`.
type reclaimOptions struct {
	apply        bool
	includeLarge bool
	auto         bool
}

const diskReclaimLong = "Removes finished work, as a dry run unless --apply is given.\n\n" +
	"Worktrees: an owned linked worktree (under .autopus/worktrees, .claude/worktrees, or .worktrees, or registered with " +
	"`auto disk register`) that is clean, unlocked, does not hold the current directory, and is either released or merged " +
	"and idle beyond disk.reclaim.worktree_idle_minutes. Any other linked worktree qualifies when its HEAD is in the " +
	"default remote branch (origin/HEAD or origin/main), it is clean with no untracked files, idle, and no running process " +
	"has its working directory inside it. A Claude Code agent worktree, locked as `claude agent agent-<id> (pid N start T)`, " +
	"is unlocked and removed when it is clean and merged and either its locking process is gone or, while it runs, the " +
	"worktree is idle and holds no process. Git removes each without --force; the branch is kept.\n\n" +
	"Regenerable: node_modules, .next, dist (beside a package.json) and Cargo target directories that git ignores, inside " +
	"linked worktrees idle beyond the same threshold.\n\n" +
	"Scratchpads: a Claude Code session directory <slug>/<uuid> under the scratchpad root whose newest entry is idle " +
	"beyond disk.reclaim.scratchpad_idle_minutes. Symlinks inside are unlinked, never followed.\n\n" +
	"Large files: files of 1 GiB or more under the scratchpad root, unmodified for disk.reclaim.large_file_idle_minutes, " +
	"are listed; they may be backups, so only --include-large-files removes them.\n\n" +
	"Exit status: 0 done, 1 a refused root, a failed removal, or a usage, configuration, or measurement failure."

func newDiskReclaimCmd() *cobra.Command {
	var flags diskFlags
	var opts reclaimOptions
	cmd := &cobra.Command{
		Use:           "reclaim [--apply] [--include-large-files]",
		Short:         "Remove finished worktrees, idle build output, and idle session scratchpads (dry run by default)",
		Long:          diskReclaimLong,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDiskReclaim(cmd, flags, opts)
		},
	}
	flags.register(cmd)
	cmd.Flags().BoolVar(&opts.apply, "apply", false, "remove the candidates instead of listing what would go")
	cmd.Flags().BoolVar(&opts.includeLarge, "include-large-files", false, "also remove the listed large scratchpad files (they may be backups)")
	cmd.Flags().BoolVar(&opts.auto, "auto", false, "only merged, clean worktrees and idle build output, as the Stop hook runs it")
	_ = cmd.Flags().MarkHidden("auto")
	return cmd
}

func runDiskReclaim(cmd *cobra.Command, flags diskFlags, opts reclaimOptions) error {
	jsonMode, err := resolveJSONMode(flags.jsonOut, flags.format)
	if err != nil {
		return err
	}
	dc, err := loadDiskContext(cmd, flags, jsonMode, "reclaim")
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	if opts.auto {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, autoReclaimBudget)
		defer cancel()
	}
	plan := planReclaim(ctx, dc)
	plan.includeLarge = opts.includeLarge && !opts.auto
	if opts.auto {
		plan = plan.automatic()
	}
	data := plan.data(dc, "dry-run")
	if opts.apply {
		data.Run = "apply"
		result := plan.apply(ctx)
		data.Result = &result
		event := diskspace.Event{Kind: diskspace.EventReclaimApply, FreeBytes: dc.space.AvailBytes,
			ReclaimedBytes: result.DeletedBytes, Count: result.Worktrees + result.Scratchpads + result.Regenerable + result.LargeFiles}
		if opts.auto {
			event.Kind, event.Message = diskspace.EventAutoReclaim, plan.summary()
		}
		recordDiskEvent(event)
	}
	if !jsonMode {
		writeReclaimText(cmd.OutOrStdout(), data)
	}
	switch {
	case data.Refused != nil && opts.apply && !opts.auto:
		return diskFail(cmd, jsonMode, 1, "disk_reclaim_refused", "disk reclaim: refused: "+data.Refused.Message, data)
	case data.Result != nil && data.Result.Failed > 0:
		return diskFail(cmd, jsonMode, 1, "disk_reclaim_failed", strings.Join(data.Result.Failures, "; "), data)
	case jsonMode:
		return writeJSONResult(cmd, jsonStatusOK, data, nil, nil)
	}
	return nil
}

// planReclaim scans every kind. A state dir or ledger that cannot be read
// leaves only the harness worktrees owned; it never stops the scan.
func planReclaim(ctx context.Context, dc diskContext, project ...diskspace.WorktreeScan) reclaimPlan {
	plan := reclaimPlan{uid: diskUID()}
	var ledger []diskspace.LedgerEntry
	if state, err := openDiskState(); err == nil {
		plan.state = &state
		ledger, _ = diskspace.LoadLedger(state)
	}
	cwd, _ := diskGetwd()
	now := diskNow()
	input := diskspace.WorktreeReclaimInput{Run: diskRunner, ProjectDir: dc.projectDir,
		Cwd: cwd, Now: now, IdleMinutes: dc.disk.Reclaim.WorktreeIdleMinutes, UID: plan.uid, Ledger: ledger,
		Cwds: diskCwdLister(), Alive: diskAgentAlive()}
	if len(project) > 0 {
		input.Project = &project[0]
	}
	plan.worktrees = diskspace.ScanWorktreeReclaim(ctx, input)
	plan.regen = diskspace.ScanRegenerable(ctx, diskRunner, plan.worktrees.Idle, now, plan.uid)
	root := diskspace.ResolveScratchRoot(diskScratchpad(plan.uid), plan.uid)
	plan.scratch = diskspace.ScanSessions(root, now, dc.disk.Reclaim.ScratchpadIdleMinutes, plan.uid, cwd, diskspace.ScratchTree)
	plan.large = diskspace.ScanLargeFiles(root, now, dc.disk.Reclaim.LargeFileIdleMinutes, plan.uid)
	return plan
}

// automatic keeps what the automatic reclaim may remove: worktrees selected
// as merged and clean, and idle build output. Released worktrees, which may
// hold unmerged commits, scratchpads, and large files stay for a person.
func (p reclaimPlan) automatic() reclaimPlan {
	var kept []diskspace.ReclaimCandidate
	for _, c := range p.worktrees.Candidates {
		if c.Reason != diskspace.ReasonReleased {
			kept = append(kept, c)
		}
	}
	p.worktrees.Candidates = kept
	p.scratch.Candidates, p.large, p.includeLarge = nil, nil, false
	return p
}

func (p reclaimPlan) apply(ctx context.Context) diskspace.ReclaimCounters {
	result := diskspace.ApplyWorktrees(ctx, diskRunner, p.worktrees.Candidates, p.state, p.uid, diskReclaimBeforeWorktree)
	scratch, err := diskspace.ApplyScratch(p.scratch, p.uid, diskReclaimBeforeScratch)
	if err != nil {
		scratch.Failed += len(p.scratch.Candidates)
		scratch.Failures = append(scratch.Failures, err.Error())
	}
	result.Add(scratch)
	result.Add(diskspace.ApplyParented(p.regen.Candidates, p.uid))
	if p.includeLarge {
		result.Add(diskspace.ApplyParented(p.large, p.uid))
	}
	return result
}

func (p reclaimPlan) data(dc diskContext, run string) diskReclaimData {
	kind := func(cands []diskspace.ReclaimCandidate, skipped map[string]int, idle int) diskReclaimKind {
		k := diskReclaimKind{IdleMinutes: idle, Candidates: cands, Skipped: skipped}
		if k.Candidates == nil {
			k.Candidates = []diskspace.ReclaimCandidate{}
		}
		if k.Skipped == nil {
			k.Skipped = map[string]int{}
		}
		for _, c := range cands {
			k.Bytes += c.Bytes
		}
		return k
	}
	r := dc.disk.Reclaim
	d := diskReclaimData{Run: run,
		Worktrees:   kind(p.worktrees.Candidates, p.worktrees.Skipped, r.WorktreeIdleMinutes),
		Scratchpads: kind(p.scratch.Candidates, p.scratch.Skipped, r.ScratchpadIdleMinutes),
		Regenerable: kind(p.regen.Candidates, p.regen.Skipped, r.WorktreeIdleMinutes),
		LargeFiles:  kind(p.large, nil, r.LargeFileIdleMinutes),
		ScratchRoot: rootData(p.scratch.Root)}
	if p.scratch.Root.Refused {
		d.Refused = refusalData(p.scratch.Root)
	}
	d.TotalBytes = d.Worktrees.Bytes + d.Scratchpads.Bytes + d.Regenerable.Bytes
	if p.includeLarge {
		d.TotalBytes += d.LargeFiles.Bytes
	}
	return d
}

// summary names up to five removal candidates for the event log.
func (p reclaimPlan) summary() string {
	var paths []string
	for _, c := range append(append([]diskspace.ReclaimCandidate{}, p.worktrees.Candidates...), p.regen.Candidates...) {
		paths = append(paths, c.Kind+" "+diskspace.DisplayPath(c.Path))
	}
	if len(paths) > 5 {
		paths = append(paths[:5], "...")
	}
	return strings.Join(paths, ", ")
}
