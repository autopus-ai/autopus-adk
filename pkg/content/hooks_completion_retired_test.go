// SPEC-PANERM-001 REQ-12 (S11 generation half): no platform receives an
// orchestra completion or ready hook any more. Those hooks fed the hook IPC of
// the retired pane backend; every provider now runs as a subprocess or through
// OMP, so nothing consumes them.
package content_test

import (
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/content"
	"github.com/insajin/autopus-adk/templates"
)

// retiredCompletionHookAssets are the group H canonical assets.
var retiredCompletionHookAssets = []string{
	"hooks/hook-claude-sessionstart.sh",
	"hooks/hook-claude-stop.sh",
	"hooks/hook-codex-sessionstart.sh",
	"hooks/hook-codex-stop.sh",
	"hooks/hook-gemini-sessionstart.sh",
	"hooks/hook-gemini-stop.sh",
	"hooks/hook-gemini-afteragent.sh",
	"hooks/hook-opencode-complete.ts",
}

func TestGenerateProjectHookConfigs_EmitsNoOrchestraCompletionOrReadyHook(t *testing.T) {
	t.Parallel()

	// Switch on every native hook the config offers, so a completion hook
	// cannot hide behind a disabled feature.
	cfg := config.DefaultFullConfig("demo")
	cfg.Hooks = config.HooksConf{PreCommitArch: true, PreCommitLore: true, ReactCIFailure: true, ReactReview: true}
	cfg.Features.CC21 = config.CC21FeaturesConf{Enabled: true, TaskCreatedEnabled: true}

	for _, platform := range []string{"claude-code", "claude", "codex", "antigravity-cli", "gemini", "gemini-cli", "opencode"} {
		hooks, _, err := content.GenerateProjectHookConfigs(cfg, platform, true)
		require.NoError(t, err, platform)
		require.NotEmpty(t, hooks, "%s keeps its other native hooks", platform)
		for _, hook := range hooks {
			assert.NotContains(t, []string{"Stop", "SessionStart", "AfterAgent"}, hook.Event, "%s: %+v", platform, hook)
			assert.NotRegexp(t, `hook-(claude|codex|gemini|opencode)-`, hook.Command, "%s: %+v", platform, hook)
		}
	}
}

func TestEmbeddedContent_ShipsNoCompletionHookAssets(t *testing.T) {
	t.Parallel()

	for _, name := range retiredCompletionHookAssets {
		_, err := fs.Stat(contentfs.FS, name)
		assert.ErrorIs(t, err, fs.ErrNotExist, name)
	}
	_, err := fs.Stat(templates.FS, "hooks/completion-hook.sh.tmpl")
	assert.ErrorIs(t, err, fs.ErrNotExist, "templates/hooks/completion-hook.sh.tmpl")
}

// findHook returns the first HookConfig whose Event matches, or nil.
func findHook(hooks []adapter.HookConfig, event string) *adapter.HookConfig {
	for i := range hooks {
		if hooks[i].Event == event {
			return &hooks[i]
		}
	}
	return nil
}

func eventNames(hooks []adapter.HookConfig) []string {
	names := make([]string, len(hooks))
	for i, h := range hooks {
		names[i] = h.Event
	}
	return names
}
