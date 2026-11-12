package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func retiredCommandsFixture(commands map[string]CommandEntry) *HarnessConfig {
	cfg := DefaultFullConfig("demo")
	cfg.Orchestra.Commands = commands
	return cfg
}

func TestDefaultFullConfig_ShipsOnlyReviewAndSecureCommands(t *testing.T) {
	t.Parallel()
	cfg := DefaultFullConfig("demo")
	assert.ElementsMatch(t, []string{"review", "secure"}, commandKeys(cfg.Orchestra.Commands))
}

func TestPruneRetiredOrchestraCommands_RemovesShippedDefaults(t *testing.T) {
	t.Parallel()
	all := []string{"claude", "codex", "gemini"}
	cfg := retiredCommandsFixture(map[string]CommandEntry{
		"review":     {Strategy: "debate", Providers: all},
		"plan":       {Strategy: "consensus", Providers: all},
		"brainstorm": {Strategy: "debate", Providers: []string{"gemini", "claude", "codex"}},
	})

	removed := PruneRetiredOrchestraCommands(cfg)

	assert.Equal(t, []string{"orchestra.commands.brainstorm", "orchestra.commands.plan"}, removed)
	assert.ElementsMatch(t, []string{"review"}, commandKeys(cfg.Orchestra.Commands))
}

func TestPruneRetiredOrchestraCommands_ProviderListFollowingProvidersIsOwned(t *testing.T) {
	t.Parallel()
	// Migration 3 appends every orchestra provider to every command, so a
	// list that names exactly the configured providers is still ADK's.
	cfg := retiredCommandsFixture(map[string]CommandEntry{
		"plan": {Strategy: "consensus", Providers: []string{"claude", "codex", "gemini", "opencode"}},
	})
	cfg.Orchestra.Providers["opencode"] = ProviderEntry{Binary: "opencode"}

	assert.Equal(t, []string{"orchestra.commands.plan"}, PruneRetiredOrchestraCommands(cfg))
}

func TestPruneRetiredOrchestraCommands_KeepsCustomizedEntries(t *testing.T) {
	t.Parallel()
	all := []string{"claude", "codex", "gemini"}
	for name, entry := range map[string]CommandEntry{
		"subset":    {Strategy: "consensus", Providers: []string{"claude"}},
		"strategy":  {Strategy: "fastest", Providers: all},
		"judge":     {Strategy: "consensus", Providers: all, Judge: "codex"},
		"threshold": {Strategy: "consensus", Providers: all, ConsensusThreshold: 0.8},
	} {
		cfg := retiredCommandsFixture(map[string]CommandEntry{"plan": entry})
		assert.Empty(t, PruneRetiredOrchestraCommands(cfg), name)
		assert.Contains(t, cfg.Orchestra.Commands, "plan", name)
	}
}

func TestPruneRetiredOrchestraCommands_LeavesUserCommandsAndNilMap(t *testing.T) {
	t.Parallel()
	cfg := retiredCommandsFixture(map[string]CommandEntry{
		"custom": {Strategy: "consensus", Providers: []string{"claude", "codex", "gemini"}},
	})
	assert.Empty(t, PruneRetiredOrchestraCommands(cfg))
	assert.Contains(t, cfg.Orchestra.Commands, "custom")

	assert.Empty(t, PruneRetiredOrchestraCommands(retiredCommandsFixture(nil)))
	assert.Empty(t, PruneRetiredOrchestraCommands(nil))
}

func commandKeys(m map[string]CommandEntry) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}
