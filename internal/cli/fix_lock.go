package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/editguard"
)

// exitCodeFixUnverified is the `auto fix unlock` status when a verdict is not
// unchanged: the locks are released, but the fix is not complete (REQ-EG-08).
// A usage, store, or removal error stays exit 1.
const exitCodeFixUnverified = 3

const noFixLocks = "no fix locks"

// fixUnverifiedError reports released locks whose files did not stay
// unchanged.
type fixUnverifiedError struct{ changed, total int }

func (e *fixUnverifiedError) Error() string {
	return fmt.Sprintf("%d of %d unlocked files did not stay unchanged; the fix is not complete", e.changed, e.total)
}

// ExitCode satisfies exitCoder.
func (e *fixUnverifiedError) ExitCode() int { return exitCodeFixUnverified }

// newFixCmd builds the `auto fix` namespace that locks the reproduction test
// of an /auto fix (SPEC-EDITGUARD-001).
func newFixCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "fix",
		Short:         "Lock and verify the reproduction test of an /auto fix",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newFixLockCmd(), newFixUnlockCmd())
	return cmd
}

func newFixLockCmd() *cobra.Command {
	var (
		list, asJSON bool
		ttl          time.Duration
	)
	cmd := &cobra.Command{
		Use:   "lock [--ttl <duration>] [--] <path>... | lock --list [--json]",
		Short: "Lock reproduction tests so the edit guard denies edits to them",
		Long: "Records the SHA-256 of each file. While the lock is active the edit guard denies " +
			"file-editing tool calls on it, and auto fix unlock reports whether it stayed unchanged. " +
			"Exit status: 0 locked, 1 nothing locked.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ttlSet := cmd.Flags().Changed("ttl")
			if list {
				if len(args) > 0 || ttlSet {
					return errors.New("auto fix lock --list takes no path and no --ttl")
				}
				return listFixLocks(cmd.OutOrStdout(), asJSON)
			}
			if len(args) == 0 {
				return errors.New("auto fix lock needs at least one path, or --list")
			}
			if ttlSet && ttl == 0 {
				return editguard.ErrInvalidTTL // 0 would select the default
			}
			store, err := openFixStore()
			if err != nil {
				return err
			}
			return store.Lock(args, ttl)
		},
	}
	cmd.Flags().BoolVar(&list, "list", false, "list every lock with its state and current integrity")
	cmd.Flags().BoolVar(&asJSON, "json", false, "with --list, print autopus.fix_lock_list.v1 JSON")
	cmd.Flags().DurationVar(&ttl, "ttl", 0, "lock lifetime between 1m and 168h (default 24h)")
	return cmd
}

func newFixUnlockCmd() *cobra.Command {
	var all, asJSON bool
	cmd := &cobra.Command{
		Use:   "unlock [--json] (--all | [--] <path>...)",
		Short: "Release locks and report whether each locked file stayed unchanged",
		Long: "Computes every verdict (unchanged, modified, missing, unverifiable) before removing a lock. " +
			"Exit status: 0 all unchanged, 3 any other verdict, 1 nothing released or a removal error " +
			"(the remaining locks stay for a rerun).",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if all == (len(args) > 0) {
				return errors.New("auto fix unlock needs either paths or --all")
			}
			store, err := openFixStore()
			if err != nil {
				return err
			}
			var results []editguard.UnlockResult
			if all {
				results, err = store.UnlockAll()
			} else {
				results, err = store.Unlock(args)
			}
			// After a removal error the computed verdicts are still the only
			// record of the released locks, so they are printed before exit 1.
			if err == nil || len(results) > 0 {
				if writeErr := writeUnlockReport(cmd.OutOrStdout(), results, asJSON); writeErr != nil {
					return errors.Join(err, writeErr)
				}
			}
			if err != nil {
				return err
			}
			return unverifiedFix(results)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "release every lock, stale and unreadable ones included")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print autopus.fix_unlock.v1 JSON")
	return cmd
}

// openFixStore opens the lock store of the project that contains the working
// directory; relative paths resolve against it.
func openFixStore() (*editguard.Store, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	store, err := editguard.OpenStore(cwd)
	if err != nil {
		return nil, err
	}
	store.Now = editGuardClock
	return store, nil
}

func listFixLocks(out io.Writer, asJSON bool) error {
	store, err := openFixStore()
	if err != nil {
		return err
	}
	entries, err := store.List()
	if err != nil {
		return err
	}
	if asJSON {
		return writeFixJSON(out, editguard.NewListReport(entries))
	}
	rows := [][]string{{"PATH", "STATE", "CREATED_AT", "EXPIRES_AT", "INTEGRITY"}}
	for _, entry := range entries {
		rows = append(rows, []string{printablePath(entry.Path), entry.State, fixCell(entry.CreatedAt),
			fixCell(entry.ExpiresAt), string(entry.Integrity)})
	}
	return writeFixTable(out, rows)
}

func writeUnlockReport(out io.Writer, results []editguard.UnlockResult, asJSON bool) error {
	if asJSON {
		return writeFixJSON(out, editguard.NewUnlockReport(results))
	}
	rows := [][]string{{"PATH", "VERDICT"}}
	for _, result := range results {
		rows = append(rows, []string{printablePath(result.Path), string(result.Verdict)})
	}
	return writeFixTable(out, rows)
}

func unverifiedFix(results []editguard.UnlockResult) error {
	changed := 0
	for _, result := range results {
		if result.Verdict != editguard.VerdictUnchanged {
			changed++
		}
	}
	if changed == 0 {
		return nil
	}
	return &fixUnverifiedError{changed: changed, total: len(results)}
}

// writeFixJSON prints v as one JSON line, paths byte for byte.
func writeFixJSON(out io.Writer, v any) error {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(v); err != nil {
		return err
	}
	_, err := out.Write(buf.Bytes())
	return err
}

// writeFixTable aligns a header row and its body rows; a header alone means
// there is nothing to list.
func writeFixTable(out io.Writer, rows [][]string) error {
	if len(rows) == 1 {
		_, err := fmt.Fprintln(out, noFixLocks)
		return err
	}
	table := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, row := range rows {
		fmt.Fprintln(table, strings.Join(row, "\t"))
	}
	return table.Flush()
}

// printablePath quotes a recorded path whose control bytes or invalid UTF-8
// would otherwise reach the terminal.
func printablePath(p string) string {
	if utf8.ValidString(p) && !strings.ContainsFunc(p, unicode.IsControl) {
		return p
	}
	return strconv.Quote(p)
}

// fixCell marks a timestamp an unreadable record does not have.
func fixCell(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
