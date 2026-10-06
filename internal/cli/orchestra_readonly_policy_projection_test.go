package cli

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// claudeReadOnlySuffix is the policy tail every projected claude argv ends with.
var claudeReadOnlySuffix = []string{
	"--permission-mode", "plan", "--safe-mode", "--no-session-persistence", "--disable-slash-commands",
	"--strict-mcp-config", "--tools=Read,Grep,Glob",
}

func withClaudeReadOnlySuffix(args ...string) []string {
	return append(append([]string(nil), args...), claudeReadOnlySuffix...)
}

func projectReadOnlyArgsForTest(t *testing.T, name, binary string, args []string) ([]string, []string, error) {
	t.Helper()
	got, err := applyReadOnlyProviderPolicy([]orchestra.ProviderConfig{{
		Name: name, Binary: binary, Args: append([]string(nil), args...), PaneArgs: append([]string(nil), args...),
	}}, readOnlyPolicyOptions{})
	if err != nil {
		return nil, nil, err
	}
	require.Len(t, got, 1)
	return got[0].Args, got[0].PaneArgs, nil
}

// S4: claude built-in tools shrink to Read, Grep, Glob and MCP servers are
// closed; config may name --tools only with exactly that value.
func TestReadOnlyProviderPolicy_ClaudeRestrictsToolsAndMCP(t *testing.T) {
	t.Parallel()

	accepted := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "separated policy tools",
			args: []string{"--print", "--tools", "Read,Grep,Glob"},
			want: withClaudeReadOnlySuffix("--print"),
		},
		{
			name: "inline policy tools",
			args: []string{"--print", "--tools=Read,Grep,Glob", "--model", "m"},
			want: withClaudeReadOnlySuffix("--print", "--model", "m"),
		},
		{
			name: "strict mcp stays in place",
			args: []string{"--print", "--strict-mcp-config", "--model", "m"},
			want: []string{
				"--print", "--strict-mcp-config", "--model", "m", "--permission-mode", "plan", "--safe-mode",
				"--no-session-persistence", "--disable-slash-commands", "--tools=Read,Grep,Glob",
			},
		},
	}
	for _, tt := range accepted {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			args, paneArgs, err := projectReadOnlyArgsForTest(t, "claude", "claude", tt.args)
			require.NoError(t, err)
			assert.Equal(t, tt.want, args)
			assert.Equal(t, tt.want, paneArgs)
		})
	}

	rejected := []struct {
		name string
		args []string
		want string
	}{
		{name: "widened tools", args: []string{"--print", "--tools", "Bash,Read"}, want: `read-only provider policy: provider "claude" contains unsafe value for "--tools"`},
		{name: "default tools", args: []string{"--print", "--tools=default"}, want: `read-only provider policy: provider "claude" contains unsafe value for "--tools"`},
		{name: "variadic tools", args: []string{"--print", "--tools", "Read", "Grep"}, want: `read-only provider policy: provider "claude" contains unsafe value for "--tools"`},
		{name: "mcp config", args: []string{"--print", "--mcp-config", `{"mcpServers":{}}`}, want: `read-only provider policy: provider "claude" contains unsupported argv "--mcp-config"`},
	}
	for _, tt := range rejected {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := projectReadOnlyArgsForTest(t, "claude", "claude", tt.args)
			require.Error(t, err)
			assert.Equal(t, tt.want, err.Error())
		})
	}
}

// S7: values the projection narrows toward read-only are replaced in place of
// a rejection, and listed benign values are kept.
func TestReadOnlyProviderPolicy_ReplacesNarrowingValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, provider, binary string
		args, want             []string
	}{
		{
			name: "claude permission mode", provider: "claude", binary: "claude",
			args: []string{"--print", "--permission-mode", "acceptEdits"},
			want: withClaudeReadOnlySuffix("--print"),
		},
		{
			name: "claude benign values", provider: "claude", binary: "claude",
			args: []string{"--print", "--model", "claude-opus-5-5", "--output-format", "json"},
			want: withClaudeReadOnlySuffix("--print", "--model", "claude-opus-5-5", "--output-format", "json"),
		},
		{
			// codex-cli 0.160.0 exits 2 on "-s workspace-write --sandbox read-only".
			name: "codex short sandbox alias", provider: "codex", binary: "codex",
			args: []string{"exec", "-s", "workspace-write"},
			want: []string{"exec", "--sandbox", "read-only", "--ephemeral", "--ignore-user-config", "--ignore-rules"},
		},
		{
			name: "codex inline short sandbox alias", provider: "codex", binary: "codex",
			args: []string{"exec", "-s=workspace-write", "--json"},
			want: []string{"exec", "--json", "--sandbox", "read-only", "--ephemeral", "--ignore-user-config", "--ignore-rules"},
		},
		{
			name: "gemini mode", provider: "gemini", binary: "agy",
			args: []string{"--print", "", "--mode", "default"},
			want: []string{"--print", "", "--mode", "plan", "--sandbox", "--disable-slash-commands"},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			args, _, err := projectReadOnlyArgsForTest(t, tt.provider, tt.binary, tt.args)
			require.NoError(t, err)
			assert.Equal(t, tt.want, args)
		})
	}
}

// readOnlyArgvRecorderScript records argv one item per line and stdin, then
// answers like a reviewer. It never touches the caller's worktree.
const readOnlyArgvRecorderScript = `#!/bin/sh
name=$(basename "$0")
cat > "$AUTOPUS_TEST_ARGV_DIR/$name.stdin"
printf '%s\n' "$@" > "$AUTOPUS_TEST_ARGV_DIR/$name.argv"
printf 'VERDICT: PASS\n'
`

// installReadOnlyArgvRecorders puts recorder binaries with the given native
// names first on PATH and returns the evidence directory.
func installReadOnlyArgvRecorders(t *testing.T, names ...string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("argv recorders require a POSIX shell")
	}
	binDir, evidenceDir := t.TempDir(), t.TempDir()
	for _, name := range names {
		require.NoError(t, os.WriteFile(filepath.Join(binDir, name), []byte(readOnlyArgvRecorderScript), 0o755))
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AUTOPUS_TEST_ARGV_DIR", evidenceDir)
	return evidenceDir
}

func readRecordedArgv(t *testing.T, evidenceDir, name string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(evidenceDir, name+".argv"))
	require.NoError(t, err, "%s must have executed", name)
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

// S5: the tool restriction never swallows a positional prompt. RFP-1 showed
// the separated --tools form consumed the prompt (exit 1, 0 requests).
func TestReadOnlyProviderPolicy_ClaudePromptViaArgsStaysPositional(t *testing.T) {
	evidence := installReadOnlyArgvRecorders(t, "claude")
	projected, err := applyReadOnlyProviderPolicy([]orchestra.ProviderConfig{{
		Name: "claude", Binary: "claude", PromptViaArgs: true,
		Args: []string{"--print", "--model", "claude-fable-5-1", "--effort", "max", "--tools", "Read,Grep,Glob"},
	}}, readOnlyPolicyOptions{})
	require.NoError(t, err)

	resp, err := orchestra.NewSubprocessBackendImpl().Execute(context.Background(), orchestra.ProviderRequest{
		Provider: "claude", Prompt: "Review SPEC-X", Config: projected[0], Timeout: 10 * time.Second,
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Execution)

	want := append(withClaudeReadOnlySuffix("--print", "--model", "claude-fable-5-1", "--effort", "max"), "Review SPEC-X")
	assert.Equal(t, append([]string{"claude"}, want...), resp.Execution.Command)
	assert.Equal(t, want, readRecordedArgv(t, evidence, "claude"))
	assert.NotContains(t, want, "--tools", "no separated --tools item may precede the prompt")
}

// fDefaultReadOnlyProviders is acceptance fixture F-default as provider configs.
func fDefaultReadOnlyProviders() []orchestra.ProviderConfig {
	return []orchestra.ProviderConfig{
		{
			Name: "claude", Binary: "claude",
			Args:     []string{"--print", "--model", "claude-fable-5-1", "--effort", "max"},
			PaneArgs: []string{"--print", "--model", "claude-fable-5-1", "--effort", "max"},
		},
		{
			Name: "codex", Binary: "codex", SchemaFlag: "--output-schema",
			Args:     []string{"exec", "--json", "--sandbox", "workspace-write", "-m", "gpt-5.6-sol", "-c", `model_reasoning_effort="max"`},
			PaneArgs: []string{"-m", "gpt-5.6-sol", "-c", `model_reasoning_effort="max"`},
		},
		{Name: "gemini", Binary: "agy", Args: []string{"--print", ""}, PaneArgs: []string{}, PromptViaArgs: true},
	}
}

// S3 idempotency: projecting a projected config returns identical argv and
// stamp, so a judge reused from a projected reviewer stays unchanged.
func TestReadOnlyProviderPolicy_ProjectionIsIdempotent(t *testing.T) {
	t.Parallel()

	once, err := applyReadOnlyProviderPolicy(fDefaultReadOnlyProviders(), readOnlyPolicyOptions{})
	require.NoError(t, err)
	twice, err := applyReadOnlyProviderPolicy(once, readOnlyPolicyOptions{})
	require.NoError(t, err)
	assert.Equal(t, once, twice)
	assert.Equal(t, withClaudeReadOnlySuffix("--print", "--model", "claude-fable-5-1", "--effort", "max"), twice[0].Args)
	for _, provider := range twice {
		want := orchestra.SandboxModeReadOnly
		if provider.Name == "gemini" {
			want = orchestra.SandboxModeUnverified
		}
		assert.Equal(t, want, provider.SandboxMode, provider.Name)
	}
}

// A separated value that looks like a flag is ambiguous: the projection keeps
// it as a flag while the provider CLI may consume it as the value, so an
// unvalidated flag would escape the allowlist or a policy flag would be
// swallowed (scope expansion of REQ-04, flagged in the change report).
func TestReadOnlyProviderPolicy_FlagLikeSeparatedValueFailsClosed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, provider, binary string
		args                   []string
		want                   string
	}{
		{name: "claude value swallows a policy flag", provider: "claude", binary: "claude", args: []string{"--print", "--model", "--safe-mode"}, want: `read-only provider policy: provider "claude" contains unsafe value for "--model"`},
		{name: "codex sandbox smuggles a flag", provider: "codex", binary: "codex", args: []string{"exec", "--sandbox", "--search"}, want: `read-only provider policy: provider "codex" contains unsafe value for "--sandbox"`},
		{name: "codex short alias smuggles a flag", provider: "codex", binary: "codex", args: []string{"exec", "-s", "--search"}, want: `read-only provider policy: provider "codex" contains unsafe value for "-s"`},
		{name: "gemini mode smuggles a flag", provider: "gemini", binary: "agy", args: []string{"--print", "", "--mode", "--sandbox"}, want: `read-only provider policy: provider "gemini" contains unsafe value for "--mode"`},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := projectReadOnlyArgsForTest(t, tt.provider, tt.binary, tt.args)
			require.Error(t, err)
			assert.Equal(t, tt.want, err.Error())
		})
	}
}
