package adapter_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/antigravity"
	"github.com/insajin/autopus-adk/pkg/adapter/claude"
	"github.com/insajin/autopus-adk/pkg/adapter/opencode"
	"github.com/insajin/autopus-adk/pkg/config"
)

// alwaysRuleFiles are the rules that stay unconditional on every platform.
var alwaysRuleFiles = map[string]bool{
	"language-policy.md": true,
}

// generatePlatform writes one platform surface into a fresh root.
func generatePlatform(t *testing.T, platform string) string {
	t.Helper()
	dir := t.TempDir()
	ctx := context.Background()
	cfg := config.DefaultFullConfig("golden")

	var err error
	switch platform {
	case "claude":
		_, err = claude.NewWithRoot(dir).Generate(ctx, cfg)
	case "gemini":
		_, err = antigravity.NewWithRoot(dir).Generate(ctx, cfg)
	case "opencode":
		_, err = opencode.NewWithRoot(dir, opencode.WithCLIVersion("1.18.7")).Generate(ctx, cfg)
	default:
		t.Fatalf("unknown platform %q", platform)
	}
	require.NoError(t, err)
	return dir
}

// TestRules_NoHookEntryReferencesUnconditionalRules completes S8: no rule that
// carries no runtime trigger may be wired into a hook command. Issue #185 adds
// the skill-scoped four, whose whole point is that nothing fires them.
func TestRules_NoHookEntryReferencesUnconditionalRules(t *testing.T) {
	t.Parallel()

	dir := generatePlatform(t, "claude")
	raw, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	require.NoError(t, err)

	settings := string(raw)
	for name := range alwaysRuleFiles {
		assert.NotContains(t, settings, strings.TrimSuffix(name, ".md"),
			"%s must not be referenced by a hook entry", name)
	}
}
