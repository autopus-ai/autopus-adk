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
func maybeStartAutoReclaim(dir string) bool {
	if strings.EqualFold(os.Getenv("AUTOPUS_DISK_AUTO_RECLAIM"), "off") || dir == "" {
		return false
	}
	harness, err := config.LoadPreview(dir)
	if err != nil {
		return false
	}
	reclaim := harness.Disk.Effective().Reclaim
	if !reclaim.Auto {
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
	cmd := exec.Command(exe, "disk", "reclaim", "--auto", "--apply", "--dir", dir)
	cmd.Dir = dir
	cmd.SysProcAttr = detachedProcAttr()
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
