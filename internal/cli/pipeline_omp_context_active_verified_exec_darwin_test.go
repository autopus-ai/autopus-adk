//go:build darwin

package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const pipelineOMPPrivatePathOuterSandboxFixture = "AUTOPUS_TEST_OMP_PRIVATE_PATH_OUTER_SANDBOX"

func TestPipelineOMPVerifiedExecCommand_InheritedSandboxUsesVerifiedPrivateCopyWithoutPtrace(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Chmod(root, 0o700))
	executable := filepath.Join(root, "omp")
	copyPipelineOMPVerifiedExecFixture(t, os.Args[0], executable)
	path, identity, err := canonicalPipelineOMPExecutable(executable)
	require.NoError(t, err)
	command, err := newPipelineOMPVerifiedExecCommandContext(context.Background(), path, identity, "--version")
	require.NoError(t, err)
	require.NoError(t, configurePipelineOMPVerifiedExecSandboxMode(
		command, pipelineOMPActiveSandboxInheritedParent, true,
	))
	require.NoError(t, configureWorkflowContextManagedRPCProcessGroup(command.cmd))
	assert.Equal(t, path, command.cmd.Path)
	assert.Empty(t, command.cmd.ExtraFiles)
	require.NotNil(t, command.cmd.SysProcAttr)
	assert.False(t, command.cmd.SysProcAttr.Ptrace)
	var output bytes.Buffer
	command.cmd.Stdout = &output
	require.NoError(t, command.Start())
	require.NoError(t, command.cmd.Wait())
	assert.Equal(t, "omp/17.2.7\n", output.String())
}

func TestPipelineOMPVerifiedExecCommand_InheritedSandboxRejectsPrivateCopyIdentityDrift(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Chmod(root, 0o700))
	executable := filepath.Join(root, "omp")
	copyPipelineOMPVerifiedExecFixture(t, os.Args[0], executable)
	path, identity, err := canonicalPipelineOMPExecutable(executable)
	require.NoError(t, err)
	command, err := newPipelineOMPVerifiedExecCommandContext(context.Background(), path, identity, "--version")
	require.NoError(t, err)
	require.NoError(t, configurePipelineOMPVerifiedExecSandboxMode(
		command, pipelineOMPActiveSandboxInheritedParent, true,
	))
	replacement := executable + ".replacement"
	copyPipelineOMPVerifiedExecFixture(t, "/usr/bin/false", replacement)
	require.NoError(t, os.Rename(replacement, executable))
	require.ErrorContains(t, command.Start(), "identity changed before launch")
	assert.Nil(t, command.cmd.Process)
}

func TestPipelineOMPVerifiedExecCommand_InheritedSandboxRejectsSharedExecutableRoot(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Chmod(root, 0o755))
	executable := filepath.Join(root, "omp")
	copyPipelineOMPVerifiedExecFixture(t, os.Args[0], executable)
	path, identity, err := canonicalPipelineOMPExecutable(executable)
	require.NoError(t, err)
	command, err := newPipelineOMPVerifiedExecCommandContext(context.Background(), path, identity, "--version")
	require.NoError(t, err)

	require.ErrorContains(t, configurePipelineOMPVerifiedExecSandboxMode(
		command, pipelineOMPActiveSandboxInheritedParent, true,
	), "private OMP root is unsafe")
	require.NoError(t, command.Close())
}

func TestPipelineOMPVerifiedExecCommand_InheritedSandboxRejectsPostStartIdentityDrift(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Chmod(root, 0o700))
	executable := filepath.Join(root, "omp")
	copyPipelineOMPVerifiedExecFixture(t, os.Args[0], executable)
	path, identity, err := canonicalPipelineOMPExecutable(executable)
	require.NoError(t, err)
	command, err := newPipelineOMPVerifiedExecCommandContext(context.Background(), path, identity, "--version")
	require.NoError(t, err)
	require.NoError(t, configurePipelineOMPVerifiedExecSandboxMode(
		command, pipelineOMPActiveSandboxInheritedParent, true,
	))
	replacement := executable + ".replacement"
	copyPipelineOMPVerifiedExecFixture(t, "/usr/bin/false", replacement)
	command.afterInheritedDarwinStart = func() { require.NoError(t, os.Rename(replacement, executable)) }

	require.ErrorContains(t, command.Start(), "identity changed before launch")
	require.NotNil(t, command.cmd.ProcessState)
}

func TestPipelineOMPVerifiedExecCommand_InheritedSandboxRunsInsideDenyDefaultOuterSandbox(t *testing.T) {
	if os.Getenv(pipelineOMPPrivatePathOuterSandboxFixture) != "" {
		assertPipelineOMPPrivatePathRunsInOuterSandbox(t)
		return
	}
	root := t.TempDir()
	require.NoError(t, os.Chmod(root, 0o700))
	executable := filepath.Join(root, "omp")
	copyPipelineOMPVerifiedExecFixture(t, os.Args[0], executable)
	const profile = `(version 1)
(deny default)
(allow file-read*)
(allow file-write* (literal "/dev/null"))
(allow process*)
(allow mach*)
(allow sysctl-read)
`
	assert.NotContains(t, profile, "no-sandbox")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := sandboxCoverageFixtureCommand(t, ctx, profile,
		"TestPipelineOMPVerifiedExecCommand_InheritedSandboxRunsInsideDenyDefaultOuterSandbox")
	cmd.Env = append(cmd.Env, pipelineOMPPrivatePathOuterSandboxFixture+"="+executable)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, strings.TrimSpace(string(output)))
}

func assertPipelineOMPPrivatePathRunsInOuterSandbox(t *testing.T) {
	t.Helper()
	executable := os.Getenv(pipelineOMPPrivatePathOuterSandboxFixture)
	path, identity, err := canonicalPipelineOMPExecutable(executable)
	require.NoError(t, err)
	command, err := newPipelineOMPVerifiedExecCommandContext(context.Background(), path, identity, "--version")
	require.NoError(t, err)
	require.NoError(t, configurePipelineOMPVerifiedExecSandboxMode(
		command, pipelineOMPActiveSandboxInheritedParent, true,
	))
	require.NoError(t, configureWorkflowContextManagedRPCProcessGroup(command.cmd))
	var output bytes.Buffer
	command.cmd.Stdout = &output
	require.NoError(t, command.Start())
	require.NoError(t, command.cmd.Wait())
	assert.Equal(t, "omp/17.2.7\n", output.String())
}

func copyPipelineOMPVerifiedExecFixture(t *testing.T, source, destination string) {
	t.Helper()
	input, err := os.Open(source)
	require.NoError(t, err)
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
	require.NoError(t, err)
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	require.NoError(t, copyErr)
	require.NoError(t, closeErr)
}
