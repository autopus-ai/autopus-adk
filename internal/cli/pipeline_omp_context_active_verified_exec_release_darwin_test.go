//go:build darwin

package cli

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These run the real ptrace gate. On macOS 15.8.2 and 26.6.2 a PT_DETACH from
// the exec stop redelivered SIGTRAP and killed every gated child, including
// /bin/echo, so each case asserts the child finishes on its own terms.

func TestPipelineOMPVerifiedExecCommand_DarwinTracedProbeExitsCleanly(t *testing.T) {
	executable, identity, err := canonicalPipelineOMPExecutable("/bin/echo")
	require.NoError(t, err)
	for attempt := range 20 {
		command, err := newPipelineOMPVerifiedExecCommandContext(context.Background(), executable, identity, "--version")
		require.NoError(t, err)
		require.NoError(t, configurePipelineOMPActiveVersionProbeSandbox(command.cmd, executable))
		require.NoError(t, configureWorkflowContextManagedRPCProcessGroup(command.cmd))
		var output bytes.Buffer
		command.cmd.Stdout = &output
		require.NoError(t, command.Start(), "attempt %d", attempt)
		require.NoError(t, waitPipelineOMPDarwinGateChild(t, command.cmd), "attempt %d", attempt)
		assert.Equal(t, "--version\n", output.String(), "attempt %d", attempt)
	}
}

func TestPipelineOMPVerifiedExecCommand_DarwinGateDetachesLongRunningChild(t *testing.T) {
	executable, identity, err := canonicalPipelineOMPExecutable("/bin/cat")
	require.NoError(t, err)
	for attempt := range 3 {
		command, err := newPipelineOMPVerifiedExecCommandWithGate(context.Background(), executable, identity)
		require.NoError(t, err)
		command.cmd.Path = "/usr/bin/sandbox-exec"
		command.cmd.Args = []string{command.cmd.Path, "-p", "(version 1)\n(allow default)\n(deny network*)\n", executable}
		require.NoError(t, configureWorkflowContextManagedRPCProcessGroup(command.cmd))
		stdin, err := command.cmd.StdinPipe()
		require.NoError(t, err)
		var output bytes.Buffer
		command.cmd.Stdout = &output
		require.NoError(t, command.Start(), "attempt %d", attempt)
		// A still-traced child would stop on this ignored signal and Wait would
		// report the stop instead of the exit.
		require.NoError(t, syscall.Kill(command.cmd.Process.Pid, syscall.SIGWINCH))
		_, err = io.WriteString(stdin, "released\n")
		require.NoError(t, err)
		require.NoError(t, stdin.Close())
		require.NoError(t, waitPipelineOMPDarwinGateChild(t, command.cmd), "attempt %d", attempt)
		assert.Equal(t, "released\n", output.String(), "attempt %d", attempt)
	}
}

func waitPipelineOMPDarwinGateChild(t *testing.T, cmd *exec.Cmd) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil && strings.Contains(err.Error(), "trace/BPT trap") {
			t.Errorf("gated child died of the redelivered exec trap: %v", err)
		}
		return err
	case <-time.After(15 * time.Second):
		_ = terminateWorkflowContextManagedRPCProcessGroup(cmd)
		t.Fatalf("gated child pid %d did not finish after the gate released it", cmd.Process.Pid)
		return nil
	}
}
