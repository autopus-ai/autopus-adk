package cli

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/diskspace"
)

const (
	diskFloorSource      = "disk.min_free_gb"
	diskCodeCheckInvalid = "disk_check_invalid"
	diskCodeInsufficient = "disk_insufficient"
)

// diskCheckData is the JSON data of `auto disk check`.
type diskCheckData struct {
	OK            bool   `json:"ok"`
	FreeBytes     int64  `json:"free_bytes"`
	RequiredBytes int64  `json:"required_bytes"`
	DeficitBytes  int64  `json:"deficit_bytes"`
	Source        string `json:"source"`
	Mount         string `json:"mount"`
}

type diskCheckOptions struct {
	flags   diskFlags
	need    string
	reserve string
}

// newDiskCheckCmd builds `auto disk check`, the preflight generated guidance
// runs before a worktree, a parallel fan-out, or a full test run.
func newDiskCheckCmd() *cobra.Command {
	var opts diskCheckOptions
	cmd := &cobra.Command{
		Use:   "check [--need <GiB> | --for <reserve>]",
		Short: "Exit 3 when free space is below the floor or a step's need",
		Long: "Compares free space on the file system that holds the project with the larger of " +
			"disk.min_free_gb and either --need <GiB> or the disk.reserves entry named by --for. " +
			"Sizes are GiB (1073741824 bytes), as df -g prints them.\n\n" +
			"Exit status: 0 enough free space, 3 insufficient free space, " +
			"1 a usage, configuration, or measurement failure.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDiskCheck(cmd, opts)
		},
	}
	opts.flags.register(cmd)
	cmd.Flags().StringVar(&opts.need, "need", "", "GiB of free space the next step needs")
	cmd.Flags().StringVar(&opts.reserve, "for", "", "name of a disk.reserves entry in autopus.yaml")
	return cmd
}

// runDiskCheck applies REQ-DISK-CHECK-01 to -03: the requirement is the larger
// of the floor and the need or reserve, labeled with the source that won.
func runDiskCheck(cmd *cobra.Command, opts diskCheckOptions) error {
	jsonMode, err := resolveJSONMode(opts.flags.jsonOut, opts.flags.format)
	if err != nil {
		return err
	}
	needSet, reserveSet := cmd.Flags().Changed("need"), cmd.Flags().Changed("for")
	if needSet && reserveSet {
		return diskFail(cmd, jsonMode, 1, diskCodeCheckInvalid, "disk check: --need and --for are mutually exclusive", nil)
	}
	var needBytes int64
	if needSet {
		gib, err := strconv.ParseFloat(strings.TrimSpace(opts.need), 64)
		if err != nil || !(gib > 0) || math.IsInf(gib, 0) {
			return diskFail(cmd, jsonMode, 1, diskCodeCheckInvalid, "disk check: --need must be a positive number of GiB", nil)
		}
		// Rounded up: a requirement is never rounded below what was asked.
		needBytes = int64(math.Ceil(gib * float64(diskspace.GiB)))
	}
	dc, err := loadDiskContext(cmd, opts.flags, jsonMode, "check")
	if err != nil {
		return err
	}
	recordDiskSample(dc.space)
	if state, err := openDiskState(); err == nil {
		cheapConsumerGrowth(state, dc.disk, diskNow().UTC())
	}
	data := diskCheckData{FreeBytes: dc.space.AvailBytes, RequiredBytes: dc.disk.FloorBytes(), Source: diskFloorSource, Mount: dc.space.Mount}
	switch {
	case needSet && needBytes > data.RequiredBytes:
		data.RequiredBytes, data.Source = needBytes, "--need"
	case reserveSet:
		reserveBytes, ok := dc.disk.ReserveBytes(opts.reserve)
		if !ok {
			return diskFail(cmd, jsonMode, 1, diskCodeCheckInvalid,
				fmt.Sprintf("disk check: reserve %q is not configured under disk.reserves in autopus.yaml", opts.reserve), nil)
		}
		if reserveBytes > data.RequiredBytes {
			data.RequiredBytes, data.Source = reserveBytes, "disk.reserves."+opts.reserve
		}
	}
	data.OK = data.FreeBytes >= data.RequiredBytes
	if !data.OK {
		data.DeficitBytes = data.RequiredBytes - data.FreeBytes
		line := fmt.Sprintf("disk check: insufficient free space: %s GiB free, %s GiB required (%s) on %s; free %s GiB or lower the requirement",
			diskspace.GiBString(data.FreeBytes), diskspace.GiBString(data.RequiredBytes), data.Source, data.Mount,
			diskspace.GiBString(data.DeficitBytes))
		return diskFail(cmd, jsonMode, exitCodeDiskInsufficient, diskCodeInsufficient, line, data)
	}
	if jsonMode {
		return writeJSONResult(cmd, jsonStatusOK, data, nil, nil)
	}
	fmt.Fprintln(cmd.OutOrStdout(), terminalSafe(fmt.Sprintf("disk check: ok: %s GiB free, %s GiB required (%s) on %s",
		diskspace.GiBString(data.FreeBytes), diskspace.GiBString(data.RequiredBytes), data.Source, data.Mount)))
	return nil
}
