// Package cli tests orchestra provider config argv correctness.
// SPEC-ORCH-021 REQ-014/015/016: pane argv stays interactive (gemini no --print,
// codex no leading exec), codex carries the structured --output-schema flag, and
// codex participates in the default structured review provider set.
package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/config"
)

// TestProviderConfig_CodexStructuredSchema covers S18: codex's structured argv
// carries the --output-schema flag, and codex is present in the default
// structured review provider set.
func TestProviderConfig_CodexStructuredSchema(t *testing.T) {
	cfg := buildProviderConfigs([]string{"codex"})[0]
	assert.Equal(t, "--output-schema", cfg.SchemaFlag,
		"codex structured argv must carry --output-schema")

	// Codex must be in the DEFAULT review provider set (cross-check with S20).
	names := resolveSpecReviewProviderNames(config.DefaultFullConfig("argv-test"), false)
	assert.Contains(t, names, "codex",
		"codex must be present in the default structured review provider set")
}

// TestResolveSpecReviewProviders_DefaultIncludesCodex covers S20: the default
// structured review provider set contains codex and resolves to the full
// [claude, codex, gemini] set with no silent codex drop.
func TestResolveSpecReviewProviders_DefaultIncludesCodex(t *testing.T) {
	cfg := config.DefaultFullConfig("argv-test")
	names := resolveSpecReviewProviderNames(cfg, false)

	assert.Contains(t, names, "claude")
	assert.Contains(t, names, "codex", "codex must not be silently dropped from review")
	assert.Contains(t, names, "gemini")
	assert.ElementsMatch(t, []string{"claude", "codex", "gemini"}, names,
		"default review provider set must be exactly [claude, codex, gemini]")
}
