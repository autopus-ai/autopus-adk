package cli

import (
	"fmt"
	"time"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/diskspace"
)

// lowSpaceAlertInterval spaces the low_space_alert events one session's
// every-turn alert would otherwise write.
const lowSpaceAlertInterval = 30 * time.Minute

// stickyLockWait bounds each state lock the alert path takes.
const stickyLockWait = 250 * time.Millisecond

// stickyDiskAlert is the per-turn alert of SPEC-DISK-002 REQ-DISK2-ALERT-01,
// or "" at or above the floor or when free space cannot be measured. It
// records a sample, and a low_space_alert event at most every 30 minutes.
func stickyDiskAlert(root string, disk config.DiskEffective) string {
	space, err := diskFree(root)
	if err != nil {
		return ""
	}
	// One open, with lock waits bounded well inside the hook's 5 seconds.
	state, stateErr := openDiskState()
	state = state.WithLockWait(stickyLockWait)
	if stateErr == nil {
		_ = diskspace.RecordSample(state, diskspace.Sample{Time: diskNow().UTC(), FreeBytes: space.AvailBytes, Mount: space.Mount})
	}
	if space.AvailBytes >= disk.FloorBytes() {
		return ""
	}
	if stateErr == nil {
		_, _ = diskspace.AppendEventUnlessRecent(state, diskspace.Event{Time: diskNow().UTC(),
			Kind: diskspace.EventLowSpaceAlert, FreeBytes: space.AvailBytes}, lowSpaceAlertInterval)
	}
	return fmt.Sprintf("Disk alert: %s GiB free on %s, below the floor of %s GiB (disk.min_free_gb). "+
		"Do not start builds, worktrees, or parallel agents until space recovers; run `auto disk status`, "+
		"then `auto disk reclaim --apply` and `auto disk trim --apply`. "+
		"If you lead other agents, tell each of them to pause heavy work.",
		diskspace.GiBString(space.AvailBytes), diskspace.DisplayPath(space.Mount), diskspace.GiBString(disk.FloorBytes()))
}
