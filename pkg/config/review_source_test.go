package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func reviewSourceConfig(gateProviders []string, judge string) *HarnessConfig {
	cfg := DefaultFullConfig("p")
	cfg.Spec.ReviewGate.Providers = gateProviders
	cfg.Spec.ReviewGate.Judge = judge
	return cfg
}

// A fresh config holds one review list: review names no providers, and the
// orchestra migration leaves the empty list empty.
func TestDefaults_ReviewCommandHasNoListOfItsOwn(t *testing.T) {
	t.Parallel()
	cfg := DefaultFullConfig("p")
	assert.Empty(t, cfg.Orchestra.Commands["review"].Providers)
	assert.Equal(t, "debate", cfg.Orchestra.Commands["review"].Strategy)
	_, err := MigrateOrchestraConfig(cfg)
	assert.NoError(t, err)
	assert.Empty(t, cfg.Orchestra.Commands["review"].Providers)
	assert.NotEmpty(t, cfg.Orchestra.Commands["secure"].Providers)
}

// `auto orchestra review` reviews with the review gate's list and judge.
func TestReviewGateOrchestraView_TakesTheGateListAndJudge(t *testing.T) {
	t.Parallel()
	cfg := reviewSourceConfig([]string{"claude", "codex"}, "codex")

	view := ReviewGateOrchestraView(cfg)

	assert.Equal(t, []string{"claude", "codex"}, view.Commands["review"].Providers)
	assert.Equal(t, "codex", view.Commands["review"].Judge)
	assert.Equal(t, "debate", view.Commands["review"].Strategy)
	assert.Empty(t, cfg.Orchestra.Commands["review"].Providers, "the caller's config is unchanged")
}

// An explicit review command list or judge, or a disabled gate, wins.
func TestReviewGateOrchestraView_KeepsExplicitOverrides(t *testing.T) {
	t.Parallel()
	cfg := reviewSourceConfig([]string{"claude", "codex"}, "codex")
	cfg.Orchestra.Commands["review"] = CommandEntry{Strategy: "debate", Providers: []string{"gemini"}, Judge: "gemini"}
	view := ReviewGateOrchestraView(cfg)
	assert.Equal(t, []string{"gemini"}, view.Commands["review"].Providers)
	assert.Equal(t, "gemini", view.Commands["review"].Judge)

	off := reviewSourceConfig([]string{"claude", "codex"}, "codex")
	off.Spec.ReviewGate.Enabled = false
	assert.Empty(t, ReviewGateOrchestraView(off).Commands["review"].Providers)
}

// The ADK-written list (shipped or every provider) yields to the gate's
// list; a list the operator narrowed stays.
func TestDeriveReviewProviders_OnlyDropsTheADKWrittenList(t *testing.T) {
	t.Parallel()
	cfg := reviewSourceConfig([]string{"claude", "codex"}, "claude")
	cfg.Orchestra.Commands["review"] = CommandEntry{Strategy: "debate", Providers: []string{"claude", "codex", "gemini"}}
	assert.True(t, DeriveReviewProviders(cfg))
	assert.Empty(t, cfg.Orchestra.Commands["review"].Providers)
	assert.False(t, DeriveReviewProviders(cfg), "nothing left to derive")

	narrowed := reviewSourceConfig([]string{"claude", "codex"}, "claude")
	narrowed.Orchestra.Commands["review"] = CommandEntry{Strategy: "debate", Providers: []string{"claude"}}
	assert.False(t, DeriveReviewProviders(narrowed))

	noGate := reviewSourceConfig(nil, "claude")
	noGate.Orchestra.Commands["review"] = CommandEntry{Strategy: "debate", Providers: []string{"claude", "codex", "gemini"}}
	assert.False(t, DeriveReviewProviders(noGate), "without a gate list there is nothing to follow")
}
