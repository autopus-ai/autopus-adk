package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/securityscan"
)

// secureExit ends `auto secure scan` with a chosen code after the report
// is already written: 1 for findings at the threshold, 2 when a scanner
// could not finish and nothing failed.
type secureExit struct {
	code int
	msg  string
}

func (e *secureExit) Error() string    { return e.msg }
func (e *secureExit) ExitCode() int    { return e.code }
func (e *secureExit) alreadyReported() {}

func newSecureCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secure",
		Short: "Local security checks with the scanners you already have",
	}
	cmd.AddCommand(newSecureScanCmd())
	return cmd
}

func newSecureScanCmd() *cobra.Command {
	var (
		dir, failOn, logOpts string
		timeout              time.Duration
		jsonOut, staged      bool
	)
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Run the installed secret and dependency scanners and summarize their findings",
		Long: "Detects Go, Node, Rust, and Python projects in --dir and runs each installed scanner: gitleaks " +
			"(always), govulncheck, npm/pnpm/yarn audit, cargo audit, and pip-audit. A scanner that is not installed " +
			"is noted and skipped; nothing is installed. Secret values are never printed: gitleaks runs with --redact " +
			"and only file:line, rule, and fingerprint are shown. Exit 1 when a finding reaches --fail-on (default: " +
			"any secret, or a vulnerability rated high or above; ungraded vulnerabilities count as high), 2 when a " +
			"scanner failed and nothing else did, 0 otherwise.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			threshold, err := securityscan.ParseFailOn(failOn)
			if err != nil {
				return err
			}
			if dir == "" {
				if dir, err = os.Getwd(); err != nil {
					return err
				}
			}
			if dir, err = filepath.Abs(dir); err != nil {
				return err
			}
			if info, err := os.Stat(dir); err != nil || !info.IsDir() {
				return fmt.Errorf("--dir %s: not a directory", dir)
			}
			rep := securityscan.Scan(cmd.Context(), securityscan.Options{Dir: dir, Timeout: timeout, Staged: staged, LogOpts: logOpts})
			failed := rep.Failed(threshold)
			if jsonOut {
				data, err := json.MarshalIndent(struct {
					securityscan.Report
					FailOn securityscan.FailOn `json:"fail_on"`
					Failed bool                `json:"failed"`
				}{rep, threshold, failed}, "", "  ")
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				securityscan.WriteTable(cmd.OutOrStdout(), rep, threshold)
			}
			switch {
			case failed:
				return &secureExit{code: 1, msg: securityscan.Verdict(rep, threshold)}
			case rep.HasErrors():
				return &secureExit{code: 2, msg: securityscan.Verdict(rep, threshold)}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "Project directory to scan (default: current directory)")
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "Time limit for each scanner")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print the report as JSON")
	cmd.Flags().StringVar(&failOn, "fail-on", string(securityscan.FailOnHigh), "Exit 1 at this threshold: none, secret, critical, high, medium, low")
	cmd.Flags().BoolVar(&staged, "staged", false, "gitleaks: scan only staged changes")
	cmd.Flags().StringVar(&logOpts, "log-opts", "", "gitleaks: git log range to scan, such as origin/main..HEAD")
	return cmd
}
