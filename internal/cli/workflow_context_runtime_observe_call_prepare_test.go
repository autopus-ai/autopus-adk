package cli

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func observeCallPrepareOptions(t *testing.T) workflowContextObserveCallOptions {
	t.Helper()
	project := t.TempDir()
	writeWorkflowContextObserveCanonicalDocuments(t, project, "SPEC-OMP-004")
	return workflowContextObserveCallOptions{
		ProjectDir: project, SpecID: "SPEC-OMP-004", Provider: "openai", Model: "gpt-5.6-sol",
		Endpoint: "http://127.0.0.1:43123/", CredentialLocator: "AUTOPUS_TEST_OBSERVE_PREPARE_TOKEN",
		Executable: os.Args[0],
	}
}

// The managed observe runtime requires the darwin-only network sandbox
// (workflow_context_runtime_managed_rpc_sandbox_darwin.go); elsewhere
// configureWorkflowContextManagedRPCSandbox refuses by design, so these cases
// would assert a sandbox the platform does not have.
func requireManagedObserveSandbox(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("managed OMP network sandbox is darwin-only")
	}
}

// The observe-call task root must be a fully isolated runtime: the child may not
// see the user's HOME, caches, or the real project directory.
func TestPrepareWorkflowContextObserveCall_IsolatesRuntimeAndProvesInstalledIdentity(t *testing.T) {
	requireManagedObserveSandbox(t)
	options := observeCallPrepareOptions(t)
	t.Setenv(options.CredentialLocator, "observe-prepare-secret-token")

	setup, err := prepareWorkflowContextObserveCall(context.Background(),
		workflowContextObserveCallRequest{TaskID: "task-01", Variant: "full"}, options)
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(setup.taskRoot) })

	assert.Equal(t, "omp/17.2.7", setup.version)
	require.NotEmpty(t, setup.taskRoot)
	info, err := os.Lstat(setup.taskRoot)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())

	// Canonical documents are re-read from the isolated copy, not the live project.
	assert.True(t, strings.HasPrefix(setup.projectDir, setup.taskRoot), "project %q", setup.projectDir)
	assert.NotEqual(t, options.ProjectDir, setup.projectDir)
	body, err := os.ReadFile(filepath.Join(setup.projectDir, "AGENTS.md"))
	require.NoError(t, err)
	assert.Contains(t, string(body), "CANONICAL-AGENT-DOCUMENT")
	assert.NotEmpty(t, setup.delivery.Prompt)
	assert.NotEmpty(t, setup.delivery.RequiredDocuments)

	managed := setup.options
	assert.Equal(t, "openai/gpt-5.6-sol", managed.Model)
	assert.Equal(t, "http://127.0.0.1:43123", managed.AllowedEndpoint)
	assert.Equal(t, 2*time.Minute, managed.MaxTime)
	assert.True(t, managed.CaptureOutput)
	assert.True(t, managed.CaptureStats)
	assert.Equal(t, setup.projectDir, managed.ProjectDir)
	assert.True(t, strings.HasPrefix(managed.RuntimeRoot, setup.taskRoot))
	assert.True(t, strings.HasPrefix(managed.SessionDir, managed.RuntimeRoot))
	assert.True(t, strings.HasPrefix(managed.ConfigPath, managed.RuntimeRoot))

	environment := make(map[string]string, len(managed.Environment))
	for _, entry := range managed.Environment {
		key, value, found := strings.Cut(entry, "=")
		require.True(t, found, "entry %q", entry)
		_, duplicate := environment[key]
		assert.False(t, duplicate, "duplicate environment key %q", key)
		environment[key] = value
	}
	assert.Equal(t, filepath.Join(managed.RuntimeRoot, "home"), environment["HOME"])
	assert.Equal(t, filepath.Join(managed.RuntimeRoot, "tmp"), environment["TMPDIR"])
	assert.Equal(t, filepath.Join(managed.RuntimeRoot, "cache"), environment["XDG_CACHE_HOME"])
	assert.Equal(t, filepath.Join(managed.RuntimeRoot, "state"), environment["XDG_STATE_HOME"])
	assert.Equal(t, managed.RuntimeRoot, environment["PI_CODING_AGENT_DIR"])
	assert.Equal(t, "observe-prepare-secret-token", environment[options.CredentialLocator])
	for _, name := range []string{"home", "tmp", "cache", "config", "data", "state"} {
		entry, statErr := os.Lstat(filepath.Join(managed.RuntimeRoot, name))
		require.NoError(t, statErr)
		assert.Equal(t, os.FileMode(0o700), entry.Mode().Perm(), "runtime dir %q", name)
	}
	authority, err := os.ReadFile(filepath.Join(managed.RuntimeRoot, "models.yml"))
	require.NoError(t, err)
	assert.NotContains(t, string(authority), "observe-prepare-secret-token")
}

func TestPrepareWorkflowContextObserveCall_RefusesBlankCredential(t *testing.T) {
	options := observeCallPrepareOptions(t)
	request := workflowContextObserveCallRequest{TaskID: "task-01", Variant: "full"}

	for _, credential := range []string{"", "   ", "\t\n"} {
		t.Setenv(options.CredentialLocator, credential)
		_, err := prepareWorkflowContextObserveCall(context.Background(), request, options)
		require.ErrorContains(t, err, "credential is unavailable")
	}
}

// A canonical source that is not a regular file must be refused before the
// setup builds any admission payload.
func TestPrepareWorkflowContextObserveCall_RefusesSymlinkedCanonicalDocument(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixture requires POSIX")
	}
	options := observeCallPrepareOptions(t)
	t.Setenv(options.CredentialLocator, "observe-prepare-secret-token")
	decoy := filepath.Join(options.ProjectDir, "decoy.md")
	require.NoError(t, os.WriteFile(decoy, []byte("CANONICAL-AGENT-DOCUMENT\n"), 0o600))
	agents := filepath.Join(options.ProjectDir, "AGENTS.md")
	require.NoError(t, os.Remove(agents))
	require.NoError(t, os.Symlink(decoy, agents))

	_, err := prepareWorkflowContextObserveCall(context.Background(),
		workflowContextObserveCallRequest{TaskID: "task-01", Variant: "full"}, options)
	require.ErrorContains(t, err, "must be a regular file")
}

// A runtime whose identity probe fails must not leave its task root on disk.
func TestPrepareWorkflowContextObserveCall_RemovesTaskRootWhenIdentityProbeFails(t *testing.T) {
	requireManagedObserveSandbox(t)
	options := observeCallPrepareOptions(t)
	options.Executable = "/usr/bin/true"
	t.Setenv(options.CredentialLocator, "observe-prepare-secret-token")
	tempRoot := t.TempDir()
	t.Setenv("TMPDIR", tempRoot)

	_, err := prepareWorkflowContextObserveCall(context.Background(),
		workflowContextObserveCallRequest{TaskID: "task-01", Variant: "full"}, options)
	require.ErrorContains(t, err, "identity probe failed")
	entries, readErr := os.ReadDir(tempRoot)
	require.NoError(t, readErr)
	assert.Empty(t, entries, "failed setup leaked a task root")
}

func TestProbeWorkflowContextObserveVersion_RejectsExecutableWithoutInstalledVersionOutput(t *testing.T) {
	requireManagedObserveSandbox(t)
	workspace := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	got, err := probeWorkflowContextObserveVersion(ctx, WorkflowContextManagedRPCOptions{
		Executable: os.Args[0], Workspace: workspace, Environment: []string{"PATH=" + os.Getenv("PATH")},
		AllowedEndpoint: "http://127.0.0.1:43123",
	}, pipelineOMPActiveSandboxManaged)
	require.NoError(t, err)
	assert.Equal(t, "omp/17.2.7", got)

	_, err = probeWorkflowContextObserveVersion(ctx, WorkflowContextManagedRPCOptions{
		Executable: "/usr/bin/true", Workspace: workspace, Environment: []string{"PATH=" + os.Getenv("PATH")},
		AllowedEndpoint: "http://127.0.0.1:43123",
	}, pipelineOMPActiveSandboxManaged)
	require.ErrorContains(t, err, "identity probe failed")
}
