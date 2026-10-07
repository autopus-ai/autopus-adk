package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestResolveProviders_FlagOverride(t *testing.T) {
	t.Parallel()

	conf := &config.OrchestraConf{
		Providers: map[string]config.ProviderEntry{
			"claude": {Binary: "claude", Args: []string{"--print"}},
			"gemini": {Binary: "gemini", Args: []string{"-m", "gemini-3.1-pro-preview", "-p", ""}, PromptViaArgs: false},
		},
		Commands: map[string]config.CommandEntry{
			"review": {Strategy: "debate", Providers: []string{"claude", "gemini"}},
		},
	}

	// CLI flags override config
	providers := resolveProviders(conf, "review", []string{"claude"})
	require.Len(t, providers, 1)
	assert.Equal(t, "claude", providers[0].Name)
}

func TestResolveProviders_CommandConfig(t *testing.T) {
	t.Parallel()

	conf := &config.OrchestraConf{
		Providers: map[string]config.ProviderEntry{
			"claude": {Binary: "claude", Args: []string{"--print"}},
			"gemini": {Binary: "gemini", Args: []string{"-m", "gemini-3.1-pro-preview", "-p", ""}, PromptViaArgs: false},
			"codex":  {Binary: "codex", Args: []string{"--quiet"}},
		},
		Commands: map[string]config.CommandEntry{
			"review": {Strategy: "debate", Providers: []string{"claude", "gemini"}},
		},
	}

	// No flag providers: fall back to command config
	providers := resolveProviders(conf, "review", nil)
	require.Len(t, providers, 2)

	names := make([]string, len(providers))
	for i, p := range providers {
		names[i] = p.Name
	}
	assert.Contains(t, names, "claude")
	assert.Contains(t, names, "gemini")
}

func TestResolveProviders_AllConfigProviders(t *testing.T) {
	t.Parallel()

	conf := &config.OrchestraConf{
		Providers: map[string]config.ProviderEntry{
			"claude": {Binary: "claude", Args: []string{"--print"}},
			"gemini": {Binary: "gemini", Args: []string{"-m", "gemini-3.1-pro-preview", "-p", ""}, PromptViaArgs: false},
		},
		Commands: map[string]config.CommandEntry{},
	}

	// No flags, no command config: fall back to all providers
	providers := resolveProviders(conf, "review", nil)
	assert.Len(t, providers, 2)
}

func TestResolveProviders_PromptViaArgsPropagated(t *testing.T) {
	t.Parallel()

	conf := &config.OrchestraConf{
		Providers: map[string]config.ProviderEntry{
			"gemini": {Binary: "gemini", Args: []string{"-m", "gemini-3.1-pro-preview", "-p", ""}, PromptViaArgs: false},
			"claude": {Binary: "claude", Args: []string{"--print"}, PromptViaArgs: false},
		},
		Commands: map[string]config.CommandEntry{},
	}

	providers := resolveProviders(conf, "review", []string{"gemini", "claude"})
	require.Len(t, providers, 2)

	for _, p := range providers {
		if p.Name == "gemini" {
			assert.False(t, p.PromptViaArgs, "gemini must have PromptViaArgs=false")
		}
		if p.Name == "claude" {
			assert.False(t, p.PromptViaArgs, "claude must have PromptViaArgs=false")
		}
	}
}

func TestResolveProviders_UnknownProviderFallback(t *testing.T) {
	t.Parallel()

	conf := &config.OrchestraConf{
		Providers: map[string]config.ProviderEntry{},
		Commands:  map[string]config.CommandEntry{},
	}

	// Unknown provider name: fallback to binary=name, args=[], PromptViaArgs=false
	providers := resolveProviders(conf, "review", []string{"unknown-tool"})
	require.Len(t, providers, 1)
	assert.Equal(t, "unknown-tool", providers[0].Name)
	assert.Equal(t, "unknown-tool", providers[0].Binary)
	assert.False(t, providers[0].PromptViaArgs)
}

func TestResolveStrategy_FlagOverride(t *testing.T) {
	t.Parallel()

	conf := &config.OrchestraConf{
		DefaultStrategy: "consensus",
		Commands: map[string]config.CommandEntry{
			"review": {Strategy: "debate"},
		},
	}

	// Flag overrides config
	s := resolveStrategy(conf, "review", "fastest")
	assert.Equal(t, "fastest", s)
}

func TestResolveStrategy_CommandConfig(t *testing.T) {
	t.Parallel()

	conf := &config.OrchestraConf{
		DefaultStrategy: "consensus",
		Commands: map[string]config.CommandEntry{
			"review": {Strategy: "debate"},
		},
	}

	// No flag: command config used
	s := resolveStrategy(conf, "review", "")
	assert.Equal(t, "debate", s)
}

func TestResolveStrategy_DefaultStrategy(t *testing.T) {
	t.Parallel()

	conf := &config.OrchestraConf{
		DefaultStrategy: "consensus",
		Commands:        map[string]config.CommandEntry{},
	}

	// No flag, no command config: default strategy
	s := resolveStrategy(conf, "plan", "")
	assert.Equal(t, "consensus", s)
}

func TestResolveStrategy_FallbackConsensus(t *testing.T) {
	t.Parallel()

	conf := &config.OrchestraConf{
		DefaultStrategy: "",
		Commands:        map[string]config.CommandEntry{},
	}

	// No config at all: hardcoded "consensus"
	s := resolveStrategy(conf, "plan", "")
	assert.Equal(t, "consensus", s)
}

func TestResolveJudge_FlagOverride(t *testing.T) {
	t.Parallel()

	conf := &config.OrchestraConf{
		Commands: map[string]config.CommandEntry{
			"review": {Judge: "gemini"},
		},
	}

	// CLI flag overrides config
	j := resolveJudge(conf, "review", "claude")
	assert.Equal(t, "claude", j)
}

func TestResolveJudge_CommandConfig(t *testing.T) {
	t.Parallel()

	conf := &config.OrchestraConf{
		Commands: map[string]config.CommandEntry{
			"review": {Judge: "gemini"},
		},
	}

	// No flag: command config used
	j := resolveJudge(conf, "review", "")
	assert.Equal(t, "gemini", j)
}

func TestResolveJudge_NoConfig(t *testing.T) {
	t.Parallel()

	conf := &config.OrchestraConf{
		Commands: map[string]config.CommandEntry{},
	}

	// No flag, no command config: empty string
	j := resolveJudge(conf, "review", "")
	assert.Equal(t, "", j)
}

func TestResolveProviders_OpencodeAndClaudeKeepConfiguredArgv(t *testing.T) {
	t.Parallel()

	conf := &config.OrchestraConf{
		Providers: map[string]config.ProviderEntry{
			"opencode": {Binary: "opencode", Args: []string{"run", "-m", "gpt-5.4"}},
			"claude":   {Binary: "claude", Args: []string{"-p"}},
		},
		Commands: map[string]config.CommandEntry{},
	}

	providers := resolveProviders(conf, "review", []string{"opencode", "claude"})
	require.Len(t, providers, 2)
	assert.Equal(t, []string{"run", "-m", "gpt-5.4"}, providers[0].Args)
	assert.Equal(t, []string{"-p"}, providers[1].Args)
}

func TestBuildProviderConfigs_OpencodeUsesUnknownProviderFallback(t *testing.T) {
	t.Parallel()

	configs := buildProviderConfigs([]string{"opencode", "claude"})
	require.Len(t, configs, 2)
	assert.Equal(t, "opencode", configs[0].Binary)
	assert.Empty(t, configs[0].Args, "opencode has no hardcoded argv")
	assert.Equal(t, config.DefaultClaudeProviderEntry().Args, configs[1].Args)
}

func TestResolveJudge_CommandWithoutJudge(t *testing.T) {
	t.Parallel()

	conf := &config.OrchestraConf{
		Commands: map[string]config.CommandEntry{
			"review": {Strategy: "debate"}, // judge field empty
		},
	}

	j := resolveJudge(conf, "review", "")
	assert.Equal(t, "", j)
}

func TestResolveJudge_GlobalFallback(t *testing.T) {
	t.Parallel()

	conf := &config.OrchestraConf{
		Judge: "claude",
		Commands: map[string]config.CommandEntry{
			"brainstorm": {Strategy: "debate"}, // no per-command judge
		},
	}

	// No flag, no command judge → falls back to global judge
	j := resolveJudge(conf, "brainstorm", "")
	assert.Equal(t, "claude", j)
}

func TestResolveJudge_CommandOverridesGlobal(t *testing.T) {
	t.Parallel()

	conf := &config.OrchestraConf{
		Judge: "claude",
		Commands: map[string]config.CommandEntry{
			"review": {Judge: "gemini"}, // per-command judge overrides global
		},
	}

	j := resolveJudge(conf, "review", "")
	assert.Equal(t, "gemini", j)
}
