package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestProviderEntryModelPolicyYAMLRoundTrip(t *testing.T) {
	t.Parallel()

	want := ProviderEntry{
		Binary:      "codex",
		Args:        []string{"exec", "--json"},
		ModelPolicy: ProviderModelPolicyQuality,
	}
	data, err := yaml.Marshal(want)
	require.NoError(t, err)
	assert.Contains(t, string(data), "model_policy: quality")

	var got ProviderEntry
	require.NoError(t, yaml.Unmarshal(data, &got))
	assert.Equal(t, want, got)
}

func TestHarnessConfigValidateRejectsUnknownProviderModelPolicy(t *testing.T) {
	t.Parallel()

	cfg := DefaultFullConfig("invalid-model-policy")
	entry := cfg.Orchestra.Providers["codex"]
	entry.ModelPolicy = "automatic"
	cfg.Orchestra.Providers["codex"] = entry

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "model_policy")
	assert.Contains(t, err.Error(), "quality or pinned")
}

func TestApplyCodexProviderProfilePreservesNonModelArguments(t *testing.T) {
	t.Parallel()

	entry := ProviderEntry{
		Binary:      "codex",
		ModelPolicy: ProviderModelPolicyQuality,
		Args:        []string{"exec", "--json", "-c", `foo="bar"`, "-m", CodexLegacyModel, "-c", `model_reasoning_effort="xhigh"`, "--sandbox", "workspace-write"},
	}

	got := ApplyCodexProviderProfile(entry, CodexProfile{Model: CodexSolModel, Effort: CodexEffortUltra})
	assert.Equal(t, []string{"exec", "--json", "-c", `foo="bar"`, "-m", CodexSolModel, "-c", `model_reasoning_effort="ultra"`, "--sandbox", "workspace-write"}, got.Args)
}

func TestApplyCodexProviderProfileOmitsManagedFieldsForRuntimeDefault(t *testing.T) {
	t.Parallel()

	entry := ProviderEntry{
		Binary:      "codex",
		ModelPolicy: ProviderModelPolicyQuality,
		Args:        []string{"exec", "--sandbox", "workspace-write", "-m", CodexSolModel, "-c", `model_reasoning_effort="ultra"`, "-c", `foo="bar"`},
	}

	got := ApplyCodexProviderProfile(entry, CodexProfile{})
	assert.Equal(t, []string{"exec", "--json", "--sandbox", "workspace-write", "-c", `foo="bar"`}, got.Args)
}

func TestApplyCodexProviderProfilePreservesTerminatorSuffix(t *testing.T) {
	t.Parallel()

	entry := ProviderEntry{
		Binary:      "codex",
		ModelPolicy: ProviderModelPolicyQuality,
		Args: []string{
			"exec", "--model=" + CodexLegacyModel,
			"--", "child", "-m", "child-model", "--config=model_reasoning_effort=low",
		},
	}

	got := ApplyCodexProviderProfile(entry, CodexProfile{Model: CodexSolModel, Effort: CodexEffortUltra})
	assert.Equal(t, []string{
		"exec", "--json", "--model=" + CodexSolModel, "-c", `model_reasoning_effort="ultra"`,
		"--", "child", "-m", "child-model", "--config=model_reasoning_effort=low",
	}, got.Args)

	runtimeDefault := ApplyCodexProviderProfile(entry, CodexProfile{})
	assert.Equal(t, []string{
		"exec", "--json", "--", "child", "-m", "child-model", "--config=model_reasoning_effort=low",
	}, runtimeDefault.Args)
}

func TestApplyCodexProviderProfileHandlesLongConfigOption(t *testing.T) {
	t.Parallel()

	entry := ProviderEntry{
		Binary:      "codex",
		ModelPolicy: ProviderModelPolicyQuality,
		Args: []string{
			"exec", "--model=" + CodexLegacyModel,
			`--config=model_reasoning_effort="xhigh"`, "--config=foo=bar",
		},
	}

	got := ApplyCodexProviderProfile(entry, CodexProfile{Model: CodexSolModel, Effort: CodexEffortUltra})
	assert.Equal(t, []string{
		"exec", "--json", "--model=" + CodexSolModel,
		`--config=model_reasoning_effort="ultra"`, "--config=foo=bar",
	}, got.Args)

	got = ApplyCodexProviderProfile(entry, CodexProfile{})
	assert.Equal(t, []string{"exec", "--json", "--config=foo=bar"}, got.Args)

	// A long effort option without a model keeps its place, and the missing
	// model is appended.
	entry.Args = []string{"exec", "--config=model_reasoning_effort=xhigh"}
	got = ApplyCodexProviderProfile(entry, CodexProfile{Model: CodexSolModel, Effort: CodexEffortUltra})
	assert.Equal(t, []string{"exec", "--json", "--config=model_reasoning_effort=\"ultra\"", "-m", CodexSolModel}, got.Args)
}

func TestCodexProfileFromArgsIgnoresTerminatorSuffixAndReadsLongConfig(t *testing.T) {
	t.Parallel()

	got := codexProfileFromArgs([]string{
		"exec", "--model=" + CodexSolModel, "--config=model_reasoning_effort=max",
		"--", "child", "--model=child-model", "--config=model_reasoning_effort=low",
	})
	assert.Equal(t, CodexProfile{Model: CodexSolModel, Effort: CodexEffortMax}, got)
}

func TestResolveCodexProviderProfileUsesCatalogResolution(t *testing.T) {
	t.Parallel()

	entry := ApplyCodexProviderProfile(
		CodexProviderEntryForQuality(QualityConf{Default: "ultra"}),
		CodexProfile{Model: CodexAstraModel, Effort: CodexEffortUltra},
	)
	catalog := []byte(`{"models":[{"slug":"gpt-6-astra","supported_reasoning_levels":[{"effort":"xhigh"},{"effort":"max"}]}]}`)

	got, resolution := ResolveCodexProviderProfile(entry, catalog)
	assert.Equal(t, CodexResolutionEffortUnavailable, resolution.Reason)
	assert.Contains(t, got.Args, CodexAstraModel)
	assert.Contains(t, got.Args, `model_reasoning_effort="max"`)
}
