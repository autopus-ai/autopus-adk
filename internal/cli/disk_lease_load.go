package cli

import (
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/diskspace"
)

// Machine overrides of the load- and memory-aware admission, for one host
// (a CI runner's .env file, for instance) without touching autopus.yaml.
const (
	envLeaseMaxLoadRatio = "AUTOPUS_LEASE_MAX_LOAD_RATIO"
	envLeaseMinFreeMemGB = "AUTOPUS_LEASE_MIN_FREE_MEM_GB"
)

// leaseMachineSampler reads the machine's load and memory; tests replace it.
var leaseMachineSampler = diskspace.SystemSampler()

// leaseLoadPolicy resolves the load policy: the environment overrides
// disk.lease.max_load_ratio and disk.lease.min_free_mem_gb. The load limit
// scales with the logical CPUs, not the slots: the load average counts
// every process on the machine, leased or not.
func leaseLoadPolicy(lease config.DiskLeaseEffective) diskspace.LoadPolicy {
	ratio, floorGB := lease.MaxLoadRatio, float64(lease.MinFreeMemGB)
	if v, err := strconv.ParseFloat(strings.TrimSpace(os.Getenv(envLeaseMaxLoadRatio)), 64); err == nil &&
		v >= 0 && v <= config.DiskMaxLeaseLoadRatio {
		ratio = v
	}
	if v, err := strconv.ParseFloat(strings.TrimSpace(os.Getenv(envLeaseMinFreeMemGB)), 64); err == nil &&
		v >= 0 && v <= config.DiskMaxLeaseMinFreeMemGB {
		floorGB = v
	}
	return diskspace.LoadPolicy{
		Sampler:         leaseMachineSampler,
		MaxLoadRatio:    ratio,
		LogicalCPUs:     max(leaseNumCPU(), 1),
		MinFreeMemBytes: int64(math.Ceil(floorGB * float64(diskspace.GiB))),
		MaxLoadWait:     time.Duration(lease.MaxLoadWaitMinutes) * time.Minute,
	}
}

// parseLeaseMem reads --mem: a number of bytes with a K, M, G, or T suffix
// (KiB-based, an optional "B" or "iB" after it); a bare number is GiB.
func parseLeaseMem(s string) (int64, error) {
	text := strings.ToUpper(strings.TrimSpace(s))
	text = strings.TrimSuffix(strings.TrimSuffix(text, "B"), "I")
	unit := diskspace.GiB
	if n := len(text); n > 0 {
		switch text[n-1] {
		case 'K':
			unit, text = diskspace.KiB, text[:n-1]
		case 'M':
			unit, text = diskspace.MiB, text[:n-1]
		case 'G':
			text = text[:n-1]
		case 'T':
			unit, text = diskspace.GiB<<10, text[:n-1]
		}
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	limit := float64(diskspace.MaxLeaseGiB) * float64(diskspace.GiB)
	// Written as a negated range so NaN fails it too.
	if err != nil || !(value > 0 && value*float64(unit) <= limit) {
		return 0, fmt.Errorf("--mem must be a size such as 8G or 512M, above 0 and at most %d GiB", diskspace.MaxLeaseGiB)
	}
	return int64(math.Ceil(value * float64(unit))), nil
}

// leasePollInterval is the CPU wait between polls, jittered by ±25% so
// waiters that started together do not poll in lockstep.
func leasePollInterval() time.Duration {
	quarter := int64(leaseCPUPoll / 4)
	if quarter <= 0 {
		return leaseCPUPoll
	}
	return leaseCPUPoll - time.Duration(quarter) + time.Duration(rand.Int64N(2*quarter+1))
}

// reportForcedAdmission says, and logs, that a lease starts despite load.
func reportForcedAdmission(cmd *cobra.Command, req leaseRequest, a diskspace.Admission) {
	reason := "load"
	if a.Machine != nil {
		reason = a.Machine.Reason()
	}
	line := fmt.Sprintf("disk lease: starting %s despite %s: no lease ran for %s (disk.lease.max_load_wait_minutes)",
		req.label, reason, req.load.MaxLoadWait)
	fmt.Fprintln(cmd.ErrOrStderr(), terminalSafe(line))
	recordDiskEvent(diskspace.Event{Kind: diskspace.EventLeaseForced, Message: req.label})
}

// machineSuffix is the "; waiting: <reason>" tail of a refusal or status
// line held by the machine, or "".
func machineSuffix(reason string) string {
	if reason == "" {
		return ""
	}
	return "; waiting: " + reason
}

// resolveLeaseMachine fills the load policy and --mem of a CPU request,
// and refuses at once a request that could never fit on this machine.
func resolveLeaseMachine(cmd *cobra.Command, opts diskLeaseOptions, dc diskContext, req *leaseRequest) error {
	req.load = leaseLoadPolicy(dc.disk.Lease)
	if cmd.Flags().Changed("mem") {
		mem, err := parseLeaseMem(opts.mem)
		if err != nil {
			return diskFail(cmd, false, 1, "", "disk lease: "+err.Error(), nil)
		}
		req.mem = mem
	}
	if why := req.load.NeverFits(req.mem); why != "" {
		return diskFail(cmd, false, 1, "", "disk lease: "+why+", so it could never start", nil)
	}
	return nil
}

// memText is the ", N GiB mem" part of a lease or waiter line, or "".
func memText(bytes int64) string {
	if bytes <= 0 {
		return ""
	}
	return ", " + diskspace.GiBString(bytes) + " GiB mem"
}
