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
	wait    time.Duration
	label   string
}

const diskLeaseLong = "Runs a heavy command only while free space minus the reservations of other running leases, minus " +
	"its own need, stays at or above disk.min_free_gb, and while fewer than disk.lease.max_concurrent leases run when " +
	"that is above 0. Otherwise it reclaims once (disk.lease.reclaim_on_shortage) and waits, polling every 5 seconds, " +
	"up to --wait (default disk.lease.wait_minutes).\n\n" +
	"The reservation is held for the whole run and released when the command exits, or when this process dies.\n\n" +
	"Exit status: the command's own status, 128+N when signal N ended it, 3 when the wait ran out, 127 when the " +
	"command could not start, 1 a usage, configuration, or measurement failure."

func newDiskLeaseCmd() *cobra.Command {
	var opts diskLeaseOptions
	cmd := &cobra.Command{
		Use:           "lease (--need <GiB> | --for <reserve>) [--wait <duration>] [--label <text>] -- <command> [args...]",
		Short:         "Run a heavy command under a machine-wide disk reservation",
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
	cmd.Flags().DurationVar(&opts.wait, "wait", -1, "how long to wait for space (default disk.lease.wait_minutes)")
	cmd.Flags().StringVar(&opts.label, "label", "", "what the lease is for, shown by `auto disk status`")
	return cmd
}

func runDiskLease(cmd *cobra.Command, opts diskLeaseOptions, args []string) error {
	usage := func(line string) error { return diskFail(cmd, false, 1, "", "disk lease: "+line, nil) }
	needSet, reserveSet := cmd.Flags().Changed("need"), cmd.Flags().Changed("for")
	switch {
	case needSet == reserveSet:
		return usage("give exactly one of --need <GiB> and --for <reserve>")
	case cmd.Flags().Changed("wait") && opts.wait < 0:
		return usage("--wait must not be negative")
	case cmd.ArgsLenAtDash() != 0 || len(args) == 0:
		return usage("name the command after --, as in: auto disk lease --need 20 -- go test ./...")
	}
	dc, err := loadDiskContext(cmd, opts.flags, false, "lease")
	if err != nil {
		return err
	}
	var needBytes int64
	if needSet {
		gib, err := strconv.ParseFloat(strings.TrimSpace(opts.need), 64)
		// Written as a negated range so NaN fails it too.
		if err != nil || !(gib >= 1 && gib <= diskspace.MaxLeaseGiB) {
			return usage(fmt.Sprintf("--need must be a number of GiB from 1 to %d", diskspace.MaxLeaseGiB))
		}
		needBytes = int64(math.Ceil(gib * float64(diskspace.GiB)))
	} else {
		var ok bool
		if needBytes, ok = dc.disk.ReserveBytes(opts.reserve); !ok {
			return usage(fmt.Sprintf("reserve %q is not configured under disk.reserves in autopus.yaml", opts.reserve))
		}
		if gb := dc.disk.Reserves[opts.reserve]; gb > diskspace.MaxLeaseGiB {
			return usage(fmt.Sprintf("reserve %q is above %d GiB", opts.reserve, diskspace.MaxLeaseGiB))
		}
	}
	state, err := openDiskState()
	if err != nil {
		return diskFail(cmd, false, 1, "", err.Error(), nil)
	}
	wait := opts.wait
	if wait < 0 {
		wait = time.Duration(dc.disk.Lease.WaitMinutes) * time.Minute
	}
	// The default label is the program name only: the full argv may carry
	// tokens, and the label is stored and printed (security review L4).
	label := opts.label
	if label == "" {
		label = filepath.Base(args[0])
	}
	label = diskspace.CleanLabel(label)
	lease, err := waitForLease(cmd, dc, state, needBytes, wait, label)
	if lease == nil {
		return err
	}
	defer lease.Release()
	return leaseExit(runLeased(cmd, args, lease))
}

// waitForLease admits the request, reclaiming once on the first shortage
// when configured, and polls until the wait ends (REQ-DISK2-LEASE-02).
func waitForLease(cmd *cobra.Command, dc diskContext, state diskspace.StateDir, need int64, wait time.Duration, label string) (*diskspace.Lease, error) {
	ctx := cmd.Context()
	start := diskNow()
	reclaimed := !dc.disk.Lease.ReclaimOnShortage
	for {
		admission, err := diskspace.Admit(ctx, state, diskspace.AdmitRequest{
			Free:       func() (int64, error) { space, err := diskFree(dc.projectDir); return space.AvailBytes, err },
			FloorBytes: dc.disk.FloorBytes(), NeedBytes: need, MaxConcurrent: dc.disk.Lease.MaxConcurrent,
			Label: label, PID: os.Getpid(), Now: diskNow()})
		if err != nil {
			return nil, diskFail(cmd, false, 1, "", "disk lease: "+err.Error(), nil)
		}
		recordDiskSample(diskspace.Space{AvailBytes: admission.FreeBytes, Mount: dc.space.Mount})
		if admission.Admitted() {
			return admission.Lease, nil
		}
		if !reclaimed && !admission.CountLimited {
			reclaimed = true
			freed := leaseShortageReclaim(ctx, dc)
			recordDiskEvent(diskspace.Event{Kind: diskspace.EventLeaseReclaimed, FreeBytes: admission.FreeBytes,
				ReclaimedBytes: freed, Message: label})
			continue
		}
		waited := diskNow().Sub(start)
		if waited >= wait {
			line := fmt.Sprintf("disk lease: %s GiB needed above the floor, %s GiB available after %s GiB reserved by %d leases; waited %s",
				diskspace.GiBString(need), diskspace.GiBString(admission.AvailableBytes), diskspace.GiBString(admission.ReservedBytes),
				len(admission.Live), waited.Round(time.Second))
			if admission.CountLimited {
				line = fmt.Sprintf("disk lease: %d leases already run (disk.lease.max_concurrent=%d); waited %s",
					len(admission.Live), dc.disk.Lease.MaxConcurrent, waited.Round(time.Second))
			}
			recordDiskEvent(diskspace.Event{Kind: diskspace.EventLeaseRefused, FreeBytes: admission.FreeBytes, Message: label})
			return nil, diskFail(cmd, false, exitCodeDiskInsufficient, "", line, nil)
		}
		diskLeaseSleep(ctx, min(diskLeasePoll, wait-waited))
	}
}

// leaseShortageReclaim runs the trim and reclaim applies once and returns
// the bytes they freed. Their own output is not printed: the lease's
// stdout belongs to the command it wraps.
func leaseShortageReclaim(ctx context.Context, dc diskContext) int64 {
	var freed int64
	if s := surveyDisk(ctx, dc, ""); !s.plan.Inactive && s.refused() == nil {
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

// runLeased runs argv with the inherited streams and returns its exit code
// (REQ-DISK2-LEASE-03), holding the lease. SIGTERM and SIGHUP sent to this process are
// forwarded. SIGINT is caught but not forwarded: a terminal delivers it to
// the whole foreground process group, the command included, and a second
// copy would read as a forced abort to tools such as cargo.
func runLeased(cmd *cobra.Command, argv []string, lease *diskspace.Lease) int {
	child := exec.Command(argv[0], argv[1:]...)
	child.Stdin, child.Stdout, child.Stderr = cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()
	// The command holds the lease's open file description too (as fd 3), so
	// the flock, and with it the reservation, outlives a killed wrapper for
	// as long as the command runs (REQ-DISK2-LEASE-03).
	child.ExtraFiles = []*os.File{lease.File()}
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
