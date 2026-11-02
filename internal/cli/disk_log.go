package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/diskspace"
)

func newDiskLogCmd() *cobra.Command {
	var flags diskFlags
	var limit int
	cmd := &cobra.Command{
		Use:           "log [--limit <n>]",
		Short:         "Show the latest disk events: trims, reclaims, lease refusals, and low-space alerts",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			jsonMode, err := resolveJSONMode(flags.jsonOut, flags.format)
			if err != nil {
				return err
			}
			if limit < 1 {
				return diskFail(cmd, jsonMode, 1, "disk_log_invalid", "disk log: --limit must be at least 1", nil)
			}
			state, err := openDiskState()
			var events []diskspace.Event
			if err == nil {
				events, err = diskspace.TailEvents(state, limit)
			}
			if err != nil {
				return diskFail(cmd, jsonMode, 1, "disk_log_unreadable", "disk log: "+err.Error(), nil)
			}
			if events == nil {
				events = []diskspace.Event{}
			}
			if jsonMode {
				return writeJSONResult(cmd, jsonStatusOK, map[string]any{"events": events}, nil, nil)
			}
			if len(events) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "disk log: no events")
			}
			for _, e := range events {
				line := fmt.Sprintf("%s %-15s free=%s GiB reclaimed=%s count=%d", e.Time.Local().Format(time.DateTime), e.Kind,
					diskspace.GiBString(e.FreeBytes), diskspace.SizeString(e.ReclaimedBytes), e.Count)
				if e.Message != "" {
					line += " " + e.Message
				}
				fmt.Fprintln(cmd.OutOrStdout(), terminalSafe(line))
			}
			return nil
		},
	}
	flags.register(cmd)
	cmd.Flags().IntVar(&limit, "limit", 20, "how many of the latest events to show")
	return cmd
}
