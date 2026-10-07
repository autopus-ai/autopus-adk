package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// newLegacyOrchestraJobStatusCmd creates the "orchestra status" subcommand.
// Retired (SPEC-PANERM-001): the command tree registers the stub of
// orchestra_retired.go instead; this body stays until the pane backend deletion.
// Loads a job by ID and prints its current status and per-provider completion.
// @AX:NOTE [AUTO] REQ-5 job lifecycle CLI — reads persisted job JSON; prints stored status without recalculating
func newLegacyOrchestraJobStatusCmd() *cobra.Command {
	var jobDir string

	cmd := &cobra.Command{
		Use:   "status <jobID>",
		Short: "Show the status of a detached orchestra job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jobID := args[0]
			job, err := orchestra.LoadJob(jobDir, jobID)
			if err != nil {
				return fmt.Errorf("job %q not found: %w", jobID, err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Job: %s\n", job.ID)
			fmt.Fprintf(cmd.OutOrStdout(), "Status: %s\n", job.Status)
			fmt.Fprintf(cmd.OutOrStdout(), "Strategy: %s\n", job.Strategy)
			fmt.Fprintf(cmd.OutOrStdout(), "Providers: %v\n", job.Providers)

			for _, p := range job.Providers {
				state := "pending"
				if job.Results[p] != nil {
					state = "done"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s\n", p, state)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&jobDir, "job-dir", os.TempDir(), "Directory containing job files")
	return cmd
}

// newLegacyOrchestraJobWaitCmd creates the "orchestra wait" subcommand.
// Retired (SPEC-PANERM-001): the command tree registers the stub of
// orchestra_retired.go instead; this body stays until the pane backend deletion.
// Polls CheckStatus until done/timeout and prints the final status.
// @AX:NOTE [AUTO] REQ-5 blocking wait — 1s poll interval; reloads job JSON each cycle to pick up new results
func newLegacyOrchestraJobWaitCmd() *cobra.Command {
	var (
		jobDir       string
		outputFormat string
	)

	cmd := &cobra.Command{
		Use:   "wait <jobID>",
		Short: "Wait for a detached orchestra job to complete",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jobID := args[0]
			job, err := orchestra.LoadJob(jobDir, jobID)
			if err != nil {
				return fmt.Errorf("job %q not found: %w", jobID, err)
			}

			// Poll until terminal status
			for {
				status := job.CheckStatus()
				if status == orchestra.JobStatusDone || status == orchestra.JobStatusTimeout || status == orchestra.JobStatusError {
					return writeOrchestraJobWaitOutput(cmd.OutOrStdout(), job.ID, status, outputFormat)
				}
				time.Sleep(1 * time.Second)
				// Reload job to pick up new results
				job, err = orchestra.LoadJob(jobDir, jobID)
				if err != nil {
					return fmt.Errorf("reload job: %w", err)
				}
			}
		},
	}

	cmd.Flags().StringVar(&jobDir, "job-dir", os.TempDir(), "Directory containing job files")
	cmd.Flags().StringVar(&outputFormat, "format", orchestraOutputText, "Output format (text|json)")
	return cmd
}

// newLegacyOrchestraJobResultCmd creates the "orchestra result" subcommand.
// Retired (SPEC-PANERM-001): the command tree registers the stub of
// orchestra_retired.go instead; this body stays until the pane backend deletion.
// Collects results, prints merged output, and optionally cleans up.
// @AX:NOTE [AUTO] REQ-5 result retrieval — --cleanup removes both job subdir and JSON file
func newLegacyOrchestraJobResultCmd() *cobra.Command {
	var (
		jobDir       string
		cleanup      bool
		outputFormat string
	)

	cmd := &cobra.Command{
		Use:   "result <jobID>",
		Short: "Show results of a completed orchestra job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jobID := args[0]
			job, err := orchestra.LoadJob(jobDir, jobID)
			if err != nil {
				return fmt.Errorf("job %q not found: %w", jobID, err)
			}

			result, err := job.CollectResults()
			if err != nil {
				return fmt.Errorf("collect results: %w", err)
			}

			if err := writeOrchestraCLIOutput(cmd.OutOrStdout(), result, outputFormat); err != nil {
				return err
			}

			if cleanup {
				// Remove job subdirectory if it exists
				subDir := filepath.Join(jobDir, jobID)
				if _, err := os.Stat(subDir); err == nil {
					_ = os.RemoveAll(subDir)
				}
				// Remove the job's own Dir if set
				if job.Dir != "" {
					_ = os.RemoveAll(job.Dir)
				}
				// Remove the job JSON file
				_ = os.Remove(filepath.Join(jobDir, jobID+".json"))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&jobDir, "job-dir", os.TempDir(), "Directory containing job files")
	cmd.Flags().BoolVar(&cleanup, "cleanup", false, "Remove job directory after displaying results")
	cmd.Flags().StringVar(&outputFormat, "format", orchestraOutputText, "Output format (text|json)")
	return cmd
}
