//go:build darwin

package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These run the real Darwin gate on the host kernel. The ptrace gate it
// replaced killed, stalled, or froze gated children on macOS 15.8.2 and
// 26.6.2, so each case asserts the child finishes on its own terms.

func TestPipelineOMPVerifiedExecCommand_DarwinGateProbeExitsCleanly(t *testing.T) {
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
		require.NoError(t, waitPipelineOMPDarwinGateChild(t, command), "attempt %d", attempt)
		assert.Equal(t, "--version\n", output.String(), "attempt %d", attempt)
	}
}

func TestPipelineOMPVerifiedExecCommand_DarwinGateLongRunningChildTakesSignals(t *testing.T) {
	executable, identity, err := canonicalPipelineOMPExecutable("/bin/cat")
	require.NoError(t, err)
	for attempt := range 20 {
		command := startPipelineOMPDarwinGatedLongRunningChild(t, executable, identity)
		stdin, err := command.cmd.StdinPipe()
		require.NoError(t, err)
		var output bytes.Buffer
		command.cmd.Stdout = &output
		require.NoError(t, command.Start(), "attempt %d", attempt)
		// An ignored signal must leave the child running; a traced child would
		// stop on it and the write below would never echo.
		require.NoError(t, syscall.Kill(command.cmd.Process.Pid, syscall.SIGWINCH))
		_, err = io.WriteString(stdin, "released\n")
		require.NoError(t, err)
		require.NoError(t, stdin.Close())
		require.NoError(t, waitPipelineOMPDarwinGateChild(t, command), "attempt %d", attempt)
		assert.Equal(t, "released\n", output.String(), "attempt %d", attempt)
	}
}

func TestPipelineOMPVerifiedExecCommand_DarwinGateLongRunningChildDiesOfTermination(t *testing.T) {
	executable, identity, err := canonicalPipelineOMPExecutable("/bin/sleep")
	require.NoError(t, err)
	command := startPipelineOMPDarwinGatedLongRunningChild(t, executable, identity, "30")
	require.NoError(t, command.Start())
	require.NoError(t, syscall.Kill(command.cmd.Process.Pid, syscall.SIGTERM))
	err = waitPipelineOMPDarwinGateChild(t, command)
	require.Error(t, err)
	status, ok := command.cmd.ProcessState.Sys().(syscall.WaitStatus)
	require.True(t, ok)
	assert.True(t, status.Signaled(), "SIGTERM must end the child, not stop it: %v", status)
	assert.Equal(t, syscall.SIGTERM, status.Signal())
}

func startPipelineOMPDarwinGatedLongRunningChild(
	t *testing.T,
	executable string,
	identity pipelineOMPExecutableIdentity,
	args ...string,
) *pipelineOMPVerifiedExecCommand {
	t.Helper()
	command, err := newPipelineOMPVerifiedExecCommandWithGate(context.Background(), executable, identity, args...)
	require.NoError(t, err)
	command.cmd.Path = "/usr/bin/sandbox-exec"
	command.cmd.Args = append([]string{
		command.cmd.Path, "-p", "(version 1)\n(allow default)\n(deny network*)\n", executable,
	}, args...)
	require.NoError(t, configureWorkflowContextManagedRPCProcessGroup(command.cmd))
	return command
}

func waitPipelineOMPDarwinGateChild(t *testing.T, command *pipelineOMPVerifiedExecCommand) error {
	t.Helper()
	deadline := time.After(15 * time.Second)
	done := make(chan error, 1)
	go func() { done <- command.cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil && strings.Contains(err.Error(), "trace/BPT trap") {
			t.Errorf("gated child died of the redelivered exec trap: %v", err)
		}
		return err
	case <-deadline:
		_ = terminateWorkflowContextManagedRPCProcessGroup(command.cmd)
		t.Fatalf("gated child pid %d did not finish after the gate released it", command.cmd.Process.Pid)
		return nil
	}
}
