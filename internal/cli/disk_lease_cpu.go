package cli

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/diskspace"
)

// Environment of the CPU lease. The first two configure one machine (a CI
// runner's .env file, for instance) without touching a repo's autopus.yaml;
// the last two are exported to the command run under a CPU lease.
const (
	envLeaseDir      = "AUTOPUS_LEASE_DIR"
	envLeaseCPUSlots = "AUTOPUS_LEASE_CPU_SLOTS"
	envLeaseCPUs     = "AUTOPUS_LEASE_CPUS"
	envGOMAXPROCS    = "GOMAXPROCS"
)

// Seams of the CPU lease: the logical CPU count, whether this system can
// hold a lease at all, and the poll interval of a CPU wait.
var (
	leaseNumCPU       = runtime.NumCPU
	leaseCPUSupported = runtime.GOOS == "darwin" || runtime.GOOS == "linux"
	leaseCPUPoll      = time.Second
)

// leaseCPUSlots is the machine's slot count: AUTOPUS_LEASE_CPU_SLOTS, else
// disk.lease.cpu_slots, else the logical CPU count.
func leaseCPUSlots(lease config.DiskLeaseEffective) int {
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(envLeaseCPUSlots))); err == nil && n > 0 {
		return min(n, config.DiskMaxLeaseCPUSlots)
	}
	if lease.CPUSlots > 0 {
		return lease.CPUSlots
	}
	return max(leaseNumCPU(), 1)
}

// openLeaseDir is AUTOPUS_LEASE_DIR, else disk.lease.dir, as a shared lease
// dir, else the per-user state dir.
func openLeaseDir(lease config.DiskLeaseEffective) (diskspace.LeaseDir, error) {
	dir := strings.TrimSpace(os.Getenv(envLeaseDir))
	if dir == "" {
		dir = lease.Dir
	}
	if dir != "" {
		return diskspace.OpenSharedLeaseDir(dir)
	}
	state, err := openDiskState()
	if err != nil {
		return diskspace.LeaseDir{}, err
	}
	return state.Leases(), nil
}

// leaseEnv is base with AUTOPUS_LEASE_CPUS set to the grant, and GOMAXPROCS
// set to it too unless the caller chose its own. A GOMAXPROCS equal to an
// inherited AUTOPUS_LEASE_CPUS came from an outer lease and is replaced.
func leaseEnv(base []string, cpus int) []string {
	n := strconv.Itoa(cpus)
	outer, outerSet := lookupEnv(base, envLeaseCPUs)
	procs, procsSet := lookupEnv(base, envGOMAXPROCS)
	keepProcs := procsSet && procs != "" && !(outerSet && procs == outer)
	env := make([]string, 0, len(base)+2)
	for _, kv := range base {
		key, _, _ := strings.Cut(kv, "=")
		if key == envLeaseCPUs || (key == envGOMAXPROCS && !keepProcs) {
			continue
		}
		env = append(env, kv)
	}
	if !keepProcs {
		env = append(env, envGOMAXPROCS+"="+n)
	}
	return append(env, envLeaseCPUs+"="+n)
}

func lookupEnv(env []string, key string) (string, bool) {
	value, found := "", false
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			value, found = v, true
		}
	}
	return value, found
}

// cpuRefusalLine is the timeout line of a CPU wait.
func cpuRefusalLine(cpus, slots int, a diskspace.Admission, waited time.Duration) string {
	return fmt.Sprintf("disk lease: %d CPUs requested, %d of %d slots free after %d reserved by %d leases, %d waiters ahead; waited %s",
		cpus, max(slots-a.ReservedCPUs, 0), slots, a.ReservedCPUs, len(a.Live), a.Waiting, waited.Round(time.Second))
}

// writeLeaseStatusText prints the live leases and, when any CPU lease or
// waiter exists, the slot summary and the queue.
func writeLeaseStatusText(w io.Writer, leases, waiters []diskspace.LeaseInfo, slots int) {
	reserved := 0
	for _, lease := range leases {
		reserved += lease.CPUs
	}
	if reserved > 0 || len(waiters) > 0 {
		fmt.Fprintf(w, "cpu slots: %d of %d reserved, %d waiting\n", reserved, slots, len(waiters))
	}
	if len(leases) == 0 {
		fmt.Fprintln(w, "disk leases: none")
	}
	for _, lease := range leases {
		amount := diskspace.GiBString(lease.NeedBytes) + " GiB"
		if lease.CPUs > 0 {
			amount += fmt.Sprintf(", %d CPUs", lease.CPUs)
		}
		line := fmt.Sprintf("disk lease: %s for %s (pid %d, %s)", amount, lease.Label, lease.PID,
			diskNow().Sub(lease.Started).Round(time.Second))
		if lease.Orphaned {
			line += "; orphaned: the wrapper exited and a process it started still holds the lease"
		}
		fmt.Fprintln(w, terminalSafe(line))
	}
	for i, waiter := range waiters {
		fmt.Fprintln(w, terminalSafe(fmt.Sprintf("lease waiter %d: %d CPUs for %s (pid %d, waiting %s)", i+1,
			waiter.CPUs, waiter.Label, waiter.PID, diskNow().Sub(waiter.Started).Round(time.Second))))
	}
}
