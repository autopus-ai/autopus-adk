package omp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/processprobe"
)

type reasonOMPProbeRunner struct {
	err   error
	block time.Duration
}

func (runner reasonOMPProbeRunner) Run(ctx context.Context, _ string, _ ...string) ([]byte, error) {
	if runner.block > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(runner.block):
		}
	}
	return nil, runner.err
}

// Guards against misclassifying probe failures: a timeout, an output-limit
// breach, and a nonzero exit must stay distinguishable in the diagnostics.
func TestRunOMPProbe_ClassifiesFailureReasons(t *testing.T) {
	base := OMPReadinessOptions{Executable: "omp", Root: ".", Timeout: 50 * time.Millisecond, MaxOutput: 32}

	timedOut := base
	timedOut.Runner = reasonOMPProbeRunner{block: time.Minute}
	assert.Equal(t, "timeout", runOMPProbe(context.Background(), timedOut, "--version").reason)

	limited := base
	limited.Runner = reasonOMPProbeRunner{err: processprobe.ErrOutputLimit}
	assert.Equal(t, "output_oversized", runOMPProbe(context.Background(), limited, "--version").reason)

	failed := base
	failed.Runner = reasonOMPProbeRunner{err: errors.New("exit status 2")}
	assert.Equal(t, "exit_nonzero", runOMPProbe(context.Background(), failed, "--version").reason)
}

type oversizedOMPProbeRunner struct{ size int }

func (runner oversizedOMPProbeRunner) Run(context.Context, string, ...string) ([]byte, error) {
	return make([]byte, runner.size), nil
}

// Guards against trusting an over-budget transcript and against leaking its
// bytes into the report once the budget is exceeded.
func TestRunOMPProbe_DiscardsOversizedSuccessfulOutput(t *testing.T) {
	result := runOMPProbe(context.Background(), OMPReadinessOptions{
		Executable: "omp", Root: ".", Timeout: time.Second, MaxOutput: 8,
		Runner: oversizedOMPProbeRunner{size: 9},
	}, "--help")
	assert.Equal(t, "output_oversized", result.reason)
	assert.Empty(t, result.output)
}

// Guards against a readiness report that stops short after identity failure:
// every declared capability ID must still be present with the shared reason,
// so a consumer never mistakes absence for support.
func TestProbeOMPReadiness_ReportsEveryCapabilityAfterIdentityFailure(t *testing.T) {
	runner := &providerFreeReadinessRunner{outputs: map[string][]byte{
		"--version": []byte("codex-cli 1.2.3\n"),
	}}

	report := ProbeOMPReadiness(context.Background(), OMPReadinessOptions{
		Executable: "omp", Root: ".", Runner: runner,
	})

	require.Len(t, report.Capabilities, len(ompReadinessCapabilityIDs))
	assert.Empty(t, report.Version)
	for index, capability := range report.Capabilities {
		assert.Equal(t, ompReadinessCapabilityIDs[index], capability.ID)
		assert.False(t, capability.Supported)
	}
	assert.Equal(t, "output_invalid", report.Capabilities[0].Reason)
	for _, capability := range report.Capabilities[1:] {
		assert.Equal(t, "identity_unverified", capability.Reason,
			"%s must name identity as the missing evidence", capability.ID)
	}
	assert.Len(t, runner.calls, 1, "identity failure must stop further probing")
}
