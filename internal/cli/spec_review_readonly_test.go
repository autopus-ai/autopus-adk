package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// fDefaultSpecReviewConfig is acceptance fixture F-default as autopus.yaml
// entries, with reviewers [claude, codex, gemini] and judge claude.
func fDefaultSpecReviewConfig() *config.HarnessConfig {
	cfg := &config.HarnessConfig{}
	cfg.Spec.ReviewGate.Providers = []string{"claude", "codex", "gemini"}
	cfg.Spec.ReviewGate.Judge = "claude"
	cfg.Orchestra.Providers = map[string]config.ProviderEntry{}
	for _, provider := range fDefaultReadOnlyProviders() {
		cfg.Orchestra.Providers[provider.Name] = config.ProviderEntry{
			Binary: provider.Binary, Args: provider.Args, PaneArgs: provider.PaneArgs, PromptViaArgs: provider.PromptViaArgs,
			Subprocess: config.SubprocessProvConf{SchemaFlag: provider.SchemaFlag},
		}
	}
	return cfg
}

// countCodexCatalogProbes replaces the codex catalog probe, the only step of
// the assembly that executes a configured binary, with a call counter.
func countCodexCatalogProbes(t *testing.T) *atomic.Int32 {
	t.Helper()
	originalProbe, originalWriter := runtimeCodexCatalogProbe, runtimeCodexFallbackWriter
	t.Cleanup(func() { runtimeCodexCatalogProbe, runtimeCodexFallbackWriter = originalProbe, originalWriter })
	var calls atomic.Int32
	runtimeCodexCatalogProbe = func(context.Context, string) ([]byte, error) {
		calls.Add(1)
		return nil, errors.New("catalog probe disabled in test")
	}
	runtimeCodexFallbackWriter = io.Discard
	return &calls
}

// writeMarkerWrapper installs an executable that leaves a marker when it runs.
func writeMarkerWrapper(t *testing.T, dir, name, marker string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0o755))
	return path
}

func editProvider(cfg *config.HarnessConfig, name string, edit func(*config.ProviderEntry)) {
	entry := cfg.Orchestra.Providers[name]
	edit(&entry)
	cfg.Orchestra.Providers[name] = entry
}

// S6: explicit conflicting config fails closed with the Error Contract before
// the installed filter, the codex catalog probe, or any provider runs.
func TestAssembleSpecReviewProviders_ExplicitViolationFailsBeforeExecution(t *testing.T) {
	wrappers := t.TempDir()
	marker := filepath.Join(wrappers, "executed")
	claudeWrapper := writeMarkerWrapper(t, wrappers, "claude-wrapper", marker)
	codexWrapper := writeMarkerWrapper(t, wrappers, "codex-wrapper", marker)

	tests := []struct {
		name     string
		provider string
		edit     func(*config.ProviderEntry)
		want     string
	}{
		{"claude verbose", "claude", func(e *config.ProviderEntry) { e.Args = []string{"--print", "--verbose"} },
			`spec review: provider "claude" rejected by the read-only policy: contains unsupported argv "--verbose" (config key: orchestra.providers.claude.args; remedy: remove "--verbose" from orchestra.providers.claude.args)`},
		{"claude allowed tools", "claude", func(e *config.ProviderEntry) { e.Args = []string{"--print", "--allowedTools", "Bash(git *)"} },
			`spec review: provider "claude" rejected by the read-only policy: contains unsupported argv "--allowedTools" (config key: orchestra.providers.claude.args; remedy: remove "--allowedTools" from orchestra.providers.claude.args)`},
		{"claude pane bypass", "claude", func(e *config.ProviderEntry) { e.PaneArgs = []string{"--print", "--dangerously-skip-permissions"} },
			`spec review: provider "claude" rejected by the read-only policy: contains unsafe argv "--dangerously-skip-permissions" (config key: orchestra.providers.claude.pane_args; remedy: remove "--dangerously-skip-permissions" from orchestra.providers.claude.pane_args)`},
		{"claude wrapper", "claude", func(e *config.ProviderEntry) { e.Binary = claudeWrapper },
			`spec review: provider "claude" rejected by the read-only policy: requires native binary "claude" (config key: orchestra.providers.claude.binary; remedy: set orchestra.providers.claude.binary to "claude")`},
		{"codex quality wrapper", "codex", func(e *config.ProviderEntry) {
			e.Binary, e.ModelPolicy = codexWrapper, config.ProviderModelPolicyQuality
		},
			`spec review: provider "codex" rejected by the read-only policy: requires native binary "codex" (config key: orchestra.providers.codex.binary; remedy: set orchestra.providers.codex.binary to "codex")`},
		{"codex full access", "codex", func(e *config.ProviderEntry) { e.Args = []string{"exec", "--sandbox", "danger-full-access"} },
			`spec review: provider "codex" rejected by the read-only policy: contains unsafe value for "--sandbox" (config key: orchestra.providers.codex.args; remedy: remove "--sandbox danger-full-access" from orchestra.providers.codex.args)`},
		{"codex sandbox config", "codex", func(e *config.ProviderEntry) { e.Args = []string{"exec", "-c", "sandbox_mode=workspace-write"} },
			`spec review: provider "codex" rejected by the read-only policy: contains unsafe value for "-c" (config key: orchestra.providers.codex.args; remedy: remove "-c sandbox_mode=workspace-write" from orchestra.providers.codex.args)`},
		{"claude schema flag", "claude", func(e *config.ProviderEntry) { e.Subprocess.SchemaFlag = "--permission-mode=bypassPermissions" },
			`spec review: provider "claude" rejected by the read-only policy: unsupported schema flag "--permission-mode=bypassPermissions" (config key: orchestra.providers.claude.subprocess.schema_flag; remedy: remove orchestra.providers.claude.subprocess.schema_flag)`},
		{"gemini yolo", "gemini", func(e *config.ProviderEntry) { e.Args = []string{"--print", "", "--yolo"} },
			`spec review: provider "gemini" rejected by the read-only policy: contains unsafe argv "--yolo" (config key: orchestra.providers.gemini.args; remedy: remove "--yolo" from orchestra.providers.gemini.args)`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			probes := countCodexCatalogProbes(t)
			cfg := fDefaultSpecReviewConfig()
			editProvider(cfg, tt.provider, tt.edit)

			set, err := assembleSpecReviewProviders(context.Background(), specReviewProviderRequest{Config: cfg})

			require.Error(t, err)
			assert.Equal(t, tt.want, err.Error())
			assert.Empty(t, set.Providers)
			assert.Zero(t, probes.Load(), "no configured binary may run before the gate accepts it")
			assert.NoFileExists(t, marker)
		})
	}
}

// S6 discriminator: with the real catalog probe, a rejected wrapper binary
// still never runs (today's order would run "<wrapper> debug models").
func TestAssembleSpecReviewProviders_RejectedWrapperNeverRunsThroughCatalogProbe(t *testing.T) {
	wrappers := t.TempDir()
	marker := filepath.Join(wrappers, "executed")
	cfg := fDefaultSpecReviewConfig()
	editProvider(cfg, "codex", func(e *config.ProviderEntry) {
		e.Binary, e.ModelPolicy = writeMarkerWrapper(t, wrappers, "codex-wrapper", marker), config.ProviderModelPolicyQuality
	})

	_, err := assembleSpecReviewProviders(context.Background(), specReviewProviderRequest{Config: cfg, FlagProviders: []string{"codex"}})

	require.Error(t, err)
	assert.NoFileExists(t, marker)
}

// S8: an explicit unsupported provider fails closed with its selection
// source as the config key, before any missing-binary warning.
func TestAssembleSpecReviewProviders_ExplicitUnsupportedProviderNamesSelectionSource(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	var err error
	stderr := captureStderr(t, func() {
		_, err = assembleSpecReviewProviders(context.Background(), specReviewProviderRequest{
			Config: fDefaultSpecReviewConfig(), FlagProviders: []string{"claude", "opencode"},
		})
	})
	require.Error(t, err)
	assert.Equal(t, `spec review: provider "opencode" rejected by the read-only policy: unsupported provider "opencode" (config key: --providers; remedy: remove "opencode" from --providers)`, err.Error())
	assert.Empty(t, stderr, "the gate must fail before the installed filter warns about missing binaries")

	cfg := fDefaultSpecReviewConfig()
	cfg.Spec.ReviewGate.Providers = []string{"claude", "opencode"}
	_, err = assembleSpecReviewProviders(context.Background(), specReviewProviderRequest{Config: cfg})
	require.Error(t, err)
	assert.Equal(t, `spec review: provider "opencode" rejected by the read-only policy: unsupported provider "opencode" (config key: spec.review_gate.providers; remedy: remove "opencode" from spec.review_gate.providers)`, err.Error())
}

// S8: a rejected provider that enters only through --multi discovery is
// excluded with a warning and leaves the quorum denominator.
func TestAssembleSpecReviewProviders_DiscoveredViolationIsExcluded(t *testing.T) {
	installReadOnlyArgvRecorders(t, "claude", "codex")
	countCodexCatalogProbes(t)
	cfg := fDefaultSpecReviewConfig()
	cfg.Spec.ReviewGate.Providers = []string{"claude", "codex"}
	delete(cfg.Orchestra.Providers, "gemini")
	cfg.Orchestra.Providers["opencode"] = config.ProviderEntry{Binary: "opencode"}
	cfg.Orchestra.Commands = map[string]config.CommandEntry{"review": {Providers: []string{"claude", "codex", "opencode"}}}

	var warnings bytes.Buffer
	set, err := assembleSpecReviewProviders(context.Background(), specReviewProviderRequest{Config: cfg, Multi: true, Warnings: &warnings})

	require.NoError(t, err)
	assert.Equal(t, []string{"claude", "codex"}, providerConfigNames(set.Providers))
	assert.Equal(t, []string{"claude", "codex"}, set.Names, "excluded providers leave the quorum denominator")
	assert.Equal(t, []specReviewExclusion{
		{Provider: "opencode", Reason: `unsupported provider "opencode"`},
		{Provider: "gemini", Reason: `requires native binary "agy"`},
	}, set.Excluded)
	assert.Equal(t, "spec review: excluding discovered provider \"opencode\": unsupported provider \"opencode\"\n"+
		"spec review: excluding discovered provider \"gemini\": requires native binary \"agy\"\n", warnings.String())
}

// S8: a discovered gemini behind a wrapper binary is excluded, not executed.
func TestAssembleSpecReviewProviders_DiscoveredWrapperIsExcluded(t *testing.T) {
	installReadOnlyArgvRecorders(t, "claude", "codex")
	wrappers := t.TempDir()
	cfg := fDefaultSpecReviewConfig()
	cfg.Spec.ReviewGate.Providers = []string{"claude", "codex"}
	editProvider(cfg, "gemini", func(e *config.ProviderEntry) {
		e.Binary = writeMarkerWrapper(t, wrappers, "agy-wrapper", filepath.Join(wrappers, "executed"))
	})

	set, err := assembleSpecReviewProviders(context.Background(), specReviewProviderRequest{Config: cfg, Multi: true})

	require.NoError(t, err)
	assert.Equal(t, []string{"claude", "codex"}, providerConfigNames(set.Providers))
	assert.Equal(t, []specReviewExclusion{{Provider: "gemini", Reason: `requires native binary "agy"`}}, set.Excluded)
	assert.NoFileExists(t, filepath.Join(wrappers, "executed"))
}

// The Error Contract cuts a rejected value at 64 runes in the remedy and
// keeps the inline spelling of an inline value.
func TestSpecReviewPolicyError_RemedyCutsLongValues(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("é", 70)
	cut := strings.Repeat("é", 64)
	err := specReviewPolicyError(&readOnlyPolicyViolation{
		Provider: "codex", Field: readOnlyFieldArgs, Item: "-c", Value: long, Kind: readOnlyUnsafeValue,
	}, specReviewSourceGateProviders)
	assert.Equal(t, `spec review: provider "codex" rejected by the read-only policy: contains unsafe value for "-c" (config key: orchestra.providers.codex.args; remedy: remove "-c `+cut+`" from orchestra.providers.codex.args)`, err.Error())

	err = specReviewPolicyError(&readOnlyPolicyViolation{
		Provider: "claude", Field: readOnlyFieldArgs, Item: "--tools", Value: long, Inline: true, Kind: readOnlyUnsafeValue,
	}, specReviewSourceGateProviders)
	assert.Contains(t, err.Error(), `remedy: remove "--tools=`+cut+`" from orchestra.providers.claude.args)`)

	plain := errors.New("not a policy violation")
	assert.Same(t, plain, specReviewPolicyError(plain, specReviewSourceGateProviders))
}
