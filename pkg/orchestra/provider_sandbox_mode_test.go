package orchestra

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// projectedClaudeArgv is the read-only claude argv (acceptance P_claude).
var projectedClaudeArgv = []string{
	"--print", "--model", "claude-fable-5-1", "--effort", "max", "--permission-mode", "plan", "--safe-mode",
	"--no-session-persistence", "--disable-slash-commands", "--strict-mcp-config", "--tools=Read,Grep,Glob",
}

func TestProviderSandboxMode_InfersFromArgv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider ProviderConfig
		args     []string
		want     string
	}{
		{name: "bypass flag beats policy stamp", provider: ProviderConfig{SandboxMode: SandboxModeReadOnly}, args: []string{"--dangerously-skip-permissions"}, want: SandboxModeUnrestricted},
		{name: "codex workspace write", args: []string{"exec", "--sandbox", "workspace-write", "-m", "gpt"}, want: SandboxModeWorkspaceWrite},
		{name: "codex inline sandbox", args: []string{"exec", "--sandbox=read-only"}, want: SandboxModeReadOnly},
		{name: "claude plan permission", args: []string{"--print", "--permission-mode", "plan"}, want: SandboxModeReadOnly},
		{name: "claude bypass", args: []string{"--print", "--dangerously-skip-permissions"}, want: SandboxModeUnrestricted},
		{name: "gemini boolean sandbox flag is not a value", args: []string{"--print", "", "--sandbox", "--disable-slash-commands"}, want: SandboxModeUnrestricted},
		{name: "no restriction declared", args: []string{"--print", "--model", "opus"}, want: SandboxModeUnrestricted},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, ProviderSandboxMode(tt.provider, tt.args))
		})
	}
}

// S9: a permission or sandbox bypass in the argv beats the policy stamp, so a
// stamped provider reports read-only only while no bypass reaches its argv.
func TestProviderSandboxMode_BypassBeatsReadOnlyStamp(t *testing.T) {
	t.Parallel()

	stamped := ProviderConfig{Name: "claude", Binary: "claude", SandboxMode: SandboxModeReadOnly}
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "projected claude argv", args: projectedClaudeArgv, want: SandboxModeReadOnly},
		{name: "no argv keeps the stamp", args: nil, want: SandboxModeReadOnly},
		{name: "claude skip permissions", args: []string{"--print", "--dangerously-skip-permissions"}, want: SandboxModeUnrestricted},
		{name: "bypass after plan mode", args: append(append([]string(nil), projectedClaudeArgv...), "--dangerously-skip-permissions"), want: SandboxModeUnrestricted},
		{name: "codex approvals and sandbox bypass", args: []string{"exec", "--dangerously-bypass-approvals-and-sandbox"}, want: SandboxModeUnrestricted},
		{name: "yolo", args: []string{"--print", "prompt", "--yolo"}, want: SandboxModeUnrestricted},
		{name: "short yolo", args: []string{"--print", "prompt", "-y"}, want: SandboxModeUnrestricted},
		{name: "claude bypass permission mode", args: []string{"--print", "--permission-mode", "bypassPermissions"}, want: SandboxModeUnrestricted},
		{name: "inline bypass permission mode", args: []string{"--print", "--permission-mode=bypassPermissions"}, want: SandboxModeUnrestricted},
		{name: "codex full access sandbox", args: []string{"exec", "--sandbox", "danger-full-access"}, want: SandboxModeUnrestricted},
		{name: "codex inline full access sandbox", args: []string{"exec", "--sandbox=danger-full-access"}, want: SandboxModeUnrestricted},
		{name: "codex short alias full access", args: []string{"exec", "-s", "danger-full-access"}, want: SandboxModeUnrestricted},
		{name: "prompt text naming a bypass is not a flag", args: []string{"--print", "review the bypass and yolo paths"}, want: SandboxModeReadOnly},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, ProviderSandboxMode(stamped, tt.args))
		})
	}
}

// REQ-07: the recorded sandbox mode reads the executed argv, which includes
// runtime items the subprocess backend appends after the projected Args.
func TestSubprocessBackend_RecordedSandboxModeReadsExecutedArgv(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	binary := writePwdRecordingProvider(t, dir, "claude", filepath.Join(dir, "pwd.txt"), "VERDICT: PASS")
	execute := func(schemaFlag string) *ProviderExecution {
		resp, err := NewSubprocessBackendImpl().Execute(context.Background(), ProviderRequest{
			Provider: "claude", Prompt: "Review SPEC-X", Timeout: 10 * time.Second, SchemaPath: filepath.Join(dir, "schema.json"),
			Config: ProviderConfig{
				Name: "claude", Binary: binary, Args: append([]string(nil), projectedClaudeArgv...),
				SchemaFlag: schemaFlag, SandboxMode: SandboxModeReadOnly, OutputFormat: "text",
			},
		})
		require.NoError(t, err)
		require.NotNil(t, resp.Execution)
		return resp.Execution
	}

	clean := execute("")
	assert.Equal(t, append([]string{binary}, projectedClaudeArgv...), clean.Command)
	assert.Equal(t, SandboxModeReadOnly, clean.SandboxMode)

	bypassed := execute("--permission-mode=bypassPermissions")
	assert.Contains(t, bypassed.Command, "--permission-mode=bypassPermissions")
	assert.Equal(t, SandboxModeUnrestricted, bypassed.SandboxMode, "a bypass in a runtime item must not keep the read-only stamp")
}

// SPEC-REVIEWRO-001 M1: agy's read-only flags have no live evidence until
// RFP-3 passes, so an unverified stamp survives the projected agy argv and a
// bypass in the executed argv still beats it.
func TestProviderSandboxMode_UnverifiedStampIsKeptUnlessBypassed(t *testing.T) {
	t.Parallel()

	stamped := ProviderConfig{Name: "gemini", Binary: "agy", SandboxMode: SandboxModeUnverified}
	projected := []string{"--print", "Review SPEC-X", "--mode", "plan", "--sandbox", "--disable-slash-commands"}

	assert.Equal(t, "unverified", ProviderSandboxMode(stamped, projected))
	assert.Equal(t, SandboxModeUnrestricted, ProviderSandboxMode(stamped, append(projected, "--yolo")))
}
