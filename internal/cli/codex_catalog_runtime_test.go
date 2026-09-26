package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveCodexProviderCapabilities_DowngradesSameModelEffort(t *testing.T) {
	t.Parallel()

	providers := []orchestra.ProviderConfig{managedRuntimeCodexProvider(config.CodexEffortUltra)}
	probe := func(context.Context, string) ([]byte, error) {
		return []byte(`{"models":[{"slug":"gpt-6-sol","supported_reasoning_levels":[{"effort":"xhigh"},{"effort":"max"}]}]}`), nil
	}
	var receipt bytes.Buffer

	got := resolveCodexProviderCapabilitiesWith(context.Background(), providers, probe, &receipt)
	require.Len(t, got, 1)
	assertCodexProfileInArgs(t, got[0].Args, config.CodexSolModel, config.CodexEffortMax)
	assertCodexProfileInArgs(t, got[0].PaneArgs, config.CodexSolModel, config.CodexEffortMax)
	assert.Contains(t, receipt.String(), "requested=gpt-6-sol/ultra")
	assert.Contains(t, receipt.String(), "selected=gpt-6-sol/max")
	assert.Contains(t, receipt.String(), "reason=effort_unavailable")
	assert.Equal(t, 1, strings.Count(receipt.String(), "reason=effort_unavailable"))
}

func TestResolveCodexProviderCapabilities_PreservesPinnedProvider(t *testing.T) {
	t.Parallel()

	provider := orchestra.ProviderConfig{
		Name:        "codex",
		Binary:      "codex-wrapper",
		Args:        []string{"exec", "-m", "custom-model", "-c", `model_reasoning_effort="ultra"`},
		PaneArgs:    []string{"--search"},
		ModelPolicy: config.ProviderModelPolicyPinned,
	}
	called := false
	probe := func(context.Context, string) ([]byte, error) {
		called = true
		return nil, nil
	}

	got := resolveCodexProviderCapabilitiesWith(context.Background(), []orchestra.ProviderConfig{provider}, probe, &bytes.Buffer{})
	assert.False(t, called)
	assert.Equal(t, provider, got[0])
}

func TestResolveCodexProviderCapabilities_UnknownCatalogUsesLegacy(t *testing.T) {
	t.Parallel()

	provider := managedRuntimeCodexProvider(config.CodexEffortUltra)
	probe := func(context.Context, string) ([]byte, error) {
		return nil, errors.New("debug models unsupported")
	}
	var receipt bytes.Buffer

	got := resolveCodexProviderCapabilitiesWith(context.Background(), []orchestra.ProviderConfig{provider}, probe, &receipt)
	assertCodexProfileInArgs(t, got[0].Args, config.CodexLegacyModel, config.CodexEffortXHigh)
	assertCodexProfileInArgs(t, got[0].PaneArgs, config.CodexLegacyModel, config.CodexEffortXHigh)
	assert.Contains(t, receipt.String(), "reason=catalog_unknown")
}

func TestResolveCodexProviderCapabilities_MissingModelUsesLegacy(t *testing.T) {
	t.Parallel()

	provider := managedRuntimeCodexProvider(config.CodexEffortUltra)
	probe := func(context.Context, string) ([]byte, error) {
		return []byte(`{"models":[{"slug":"gpt-5.5","supported_reasoning_levels":[{"effort":"xhigh"}]}]}`), nil
	}
	var receipt bytes.Buffer

	got := resolveCodexProviderCapabilitiesWith(context.Background(), []orchestra.ProviderConfig{provider}, probe, &receipt)
	assertCodexProfileInArgs(t, got[0].Args, config.CodexLegacyModel, config.CodexEffortXHigh)
	assertCodexProfileInArgs(t, got[0].PaneArgs, config.CodexLegacyModel, config.CodexEffortXHigh)
	assert.Contains(t, receipt.String(), "reason=model_unavailable")
	assert.Equal(t, 1, strings.Count(receipt.String(), "reason=model_unavailable"))
}

func TestResolveCodexProviderCapabilities_OversizedCatalogUsesLegacy(t *testing.T) {
	t.Parallel()

	provider := managedRuntimeCodexProvider(config.CodexEffortUltra)
	probe := func(context.Context, string) ([]byte, error) {
		return bytes.Repeat([]byte("x"), config.MaxCodexModelCatalogBytes+1), nil
	}
	var receipt bytes.Buffer

	got := resolveCodexProviderCapabilitiesWith(context.Background(), []orchestra.ProviderConfig{provider}, probe, &receipt)
	assertCodexProfileInArgs(t, got[0].Args, config.CodexLegacyModel, config.CodexEffortXHigh)
	assertCodexProfileInArgs(t, got[0].PaneArgs, config.CodexLegacyModel, config.CodexEffortXHigh)
	assert.Contains(t, receipt.String(), "reason=catalog_unknown")
}

func TestResolveCodexProviderCapabilities_OmitsOverridesForRuntimeDefault(t *testing.T) {
	t.Parallel()

	provider := managedRuntimeCodexProvider(config.CodexEffortUltra)
	probe := func(context.Context, string) ([]byte, error) {
		return []byte(`{"models":[{"slug":"other-model","supported_reasoning_levels":[{"effort":"medium"}]}]}`), nil
	}
	var receipt bytes.Buffer

	got := resolveCodexProviderCapabilitiesWith(context.Background(), []orchestra.ProviderConfig{provider}, probe, &receipt)
	joined := strings.Join(append(append([]string{}, got[0].Args...), got[0].PaneArgs...), "\x00")
	assert.NotContains(t, joined, "gpt-5.6")
	assert.NotContains(t, joined, "model_reasoning_effort")
	assert.NotContains(t, joined, "-m")
	assert.Contains(t, receipt.String(), "selected=runtime-default")
	assert.Contains(t, receipt.String(), "reason=runtime_default")
}

func TestResolveCodexProviderCapabilities_NoLowerEffortKeepsModel(t *testing.T) {
	t.Parallel()

	provider := managedRuntimeCodexProvider(config.CodexEffortLow)
	probe := func(context.Context, string) ([]byte, error) {
		return []byte(`{"models":[{"slug":"gpt-6-sol","supported_reasoning_levels":[{"effort":"medium"}]}]}`), nil
	}
	var receipt bytes.Buffer

	got := resolveCodexProviderCapabilitiesWith(context.Background(), []orchestra.ProviderConfig{provider}, probe, &receipt)
	require.Len(t, got, 1)
	for _, args := range [][]string{got[0].Args, got[0].PaneArgs} {
		joined := strings.Join(args, "\x00")
		assert.Contains(t, joined, config.CodexSolModel)
		assert.NotContains(t, joined, "model_reasoning_effort")
	}
	assert.Contains(t, receipt.String(), "selected=gpt-6-sol")
	assert.Contains(t, receipt.String(), "reason=runtime_default")
}

func TestResolveCodexProviderCapabilities_SkipsOMPBackend(t *testing.T) {
	t.Parallel()
	provider := managedRuntimeCodexProvider(config.CodexEffortUltra)
	provider.Backend = config.ProviderBackendOMP
	calls := 0
	probe := func(context.Context, string) ([]byte, error) {
		calls++
		return nil, errors.New("OMP routes must not probe codex")
	}

	got := resolveCodexProviderCapabilitiesWith(
		context.Background(), []orchestra.ProviderConfig{provider}, probe, &bytes.Buffer{},
	)

	assert.Zero(t, calls)
	assert.Equal(t, provider, got[0])
}

func managedRuntimeCodexProvider(effort string) orchestra.ProviderConfig {
	entry := config.CodexProviderEntryForQuality(config.QualityConf{Default: "ultra"})
	entry = config.ApplyCodexProviderProfile(entry, config.CodexProfile{Model: config.CodexSolModel, Effort: effort})
	return providerConfigFromEntry("codex", entry, "")
}
