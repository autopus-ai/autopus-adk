package cli

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/insajin/autopus-adk/pkg/diskspace"
)

func writeReclaimText(w io.Writer, d diskReclaimData) {
	fmt.Fprintf(w, "disk reclaim: run=%s worktrees=%d (%s, idle>%dm) scratchpads=%d (%s, idle>%dm) regenerable=%d (%s) large_files=%d (%s)\n", d.Run,
		len(d.Worktrees.Candidates), diskspace.SizeString(d.Worktrees.Bytes), d.Worktrees.IdleMinutes,
		len(d.Scratchpads.Candidates), diskspace.SizeString(d.Scratchpads.Bytes), d.Scratchpads.IdleMinutes,
		len(d.Regenerable.Candidates), diskspace.SizeString(d.Regenerable.Bytes),
		len(d.LargeFiles.Candidates), diskspace.SizeString(d.LargeFiles.Bytes))
	if !d.ScratchRoot.Available {
		fmt.Fprintf(w, "  scratchpad root: unavailable (%s)\n", d.ScratchRoot.Reason)
	}
	for _, kind := range []diskReclaimKind{d.Worktrees, d.Regenerable, d.Scratchpads, d.LargeFiles} {
		for _, c := range kind.Candidates {
			fmt.Fprintln(w, terminalSafe(fmt.Sprintf("  %s %s reason=%s idle=%dm size=%s", c.Kind,
				diskspace.DisplayPath(c.Path), c.Reason, c.IdleMinutes, diskspace.SizeString(c.Bytes))))
		}
	}
	if len(d.LargeFiles.Candidates) > 0 {
		fmt.Fprintln(w, "  large files may be backups: they are removed only with --include-large-files")
	}
	for _, kind := range []struct {
		name    string
		skipped map[string]int
	}{{"worktree", d.Worktrees.Skipped}, {"regenerable", d.Regenerable.Skipped}, {"scratchpad", d.Scratchpads.Skipped}} {
		if line := skipLine(kind.skipped); line != "" {
			fmt.Fprintf(w, "  kept %s: %s\n", kind.name, line)
		}
	}
	if d.Result == nil {
		fmt.Fprintf(w, "disk reclaim: would reclaim %s (dry-run; pass --apply to delete)\n", diskspace.SizeString(d.TotalBytes))
		return
	}
	r := d.Result
	fmt.Fprintf(w, "disk reclaim: reclaimed %s (worktrees=%d scratchpads=%d regenerable=%d large_files=%d skipped_changed=%d skipped_symlink=%d skipped_device=%d vanished=%d failed=%d)\n",
		diskspace.SizeString(r.DeletedBytes), r.Worktrees, r.Scratchpads, r.Regenerable, r.LargeFiles,
		r.SkippedChanged, r.SkippedSymlink, r.SkippedDevice, r.Vanished, r.Failed)
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
