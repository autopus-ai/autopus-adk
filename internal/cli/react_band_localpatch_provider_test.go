package cli

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// lpOMPEntry is this repository's OMP entry of a provider (autopus.yaml).
func lpOMPEntry(model string) config.ProviderEntry {
	return config.ProviderEntry{Backend: config.ProviderBackendOMP, Model: model}
}

func lpHarness(localPatchProvider, judge, diagnosisProvider string, providers map[string]config.ProviderEntry) *config.HarnessConfig {
	h := bandHarness(judge, diagnosisProvider, providers)
	h.HealthBand.AllowLocalPatch, h.HealthBand.LocalPatchProvider = true, localPatchProvider
	return h
}

func TestBandConfinedProvider_Resolve_SubprocessClaude(t *testing.T) {
	t.Parallel()
	subprocess := config.ProviderEntry{Binary: "claude", Args: []string{"--print", "--model", "sonnet"}}
	allOMP := map[string]config.ProviderEntry{
		"claude": lpOMPEntry("anthropic/claude-opus-5-5:max"), "codex": lpOMPEntry("openai/gpt-5.5"), "gemini": lpOMPEntry("google/gemini-3"),
	}
	cases := []struct {
		name    string
		harness *config.HarnessConfig
		head    []string
	}{
		{"key with OMP claude takes the model rule", lpHarness("claude", "claude", "", allOMP),
			[]string{"--print", "--model", "claude-opus-5-5", "--effort", "max"}},
		{"key wins over diagnosis_provider codex", lpHarness(" claude ", "claude", "codex", allOMP),
			[]string{"--print", "--model", "claude-opus-5-5", "--effort", "max"}},
		{"OMP model of another provider keeps the default", lpHarness("claude", "", "", map[string]config.ProviderEntry{"claude": lpOMPEntry("openai/gpt-5.5")}),
			[]string{"--print", "--model", config.ClaudeFableModel, "--effort", "max"}},
		{"OMP alias model keeps the default", lpHarness("claude", "", "", map[string]config.ProviderEntry{"claude": lpOMPEntry("anthropic/sonnet")}),
			[]string{"--print", "--model", config.ClaudeFableModel, "--effort", "max"}},
		{"no claude entry takes the shipped default", lpHarness("claude", "", "", nil),
			[]string{"--print", "--model", config.ClaudeFableModel, "--effort", "max"}},
		{"key with a subprocess entry takes the entry", lpHarness("claude", "", "", map[string]config.ProviderEntry{"claude": subprocess}),
			[]string{"--print", "--model", "sonnet"}},
		{"no key: 001's selection of a subprocess claude", lpHarness("", "claude", "", map[string]config.ProviderEntry{"claude": subprocess}),
			[]string{"--print", "--model", "sonnet"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			provider, reason := newBandConfinedProvider(tc.harness).resolve()
			require.Empty(t, reason)
			assert.Equal(t, "claude", provider.Binary)
			assert.Empty(t, provider.Backend)
			assert.Equal(t, tc.head, provider.Args[:len(tc.head)])
			for _, flag := range []string{"--restricted", "--verbose", "--strict-mcp-config", "--safe-mode"} {
				assert.Contains(t, provider.Args, flag)
			}
			assert.Equal(t, []string{"stream-json"}, bandFlagValues(provider.Args, "--output-format"))
			assert.Equal(t, []string{"plan"}, bandFlagValues(provider.Args, "--permission-mode"))
			assert.Equal(t, "--tools=Read,Grep,Glob", provider.Args[len(provider.Args)-1])
			assert.NotContains(t, provider.Args, "--bare")
			assert.NotContains(t, provider.Args, "anthropic/claude-opus-5-5:max")
		})
	}
}

func TestBandConfinedProvider_Resolve_Unavailable(t *testing.T) {
	t.Parallel()
	subprocess := config.ProviderEntry{Binary: "claude", Args: []string{"--print"}}
	codex := config.ProviderEntry{Binary: "codex", Args: []string{"exec"}}
	allOMP := map[string]config.ProviderEntry{"claude": lpOMPEntry("anthropic/claude-opus-5-5:max"), "codex": lpOMPEntry("openai/gpt-5.5"), "gemini": lpOMPEntry("google/x")}
	cases := []struct {
		name    string
		harness *config.HarnessConfig
		want    string
	}{
		{"all-OMP orchestra without the key", lpHarness("", "claude", "", allOMP), bandProviderUnconfined},
		{"spaces-only key", lpHarness("   ", "claude", "", allOMP), bandProviderUnconfined},
		{"diagnosis_provider codex", lpHarness("", "claude", "codex", map[string]config.ProviderEntry{"codex": codex, "claude": subprocess}), bandProviderUnconfined},
		{"diagnosis_provider gemini", lpHarness("", "", "gemini", allOMP), bandProviderUnconfined},
		{"a set key naming codex is final", lpHarness("codex", "claude", "", map[string]config.ProviderEntry{"claude": subprocess}), bandProviderUnconfined},
		{"judge claude without an entry", lpHarness("", "claude", "", nil), bandProviderUnconfined},
		{"no harness", nil, bandProviderUnconfined},
		{"configured --verbose", lpHarness("claude", "", "", map[string]config.ProviderEntry{"claude": {Binary: "claude", Args: []string{"--print", "--verbose"}}}), bandProviderPolicyRejected},
		{"configured --bare", lpHarness("claude", "", "", map[string]config.ProviderEntry{"claude": {Binary: "claude", Args: []string{"--bare"}}}), bandProviderPolicyRejected},
		{"configured --add-dir", lpHarness("claude", "", "", map[string]config.ProviderEntry{"claude": {Binary: "claude", Args: []string{"--add-dir", "/"}}}), bandProviderPolicyRejected},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, reason := newBandConfinedProvider(tc.harness).resolve()
			assert.Equal(t, tc.want, reason)
		})
	}
}

func TestBandConfinedProvider_Resolve_ProjectionWithoutRestricted_Unconfined(t *testing.T) {
	t.Parallel()
	provider := newBandConfinedProvider(lpHarness("claude", "", "", nil))
	provider.project = func(in []orchestra.ProviderConfig, opts readOnlyPolicyOptions) ([]orchestra.ProviderConfig, error) {
		out, err := applyReadOnlyProviderPolicy(in, opts)
		if err == nil {
			out[0].Args = slices.DeleteFunc(out[0].Args, func(arg string) bool { return arg == "--restricted" })
		}
		return out, err
	}
	_, reason := provider.resolve()
	assert.Equal(t, bandProviderUnconfined, reason)
}

// lpFakeClaudeScript records its argv, cwd, environment, and stdin under
// LP_FAKE_LOG and prints the stream file LP_FAKE_STREAM.
const lpFakeClaudeScript = `#!/bin/sh
echo call >> "$LP_FAKE_LOG/calls"
printf '%s\n' "$@" > "$LP_FAKE_LOG/argv"
pwd -P > "$LP_FAKE_LOG/cwd"
env > "$LP_FAKE_LOG/env"
cat > "$LP_FAKE_LOG/stdin"
cat "$LP_FAKE_STREAM"
`

// lpFakeClaude is the fake claude binary on PATH and its record.
type lpFakeClaude struct{ logs, stream string }

func installLPFakeClaude(t *testing.T) lpFakeClaude {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake claude binary is a POSIX shell script")
	}
	bin, logs := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bin, "claude"), []byte(lpFakeClaudeScript), 0o755))
	fake := lpFakeClaude{logs: logs, stream: filepath.Join(logs, "stream.jsonl")}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("LP_FAKE_LOG", logs)
	t.Setenv("LP_FAKE_STREAM", fake.stream)
	return fake
}

func (f lpFakeClaude) setStream(t *testing.T, stream string) {
	t.Helper()
	require.NoError(t, os.WriteFile(f.stream, []byte(stream), 0o600))
}

func (f lpFakeClaude) record(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.logs, name))
	if os.IsNotExist(err) {
		return ""
	}
	require.NoError(t, err)
	return string(data)
}

func TestBandConfinedProvider_Request_ConfinedSubprocess(t *testing.T) {
	fake := installLPFakeClaude(t)
	t.Setenv("GH_TOKEN", "ghp_synthetic")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "x")
	t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", "http://169.254.170.2/x")
	t.Setenv("GIT_DIR", "/elsewhere/.git")
	t.Setenv("GIT_CONFIG_PARAMETERS", "'core.hooksPath'='x'")
	fake.setStream(t, lpStream(lpInit55, lpAssistant55, lpResult("### Summary\nok")))
	workDir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	provider := newBandConfinedProvider(lpHarness("claude", "claude", "", map[string]config.ProviderEntry{"claude": lpOMPEntry("anthropic/claude-opus-5-5:max")}))
	projected, reason := provider.resolve()
	require.Empty(t, reason)
	reply, reason := provider.request(context.Background(), projected, workDir, "prompt text", lpRequestDiagnosis)
	require.Empty(t, reason)
	assert.Equal(t, localPatchModel{Request: lpRequestDiagnosis, Requested: "claude-opus-5-5", Actual: "claude-opus-5-5"}, reply.model)
	text, _, reason := reply.diagnosisText()
	assert.Empty(t, reason)
	assert.Equal(t, "### Summary\nok", text)
	assert.Equal(t, workDir+"\n", fake.record(t, "cwd"))
	assert.Equal(t, "prompt text", fake.record(t, "stdin"))
	argv := strings.Split(strings.TrimSuffix(fake.record(t, "argv"), "\n"), "\n")
	assert.Equal(t, []string{"--print", "--model", "claude-opus-5-5", "--effort", "max"}, argv[:5])
	assert.NotContains(t, argv, "--add-dir")
	for _, line := range strings.Split(fake.record(t, "env"), "\n") {
		name, _, _ := strings.Cut(line, "=")
		assert.False(t, strings.HasPrefix(name, "GIT_"), "inherited %s", name)
		assert.NotContains(t, []string{"GH_TOKEN", "ACTIONS_ID_TOKEN_REQUEST_TOKEN", "AWS_CONTAINER_CREDENTIALS_FULL_URI"}, name)
	}
}

func TestBandConfinedProvider_Request_StreamPastBound(t *testing.T) {
	fake := installLPFakeClaude(t)
	padding := `{"type":"user","message":{"content":[{"type":"tool_result","content":"` + strings.Repeat("a", bandProviderStreamBytes) + `"}]}}`
	fake.setStream(t, lpStream(lpInit55, lpAssistant55, padding, lpResult("x")))
	provider := newBandConfinedProvider(lpHarness("claude", "", "", nil))
	projected, reason := provider.resolve()
	require.Empty(t, reason)
	reply, reason := provider.request(context.Background(), projected, t.TempDir(), "p", lpRequestPatch)
	require.Empty(t, reason)
	assert.True(t, reply.overBound)
	assert.True(t, reply.patchReply().Dropped)
	_, _, reason = reply.diagnosisText()
	assert.Equal(t, bandProviderEmptyOutput, reason)
}

func TestBandConfinedProvider_Request_FailuresKeep001Reasons(t *testing.T) {
	fake := installLPFakeClaude(t)
	provider := newBandConfinedProvider(lpHarness("claude", "", "", nil))
	projected, _ := provider.resolve()
	fake.setStream(t, " \n")
	_, reason := provider.request(context.Background(), projected, t.TempDir(), "p", lpRequestPatch)
	assert.Equal(t, bandProviderEmptyOutput, reason)
	missing := projected
	missing.Binary = "lp-no-such-claude-binary"
	_, reason = provider.request(context.Background(), missing, t.TempDir(), "p", lpRequestPatch)
	assert.Equal(t, bandProviderMissing, reason)
}
