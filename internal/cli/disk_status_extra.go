package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/insajin/autopus-adk/pkg/diskspace"
)

// statusExtras adds the SPEC-DISK-002 sections to a status: the reclaim
// preview, the live leases, the free-space trend, and which worktrees reclaim
// owns (REQ-DISK2-STATUS-01, REQ-DISK2-LOG-04). It records a sample first,
// and reads leases without discarding dead ones: status changes nothing.
func statusExtras(ctx context.Context, s diskSurvey, worktrees diskspace.WorktreeScan, d *diskStatusData) {
	samples := recordDiskSample(s.dc.space)
	d.Trend = diskspace.ComputeTrend(samples, s.dc.space.Mount, s.now, s.dc.space.AvailBytes)
	d.Consumers.Extra = measureExtraConsumers(s.dc.disk, statusConsumerBudget)
	d.Growth = recordConsumerGrowth(consumerBytes(*d), s.now.UTC())
	if d.Growth == nil {
		d.Growth = []diskspace.Growth{}
	}
	plan := planReclaim(ctx, s.dc, worktrees)
	d.ReclaimPreview = plan.data(s.dc, "dry-run")
	for i := range d.Consumers.Worktrees.Items {
		item := &d.Consumers.Worktrees.Items[i]
		item.Owned = plan.worktrees.IsOwned(item.Path)
	}
	d.Leases, d.LeaseWaiters = []diskspace.LeaseInfo{}, []diskspace.LeaseInfo{}
	d.CPUSlots = leaseCPUSlots(s.dc.disk.Lease)
	if dir, err := openLeaseDir(s.dc.disk.Lease); err == nil {
		if live, err := diskspace.LiveLeasesIn(dir, false); err == nil && live != nil {
			d.Leases = live
		}
		if waiters, err := diskspace.LiveWaiters(dir); err == nil && waiters != nil {
			d.LeaseWaiters = waiters
		}
	}
}

func writeStatusExtrasText(w io.Writer, d diskStatusData) {
	r := d.ReclaimPreview
	fmt.Fprintf(w, "disk reclaim: worktrees=%d (%s) scratchpads=%d (%s) regenerable=%d (%s) large_files=%d (%s); run `auto disk reclaim` for the list\n",
		len(r.Worktrees.Candidates), diskspace.SizeString(r.Worktrees.Bytes),
		len(r.Scratchpads.Candidates), diskspace.SizeString(r.Scratchpads.Bytes),
		len(r.Regenerable.Candidates), diskspace.SizeString(r.Regenerable.Bytes),
		len(r.LargeFiles.Candidates), diskspace.SizeString(r.LargeFiles.Bytes))
	for _, item := range d.Consumers.Worktrees.Items {
		if item.Owned {
			fmt.Fprintln(w, terminalSafe("  owned worktree: "+diskspace.DisplayPath(item.Path)))
		}
	}
	writeLeaseStatusText(w, d.Leases, d.LeaseWaiters, d.CPUSlots)
	if d.Trend != nil {
		sign := ""
		if d.Trend.BytesPerHour >= 0 {
			sign = "+"
		}
		fmt.Fprintf(w, "free trend: %s%s GiB/h over %s\n", sign, diskspace.GiBString(d.Trend.BytesPerHour),
			(time.Duration(d.Trend.WindowMinutes) * time.Minute).String())
	}
	writeGrowthText(w, d.Growth)
}
