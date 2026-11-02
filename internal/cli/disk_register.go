package cli

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/diskspace"
)

// diskRegistryItem is one ledger entry as `auto disk registry` shows it.
type diskRegistryItem struct {
	diskspace.LedgerEntry
	State string `json:"state"`
}

func newDiskRegisterCmd() *cobra.Command {
	var owner string
	cmd := &cobra.Command{
		Use:   "register <worktree>",
		Short: "Record a linked worktree this session created, so reclaim may remove it once it is done",
		Long: "Records the top-level directory of a linked git worktree in the machine-wide ownership ledger. " +
			"`auto disk reclaim --apply` removes a registered worktree once it is clean and either released or merged and idle. " +
			"A worktree outside .autopus/worktrees, .claude/worktrees, and .worktrees is never removed unless it is registered.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := diskspace.ResolveLinkedWorktree(cmd.Context(), diskRunner, args[0])
			if err != nil {
				return diskFail(cmd, false, 1, "", fmt.Sprintf("disk register: %s %s", args[0], diskspace.ErrNotLinkedWorktree), nil)
			}
			state, err := openDiskState()
			if err == nil {
				err = diskspace.RegisterWorktree(cmd.Context(), state, path, owner, diskNow())
			}
			switch {
			case errors.Is(err, diskspace.ErrNotOwnedWorktree):
				return diskFail(cmd, false, 1, "", "disk register: "+err.Error(), nil)
			case err != nil:
				return diskFail(cmd, false, 1, "", ledgerFailure(err), nil)
			}
			fmt.Fprintln(cmd.OutOrStdout(), terminalSafe("disk register: "+path))
			return nil
		},
	}
	cmd.Flags().StringVar(&owner, "owner", "", "who created it, for the registry listing")
	return cmd
}

func newDiskReleaseCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "release <worktree>",
		Short:         "Mark a registered worktree done; reclaim removes it once it is clean",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, resolveErr := diskspace.ResolveLinkedWorktree(cmd.Context(), diskRunner, args[0])
			state, err := openDiskState()
			if err != nil {
				return diskFail(cmd, false, 1, "", ledgerFailure(err), nil)
			}
			found := false
			if resolveErr == nil {
				found, err = diskspace.ReleaseWorktree(cmd.Context(), state, path, diskNow())
				if err != nil {
					return diskFail(cmd, false, 1, "", ledgerFailure(err), nil)
				}
			}
			if !found {
				// A cleanup step must never fail on a worktree nobody registered.
				fmt.Fprintln(cmd.OutOrStdout(), terminalSafe("disk release: "+args[0]+" is not registered"))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), terminalSafe("disk release: "+path))
			return nil
		},
	}
}

func newDiskRegistryCmd() *cobra.Command {
	var flags diskFlags
	cmd := &cobra.Command{
		Use:           "registry",
		Short:         "List the ownership ledger; changes nothing",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			jsonMode, err := resolveJSONMode(flags.jsonOut, flags.format)
			if err != nil {
				return err
			}
			state, err := openDiskState()
			var entries []diskspace.LedgerEntry
			if err == nil {
				entries, err = diskspace.LoadLedger(state)
			}
			if err != nil {
				return diskFail(cmd, jsonMode, 1, "disk_ledger_unreadable", ledgerFailure(err), nil)
			}
			items := registryItems(entries)
			if jsonMode {
				return writeJSONResult(cmd, jsonStatusOK, map[string]any{"entries": items}, nil, nil)
			}
			writeRegistryText(cmd.OutOrStdout(), items)
			return nil
		},
	}
	flags.register(cmd)
	return cmd
}

func registryItems(entries []diskspace.LedgerEntry) []diskRegistryItem {
	items := make([]diskRegistryItem, 0, len(entries))
	for _, entry := range entries {
		state := "stale"
		switch {
		case entry.Live() && entry.Released():
			state = "released"
		case entry.Live():
			state = "live"
		}
		items = append(items, diskRegistryItem{LedgerEntry: entry, State: state})
	}
	return items
}

func writeRegistryText(w io.Writer, items []diskRegistryItem) {
	if len(items) == 0 {
		fmt.Fprintln(w, "disk registry: no registered worktrees")
		return
	}
	for _, item := range items {
		line := fmt.Sprintf("%-8s %s registered %s", item.State, diskspace.DisplayPath(item.Path), item.RegisteredAt.Format(time.RFC3339))
		if item.Owner != "" {
			line += " by " + item.Owner
		}
		fmt.Fprintln(w, terminalSafe(line))
	}
}

// ledgerFailure is the one-line diagnostic of a state or ledger error.
func ledgerFailure(err error) string {
	if errors.Is(err, diskspace.ErrLedgerUnreadable) {
		return "disk: " + err.Error()
	}
	return err.Error()
}
