package cli

import (
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/diskspace"
)

// autoReclaimBudget bounds the detached automatic reclaim.
const autoReclaimBudget = 5 * time.Minute

// diskSpawnAutoReclaim starts the automatic reclaim; tests replace it so no
// test binary is ever started as `auto`.
var diskSpawnAutoReclaim = spawnAutoReclaim

// maybeStartAutoReclaim is the per-turn automatic cleanup the Stop hook runs:
// unless AUTOPUS_DISK_AUTO_RECLAIM=off or disk.reclaim.auto is false, and at
// most once per disk.reclaim.auto_interval_minutes across every session of
// the user, it starts `auto disk reclaim --auto --apply` detached and
// returns. The turn waits for one config read, one lock, and one stamp
// write, never for the reclaim itself. It reports whether it started one.
// The settings are read at diskConfigRoot(dir), but the reclaim runs in dir,
// the session's cwd, so it keeps protecting the worktree that holds it.
func maybeStartAutoReclaim(dir string) bool {
	if strings.EqualFold(os.Getenv("AUTOPUS_DISK_AUTO_RECLAIM"), "off") || dir == "" {
		return false
	}
	harness, err := config.LoadPreview(diskConfigRoot(dir))
	if err != nil {
		return false
	}
	reclaim := harness.Disk.Effective().Reclaim
	if !harness.AutoReclaimEnabled() {
		return false
	}
	state, err := openDiskState()
	if err != nil {
		return false
	}
	due, err := diskspace.ClaimAutoReclaim(state, diskNow(), time.Duration(reclaim.AutoIntervalMinutes)*time.Minute)
	if err != nil || !due {
		return false
	}
	return diskSpawnAutoReclaim(dir) == nil
}

// spawnAutoReclaim starts this executable detached, with no standard
// streams, in its own session where the system has one.
func spawnAutoReclaim(dir string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := autoReclaimCommand(exe, dir)
	cmd.SysProcAttr = detachedProcAttr()
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// autoReclaimCommand is the automatic reclaim run in dir, the session's cwd,
// with no --dir: its cwd stays the base of the scan and of the protection of
// the worktree holding the session, while its config is read at the project
// root as for any `auto disk` command run without --dir.
func autoReclaimCommand(exe, dir string) *exec.Cmd {
	cmd := exec.Command(exe, "disk", "reclaim", "--auto", "--apply")
	cmd.Dir = dir
	return cmd
}
