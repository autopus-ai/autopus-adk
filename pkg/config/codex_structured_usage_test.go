package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCodexProviderEntryForQuality_StructuredUsageExactlyOnce(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{"balanced", "ultra"} {
		entry := CodexProviderEntryForQuality(QualityConf{Default: mode})
		assert.Equal(t, 1, countString(entry.Args, "--json"), "%s subprocess args", mode)
	}
}

func TestApplyCodexProviderProfile_PreservesStructuredAndCustomArgs(t *testing.T) {
	t.Parallel()
	entry := ProviderEntry{
		Args: []string{"exec", "--json", "--custom-flag", "custom-value", "-m", "old-model"},
	}

	got := ApplyCodexProviderProfile(entry, CodexProfile{Model: "new-model", Effort: "high"})

	assert.Equal(t, 1, countString(got.Args, "--json"))
	assert.Contains(t, got.Args, "--custom-flag")
	assert.Contains(t, got.Args, "custom-value")
}

func TestApplyCodexProviderProfile_NormalizesStructuredUsageExactlyOnce(t *testing.T) {
	t.Parallel()
	entry := ProviderEntry{
		ModelPolicy: ProviderModelPolicyQuality,
		Args:        []string{"exec", "--json", "--custom", "--json"},
	}

	got := ApplyCodexProviderProfile(entry, CodexProfile{Model: "gpt-5.4", Effort: "high"})

	assert.Equal(t, 1, countString(got.Args, "--json"))
	assert.Equal(t, []string{"exec", "--json", "--custom", "-m", "gpt-5.4", "-c", `model_reasoning_effort="high"`}, got.Args)
}

func TestApplyCodexProviderProfile_AddsStructuredUsageBeforeTerminator(t *testing.T) {
	t.Parallel()
	entry := ProviderEntry{Args: []string{"exec", "--sandbox", "workspace-write", "--", "child", "--json"}}

	got := ApplyCodexProviderProfile(entry, CodexProfile{})

	assert.Equal(t, []string{"exec", "--json", "--sandbox", "workspace-write", "--", "child", "--json"}, got.Args)
}

func countString(values []string, target string) int {
	count := 0
	for _, value := range values {
		if value == target {
			count++
		}
	}
	return count
}
