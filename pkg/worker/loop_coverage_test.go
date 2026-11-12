// Package worker - coverage tests for pure functions and mockable paths.
package worker

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/insajin/autopus-adk/pkg/worker/a2a"
	"github.com/insajin/autopus-adk/pkg/worker/budget"
	"github.com/insajin/autopus-adk/pkg/worker/security"
	"github.com/insajin/autopus-adk/pkg/worker/workspace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- RouteWorkDir ---

func TestRouteWorkDir_NilMultiWorkspace(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "/default", RouteWorkDir(nil, "ws1", "/default"))
}

func TestRouteWorkDir_EmptyWorkspaceID(t *testing.T) {
	t.Parallel()
	mw := workspace.NewMultiWorkspace()
	assert.Equal(t, "/default", RouteWorkDir(mw, "", "/default"))
}

func TestRouteWorkDir_WorkspaceNotFound(t *testing.T) {
	t.Parallel()
	mw := workspace.NewMultiWorkspace()
	// No workspaces registered — RouteTask will fail.
	result := RouteWorkDir(mw, "unknown-ws", "/fallback")
	assert.Equal(t, "/fallback", result)
}

func TestRouteWorkDir_WorkspaceFound(t *testing.T) {
	t.Parallel()
	mw := workspace.NewMultiWorkspace()
	mw.Add(workspace.WorkspaceConn{
		WorkspaceID: "ws1",
		ProjectDir:  "/projects/ws1",
		Connected:   true,
	})
	result := RouteWorkDir(mw, "ws1", "/fallback")
	assert.Equal(t, "/projects/ws1", result)
}

// --- cleanupPolicy ---

func TestCleanupPolicy_NoFile(t *testing.T) {
	t.Parallel()
	// Should not panic even when file doesn't exist.
	cleanupPolicy("nonexistent-task-id-12345")
}

func TestCleanupPolicy_FileExists(t *testing.T) {
	t.Parallel()

	taskID := "test-cleanup-task"
	require.NoError(t, security.NewPolicyCache().Write(taskID, security.SecurityPolicy{TimeoutSec: 1}))
	path := security.NewPolicyCache().PolicyPath(taskID)

	cleanupPolicy(taskID)

	// File should be removed.
	_, err := os.Stat(path)
	assert.True(t, os.IsNotExist(err), "policy file should be removed")
}

// --- StdinWriter.Write ---

func TestStdinWriter_Write(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	sw := NewStdinWriter(nopWriteCloser{&buf})

	n, err := sw.Write([]byte("hello"))
	assert.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", buf.String())
}

func TestStdinWriter_WritePromptAndWrite(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	sw := NewStdinWriter(nopWriteCloser{&buf})

	err := sw.WritePrompt("prompt> ")
	assert.NoError(t, err)

	_, err = sw.Write([]byte("follow-up"))
	assert.NoError(t, err)

	assert.Equal(t, "prompt> follow-up", buf.String())
}

// --- handleApproval edge cases ---

func TestHandleApproval_NilTUI(t *testing.T) {
	t.Parallel()

	// WorkerLoop with no TUI program — should not panic.
	wl := &WorkerLoop{config: LoopConfig{}}
	wl.handleApproval(a2a.ApprovalRequestParams{
		TaskID:    "task-1",
		Action:    "write_file",
		RiskLevel: "high",
		Context:   "context",
	})
	// No panic = pass.
}

// --- stopServices ---

func TestStopServices_NilCancel(t *testing.T) {
	t.Parallel()
	// WorkerLoop with nil lifecycleCancel and nil auditWriter — should not panic.
	wl := &WorkerLoop{config: LoopConfig{}}
	wl.stopServices() // no-op, no panic
}

// --- PipelineExecutor.SetBudget ---

func TestPipelineExecutor_SetBudget(t *testing.T) {
	t.Parallel()

	pe := NewPipelineExecutor(
		&mockAdapter{name: "test", script: "echo ok"},
		"",
		t.TempDir(),
	)
	require.NotNil(t, pe)

	pe.SetBudget(100, budget.DefaultAllocation())
	// Verify allocator is set (no public getter, so just check no panic).
}

// --- Helpers ---

// nopWriteCloser wraps an io.Writer as an io.WriteCloser with no-op Close.
type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error { return nil }
