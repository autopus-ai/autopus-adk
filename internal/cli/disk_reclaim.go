package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"slices"
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
	Run         string                     `json:"run"`
	Worktrees   diskReclaimKind            `json:"worktrees"`
	Scratchpads diskReclaimKind            `json:"scratchpads"`
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
	state     *diskspace.StateDir
	uid       int
}

const diskReclaimLong = "Removes finished work, as a dry run unless --apply is given.\n\n" +
	"Worktrees: an owned linked worktree (under .autopus/worktrees, .claude/worktrees, or .worktrees, or registered with " +
	"`auto disk register`) that is clean, unlocked, does not hold the current directory, and is either released or merged " +
	"and idle beyond disk.reclaim.worktree_idle_minutes. Git removes it without --force; the branch is kept.\n\n" +
	"Scratchpads: a Claude Code session directory <slug>/<uuid> under the scratchpad root whose newest entry is idle " +
	"beyond disk.reclaim.scratchpad_idle_minutes. Symlinks inside are unlinked, never followed.\n\n" +
	"Exit status: 0 done, 1 a refused root, a failed removal, or a usage, configuration, or measurement failure."

func newDiskReclaimCmd() *cobra.Command {
	var flags diskFlags
	var apply bool
	cmd := &cobra.Command{
		Use:           "reclaim [--apply]",
		Short:         "Remove finished worktrees and idle session scratchpads (dry run by default)",
		Long:          diskReclaimLong,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDiskReclaim(cmd, flags, apply)
		},
	}
	flags.register(cmd)
	cmd.Flags().BoolVar(&apply, "apply", false, "remove the candidates instead of listing what would go")
	return cmd
}

func runDiskReclaim(cmd *cobra.Command, flags diskFlags, apply bool) error {
	jsonMode, err := resolveJSONMode(flags.jsonOut, flags.format)
	if err != nil {
		return err
	}
	dc, err := loadDiskContext(cmd, flags, jsonMode, "reclaim")
	if err != nil {
		return err
	}
	plan := planReclaim(cmd.Context(), dc)
	data := plan.data(dc, "dry-run")
	if apply {
		data.Run = "apply"
		result := plan.apply(cmd.Context())
		data.Result = &result
		recordDiskEvent(diskspace.Event{Kind: diskspace.EventReclaimApply, FreeBytes: dc.space.AvailBytes,
			ReclaimedBytes: result.DeletedBytes, Count: result.Worktrees + result.Scratchpads})
	}
	if !jsonMode {
		writeReclaimText(cmd.OutOrStdout(), data)
	}
	switch {
	case data.Refused != nil && apply:
		return diskFail(cmd, jsonMode, 1, "disk_reclaim_refused", "disk reclaim: refused: "+data.Refused.Message, data)
	case data.Result != nil && data.Result.Failed > 0:
		return diskFail(cmd, jsonMode, 1, "disk_reclaim_failed", strings.Join(data.Result.Failures, "; "), data)
	case jsonMode:
		return writeJSONResult(cmd, jsonStatusOK, data, nil, nil)
	}
	return nil
}

// planReclaim scans both kinds. A state dir or ledger that cannot be read
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
		Cwd: cwd, Now: now, IdleMinutes: dc.disk.Reclaim.WorktreeIdleMinutes, UID: plan.uid, Ledger: ledger}
	if len(project) > 0 {
		input.Project = &project[0]
	}
	plan.worktrees = diskspace.ScanWorktreeReclaim(ctx, input)
	root := diskspace.ResolveScratchRoot(diskScratchpad(plan.uid), plan.uid)
	plan.scratch = diskspace.ScanSessions(root, now, dc.disk.Reclaim.ScratchpadIdleMinutes, plan.uid, cwd, diskspace.ScratchTree)
	return plan
}

func (p reclaimPlan) apply(ctx context.Context) diskspace.ReclaimCounters {
	result := diskspace.ApplyWorktrees(ctx, diskRunner, p.worktrees.Candidates, p.state, p.uid, diskReclaimBeforeWorktree)
	scratch, err := diskspace.ApplyScratch(p.scratch, p.uid, diskReclaimBeforeScratch)
	if err != nil {
		scratch.Failed += len(p.scratch.Candidates)
		scratch.Failures = append(scratch.Failures, err.Error())
	}
	result.Add(scratch)
	return result
}

func (p reclaimPlan) data(dc diskContext, run string) diskReclaimData {
	kind := func(cands []diskspace.ReclaimCandidate, skipped map[string]int, idle int) diskReclaimKind {
		k := diskReclaimKind{IdleMinutes: idle, Candidates: cands, Skipped: skipped}
		if k.Candidates == nil {
			k.Candidates = []diskspace.ReclaimCandidate{}
		}
		for _, c := range cands {
			k.Bytes += c.Bytes
		}
		return k
	}
	d := diskReclaimData{Run: run,
		Worktrees:   kind(p.worktrees.Candidates, p.worktrees.Skipped, dc.disk.Reclaim.WorktreeIdleMinutes),
		Scratchpads: kind(p.scratch.Candidates, p.scratch.Skipped, dc.disk.Reclaim.ScratchpadIdleMinutes),
		ScratchRoot: rootData(p.scratch.Root)}
	if p.scratch.Root.Refused {
		d.Refused = refusalData(p.scratch.Root)
	}
	d.TotalBytes = d.Worktrees.Bytes + d.Scratchpads.Bytes
	return d
}

func writeReclaimText(w io.Writer, d diskReclaimData) {
	fmt.Fprintf(w, "disk reclaim: run=%s worktrees=%d (%s, idle>%dm) scratchpads=%d (%s, idle>%dm)\n", d.Run,
		len(d.Worktrees.Candidates), diskspace.SizeString(d.Worktrees.Bytes), d.Worktrees.IdleMinutes,
		len(d.Scratchpads.Candidates), diskspace.SizeString(d.Scratchpads.Bytes), d.Scratchpads.IdleMinutes)
	if !d.ScratchRoot.Available {
		fmt.Fprintf(w, "  scratchpad root: unavailable (%s)\n", d.ScratchRoot.Reason)
	}
	for _, kind := range []diskReclaimKind{d.Worktrees, d.Scratchpads} {
		for _, c := range kind.Candidates {
			fmt.Fprintln(w, terminalSafe(fmt.Sprintf("  %s %s reason=%s idle=%dm size=%s", c.Kind,
				diskspace.DisplayPath(c.Path), c.Reason, c.IdleMinutes, diskspace.SizeString(c.Bytes))))
		}
	}
	for _, kind := range []struct {
		name    string
		skipped map[string]int
	}{{"worktree", d.Worktrees.Skipped}, {"scratchpad", d.Scratchpads.Skipped}} {
		if line := skipLine(kind.skipped); line != "" {
			fmt.Fprintf(w, "  kept %s: %s\n", kind.name, line)
		}
	}
	if d.Result == nil {
		fmt.Fprintf(w, "disk reclaim: would reclaim %s (dry-run; pass --apply to delete)\n", diskspace.SizeString(d.TotalBytes))
		return
	}
	r := d.Result
	fmt.Fprintf(w, "disk reclaim: reclaimed %s (worktrees=%d scratchpads=%d skipped_changed=%d skipped_symlink=%d skipped_device=%d vanished=%d failed=%d)\n",
		diskspace.SizeString(r.DeletedBytes), r.Worktrees, r.Scratchpads, r.SkippedChanged, r.SkippedSymlink, r.SkippedDevice, r.Vanished, r.Failed)
}

// skipLine prints skip counters in name order.
func skipLine(skipped map[string]int) string {
	names := make([]string, 0, len(skipped))
	for name := range skipped {
		names = append(names, name)
	}
	slices.Sort(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s=%d", name, skipped[name]))
	}
	return strings.Join(parts, " ")
}
