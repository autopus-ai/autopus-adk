package orchestra

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubSingleProviderCommands replaces the raw subprocess runner for one test
// and counts every command it is asked to build.
func stubSingleProviderCommands(t *testing.T, build func(name string, args []string) command) *atomic.Int32 {
	t.Helper()
	original, originalGrace := newCommand, providerWaitGracePeriod
	t.Cleanup(func() { newCommand, providerWaitGracePeriod = original, originalGrace })
	providerWaitGracePeriod = 20 * time.Millisecond
	calls := &atomic.Int32{}
	newCommand = func(_ context.Context, name string, args ...string) command {
		calls.Add(1)
		return build(name, args)
	}
	return calls
}

func TestRunSingleProvider_OMPProviderRunsOnItsRegisteredBackendOnly(t *testing.T) {
	calls := stubSingleProviderCommands(t, func(string, []string) command {
		t.Fatal("an OMP-backed provider must never reach the raw subprocess runner")
		return nil
	})
	backend := &configuredProviderBackendFake{response: ProviderResponse{Output: "### Summary\nrouted"}}
	provider := ProviderConfig{
		Name: "claude", Backend: "omp", Binary: "omp", Tools: []string{"glob", "grep", "read"},
		SandboxMode: SandboxModeReadOnly, ExecutionTimeout: 7 * time.Second,
	}
	cfg := OrchestraConfig{ProviderBackends: map[string]ExecutionBackend{"omp": backend}, ProviderWorkDir: "/work/project"}

	response, err := RunSingleProvider(context.Background(), cfg, provider, "diagnose prompt")

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, "### Summary\nrouted", response.Output)
	assert.Equal(t, "omp", response.ExecutedBackend)
	assert.Zero(t, calls.Load())
	request := backend.lastRequest(t)
	assert.Equal(t, "diagnose prompt", request.Prompt)
	assert.Equal(t, 1, request.Round)
	assert.Equal(t, 7*time.Second, request.Timeout)
	assert.Equal(t, []string{"glob", "grep", "read"}, request.Config.Tools)
	assert.Equal(t, "/work/project", request.Config.WorkDir)
}

func TestRunSingleProvider_MissingRouteFailsClosedBeforeAnyRunner(t *testing.T) {
	calls := stubSingleProviderCommands(t, func(string, []string) command {
		t.Fatal("a provider without its route must not fall back to the raw subprocess runner")
		return nil
	})
	provider := ProviderConfig{Name: "claude", Backend: "omp", Binary: "claude"}

	response, err := RunSingleProvider(context.Background(), OrchestraConfig{}, provider, "prompt")

	assert.Nil(t, response)
	require.ErrorIs(t, err, ErrBackendUnavailable)
	assert.Contains(t, err.Error(), `provider claude backend "omp"`)
	assert.Zero(t, calls.Load())
}

func TestRunSingleProvider_NativeProviderRunsOnceInItsWorkDir(t *testing.T) {
	var built *fakeCommand
	var gotName string
	var gotArgs []string
	calls := stubSingleProviderCommands(t, func(name string, args []string) command {
		gotName, gotArgs = name, args
		waitCh := make(chan error, 1)
		waitCh <- nil
		built = &fakeCommand{waitCh: waitCh, startFn: func(cmd *fakeCommand) error {
			_, _ = io.WriteString(cmd.stdout, "native diagnosis")
			return nil
		}}
		return built
	})
	provider := ProviderConfig{Name: "claude", Binary: "claude", Args: []string{"--print", "--tools=Read,Grep,Glob"}}
	cfg := OrchestraConfig{ProviderWorkDir: "/work/project", TimeoutSeconds: 5}

	response, err := RunSingleProvider(context.Background(), cfg, provider, "native prompt")

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, int32(1), calls.Load())
	assert.Equal(t, "claude", gotName)
	assert.Equal(t, []string{"--print", "--tools=Read,Grep,Glob"}, gotArgs)
	assert.Equal(t, "/work/project", built.dir)
	assert.Equal(t, "native prompt", built.stdinBuf.String())
	assert.Equal(t, "native diagnosis", response.Output)
	assert.Equal(t, "subprocess", response.ExecutedBackend)
	assert.False(t, response.TimedOut)
}

func TestRunSingleProvider_ExecutionTimeoutBoundsTheCall(t *testing.T) {
	stubSingleProviderCommands(t, func(string, []string) command {
		// The process "exits" by itself after 3 s, so an unbounded call
		// fails the assertions below instead of hanging the test.
		waitCh := make(chan error, 2)
		exited := time.AfterFunc(3*time.Second, func() { waitCh <- nil })
		t.Cleanup(func() { exited.Stop() })
		return &fakeCommand{waitCh: waitCh, exitCode: -1, terminateFn: func(*fakeCommand, string) error {
			waitCh <- context.DeadlineExceeded
			return nil
		}}
	})
	provider := ProviderConfig{Name: "codex", Binary: "codex", ExecutionTimeout: 50 * time.Millisecond}

	start := time.Now()
	response, err := RunSingleProvider(context.Background(), OrchestraConfig{TimeoutSeconds: 600}, provider, "prompt")

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.True(t, response.TimedOut)
	assert.Less(t, time.Since(start), 2*time.Second)
}
