package orchestra

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunOrchestra_AllProvidersFail_ReturnsFailureResult(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	result, err := RunOrchestra(context.Background(), OrchestraConfig{
		Providers: []ProviderConfig{
			emptyOutputProvider("claude"),
			emptyOutputProvider("codex"),
			badArgsProvider("gemini"),
		},
		Strategy:       StrategyConsensus,
		Prompt:         "diagnose orchestra failures",
		TimeoutSeconds: 10,
	})

	require.Error(t, err)
	require.NotNil(t, result)
	require.Len(t, result.FailedProviders, 3)
	assert.Empty(t, result.Responses)
	assert.Contains(t, result.Summary, "all providers failed")
	assert.Contains(t, err.Error(), "모든 프로바이더가 실패했습니다")
}

func TestBuildFailedProvider_ClassifiesTimeoutAndCapacity(t *testing.T) {
	t.Parallel()

	timeoutFailure := buildFailedProvider(
		ProviderConfig{Name: "claude"},
		&ProviderResponse{
			Provider: "claude",
			TimedOut: true,
			Duration: 90 * time.Second,
			Output:   "raw provider output should not be copied",
			Error:    "raw provider stderr should not be copied",
		},
		nil,
		120,
	)
	capacityFailure := buildFailedProvider(
		ProviderConfig{Name: "gemini"},
		&ProviderResponse{Provider: "gemini", Error: "status: RESOURCE_EXHAUSTED reason: MODEL_CAPACITY_EXHAUSTED"},
		assert.AnError,
		120,
	)

	assert.Equal(t, "timeout", timeoutFailure.FailureClass)
	assert.Contains(t, timeoutFailure.Error, "deadline")
	assert.Contains(t, timeoutFailure.NextRemediation, "increase timeout")
	assert.Equal(t, "orchestra_timeout_seconds", timeoutFailure.TimeoutSource)
	assert.Equal(t, 120*time.Second, timeoutFailure.ConfiguredDuration)
	assert.Equal(t, 90*time.Second, timeoutFailure.ElapsedDuration)
	assert.Empty(t, timeoutFailure.StderrPreview)
	assert.Empty(t, timeoutFailure.OutputPreview)
	assert.NotContains(t, timeoutFailure.Error, "raw provider")

	assert.Equal(t, "capacity_exhausted", capacityFailure.FailureClass)
	assert.Contains(t, capacityFailure.NextRemediation, "retry later")
}

func TestBuildFailedProvider_RedactsNonTimeoutFailureEvidence(t *testing.T) {
	t.Parallel()

	failure := buildFailedProvider(
		ProviderConfig{Name: "gemini"},
		&ProviderResponse{
			Provider: "gemini",
			Output:   "SECRET_TOKEN=abc123 generated content",
			Error:    "RESOURCE_EXHAUSTED at /Users/example/private/project",
		},
		fmt.Errorf("provider failed with API_KEY=secret at /Users/example/private/project"),
		120,
	)

	assert.Equal(t, "resource_exhausted", failure.FailureClass)
	assert.Equal(t, "[redacted_provider_output]", failure.OutputPreview)
	assert.Contains(t, failure.StderrPreview, "RESOURCE_EXHAUSTED")
	assert.NotContains(t, failure.StderrPreview, "/Users/example")
	assert.NotContains(t, failure.Error, "API_KEY=secret")
	assert.NotContains(t, failure.Error, "/Users/example")
}
