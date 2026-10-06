package cli

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

func TestReadOnlyProviderPolicy_ProjectsNativeProviderArgv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   orchestra.ProviderConfig
		want orchestra.ProviderConfig
	}{
		{
			name: "claude permission plan",
			in: orchestra.ProviderConfig{
				Name: "claude", Binary: "claude", ModelFamily: "anthropic",
				Args: []string{"--print", "--model", "opus"}, PaneArgs: []string{"--model", "opus"},
			},
			want: orchestra.ProviderConfig{
				Name: "claude", Binary: "claude", ModelFamily: "anthropic",
				Args:        []string{"--print", "--model", "opus", "--permission-mode", "plan", "--safe-mode", "--no-session-persistence", "--disable-slash-commands", "--strict-mcp-config", "--tools=Read,Grep,Glob"},
				PaneArgs:    []string{"--model", "opus", "--permission-mode", "plan", "--safe-mode", "--no-session-persistence", "--disable-slash-commands", "--strict-mcp-config", "--tools=Read,Grep,Glob"},
				SandboxMode: orchestra.SandboxModeReadOnly,
			},
		},
		{
			name: "codex sandbox read only",
			in: orchestra.ProviderConfig{
				Name: "codex", Binary: "codex", ModelFamily: "openai",
				Args: []string{"exec", "--sandbox", "workspace-write", "-m", "gpt-5.6-sol"}, PaneArgs: []string{"-m", "gpt-5.6-sol"},
			},
			want: orchestra.ProviderConfig{
				Name: "codex", Binary: "codex", ModelFamily: "openai",
				Args:        []string{"exec", "--sandbox", "read-only", "-m", "gpt-5.6-sol", "--ephemeral", "--ignore-user-config", "--ignore-rules"},
				PaneArgs:    []string{"-m", "gpt-5.6-sol", "--sandbox", "read-only", "--ephemeral", "--ignore-user-config", "--ignore-rules"},
				SandboxMode: orchestra.SandboxModeReadOnly,
			},
		},
		{
			name: "gemini plan sandbox",
			in: orchestra.ProviderConfig{
				Name: "gemini", Binary: "agy", ModelFamily: "google",
				Args: []string{"--print", ""}, PaneArgs: []string{"--model", "gemini-3"}, PromptViaArgs: true,
			},
			want: orchestra.ProviderConfig{
				Name: "gemini", Binary: "agy", ModelFamily: "google",
				Args:          []string{"--print", "", "--mode", "plan", "--sandbox", "--disable-slash-commands"},
				PaneArgs:      []string{"--model", "gemini-3", "--mode", "plan", "--sandbox", "--disable-slash-commands"},
				PromptViaArgs: true, SandboxMode: orchestra.SandboxModeReadOnly,
			},
		},
		{
			name: "omp backend is read-only by construction",
			in: orchestra.ProviderConfig{
				Name: "codex", Backend: config.ProviderBackendOMP, Binary: config.ProviderBackendOMP,
				Model: "openai-codex/gpt-6-astra:max", Tools: []string{"read", "grep"},
			},
			want: orchestra.ProviderConfig{
				Name: "codex", Backend: config.ProviderBackendOMP, Binary: config.ProviderBackendOMP,
				Model: "openai-codex/gpt-6-astra:max", Tools: []string{"read", "grep"},
				SandboxMode: orchestra.SandboxModeReadOnly,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			originalArgs := append([]string(nil), tt.in.Args...)
			originalPaneArgs := append([]string(nil), tt.in.PaneArgs...)

			got, err := applyReadOnlyProviderPolicy([]orchestra.ProviderConfig{tt.in}, readOnlyPolicyOptions{})
			require.NoError(t, err)
			require.Len(t, got, 1)
			assert.Equal(t, tt.want, got[0])
			assert.Equal(t, originalArgs, tt.in.Args, "projection must not mutate caller-owned Args")
			assert.Equal(t, originalPaneArgs, tt.in.PaneArgs, "projection must not mutate caller-owned PaneArgs")
		})
	}
}

func TestReadOnlyProviderPolicy_OutsideRepoAddsCodexRepoCheckSkip(t *testing.T) {
	t.Parallel()

	got, err := applyReadOnlyProviderPolicy([]orchestra.ProviderConfig{{
		Name: "codex", Binary: "codex", Args: []string{"exec", "--sandbox", "workspace-write"}, PaneArgs: []string{"-m", "gpt"},
	}}, readOnlyPolicyOptions{OutsideRepo: true})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, []string{"exec", "--sandbox", "read-only", "--ephemeral", "--ignore-user-config", "--ignore-rules", "--skip-git-repo-check"}, got[0].Args)
	assert.NotContains(t, got[0].PaneArgs, "--skip-git-repo-check", "interactive codex has no exec-only repo check flag")

	again, err := applyReadOnlyProviderPolicy(got, readOnlyPolicyOptions{OutsideRepo: true})
	require.NoError(t, err)
	assert.Equal(t, got[0].Args, again[0].Args, "projection must be idempotent")
}

func TestReadOnlyProviderPolicy_DangerousAndUnknownFailClosed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider orchestra.ProviderConfig
	}{
		{
			name: "claude dangerous permission bypass",
			provider: orchestra.ProviderConfig{
				Name: "claude", Binary: "claude", Args: []string{"--dangerously-skip-permissions"},
			},
		},
		{
			name: "codex dangerous sandbox bypass",
			provider: orchestra.ProviderConfig{
				Name: "codex", Binary: "codex", Args: []string{"exec", "--dangerously-bypass-approvals-and-sandbox"},
			},
		},
		{
			name: "gemini dangerous yolo",
			provider: orchestra.ProviderConfig{
				Name: "gemini", Binary: "agy", PaneArgs: []string{"--yolo"},
			},
		},
		{
			name: "codex profile injection",
			provider: orchestra.ProviderConfig{
				Name: "codex", Binary: "codex", Args: []string{"exec", "--profile", "attacker"},
			},
		},
		{
			name: "codex absolute binary path",
			provider: orchestra.ProviderConfig{
				Name: "codex", Binary: "/tmp/codex", Args: []string{"exec"},
			},
		},
		{
			name: "claude whitespace-padded binary",
			provider: orchestra.ProviderConfig{
				Name: "claude", Binary: " claude ", Args: []string{"--print"},
			},
		},
		{
			name: "unknown provider",
			provider: orchestra.ProviderConfig{
				Name: "custom", Binary: "custom-agent", Args: []string{"--read-only"},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := applyReadOnlyProviderPolicy([]orchestra.ProviderConfig{tt.provider}, readOnlyPolicyOptions{})
			require.Error(t, err)
			assert.Nil(t, got, "fail-close must not return a partially launchable provider set")
		})
	}
}

func TestApplyCommandReadOnlyPolicy_OnlyPlanAndBrainstormProject(t *testing.T) {
	t.Parallel()

	unsafe := []orchestra.ProviderConfig{{Name: "claude", Binary: "claude", Args: []string{"--dangerously-skip-permissions"}}}
	for _, command := range []string{"plan", "brainstorm"} {
		_, err := applyCommandReadOnlyPolicy(command, unsafe, readOnlyPolicyOptions{})
		assert.Error(t, err, "%s must fail closed on a permission bypass", command)
	}
	for _, command := range []string{"review", "secure"} {
		got, err := applyCommandReadOnlyPolicy(command, unsafe, readOnlyPolicyOptions{})
		require.NoError(t, err, "%s keeps its provider argv", command)
		assert.Equal(t, unsafe, got)
	}
}

func TestNewOrchestraPlanCmd_RegistersSafetyFlags(t *testing.T) {
	t.Parallel()

	cmd := newOrchestraPlanCmd()
	subprocessFlag := cmd.Flags().Lookup("subprocess")
	require.NotNil(t, subprocessFlag, "plan must expose the headless provider backend")
	assert.Equal(t, "false", subprocessFlag.DefValue)
	require.NoError(t, cmd.Flags().Set("subprocess", "true"))
	assert.Equal(t, "true", subprocessFlag.Value.String())

	noPersistFlag := cmd.Flags().Lookup("no-persist")
	require.NotNil(t, noPersistFlag, "plan must expose stdout-only artifact handling")
	assert.Equal(t, "false", noPersistFlag.DefValue)
	require.NoError(t, cmd.Flags().Set("no-persist", "true"))
	assert.Equal(t, "true", noPersistFlag.Value.String())
}

func TestRunOrchestraCommand_NoPersistLeavesNoOrchestraArtifact(t *testing.T) {
	root := t.TempDir()
	originalWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(originalWD) })
	require.NoError(t, os.Chdir(root))

	originalRun := runOrchestraExecute
	t.Cleanup(func() { runOrchestraExecute = originalRun })
	runOrchestraExecute = func(_ context.Context, _ orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		return &orchestra.OrchestraResult{Merged: "advisory", Summary: "done"}, nil
	}

	err = runOrchestraCommand(
		context.Background(), "plan", "consensus", []string{"claude"}, 30, "", "topic", 0, 0,
		OrchestraFlags{NoDetach: true, NoPersist: true},
	)
	require.NoError(t, err)
	_, statErr := os.Stat(".autopus/orchestra")
	require.ErrorIs(t, statErr, os.ErrNotExist)
}
