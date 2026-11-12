package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeReviewerProfile_FollowsThePreset(t *testing.T) {
	t.Parallel()
	q := DefaultFullConfig("p").Quality
	model, effort := q.ClaudeReviewerProfile()
	assert.Equal(t, [2]string{ClaudeOpusModel, "max"}, [2]string{model, effort}, "balanced")
	model, effort = q.WithGlobalOverride("ultra").ClaudeReviewerProfile()
	assert.Equal(t, [2]string{ClaudeFableModel, "max"}, [2]string{model, effort}, "ultra")
}

// A fresh balanced config already follows the reviewer: no move, so the
// shipped default never changes for balanced.
func TestClaudeReviewFollow_BalancedDefaultAlreadyFollows(t *testing.T) {
	t.Parallel()
	_, ok := ClaudeReviewFollowFor(DefaultFullConfig("p"))
	assert.False(t, ok)
}

// Choosing ultra moves an ADK-written claude review model onto Fable, the
// reviewer's model, keeping the print flag and the output format.
func TestApplyClaudeReviewFollow_UltraMovesTheShippedDefaultToFable(t *testing.T) {
	t.Parallel()
	cfg := DefaultFullConfig("p")
	cfg.Quality.Default = "ultra"

	follow, ok := ApplyClaudeReviewFollow(cfg)

	require.True(t, ok)
	assert.Equal(t, ClaudeReviewFollow{
		FromModel: ClaudeOpusModel, FromEffort: "max", ToModel: ClaudeFableModel, ToEffort: "max",
	}, follow)
	assert.Equal(t, []string{"--print", "--model", ClaudeFableModel, "--effort", "max", "--output-format", "json"},
		cfg.Orchestra.Providers["claude"].Args)
	_, again := ClaudeReviewFollowFor(cfg)
	assert.False(t, again, "a followed provider has nothing left to move")
}

// Going back to balanced moves the ultra reviewer model back to Opus.
func TestClaudeReviewFollow_BalancedMovesAnUltraReviewerModelBack(t *testing.T) {
	t.Parallel()
	cfg := DefaultFullConfig("p")
	entry := cfg.Orchestra.Providers["claude"]
	entry.Args = []string{"--print", "--model", ClaudeFableModel, "--effort", "max", "--output-format", "json"}
	cfg.Orchestra.Providers["claude"] = entry

	follow, ok := ClaudeReviewFollowFor(cfg)
	require.True(t, ok)
	assert.Equal(t, ClaudeOpusModel, follow.ToModel)
}

// A model ADK never wrote, a pinned policy, or a backend route is the
// operator's: nothing moves.
func TestClaudeReviewFollow_LeavesOperatorChoicesAlone(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*ProviderEntry){
		"custom model": func(e *ProviderEntry) {
			e.Args = []string{"--print", "--model", "claude-sonnet-5-5", "--effort", "max"}
		},
		"custom effort": func(e *ProviderEntry) { e.Args = []string{"--print", "--model", ClaudeOpusModel, "--effort", "low"} },
		"extra flag":    func(e *ProviderEntry) { e.Args = append(e.Args, "--verbose") },
		"pinned":        func(e *ProviderEntry) { e.ModelPolicy = ProviderModelPolicyPinned },
		"backend":       func(e *ProviderEntry) { e.Backend = ProviderBackendOMP },
		"wrapper":       func(e *ProviderEntry) { e.Binary = "my-claude" },
	} {
		cfg := DefaultFullConfig("p")
		cfg.Quality.Default = "ultra"
		entry := cfg.Orchestra.Providers["claude"]
		mutate(&entry)
		cfg.Orchestra.Providers["claude"] = entry
		_, ok := ClaudeReviewFollowFor(cfg)
		assert.False(t, ok, name)
	}
	assert.False(t, func() bool { _, ok := ClaudeReviewFollowFor(nil); return ok }())
}
