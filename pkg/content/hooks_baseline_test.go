package content_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/adapter/antigravity"
	"github.com/insajin/autopus-adk/pkg/adapter/claude"
	"github.com/insajin/autopus-adk/pkg/adapter/codex"
	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/adapter/opencode"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/content"
)

// SPEC-SIGMABAND-001 REQ-17 / S17: the SPEC registers no new hook. Each file
// in testdata/hooks-baseline/ was produced by hooksBaselineSnapshot at
// source_commit, the parent of the SPEC's first commit, with
// config.DefaultFullConfig. There is one file per adapter, listing every hook
// generator call that adapter makes; omp declares no hook support and makes
// none. A later SPEC that adds a hook on purpose updates the counts here.
const (
	hooksBaselineDir    = "testdata/hooks-baseline"
	hooksBaselineSchema = "autopus.hooks_baseline.v1"
	reactCheckCommand   = "auto react check --quiet"
)

type hooksBaseline struct {
	Schema        string              `json:"schema"`
	SourceCommit  string              `json:"source_commit"`
	Config        string              `json:"config"`
	Adapter       string              `json:"adapter"`
	SupportsHooks bool                `json:"supports_hooks"`
	Calls         []hooksBaselineCall `json:"calls"`
}

// hooksBaselineCall is one GenerateProjectHookConfigs call: its two inputs,
// the number of hook entries, the entries that run `auto react`, and the git
// hook paths.
type hooksBaselineCall struct {
	Platform      string               `json:"platform"`
	SupportsHooks bool                 `json:"supports_hooks"`
	HookCount     int                  `json:"hook_count"`
	ReactHooks    []adapter.HookConfig `json:"react_hooks"`
	GitHooks      []string             `json:"git_hooks"`
}

// hookAdapters are the platform adapters whose hook support the baseline pins.
func hookAdapters(root string) map[string]adapter.PlatformAdapter {
	return map[string]adapter.PlatformAdapter{
		"claude-code":     claude.NewWithRoot(root),
		"codex":           codex.NewWithRoot(root),
		"antigravity-cli": antigravity.NewWithRoot(root),
		"opencode":        opencode.NewWithRoot(root),
		"omp":             omp.NewWithRoot(root),
	}
}

// hooksBaselineSnapshot replays one generator call with the default config.
func hooksBaselineSnapshot(t *testing.T, platform string, supportsHooks bool) hooksBaselineCall {
	t.Helper()
	hooks, gitHooks, err := content.GenerateProjectHookConfigs(config.DefaultFullConfig("autopus"), platform, supportsHooks)
	require.NoError(t, err)

	call := hooksBaselineCall{
		Platform:      platform,
		SupportsHooks: supportsHooks,
		HookCount:     len(hooks),
		ReactHooks:    []adapter.HookConfig{},
		GitHooks:      []string{},
	}
	for _, hook := range hooks {
		if strings.Contains(hook.Command, "auto react") {
			call.ReactHooks = append(call.ReactHooks, hook)
		}
	}
	for _, script := range gitHooks {
		call.GitHooks = append(call.GitHooks, script.Path)
	}
	return call
}

func loadHooksBaseline(t *testing.T, name string) hooksBaseline {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(hooksBaselineDir, name+".json"))
	require.NoError(t, err, "every hook-capable adapter needs a baseline file")
	var baseline hooksBaseline
	require.NoError(t, json.Unmarshal(data, &baseline))
	require.Equal(t, hooksBaselineSchema, baseline.Schema)
	require.Equal(t, name, baseline.Adapter)
	require.NotEmpty(t, baseline.SourceCommit)
	return baseline
}

func TestHooksBaseline_DefaultConfigHookEntriesUnchanged(t *testing.T) {
	t.Parallel()

	adapters := hookAdapters(t.TempDir())
	entries, err := os.ReadDir(hooksBaselineDir)
	require.NoError(t, err)
	assert.Len(t, entries, len(adapters), "one baseline file per adapter, no stray file")

	for name, platformAdapter := range adapters {
		baseline := loadHooksBaseline(t, name)
		assert.Equal(t, baseline.SupportsHooks, platformAdapter.SupportsHooks(), "%s hook support", name)
		if !baseline.SupportsHooks {
			assert.Empty(t, baseline.Calls, "%s declares no hooks, so it makes no generator call", name)
		}
		for _, want := range baseline.Calls {
			got := hooksBaselineSnapshot(t, want.Platform, want.SupportsHooks)
			assert.Equal(t, want.HookCount, got.HookCount,
				"%s/%s supports_hooks=%v: hook entry count changed", name, want.Platform, want.SupportsHooks)
			assert.Equal(t, want.ReactHooks, got.ReactHooks,
				"%s/%s supports_hooks=%v: auto react entries changed", name, want.Platform, want.SupportsHooks)
			assert.Equal(t, want.GitHooks, got.GitHooks,
				"%s/%s supports_hooks=%v: git hooks changed", name, want.Platform, want.SupportsHooks)
		}
	}
}

func TestHooksBaseline_ClaudePostToolUseHasOneReactCheck(t *testing.T) {
	t.Parallel()

	hooks, _, err := content.GenerateProjectHookConfigs(config.DefaultFullConfig("autopus"), "claude-code", true)
	require.NoError(t, err)

	var reactChecks []adapter.HookConfig
	for _, hook := range hooks {
		if hook.Event == "PostToolUse" && hook.Command == reactCheckCommand {
			reactChecks = append(reactChecks, hook)
		}
	}
	require.Len(t, reactChecks, 1, "react_ci_failure and react_review share one entry")
	assert.Equal(t, adapter.HookConfig{
		Event: "PostToolUse", Matcher: "Bash", Type: "command", Command: reactCheckCommand, Timeout: 60,
	}, reactChecks[0])
}
