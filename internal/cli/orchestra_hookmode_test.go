package cli

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/terminal"
)

// sessionIDPattern mirrors the safe pattern enforced by NewHookSession and
// SendSessionEnvToPane; a generated session ID that violates it would be
// rejected at runtime and silently disable hook collection.
var sessionIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func TestHookCollectionEligible(t *testing.T) {
	cmux := stubTerminal{name: "cmux"}
	tmux := stubTerminal{name: "tmux"}
	plain := stubTerminal{name: "plain"}

	tests := []struct {
		name      string
		term      terminal.Terminal
		subproc   bool
		hookAvail bool
		want      bool
	}{
		{"cmux + hook installed", cmux, false, true, true},
		{"tmux + hook installed", tmux, false, true, true},
		{"cmux without hooks installed", cmux, false, false, false},
		{"plain terminal stays floor", plain, false, true, false},
		{"subprocess forced stays floor", cmux, true, true, false},
		{"nil terminal stays floor", nil, false, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hookCollectionEligible(tt.term, tt.subproc, tt.hookAvail); got != tt.want {
				t.Fatalf("hookCollectionEligible(%s, subproc=%v, hook=%v) = %v, want %v",
					tt.name, tt.subproc, tt.hookAvail, got, tt.want)
			}
		})
	}
}

func TestNewOrchSessionID_MatchesSafePattern(t *testing.T) {
	if got := newOrchSessionID(); !sessionIDPattern.MatchString(got) {
		t.Fatalf("newOrchSessionID() = %q does not match %s", got, sessionIDPattern)
	}
}

func TestNewOrchSessionID_IsUnique(t *testing.T) {
	t.Parallel()
	const count = 256
	ids := make(map[string]struct{}, count)
	for i := 0; i < count; i++ {
		id := newOrchSessionID()
		if _, exists := ids[id]; exists {
			t.Fatalf("newOrchSessionID() collision: %q", id)
		}
		ids[id] = struct{}{}
	}
}

func TestRunOrchestraCommand_PaneTerminalWithHookStaysSubprocess(t *testing.T) {
	projectRoot := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(projectRoot, "autopus.yaml"),
		[]byte("project: legacy-codex-hook-test\n"),
		0o600,
	))
	codexDir := filepath.Join(projectRoot, ".codex")
	require.NoError(t, os.MkdirAll(codexDir, 0o700))
	require.NoError(t, os.WriteFile(
		filepath.Join(codexDir, "hooks.json"),
		[]byte(`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":".codex/hooks/autopus/hook-codex-stop.sh"}]}]}}`),
		0o600,
	))
	writeHookFixture(t, projectRoot, ".codex/hooks/autopus/hook-codex-stop.sh", 0o700)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AUTOPUS_PLATFORM", "codex")
	t.Chdir(projectRoot)
	usePaneCapableContext(t)

	originalRun := runOrchestraExecute
	t.Cleanup(func() { runOrchestraExecute = originalRun })
	var captured orchestra.OrchestraConfig
	runOrchestraExecute = func(_ context.Context, cfg orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		captured = cfg
		return &orchestra.OrchestraResult{Merged: "ok", Summary: "done"}, nil
	}

	err := runOrchestraCommand(
		context.Background(), "review", "consensus", []string{"codex"},
		30, "", "topic", 0, 0, OrchestraFlags{NoDetach: true},
	)
	require.NoError(t, err)
	// The pane backend is retired: an active mux plus an installed Stop hook
	// must still run headless subprocesses without pane hook IPC.
	assert.Equal(t, "subprocess", selectRoutedBackend(captured).Name(), "orchestra must never select the pane backend")
	assert.False(t, captured.HookMode, "pane hook IPC must stay off")
}

func TestApplyHookMode_ForcedSubprocessClearsLegacyPaneState(t *testing.T) {
	cfg := orchestra.OrchestraConfig{
		Terminal:       stubTerminal{name: "cmux"},
		SubprocessMode: true,
		HookMode:       true,
		SessionID:      "stale-pane-session",
	}

	applyHookMode(&cfg)

	assert.False(t, cfg.HookMode)
	assert.Empty(t, cfg.SessionID)
}

func TestApplyHookMode_StopOnlySetsIndependentProviderCapabilities(t *testing.T) {
	_, projectRoot, _ := setupHookDiscoveryProject(t)
	writeHookDiscoveryFixture(t, projectRoot, ".claude/settings.json", `{
		"hooks":{"Stop":[{"hooks":[{"command":"autopus hook stop"}]}]}
	}`)
	on := true
	off := false
	cfg := orchestra.OrchestraConfig{
		Terminal: stubTerminal{name: "cmux"},
		Providers: []orchestra.ProviderConfig{
			{Name: "claude"},
			{Name: "codex"},
			{Name: "gemini", HasHook: &off, HasStartupHook: &on},
		},
	}

	applyHookMode(&cfg)

	require.True(t, cfg.HookMode)
	assertHookCapabilities(t, cfg.Providers[0], true, false)
	assertHookCapabilities(t, cfg.Providers[1], false, false)
	assertHookCapabilities(t, cfg.Providers[2], false, true)
}

func TestApplyHookMode_CodexOnlyDoesNotEnableClaudeDefaults(t *testing.T) {
	_, projectRoot, _ := setupHookDiscoveryProject(t)
	writeHookDiscoveryFixture(t, projectRoot, ".codex/hooks.json", `{
		"hooks":{
			"Stop":[{"hooks":[{"command":".codex/hooks/autopus/hook-codex-stop.sh"}]}],
			"SessionStart":[{"hooks":[{"command":".codex/hooks/autopus/hook-codex-sessionstart.sh"}]}]
		}
	}`)
	writeHookFixture(t, projectRoot, ".codex/hooks/autopus/hook-codex-stop.sh", 0o700)
	writeHookFixture(t, projectRoot, ".codex/hooks/autopus/hook-codex-sessionstart.sh", 0o700)
	cfg := orchestra.OrchestraConfig{
		Terminal:  stubTerminal{name: "cmux"},
		Providers: []orchestra.ProviderConfig{{Name: "claude"}, {Name: "codex"}},
	}

	applyHookMode(&cfg)

	require.True(t, cfg.HookMode)
	assertHookCapabilities(t, cfg.Providers[0], false, false)
	assertHookCapabilities(t, cfg.Providers[1], true, true)
}

func TestApplyHookMode_KnownAliasesPreserveNamesAndExplicitOverrides(t *testing.T) {
	_, projectRoot, _ := setupHookDiscoveryProject(t)
	writeHookDiscoveryFixture(t, projectRoot, ".claude/settings.json", `{
		"hooks":{
			"Stop":[{"hooks":[{"command":"autopus hook stop"}]}],
			"SessionStart":[{"hooks":[{"command":"autopus hook session-start"}]}]
		}
	}`)
	writeHookDiscoveryFixture(t, projectRoot, ".agents/hooks.json", `{
		"autopus":{"Stop":[{"hooks":[{"command":"autopus hook stop"}]}]}
	}`)
	off, on := false, true
	cfg := orchestra.OrchestraConfig{
		Terminal: stubTerminal{name: "cmux"},
		Providers: []orchestra.ProviderConfig{
			{Name: "claude-code"},
			{Name: "antigravity", Binary: "agy"},
			{Name: "antigravity-cli", Binary: "antigravity"},
			{Name: "gemini-cli", HasHook: &off, HasStartupHook: &on},
			{Name: "agy", Binary: "agy"},
		},
	}

	applyHookMode(&cfg)

	require.True(t, cfg.HookMode)
	assert.Equal(t,
		[]string{"claude-code", "antigravity", "antigravity-cli", "gemini-cli", "agy"},
		[]string{
			cfg.Providers[0].Name, cfg.Providers[1].Name, cfg.Providers[2].Name,
			cfg.Providers[3].Name, cfg.Providers[4].Name,
		},
	)
	assertHookCapabilities(t, cfg.Providers[0], true, true)
	assertHookCapabilities(t, cfg.Providers[1], true, false)
	assertHookCapabilities(t, cfg.Providers[2], true, false)
	assertHookCapabilities(t, cfg.Providers[3], false, true)
	assertHookCapabilities(t, cfg.Providers[4], true, false)
}

func TestApplyHookMode_ExplicitSelectedCompletionEnablesHookModeWithoutDiscovery(t *testing.T) {
	setupHookDiscoveryProject(t)
	on := true
	cfg := orchestra.OrchestraConfig{
		Terminal: stubTerminal{name: "cmux"},
		Providers: []orchestra.ProviderConfig{
			{Name: "opencode", Binary: "opencode", HasHook: &on},
			{Name: "custom", Binary: "custom", HasHook: &on},
		},
	}

	applyHookMode(&cfg)

	assert.True(t, cfg.HookMode)
	assert.NotEmpty(t, cfg.SessionID)
	assertHookCapabilities(t, cfg.Providers[0], true, false)
	assertHookCapabilities(t, cfg.Providers[1], true, false)
}

func TestApplyHookMode_EligibilityUsesSelectedResolvedCapabilities(t *testing.T) {
	_, projectRoot, _ := setupHookDiscoveryProject(t)
	writeHookDiscoveryFixture(t, projectRoot, ".claude/settings.json", `{
		"hooks":{"Stop":[{"hooks":[{"command":"autopus hook stop"}]}]}
	}`)
	off := false
	cfg := orchestra.OrchestraConfig{
		Terminal: stubTerminal{name: "cmux"},
		Providers: []orchestra.ProviderConfig{{
			Name: "claude", Binary: "claude", HasHook: &off,
		}},
	}

	applyHookMode(&cfg)

	assert.False(t, cfg.HookMode)
	assert.Empty(t, cfg.SessionID)
	assertHookCapabilities(t, cfg.Providers[0], false, false)
}

func assertHookCapabilities(
	t *testing.T,
	provider orchestra.ProviderConfig,
	wantCompletion, wantStartup bool,
) {
	t.Helper()
	if assert.NotNil(t, provider.HasHook) {
		assert.Equal(t, wantCompletion, *provider.HasHook)
	}
	if assert.NotNil(t, provider.HasStartupHook) {
		assert.Equal(t, wantStartup, *provider.HasStartupHook)
	}
}
