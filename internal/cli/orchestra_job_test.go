package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOrchestraStatusCmd_ValidJob verifies that the status subcommand returns
// the correct status for an existing job.
func TestOrchestraStatusCmd_ValidJob(t *testing.T) {
	t.Parallel()

	// Given: a job directory with a saved job file
	jobDir := t.TempDir()
	jobID := "status-valid-001"
	jobFile := filepath.Join(jobDir, jobID+".json")
	jobJSON := `{"id":"status-valid-001","status":"running","providers":["claude","codex"]}`
	require.NoError(t, os.WriteFile(jobFile, []byte(jobJSON), 0o644))

	// When: orchestra status command is executed with valid job ID
	cmd := newLegacyOrchestraJobStatusCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{jobID, "--job-dir", jobDir})
	err := cmd.Execute()

	// Then: command succeeds and output contains the job status
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, jobID)
	assert.Contains(t, output, "running")
}

// TestOrchestraStatusCmd_InvalidJob verifies that the status subcommand returns
// a "job not found" error for a non-existent job ID.
func TestOrchestraStatusCmd_InvalidJob(t *testing.T) {
	t.Parallel()

	// Given: an empty job directory (no jobs saved)
	jobDir := t.TempDir()

	// When: orchestra status command is executed with non-existent job ID
	cmd := newLegacyOrchestraJobStatusCmd()
	cmd.SetArgs([]string{"nonexistent-job", "--job-dir", jobDir})
	err := cmd.Execute()

	// Then: command returns an error indicating the job was not found
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestOrchestraResultCmd_AllDone verifies that the result subcommand merges
// and outputs results when all providers have completed.
func TestOrchestraResultCmd_AllDone(t *testing.T) {
	t.Parallel()

	// Given: a job directory with a completed job
	jobDir := t.TempDir()
	jobID := "result-done-001"
	jobFile := filepath.Join(jobDir, jobID+".json")
	jobJSON := `{"id":"result-done-001","status":"done","strategy":"consensus",` +
		`"providers":["claude","codex"],` +
		`"results":{"claude":{"provider":"claude","output":"ok"},"codex":{"provider":"codex","output":"ok"}}}`
	require.NoError(t, os.WriteFile(jobFile, []byte(jobJSON), 0o644))

	// When: orchestra result command is executed
	cmd := newLegacyOrchestraJobResultCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{jobID, "--job-dir", jobDir})
	err := cmd.Execute()

	// Then: command succeeds and merged output is printed
	require.NoError(t, err)
	output := buf.String()
	assert.NotEmpty(t, output, "merged result should be printed")
}

// TestOrchestraResultCmd_WithCleanup verifies that the --cleanup flag removes
// the job directory after displaying results.
func TestOrchestraResultCmd_WithCleanup(t *testing.T) {
	t.Parallel()

	// Given: a job directory with a completed job
	jobDir := t.TempDir()
	jobID := "result-cleanup-001"
	jobSubDir := filepath.Join(jobDir, jobID)
	require.NoError(t, os.MkdirAll(jobSubDir, 0o755))
	jobFile := filepath.Join(jobDir, jobID+".json")
	jobJSON := `{"id":"result-cleanup-001","status":"done","strategy":"consensus",` +
		`"providers":["claude"],` +
		`"results":{"claude":{"provider":"claude","output":"done"}}}`
	require.NoError(t, os.WriteFile(jobFile, []byte(jobJSON), 0o644))

	// When: orchestra result command is executed with --cleanup
	cmd := newLegacyOrchestraJobResultCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{jobID, "--job-dir", jobDir, "--cleanup"})
	err := cmd.Execute()

	// Then: command succeeds and job directory is removed
	require.NoError(t, err)
	_, statErr := os.Stat(jobSubDir)
	assert.True(t, os.IsNotExist(statErr), "job directory should be removed after --cleanup")
}

// TestOrchestraWaitCmd_CompletedJob verifies that the wait subcommand returns
// immediately when the job is already done.
func TestOrchestraWaitCmd_CompletedJob(t *testing.T) {
	t.Parallel()

	// Given: a job directory with a completed job
	jobDir := t.TempDir()
	jobID := "wait-done-001"
	jobFile := filepath.Join(jobDir, jobID+".json")
	jobJSON := `{"id":"wait-done-001","status":"done","strategy":"consensus",` +
		`"providers":["claude"],` +
		`"results":{"claude":{"provider":"claude","output":"done"}}}`
	require.NoError(t, os.WriteFile(jobFile, []byte(jobJSON), 0o644))

	// When: orchestra wait command is executed for a completed job
	cmd := newLegacyOrchestraJobWaitCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{jobID, "--job-dir", jobDir})
	err := cmd.Execute()

	// Then: command succeeds immediately and outputs the merged result
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "done")
}

// TestOrchestraWaitCmd_InvalidJob verifies that wait returns an error for
// non-existent jobs.
func TestOrchestraWaitCmd_InvalidJob(t *testing.T) {
	t.Parallel()

	// Given: an empty job directory
	jobDir := t.TempDir()

	// When: orchestra wait command is executed with non-existent job ID
	cmd := newLegacyOrchestraJobWaitCmd()
	cmd.SetArgs([]string{"nonexistent", "--job-dir", jobDir})
	err := cmd.Execute()

	// Then: command returns an error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestOrchestraCmd_JobSubcommands verifies that status, wait, and result
// subcommands are registered under the orchestra command.
func TestOrchestraCmd_JobSubcommands(t *testing.T) {
	t.Parallel()

	// Given: the orchestra command
	cmd := newOrchestraCmd()

	// When: checking registered subcommands
	subCmds := cmd.Commands()
	names := make([]string, len(subCmds))
	for i, sc := range subCmds {
		names[i] = sc.Name()
	}

	// Then: job management subcommands should be present
	assert.Contains(t, names, "status", "status subcommand must be registered")
	assert.Contains(t, names, "wait", "wait subcommand must be registered")
	assert.Contains(t, names, "result", "result subcommand must be registered")
}

// TestOrchestraReviewCmd_NoDetachFlag verifies that --no-detach flag is
// available on the review subcommand.
func TestOrchestraReviewCmd_NoDetachFlag(t *testing.T) {
	t.Parallel()

	// Given: the orchestra review command
	cmd := newOrchestraReviewCmd()

	// When: checking for no-detach flag
	flag := cmd.Flags().Lookup("no-detach")

	// Then: flag should exist
	assert.NotNil(t, flag, "--no-detach flag must be registered on review command")
}
