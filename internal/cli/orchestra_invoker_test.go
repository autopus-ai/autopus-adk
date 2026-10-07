package cli

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/detect"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

func TestOrchestraInvokerProviderFromSignals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		platform   string
		runtime    detect.AgentRuntime
		claudeCode bool
		codex      bool
		want       string
	}{
		{name: "Claude platform", platform: "claude-code", want: "claude"},
		{name: "Codex platform", platform: "codex", want: "codex"},
		{name: "Antigravity platform", platform: "antigravity-cli", want: "gemini"},
		{name: "Gemini legacy platform", platform: "gemini-cli", want: "gemini"},
		{name: "OpenCode platform", platform: "opencode", want: "opencode"},
		{name: "Codex ancestry", runtime: detect.AgentRuntimeCodex, want: "codex"},
		{name: "Claude ancestry", runtime: detect.AgentRuntimeClaudeCode, want: "claude"},
		{name: "Antigravity ancestry", runtime: detect.AgentRuntimeAntigravityCLI, want: "gemini"},
		{name: "OpenCode ancestry", runtime: detect.AgentRuntimeOpenCode, want: "opencode"},
		// REQ-018: omp has no orchestra provider and must stay unmapped on BOTH
		// axes. A future `case AgentRuntimeOMP: return "omp"` would silently
		// revive the provider registration this SPEC deliberately blocked.
		{name: "omp platform stays unmapped", platform: "omp", want: ""},
		{name: "omp ancestry stays unmapped", runtime: detect.AgentRuntimeOMP, want: ""},
		{name: "Claude marker", claudeCode: true, want: "claude"},
		{name: "Codex marker", codex: true, want: "codex"},
		{name: "conflicting markers", claudeCode: true, codex: true, want: ""},
		{name: "unknown", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := orchestraInvokerProviderFromSignals(
				tt.platform, tt.runtime, tt.claudeCode, tt.codex,
			)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveInvocationJudge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		flagJudge  string
		configured string
		invoker    string
		want       string
	}{
		{
			name:      "explicit flag wins",
			flagJudge: "gemini", configured: "claude", invoker: "codex", want: "gemini",
		},
		{
			name:       "Codex invoker replaces implicit Claude default",
			configured: "claude", invoker: "codex", want: "codex",
		},
		{
			name:       "Gemini invoker replaces implicit Claude default",
			configured: "claude", invoker: "gemini", want: "gemini",
		},
		{
			name:       "unknown invoker keeps config fallback",
			configured: "claude", want: "claude",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(
				t,
				tt.want,
				resolveInvocationJudge(tt.flagJudge, tt.configured, tt.invoker),
			)
		})
	}
}

func TestInvocationJudgeSelectionSource(t *testing.T) {
	t.Parallel()

	assert.Equal(t, orchestra.JudgeSelectionExplicit, invocationJudgeSelectionSource("gemini", "codex"))
	assert.Equal(t, orchestra.JudgeSelectionInvokingProvider, invocationJudgeSelectionSource("", "codex"))
	assert.Equal(t, orchestra.JudgeSelectionConfiguredFallback, invocationJudgeSelectionSource("", ""))
}

func TestRunOrchestraCommand_ImplicitClaudeConfigUsesCodexInvokerJudge(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultFullConfig("invoker-judge")
	cfg.Orchestra.Judge = "claude"
	require.NoError(t, config.Save(dir, cfg))
	t.Chdir(dir)
	t.Setenv("HOME", t.TempDir())

	originalRun := runOrchestraExecute
	originalInvokerDetector := detectOrchestraInvokingProvider
	t.Cleanup(func() {
		runOrchestraExecute = originalRun
		detectOrchestraInvokingProvider = originalInvokerDetector
	})

	detectOrchestraInvokingProvider = func() string { return "codex" }
	var captured orchestra.OrchestraConfig
	runOrchestraExecute = func(_ context.Context, runCfg orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		captured = runCfg
		return &orchestra.OrchestraResult{Merged: "ok", Summary: "done"}, nil
	}

	err := runOrchestraCommand(
		context.Background(),
		"brainstorm",
		"debate",
		nil,
		30,
		"",
		"topic",
		2,
		0,
		OrchestraFlags{},
	)

	require.NoError(t, err)
	assert.Equal(t, "codex", captured.JudgeProvider)
	assert.Equal(t, "codex", captured.InvokingProvider)
	assert.Equal(t, orchestra.JudgeSelectionInvokingProvider, captured.JudgeSelectionSource)
	require.NotNil(t, captured.JudgeConfig)
	assert.Equal(t, "codex", captured.JudgeConfig.Name)
	assert.Equal(t, []string{"claude", "codex", "gemini"}, providerConfigNames(captured.Providers))
}

func TestRunOrchestraCommand_ReviewDebateUsesGeminiInvokerJudge(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultFullConfig("invoker-judge-review")
	cfg.Orchestra.Judge = "claude"
	require.NoError(t, config.Save(dir, cfg))
	t.Chdir(dir)
	t.Setenv("HOME", t.TempDir())

	originalRun := runOrchestraExecute
	originalInvokerDetector := detectOrchestraInvokingProvider
	t.Cleanup(func() {
		runOrchestraExecute = originalRun
		detectOrchestraInvokingProvider = originalInvokerDetector
	})

	detectOrchestraInvokingProvider = func() string { return "gemini" }
	var captured orchestra.OrchestraConfig
	runOrchestraExecute = func(_ context.Context, runCfg orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		captured = runCfg
		return &orchestra.OrchestraResult{Merged: "ok", Summary: "done"}, nil
	}

	err := runOrchestraCommand(
		context.Background(),
		"review",
		"debate",
		nil,
		30,
		"",
		"topic",
		2,
		0,
		OrchestraFlags{},
	)

	require.NoError(t, err)
	assert.Equal(t, "gemini", captured.JudgeProvider)
	assert.Equal(t, "gemini", captured.InvokingProvider)
	assert.Equal(t, orchestra.JudgeSelectionInvokingProvider, captured.JudgeSelectionSource)
	assert.False(t, captured.NoJudge, "a required judge debate must run its judge")
}

func TestCodexRuntimeMarkerIncludesCurrentCodexCLIEnv(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name              string
		codex             string
		codexCI           string
		codexThreadID     string
		codexManagedByNPM string
		want              bool
	}{
		{"legacy CODEX", "1", "", "", "", true},
		{"codex ci marker", "", "1", "", "", true},
		{"codex thread marker", "", "", "019f4028-f88f-7051-a738-072a27975cc3", "", true},
		{"codex npm marker", "", "", "", "1", true},
		{"no codex marker", "", "", "", "", false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, hasCodexRuntimeMarker(tt.codex, tt.codexCI, tt.codexThreadID, tt.codexManagedByNPM))
		})
	}
}

func TestCurrentOrchestraInvokerProvider_UsesExplicitPlatformBeforeHostMarkers(t *testing.T) {
	t.Setenv("AUTOPUS_PLATFORM", "gemini-cli")
	t.Setenv("CLAUDECODE", "1")
	t.Setenv("CODEX", "1")
	t.Setenv("CODEX_CI", "1")
	t.Setenv("CODEX_THREAD_ID", "stale")
	t.Setenv("CODEX_MANAGED_BY_NPM", "1")

	assert.Equal(t, "gemini", currentOrchestraInvokingProvider())
}
