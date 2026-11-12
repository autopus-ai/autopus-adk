package cli

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/procreap"
)

// Seams of `auto proc reap`: tests feed a fixture snapshot and record signals.
var (
	procSnapshot = procreap.Snapshot
	procSignaler = procreap.Signaler(procreap.SystemSignaler)
	procSelfPID  = os.Getpid
	procUID      = os.Getuid
)

// newProcCmd builds the `auto proc` namespace for process hygiene.
func newProcCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "proc",
		Short:         "Find and clean up processes that agent sessions left behind",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newProcReapCmd())
	return cmd
}

type procReapOptions struct {
	olderThan  time.Duration
	grace      time.Duration
	sessionPID int
	apply      bool
	jsonOut    bool
	format     string
}

// procReapRow is one candidate as text and JSON output show it.
type procReapRow struct {
	PID         int    `json:"pid"`
	PPID        int    `json:"ppid"`
	AgeSeconds  int64  `json:"age_seconds"`
	Age         string `json:"age"`
	Reason      string `json:"reason"`
	Command     string `json:"command"`
	Descendants []int  `json:"descendants"`
	Result      string `json:"result,omitempty"`
	Error       string `json:"error,omitempty"`
}

func newProcReapCmd() *cobra.Command {
	var opts procReapOptions
	cmd := &cobra.Command{
		Use:   "reap",
		Short: "List (or with --apply, stop) idle polling shells that wait forever",
		Long: "Lists shell processes of the current user that run an `until`/`while` + `sleep` loop, are older than " +
			"--older-than, and whose only live descendants are `sleep`: background waits whose condition will never " +
			"come true, such as a `pgrep -f` that matches its own command line or a `grep -q` with no deadline. " +
			"Dry run by default; --apply sends SIGTERM to each shell and its sleep children, then SIGKILL after --grace. " +
			"Other users' processes, shells running anything but sleep, non-shell processes, and this command's own " +
			"ancestors are never touched.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          func(cmd *cobra.Command, _ []string) error { return runProcReap(cmd, opts) },
	}
	cmd.Flags().DurationVar(&opts.olderThan, "older-than", 2*time.Hour, "Minimum age of a reported shell")
	cmd.Flags().DurationVar(&opts.grace, "grace", 3*time.Second, "Wait between SIGTERM and SIGKILL with --apply")
	cmd.Flags().IntVar(&opts.sessionPID, "session-pid", 0, "Only consider descendants of this process (e.g. one agent session)")
	cmd.Flags().BoolVar(&opts.apply, "apply", false, "Terminate the listed shells and their sleep children")
	addJSONFlags(cmd, &opts.jsonOut, &opts.format)
	return cmd
}

func runProcReap(cmd *cobra.Command, opts procReapOptions) error {
	jsonMode, err := resolveJSONMode(opts.jsonOut, opts.format)
	if err != nil {
		return err
	}
	if opts.olderThan < 0 || opts.grace < 0 || opts.sessionPID < 0 {
		return diskFail(cmd, jsonMode, 1, "proc_reap_usage", "proc reap: --older-than, --grace and --session-pid must not be negative", nil)
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	procs, err := procSnapshot(ctx)
	if err != nil {
		return diskFail(cmd, jsonMode, 1, "proc_snapshot_failed", "proc reap: "+err.Error(), nil)
	}
	cands := procreap.Find(procs, procreap.Options{
		UID: procUID(), SelfPID: procSelfPID(), OlderThan: opts.olderThan, SessionPID: opts.sessionPID,
	})
	rows := make([]procReapRow, len(cands))
	for i, c := range cands {
		desc := make([]int, len(c.Descendants))
		for j, d := range c.Descendants {
			desc[j] = d.PID
		}
		rows[i] = procReapRow{PID: c.PID, PPID: c.PPID, AgeSeconds: int64(c.Age.Seconds()),
			Age: procreap.FormatAge(c.Age), Reason: c.Reason, Command: c.Command, Descendants: desc}
	}
	if opts.apply && len(cands) > 0 {
		for i, o := range procreap.Reap(ctx, cands, procSignaler, opts.grace) {
			rows[i].Result, rows[i].Error = o.Result, o.Error
		}
	}
	data := map[string]any{"apply": opts.apply, "older_than": opts.olderThan.String(), "candidates": rows}
	failed := 0
	for _, r := range rows {
		if r.Result == "failed" {
			failed++
		}
	}
	if !jsonMode {
		if err := writeProcReapText(cmd, opts, rows); err != nil {
			return err
		}
	}
	if failed > 0 {
		return diskFail(cmd, jsonMode, 1, "proc_reap_failed", fmt.Sprintf("proc reap: %d shell(s) could not be signalled", failed), data)
	}
	if jsonMode {
		return writeJSONResult(cmd, jsonStatusOK, data, nil, nil)
	}
	return nil
}

func writeProcReapText(cmd *cobra.Command, opts procReapOptions, rows []procReapRow) error {
	out := cmd.OutOrStdout()
	if len(rows) == 0 {
		fmt.Fprintf(out, "proc reap: no idle polling shells older than %s\n", opts.olderThan)
		return nil
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	header := "PID\tAGE\tREASON\tCOMMAND"
	if opts.apply {
		header = "PID\tAGE\tREASON\tRESULT\tCOMMAND"
	}
	fmt.Fprintln(tw, header)
	for _, r := range rows {
		short := terminalSafe(procreap.ShortCommand(r.Command, 80))
		if opts.apply {
			fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\n", r.PID, r.Age, r.Reason, r.Result, short)
		} else {
			fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n", r.PID, r.Age, r.Reason, short)
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if !opts.apply {
		fmt.Fprintf(out, "proc reap: %d idle polling shell(s); rerun with --apply to stop them\n", len(rows))
	}
	return nil
}
