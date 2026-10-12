package cli

// Acceptance fixtures of SPEC-MODELUX-002 that SPEC-MODELUX-001's helpers do
// not build: D on other platform sets (D1, N, O3), the hand-written W2, and
// readers for the rendered table and the raw role_model_policy.

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/config"
)

// writeComposeFixture saves D, the code defaults with supervisor inherit, on
// platforms: D1 is writeComposeFixture(t, "claude-code"). The fixture is
// settled the way composeUpdateRun.settled leaves D: its language settings
// are the defaults the update's own language prompt saves, so a later update
// changes the file only through the composition step and detection.
func writeComposeFixture(t *testing.T, platforms ...string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, config.Save(dir, composeFixtureConfig(platforms...)))
	return dir
}

func composeFixtureConfig(platforms ...string) *config.HarnessConfig {
	cfg := config.DefaultFullConfig("p")
	cfg.Platforms = platforms
	cfg.Quality.SupervisorModelPolicy = config.SupervisorModelPolicyInherit
	cfg.Language = config.LanguageConf{Comments: "en", Commits: "en", AIResponses: "en"}
	return cfg
}

// writeComposeW2 writes W2: D's values by hand, with two-space indentation,
// a column-one comment inside quality, a blank line between presets, and a
// flow-style role_model_policy.
func writeComposeW2(t *testing.T) string {
	t.Helper()
	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(2)
	require.NoError(t, encoder.Encode(composeFixtureConfig("claude-code", "codex")))
	require.NoError(t, encoder.Close())
	text := buffer.String()
	require.NotContains(t, text, "role_model_policy")
	for old, replacement := range map[string]string{
		"\nquality:\n":   "\nquality:\n# team note\n",
		"\n    ultra:\n": "\n\n    ultra:\n",
	} {
		require.Contains(t, text, old)
		text = strings.Replace(text, old, replacement, 1)
	}
	text += "role_model_policy: {version: v1, profile: balanced, family: openai}\n"
	return writeBlockFixture(t, text)
}

// composeTableHeader returns the header cells of the table that follows the
// first line of out equal to marker.
func composeTableHeader(t *testing.T, out, marker string) []string {
	t.Helper()
	lines := strings.Split(out, "\n")
	for index, line := range lines {
		if line == marker && index+1 < len(lines) {
			return composeTableColumns.Split(lines[index+1], -1)
		}
	}
	t.Fatalf("no line %q in:\n%s", marker, out)
	return nil
}

// rawRoleModelPolicyOf decodes role_model_policy without environment
// expansion, the way rawQualityOf reads the quality block.
func rawRoleModelPolicyOf(t *testing.T, data string) config.RoleModelPolicyConf {
	t.Helper()
	var doc struct {
		Policy config.RoleModelPolicyConf `yaml:"role_model_policy"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(data), &doc))
	return doc.Policy
}
