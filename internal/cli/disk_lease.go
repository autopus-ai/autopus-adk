package cli

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/diskspace"
)

// Seams of `auto disk lease`: the poll interval and the wait between polls.
var (
	diskLeasePoll  = 5 * time.Second
	diskLeaseSleep = func(ctx context.Context, d time.Duration) {
		select {
		case <-ctx.Done():
		case <-time.After(d):
		}
	}
)

// exitCodeLeaseNotStarted is the status of a command that could not start,
// the shell's "command not found" code.
const exitCodeLeaseNotStarted = 127

type diskLeaseOptions struct {
	flags   diskFlags
	need    string
	reserve string
	cpu     int
	wait    time.Duration
	label   string
}

// leaseRequest is what one lease asks for, resolved from flags and config.
type leaseRequest struct {
	need  int64
	cpus  int
	slots int
	wait  time.Duration
	label string
}

const diskLeaseLong = "Runs a heavy command only while free space minus the reservations of other running leases, minus " +
	"its own need, stays at or above disk.min_free_gb, and while fewer than disk.lease.max_concurrent leases run when " +
	"that is above 0. Otherwise it reclaims once (disk.lease.reclaim_on_shortage) and waits, polling every 5 seconds, " +
	"up to --wait (default disk.lease.wait_minutes).\n\n" +
	"--cpu N also reserves N of the machine's CPU slots (disk.lease.cpu_slots or AUTOPUS_LEASE_CPU_SLOTS, default the " +
	"logical CPU count; a larger N is clamped to it). CPU waiters are served first in, first out, and the command runs " +
	"with AUTOPUS_LEASE_CPUS=N and GOMAXPROCS=N unless GOMAXPROCS is already set. --cpu alone needs no --need. " +
	"Leases live in the current user's state dir, or in the shared dir AUTOPUS_LEASE_DIR or disk.lease.dir names.\n\n" +
	"The reservation is held for the whole run and released when the command exits, or when this process dies.\n\n" +
	"Exit status: the command's own status, 128+N when signal N ended it, 3 when the wait ran out, 127 when the " +
	"command could not start, 1 a usage, configuration, or measurement failure."

func newDiskLeaseCmd() *cobra.Command {
	var opts diskLeaseOptions
	cmd := &cobra.Command{
		Use:           "lease (--need <GiB> | --for <reserve> | --cpu <N>) [--wait <duration>] [--label <text>] -- <command> [args...]",
		Short:         "Run a heavy command under a machine-wide disk and CPU reservation",
		Long:          diskLeaseLong,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDiskLease(cmd, opts, args)
		},
	}
	cmd.Flags().StringVar(&opts.flags.dir, "dir", "", "Project directory (default: current directory)")
	cmd.Flags().StringVar(&opts.need, "need", "", "GiB the command needs, at least 1")
	cmd.Flags().StringVar(&opts.reserve, "for", "", "name of a disk.reserves entry in autopus.yaml")
	cmd.Flags().IntVar(&opts.cpu, "cpu", 0, "CPU slots the command reserves, at least 1")
	cmd.Flags().DurationVar(&opts.wait, "wait", -1, "how long to wait for space (default disk.lease.wait_minutes)")
	cmd.Flags().StringVar(&opts.label, "label", "", "what the lease is for, shown by `auto disk status`")
	return cmd
}

func runDiskLease(cmd *cobra.Command, opts diskLeaseOptions, args []string) error {
	usage := func(line string) error { return diskFail(cmd, false, 1, "", "disk lease: "+line, nil) }
	needSet, reserveSet, cpuSet := cmd.Flags().Changed("need"), cmd.Flags().Changed("for"), cmd.Flags().Changed("cpu")
	switch {
	case needSet && reserveSet, !needSet && !reserveSet && !cpuSet:
		return usage("give exactly one of --need <GiB> and --for <reserve>, or --cpu <N>")
	case cpuSet && (opts.cpu < 1 || opts.cpu > diskspace.MaxLeaseCPUs):
		return usage(fmt.Sprintf("--cpu must be a number of CPU slots from 1 to %d", diskspace.MaxLeaseCPUs))
	case cmd.Flags().Changed("wait") && opts.wait < 0:
		return usage("--wait must not be negative")
	case cmd.ArgsLenAtDash() != 0 || len(args) == 0:
		return usage("name the command after --, as in: auto disk lease --need 20 -- go test ./...")
	}
	// The default label is the program name only: the full argv may carry
	// tokens, and the label is stored and printed (security review L4).
	label := opts.label
	if label == "" {
		label = filepath.Base(args[0])
	}
	label = diskspace.CleanLabel(label)
	if cpuSet && !needSet && !reserveSet && !leaseCPUSupported {
		// No lease can be held here; a CPU lease is an optimisation, so the
		// command still runs, with the CPU environment it would have had.
		cpus := min(opts.cpu, leaseCPUSlots(config.DiskLeaseEffective{}))
		fmt.Fprintln(cmd.ErrOrStderr(), "disk lease: CPU leases are not supported on this system; running unleased")
		return leaseExit(runLeased(cmd, args, nil, leaseEnv(os.Environ(), cpus)))
	}
	dc, err := loadDiskContext(cmd, opts.flags, false, "lease")
	if err != nil {
		return err
	}
	req := leaseRequest{label: label, wait: opts.wait}
	if needSet {
		gib, err := strconv.ParseFloat(strings.TrimSpace(opts.need), 64)
		// Written as a negated range so NaN fails it too.
		if err != nil || !(gib >= 1 && gib <= diskspace.MaxLeaseGiB) {
			return usage(fmt.Sprintf("--need must be a number of GiB from 1 to %d", diskspace.MaxLeaseGiB))
		}
		req.need = int64(math.Ceil(gib * float64(diskspace.GiB)))
	} else if reserveSet {
		var ok bool
		if req.need, ok = dc.disk.ReserveBytes(opts.reserve); !ok {
			return usage(fmt.Sprintf("reserve %q is not configured under disk.reserves in autopus.yaml", opts.reserve))
		}
		if gb := dc.disk.Reserves[opts.reserve]; gb > diskspace.MaxLeaseGiB {
			return usage(fmt.Sprintf("reserve %q is above %d GiB", opts.reserve, diskspace.MaxLeaseGiB))
		}
	}
	if cpuSet {
		req.slots = leaseCPUSlots(dc.disk.Lease)
		req.cpus = min(opts.cpu, req.slots)
	}
	dir, err := openLeaseDir(dc.disk.Lease)
	if err != nil {
		return diskFail(cmd, false, 1, "", "disk lease: "+err.Error(), nil)
	}
	if req.wait < 0 {
		req.wait = time.Duration(dc.disk.Lease.WaitMinutes) * time.Minute
	}
	lease, err := waitForLease(cmd, dc, dir, req)
	if lease == nil {
		return err
	}
	defer lease.Release()
	env := os.Environ()
	if req.cpus > 0 {
		env = leaseEnv(env, req.cpus)
	}
	return leaseExit(runLeased(cmd, args, lease, env))
}

// waitForLease admits the request, reclaiming once on the first disk
// shortage when configured, and polls until the wait ends
// (REQ-DISK2-LEASE-02). A CPU request keeps its queue ticket across polls.
func waitForLease(cmd *cobra.Command, dc diskContext, dir diskspace.LeaseDir, req leaseRequest) (*diskspace.Lease, error) {
	ctx := cmd.Context()
	start := diskNow()
	reclaimed := !dc.disk.Lease.ReclaimOnShortage
	poll := diskLeasePoll
	if req.cpus > 0 {
		poll = min(poll, leaseCPUPoll)
	}
	var ticket *diskspace.Ticket
	defer func() { ticket.Release() }()
	for {
		admission, err := diskspace.AdmitIn(ctx, dir, diskspace.AdmitRequest{
			Free:       func() (int64, error) { space, err := diskFree(dc.projectDir); return space.AvailBytes, err },
			FloorBytes: dc.disk.FloorBytes(), NeedBytes: req.need, MaxConcurrent: dc.disk.Lease.MaxConcurrent,
			CPUs: req.cpus, CPUSlots: req.slots, Ticket: ticket, Label: req.label, PID: os.Getpid(), Now: diskNow()})
		if err != nil {
			return nil, diskFail(cmd, false, 1, "", "disk lease: "+err.Error(), nil)
		}
		if admission.Ticket != nil {
			ticket = admission.Ticket
		}
		if req.need > 0 {
			recordDiskSample(diskspace.Space{AvailBytes: admission.FreeBytes, Mount: dc.space.Mount})
		}
		if admission.Admitted() {
			ticket = nil // consumed by the admission
			return admission.Lease, nil
		}
		if !reclaimed && admission.DiskShort {
			reclaimed = true
			freed := leaseShortageReclaim(ctx, dc)
			recordDiskEvent(diskspace.Event{Kind: diskspace.EventLeaseReclaimed, FreeBytes: admission.FreeBytes,
				ReclaimedBytes: freed, Message: req.label})
			continue
		}
		waited := diskNow().Sub(start)
		if waited >= req.wait {
			return nil, leaseRefused(cmd, dc, req, admission, waited)
		}
		diskLeaseSleep(ctx, min(poll, req.wait-waited))
	}
}

// leaseRefused reports the budget that kept the lease from starting.
func leaseRefused(cmd *cobra.Command, dc diskContext, req leaseRequest, a diskspace.Admission, waited time.Duration) error {
	line := fmt.Sprintf("disk lease: %s GiB needed above the floor, %s GiB available after %s GiB reserved by %d leases; waited %s",
		diskspace.GiBString(req.need), diskspace.GiBString(a.AvailableBytes), diskspace.GiBString(a.ReservedBytes),
		len(a.Live), waited.Round(time.Second))
	switch {
	case a.CountLimited:
		line = fmt.Sprintf("disk lease: %d leases already run (disk.lease.max_concurrent=%d); waited %s",
			len(a.Live), dc.disk.Lease.MaxConcurrent, waited.Round(time.Second))
	case !a.DiskShort && req.cpus > 0:
		line = cpuRefusalLine(req.cpus, req.slots, a, waited)
	}
	recordDiskEvent(diskspace.Event{Kind: diskspace.EventLeaseRefused, FreeBytes: a.FreeBytes, Message: req.label})
	return diskFail(cmd, false, exitCodeDiskInsufficient, "", line, nil)
}

// leaseShortageReclaim runs the trim and reclaim applies once and returns
// the bytes they freed. Their own output is not printed: the lease's
// stdout belongs to the command it wraps.
func leaseShortageReclaim(ctx context.Context, dc diskContext) int64 {
	var freed int64
	if s := surveyTrim(ctx, dc, ""); !s.plan.Inactive && s.refused() == nil {
		result := diskspace.Apply(s.plan, diskspace.ApplyOptions{Cache: s.cacheRoot, WorkDir: s.workRoot, UID: s.uid})
		freed += result.Total.DeletedBytes
		recordDiskEvent(diskspace.Event{Kind: diskspace.EventTrimApply, FreeBytes: dc.space.AvailBytes,
			ReclaimedBytes: result.Total.DeletedBytes, Count: result.Total.Deleted})
	}
	result := planReclaim(ctx, dc).apply(ctx)
	recordDiskEvent(diskspace.Event{Kind: diskspace.EventReclaimApply, FreeBytes: dc.space.AvailBytes,
		ReclaimedBytes: result.DeletedBytes, Count: result.Worktrees + result.Scratchpads})
	return freed + result.DeletedBytes
}

// runLeased runs argv with the inherited streams and env, and returns its
// exit code (REQ-DISK2-LEASE-03), holding the lease when there is one.
// SIGTERM and SIGHUP sent to this process are forwarded. SIGINT is caught
// but not forwarded: a terminal delivers it to the whole foreground process
// group, the command included, and a second copy would read as a forced
// abort to tools such as cargo.
func runLeased(cmd *cobra.Command, argv []string, lease *diskspace.Lease, env []string) int {
	child := exec.Command(argv[0], argv[1:]...)
	child.Stdin, child.Stdout, child.Stderr = cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()
	child.Env = env
	// The command holds the lease's open file description too (as fd 3), so
	// the flock, and with it the reservation, outlives a killed wrapper for
	// as long as the command runs (REQ-DISK2-LEASE-03).
	if lease != nil {
		child.ExtraFiles = []*os.File{lease.File()}
	}
	signals := make(chan os.Signal, 4)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(signals)
	if err := child.Start(); err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), terminalSafe(fmt.Sprintf("disk lease: cannot start %s: %v", argv[0], err)))
		return exitCodeLeaseNotStarted
	}
	done := make(chan struct{})
	go func() {
		for {
			select {
			case sig := <-signals:
				if sig != syscall.SIGINT {
					_ = child.Process.Signal(sig)
				}
			case <-done:
				return
			}
		}
	}()
	err := child.Wait()
	close(done)
	return childExitCode(child, err)
}

func leaseExit(code int) error {
	if code == 0 {
		return nil
	}
	return &diskExit{code: code}
}
