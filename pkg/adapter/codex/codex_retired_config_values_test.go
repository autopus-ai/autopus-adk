package codex

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

func writeExistingCodexConfig(t *testing.T, dir, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".codex"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, codexConfigRelPath), []byte(body), 0o644))
}

// Live native settings remain user-owned, even when they equal today's default.
func TestConfigPreservesExplicitNativeMultiAgent(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"false", "true"} {
		t.Run(value, func(t *testing.T) {
			dir := t.TempDir()
			line := "multi_agent = " + value + " # project policy"
			writeExistingCodexConfig(t, dir, "[features]\n"+line+"\n")
			body := renderCodexConfig(t, dir, config.DefaultFullConfig("native-policy"),
				WithCLIVersion("codex-cli 0.153.4\n"))
			assert.Contains(t, codexSection(body, "features"), line)

			cleaned := removeAutopusCodexConfig(body)
			assert.Contains(t, cleaned, line)
			var diagnostics []adapter.ValidationError
			validateDeprecatedConfigKeys(body, &diagnostics)
			assert.False(t, hasValidationMessage(diagnostics, "obsolete features.multi_agent"))
		})
	}
}
