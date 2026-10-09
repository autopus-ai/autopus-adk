package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

func TestResolveOrchestraTimeout_ConfigProvenanceAndProviderOverrides(t *testing.T) {
	t.Parallel()

	conf := &config.OrchestraConf{
		TimeoutSeconds: 120,
		Providers: map[string]config.ProviderEntry{
			"claude": {Binary: "claude"},
			"gemini": {
				Binary: "gemini",
				Subprocess: config.SubprocessProvConf{
					Timeout: 45,
				},
			},
		},
	}
	providers := []orchestra.ProviderConfig{
		{Name: "claude", Binary: "claude"},
		{Name: "gemini", Binary: "gemini"},
	}

	resolved := resolveOrchestraTimeout(conf, 300, false, providers)

	assert.Equal(t, 120, resolved.Seconds)
	assert.Equal(t, "autopus.yaml orchestra.timeout_seconds", resolved.Source)
	require.Len(t, resolved.Providers, 2)
	assert.Equal(t, "claude", resolved.Providers[0].Provider)
	assert.Equal(t, 120*time.Second, resolved.Providers[0].Duration)
	assert.Equal(t, "autopus.yaml orchestra.timeout_seconds", resolved.Providers[0].Source)
	assert.Equal(t, "gemini", resolved.Providers[1].Provider)
	assert.Equal(t, 45*time.Second, resolved.Providers[1].Duration)
	assert.Equal(t, "autopus.yaml orchestra.providers.gemini.subprocess.timeout", resolved.Providers[1].Source)
}

func TestResolveOrchestraTimeout_FlagOverridesConfig(t *testing.T) {
	t.Parallel()

	conf := &config.OrchestraConf{TimeoutSeconds: 120}

	resolved := resolveOrchestraTimeout(conf, 75, true, []orchestra.ProviderConfig{{Name: "claude", Binary: "claude"}})

	assert.Equal(t, 75, resolved.Seconds)
	assert.Equal(t, "flag --timeout", resolved.Source)
	require.Len(t, resolved.Providers, 1)
	assert.Equal(t, 75*time.Second, resolved.Providers[0].Duration)
	assert.Equal(t, "flag --timeout", resolved.Providers[0].Source)
}

func TestSaveOrchestraFailureReport_WritesStructuredArtifact(t *testing.T) {
	dir := t.TempDir()
	original, err := os.Getwd()
	require.NoError(t, err)
	defer func() {
		chdirErr := os.Chdir(original)
		require.NoError(t, chdirErr)
	}()
	require.NoError(t, os.Chdir(dir))

	resolved := ResolvedOrchestraTimeout{
		Seconds: 120,
		Source:  "autopus.yaml orchestra.timeout_seconds",
		Providers: []ResolvedProviderTimeout{
			{Provider: "claude", Duration: 120 * time.Second, Source: "autopus.yaml orchestra.timeout_seconds"},
			{Provider: "gemini", Duration: 45 * time.Second, Source: "autopus.yaml orchestra.providers.gemini.subprocess.timeout"},
		},
	}
	result := &orchestra.OrchestraResult{
		Strategy: orchestra.StrategyDebate,
		Duration: 2 * time.Second,
		Summary:  "all providers failed: claude(timeout), gemini(capacity_exhausted)",
		FailedProviders: []orchestra.FailedProvider{
			{
				Name:             "claude",
				Error:            "timeout: provider exceeded 2m0s deadline",
				FailureClass:     "timeout",
				NextRemediation:  "increase timeout or simplify strategy",
				CorrelationRunID: "run-test",
			},
			{
				Name:             "gemini",
				Error:            "gemini fast-fail: provider capacity exhausted",
				FailureClass:     "capacity_exhausted",
				NextRemediation:  "retry later or reduce provider set",
				CorrelationRunID: "run-test",
			},
		},
		RunID: "run-test",
	}

	path, err := saveOrchestraFailureReport(
		"brainstorm",
		"debate",
		[]string{"claude", "gemini"},
		resolved,
		result,
		assert.AnError,
	)
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(path))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "\"effective_timeout\"")
	assert.Contains(t, string(data), "\"source\": \"autopus.yaml orchestra.timeout_seconds\"")
	assert.Contains(t, string(data), "\"failure_class\": \"timeout\"")
	assert.Contains(t, string(data), "\"failure_class\": \"capacity_exhausted\"")
}

func TestSaveOrchestraResult_DegradedRunWritesProviderDiagnostics(t *testing.T) {
	dir := t.TempDir()
	original, err := os.Getwd()
	require.NoError(t, err)
	defer func() {
		chdirErr := os.Chdir(original)
		require.NoError(t, chdirErr)
	}()
	require.NoError(t, os.Chdir(dir))

	resolved := ResolvedOrchestraTimeout{
		Seconds: 600,
		Source:  "flag --timeout",
		Providers: []ResolvedProviderTimeout{
			{Provider: "claude", Duration: 600 * time.Second, Source: "flag --timeout"},
			{Provider: "codex", Duration: 420 * time.Second, Source: "autopus.yaml orchestra.providers.codex.subprocess.timeout"},
			{Provider: "gemini", Duration: 600 * time.Second, Source: "flag --timeout"},
		},
	}
	result := &orchestra.OrchestraResult{
		Strategy: orchestra.StrategyDebate,
		Merged:   "merged brainstorm output",
		Duration: 2 * time.Minute,
		Summary:  "토론 완료 (실패: codex, gemini)",
		FailedProviders: []orchestra.FailedProvider{
			{
				Name:            "codex",
				Error:           "timeout: provider exceeded 7m0s deadline",
				FailureClass:    "timeout",
				NextRemediation: "increase timeout or simplify strategy",
				StderrPreview:   "context deadline exceeded",
			},
			{
				Name:            "gemini",
				Error:           "subprocess gemini exited with code 1",
				FailureClass:    "capacity_exhausted",
				NextRemediation: "retry later or reduce provider set",
				StderrPreview:   "RESOURCE_EXHAUSTED MODEL_CAPACITY_EXHAUSTED",
				OutputPreview:   "No capacity available for model",
			},
		},
	}

	path, err := saveOrchestraResult(
		"brainstorm",
		"debate",
		[]string{"claude", "codex", "gemini"},
		resolved,
		result,
	)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	text := string(data)
	assert.Contains(t, text, "**Status**: degraded")
	assert.Contains(t, text, "**Effective Timeout**: 600s (flag --timeout)")
	assert.Contains(t, text, "## Provider Diagnostics")
	assert.Contains(t, text, "| codex | timeout |")
	assert.Contains(t, text, "7m0s")
	assert.Contains(t, text, "autopus.yaml orchestra.providers.codex.subprocess.timeout")
	assert.Contains(t, text, "increase timeout or simplify strategy")
	assert.Contains(t, text, "RESOURCE_EXHAUSTED MODEL_CAPACITY_EXHAUSTED")
	assert.Contains(t, text, "merged brainstorm output")
}

func TestSaveOrchestraDegradedReport_WritesSidecarJSON(t *testing.T) {
	dir := t.TempDir()
	original, err := os.Getwd()
	require.NoError(t, err)
	defer func() {
		chdirErr := os.Chdir(original)
		require.NoError(t, chdirErr)
	}()
	require.NoError(t, os.Chdir(dir))

	resolved := ResolvedOrchestraTimeout{
		Seconds: 600,
		Source:  "flag --timeout",
		Providers: []ResolvedProviderTimeout{
			{Provider: "codex", Duration: 420 * time.Second, Source: "autopus.yaml orchestra.providers.codex.subprocess.timeout"},
		},
	}
	result := &orchestra.OrchestraResult{
		Strategy: orchestra.StrategyDebate,
		Duration: time.Minute,
		Summary:  "토론 완료 (실패: codex)",
		FailedProviders: []orchestra.FailedProvider{
			{
				Name:            "codex",
				Error:           "timeout: provider exceeded 7m0s deadline",
				FailureClass:    "timeout",
				NextRemediation: "increase timeout or simplify strategy",
				StderrPreview:   "context deadline exceeded",
			},
		},
	}

	path, err := saveOrchestraDegradedReport("brainstorm", "debate", []string{"codex"}, resolved, result)
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(path))
	assert.Contains(t, filepath.Base(path), "degraded-brainstorm-debate-")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	text := string(data)
	assert.Contains(t, text, "\"failed_providers\"")
	assert.Contains(t, text, "\"failure_class\": \"timeout\"")
	assert.Contains(t, text, "\"retry_hints\"")
	assert.Contains(t, text, "\"source\": \"autopus.yaml orchestra.providers.codex.subprocess.timeout\"")
}
