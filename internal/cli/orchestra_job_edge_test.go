package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOrchestraWaitCmd_TimeoutStatus verifies that wait returns immediately
// when the job has timed out.
func TestOrchestraWaitCmd_TimeoutStatus(t *testing.T) {
	t.Parallel()

	// Given: a timed-out job
	jobDir := t.TempDir()
	jobID := "wait-timeout-001"
	jobFile := filepath.Join(jobDir, jobID+".json")
	jobJSON := `{"id":"wait-timeout-001","status":"running","strategy":"consensus",` +
		`"providers":["claude"],"results":{},` +
		`"timeout_at":"2020-01-01T00:00:00Z"}`
	require.NoError(t, os.WriteFile(jobFile, []byte(jobJSON), 0o644))

	// When: wait command runs
	cmd := newLegacyOrchestraJobWaitCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{jobID, "--job-dir", jobDir})
	err := cmd.Execute()

	// Then: returns immediately with timeout status
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "timeout")
}

// TestOrchestraResultCmd_CleanupRemovesJobDir verifies that --cleanup removes
// both the job subdirectory and the job's own Dir field.
func TestOrchestraResultCmd_CleanupRemovesJobDir(t *testing.T) {
	t.Parallel()

	// Given: a job with a Dir field pointing to a temp directory
	jobDir := t.TempDir()
	jobOwnDir := t.TempDir()
	jobID := "result-dircleanup-001"
	jobFile := filepath.Join(jobDir, jobID+".json")
	jobJSON := `{"id":"result-dircleanup-001","status":"done","strategy":"consensus",` +
		`"providers":["claude"],"dir":"` + jobOwnDir + `",` +
		`"results":{"claude":{"provider":"claude","output":"ok"}}}`
	require.NoError(t, os.WriteFile(jobFile, []byte(jobJSON), 0o644))

	// When: result command with --cleanup
	cmd := newLegacyOrchestraJobResultCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{jobID, "--job-dir", jobDir, "--cleanup"})
	err := cmd.Execute()

	// Then: job's own Dir and JSON file are removed
	require.NoError(t, err)
	_, statErr := os.Stat(jobOwnDir)
	assert.True(t, os.IsNotExist(statErr), "job's Dir should be removed")
	_, statErr2 := os.Stat(jobFile)
	assert.True(t, os.IsNotExist(statErr2), "job JSON file should be removed")
}

// TestOrchestraStatusCmd_ProviderStates verifies status output shows per-provider
// state (done vs pending).
func TestOrchestraStatusCmd_ProviderStates(t *testing.T) {
	t.Parallel()

	jobDir := t.TempDir()
	jobID := "status-providers-001"
	jobFile := filepath.Join(jobDir, jobID+".json")
	jobJSON := `{"id":"status-providers-001","status":"partial","strategy":"consensus",` +
		`"providers":["claude","codex"],` +
		`"results":{"claude":{"provider":"claude","output":"ok"}}}`
	require.NoError(t, os.WriteFile(jobFile, []byte(jobJSON), 0o644))

	cmd := newLegacyOrchestraJobStatusCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{jobID, "--job-dir", jobDir})
	err := cmd.Execute()

	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "claude: done")
	assert.Contains(t, output, "codex: pending")
}

// TestOrchestraWaitCmd_PollUntilDone verifies that wait polls and returns
// when the job transitions to done during the wait.
func TestOrchestraWaitCmd_PollUntilDone(t *testing.T) {
	t.Parallel()

	// Given: a running job that will be updated to "done" shortly
	jobDir := t.TempDir()
	jobID := "wait-poll-001"
	job := &orchestra.Job{
		ID:        jobID,
		Status:    orchestra.JobStatusRunning,
		Strategy:  orchestra.StrategyConsensus,
		Providers: []string{"claude"},
		Results:   map[string]*orchestra.ProviderResponse{},
		TimeoutAt: time.Now().Add(10 * time.Minute),
		Dir:       jobDir,
	}
	data, _ := json.MarshalIndent(job, "", "  ")
	require.NoError(t, os.WriteFile(filepath.Join(jobDir, jobID+".json"), data, 0o644))

	// Update the job to done after a short delay
	go func() {
		time.Sleep(500 * time.Millisecond)
		job.Results["claude"] = &orchestra.ProviderResponse{Provider: "claude", Output: "ok"}
		job.Status = orchestra.JobStatusDone
		updated, _ := json.MarshalIndent(job, "", "  ")
		_ = os.WriteFile(filepath.Join(jobDir, jobID+".json"), updated, 0o644)
	}()

	// When: wait command runs
	cmd := newLegacyOrchestraJobWaitCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{jobID, "--job-dir", jobDir})
	err := cmd.Execute()

	// Then: should complete after polling with "done" status
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "done")
}

// TestOrchestraResultCmd_InvalidJob verifies that result command returns
// error for non-existent job.
func TestOrchestraResultCmd_InvalidJob(t *testing.T) {
	t.Parallel()

	jobDir := t.TempDir()
	cmd := newLegacyOrchestraJobResultCmd()
	cmd.SetArgs([]string{"nonexistent", "--job-dir", jobDir})
	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}
