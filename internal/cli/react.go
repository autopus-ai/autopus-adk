package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	reactLookPath = exec.LookPath
	reactOutput   = func(name string, args ...string) ([]byte, error) {
		return exec.Command(name, args...).Output()
	}
)

// @AX:NOTE [AUTO] [downgraded from ANCHOR — fan_in < 3] ciRun struct unmarshalled from gh CLI output; field names must match JSON keys; single production caller within react.go
// ciRun represents a GitHub Actions run from `gh run list` JSON output.
type ciRun struct {
	DatabaseID int64  `json:"databaseId"`
	Name       string `json:"name"`
	Conclusion string `json:"conclusion"`
	HeadBranch string `json:"headBranch"`
	UpdatedAt  string `json:"updatedAt"`
}

// newReactCmd creates the `auto react` parent command.
func newReactCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "react",
		Short: "CI 실패 감지 및 자동 수정",
		Long:  "GitHub Actions CI 실패를 감지하고 분석 보고서를 생성합니다.",
	}

	cmd.AddCommand(newReactCheckCmd())
	cmd.AddCommand(newReactApplyCmd())
	cmd.AddCommand(newReactBandCmd())
	return cmd
}

// newReactCheckCmd creates `auto react check`.
func newReactCheckCmd() *cobra.Command {
	var quiet bool

	cmd := &cobra.Command{
		Use:   "check",
		Short: "최근 CI 실패 감지 및 분석 보고서 생성",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReactCheck(cmd, args, quiet)
		},
	}

	cmd.Flags().BoolVar(&quiet, "quiet", false, "Suppress output when no failures are found; print only summary when failures exist")
	return cmd
}

// newReactApplyCmd creates `auto react apply {run-id}`.
func newReactApplyCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "apply <run-id>",
		Short: "분석 보고서를 기반으로 수정 적용",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReactApply(cmd, args[0], force)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "확인 없이 즉시 적용")
	return cmd
}

func runReactCheck(cmd *cobra.Command, _ []string, quiet bool) error {
	// Verify gh CLI is installed
	if _, err := reactLookPath("gh"); err != nil {
		return fmt.Errorf("gh CLI not found. Install it from: https://cli.github.com/")
	}

	out := cmd.OutOrStdout()
	if !quiet {
		fmt.Fprintln(out, "Checking for CI failures...")
	}
	hasRemote, err := hasGitRemote()
	if err != nil {
		return err
	}
	if !hasRemote {
		if !quiet {
			fmt.Fprintln(out, "No git remote configured. Skipping CI checks.")
		}
		return nil
	}

	// Fetch failed runs
	result, err := reactOutput("gh", "run", "list",
		"--status", "failure",
		"--limit", "5",
		"--json", "databaseId,name,conclusion,headBranch,updatedAt",
	)
	if err != nil {
		return fmt.Errorf("failed to list CI runs: %w", err)
	}

	var runs []ciRun
	if err := json.Unmarshal(result, &runs); err != nil {
		return fmt.Errorf("failed to parse CI runs: %w", err)
	}

	if len(runs) == 0 {
		// When quiet is set and no failures found, exit silently.
		if !quiet {
			fmt.Fprintln(out, "No recent CI failures found.")
		}
		return nil
	}

	// Print summary line when quiet; full output otherwise.
	fmt.Fprintf(out, "Found %d failed run(s):\n", len(runs))
	if quiet {
		// In quiet mode, only the summary line above is printed.
		return nil
	}
	fmt.Fprintln(out)

	// @AX:NOTE [AUTO] @AX:REASON: magic constant for react report storage path
	reactDir := ".autopus/react"
	if err := os.MkdirAll(reactDir, 0o755); err != nil {
		return fmt.Errorf("failed to create react directory: %w", err)
	}

	for _, run := range runs {
		fmt.Fprintf(out, "  [%d] %s (branch: %s, updated: %s)\n",
			run.DatabaseID, run.Name, run.HeadBranch, run.UpdatedAt)

		// Fetch failure logs
		logs, err := fetchRunLogs(run.DatabaseID)
		if err != nil {
			fmt.Fprintf(out, "  Warning: could not fetch logs for run %d: %v\n", run.DatabaseID, err)
			logs = "Log fetch failed."
		}

		// Write analysis report
		reportPath := filepath.Join(reactDir, fmt.Sprintf("%d.md", run.DatabaseID))
		if err := writeReactReport(reportPath, run, logs); err != nil {
			fmt.Fprintf(out, "  Warning: could not write report for run %d: %v\n", run.DatabaseID, err)
			continue
		}

		fmt.Fprintf(out, "  Report saved: %s\n", reportPath)
	}

	return nil
}

func hasGitRemote() (bool, error) {
	out, err := reactOutput("git", "remote")
	if err != nil {
		return false, nil
	}
	return strings.TrimSpace(string(out)) != "", nil
}

func fetchRunLogs(runID int64) (string, error) {
	out, err := reactOutput("gh", "run", "view",
		fmt.Sprintf("%d", runID),
		"--log-failed",
	)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func writeReactReport(path string, run ciRun, logs string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintf(f, "# CI Failure Report\n\n")
	fmt.Fprintf(f, "- **Run ID**: %d\n", run.DatabaseID)
	fmt.Fprintf(f, "- **Name**: %s\n", run.Name)
	fmt.Fprintf(f, "- **Branch**: %s\n", run.HeadBranch)
	fmt.Fprintf(f, "- **Conclusion**: %s\n", run.Conclusion)
	fmt.Fprintf(f, "- **Updated**: %s\n", run.UpdatedAt)
	fmt.Fprintf(f, "- **Generated**: %s\n\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(f, "## Failure Logs\n\n```\n%s\n```\n", logs)

	return nil
}

func runReactApply(cmd *cobra.Command, runID string, force bool) error {
	if _, err := strconv.ParseInt(runID, 10, 64); err != nil {
		return fmt.Errorf("invalid run ID %q: must be a numeric ID", runID)
	}

	reportPath := filepath.Join(".autopus/react", runID+".md")

	data, err := os.ReadFile(reportPath)
	if err != nil {
		return fmt.Errorf("report not found at %s — run `auto react check` first", reportPath)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Report for run %s:\n\n", runID)
	fmt.Fprintln(out, extractReportSummary(string(data)))

	if !force {
		fmt.Fprint(out, "\nProceed with applying fix? [y/N] ")
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if answer != "y" && answer != "yes" {
			fmt.Fprintln(out, "Aborted.")
			return nil
		}
	}

	// Stash current changes before applying
	if err := exec.Command("git", "stash").Run(); err != nil {
		fmt.Fprintf(out, "Warning: git stash failed: %v\n", err)
	} else {
		fmt.Fprintln(out, "Current changes stashed.")
	}

	fmt.Fprintln(out, "Fix context set up. Delegate to debugger agent for actual fix logic.")

	// Restore stashed changes after fix is delegated.
	if err := exec.Command("git", "stash", "pop").Run(); err != nil {
		fmt.Fprintf(out, "Note: run `git stash pop` to restore your stashed changes.\n")
	} else {
		fmt.Fprintln(out, "Stashed changes restored.")
	}

	return nil
}

func extractReportSummary(content string) string {
	lines := strings.Split(content, "\n")
	var summary []string
	for _, line := range lines {
		if strings.HasPrefix(line, "- **") || strings.HasPrefix(line, "# ") {
			summary = append(summary, line)
		}
		if len(summary) >= 6 {
			break
		}
	}
	return strings.Join(summary, "\n")
}
