package cli

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// Known-good argv of config.DefaultClaudeProviderEntry() through the shared
// projection (SPEC-REVIEWRO-001) and through the confined projection
// (SPEC-SIGMABAND-002 Local Patch Provider Contract item 3).
var (
	sharedDefaultClaudeArgv = []string{
		"--print", "--model", "claude-fable-5-1", "--effort", "max",
		"--permission-mode", "plan", "--safe-mode", "--no-session-persistence", "--disable-slash-commands",
		"--strict-mcp-config", "--tools=Read,Grep,Glob",
	}
	confinedDefaultClaudeArgv = []string{
		"--print", "--model", "claude-fable-5-1", "--effort", "max",
		"--permission-mode", "plan", "--safe-mode", "--no-session-persistence", "--disable-slash-commands",
		"--strict-mcp-config", "--output-format", "stream-json", "--verbose", "--restricted", "--tools=Read,Grep,Glob",
	}
)

func projectConfinedForTest(t *testing.T, providers ...orchestra.ProviderConfig) ([]orchestra.ProviderConfig, error) {
	t.Helper()
	return applyReadOnlyProviderPolicy(providers, readOnlyPolicyOptions{Confined: true})
}

// REQ-03, S7, S13: the confined option adds --output-format stream-json,
// --verbose, and --restricted before the last item --tools=Read,Grep,Glob of
// the shared claude projection, which stays byte-identical without it.
func TestReadOnlyProviderPolicy_ConfinedClaudeAddsRestrictedStreamBeforeTools(t *testing.T) {
	t.Parallel()
	entry := providerConfigFromEntry("claude", config.DefaultClaudeProviderEntry())
	original := append([]string(nil), entry.Args...)

	for _, tc := range []struct {
		name string
		opts readOnlyPolicyOptions
		want []string
	}{
		{name: "shared projection", opts: readOnlyPolicyOptions{}, want: sharedDefaultClaudeArgv},
		{name: "confined projection", opts: readOnlyPolicyOptions{Confined: true}, want: confinedDefaultClaudeArgv},
	} {
		got, err := applyReadOnlyProviderPolicy([]orchestra.ProviderConfig{entry}, tc.opts)
		require.NoError(t, err, tc.name)
		require.Len(t, got, 1, tc.name)
		assert.Equal(t, tc.want, got[0].Args, tc.name)
		assert.Empty(t, got[0].Backend, tc.name)
		assert.Equal(t, orchestra.SandboxModeReadOnly, got[0].SandboxMode, tc.name)
	}
	assert.Equal(t, original, entry.Args, "projection must not mutate caller-owned Args")
}

// Provider Contract item 3: stream-json replaces a configured --output-format
// in either form, so exactly one separated pair sits before --verbose.
func TestReadOnlyProviderPolicy_ConfinedReplacesConfiguredOutputFormat(t *testing.T) {
	t.Parallel()
	want := []string{
		"--print", "--model", "m", "--permission-mode", "plan", "--safe-mode", "--no-session-persistence",
		"--disable-slash-commands", "--strict-mcp-config", "--output-format", "stream-json", "--verbose", "--restricted",
		"--tools=Read,Grep,Glob",
	}
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "separated json", args: []string{"--print", "--output-format", "json", "--model", "m"}},
		{name: "inline text", args: []string{"--print", "--output-format=text", "--model", "m"}},
		{name: "repeated forms", args: []string{"--print", "--output-format", "json", "--model", "m", "--output-format=stream-json"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := projectConfinedForTest(t, orchestra.ProviderConfig{Name: "claude", Binary: "claude", Args: tc.args})
			require.NoError(t, err)
			assert.Equal(t, want, got[0].Args)
		})
	}
}

// Provider Contract item 3: a confined projection refuses a provider with a
// Backend or a name other than claude before any other check, because the
// shared projection passes an OMP-backed provider through as-is. The refusal
// is no argv policy violation, so band can report it as
// unavailable(provider_unconfined) apart from provider_policy_rejected.
func TestReadOnlyProviderPolicy_ConfinedRefusesBackendOrOtherProvider(t *testing.T) {
	t.Parallel()
	ompClaude := providerConfigFromEntry("claude", config.ProviderEntry{
		Backend: config.ProviderBackendOMP, Model: "anthropic/claude-opus-5-5:max",
	})
	shared, err := applyReadOnlyProviderPolicy([]orchestra.ProviderConfig{ompClaude}, readOnlyPolicyOptions{})
	require.NoError(t, err, "the shared projection passes an OMP-backed claude as-is")
	require.Equal(t, config.ProviderBackendOMP, shared[0].Backend)

	const refused = ": only a subprocess claude can be confined"
	for _, tc := range []struct {
		name      string
		providers []orchestra.ProviderConfig
		want      string
	}{
		{name: "omp-backed claude", providers: []orchestra.ProviderConfig{ompClaude}, want: `read-only provider policy: provider "claude" on backend "omp"` + refused},
		{name: "claude on another backend", providers: []orchestra.ProviderConfig{{Name: "claude", Binary: "claude", Backend: "custom"}}, want: `read-only provider policy: provider "claude" on backend "custom"` + refused},
		{name: "subprocess codex", providers: []orchestra.ProviderConfig{{Name: "codex", Binary: "codex", Args: []string{"exec"}}}, want: `read-only provider policy: provider "codex"` + refused},
		{name: "codex with a bypass flag", providers: []orchestra.ProviderConfig{{Name: "codex", Binary: "codex", Args: []string{"exec", "--dangerously-bypass-approvals-and-sandbox"}}}, want: `read-only provider policy: provider "codex"` + refused},
		{name: "gemini on agy", providers: []orchestra.ProviderConfig{{Name: "gemini", Binary: "agy", Args: []string{"--print", ""}}}, want: `read-only provider policy: provider "gemini"` + refused},
		{name: "unsupported name", providers: []orchestra.ProviderConfig{{Name: "opencode", Binary: "opencode"}}, want: `read-only provider policy: provider "opencode"` + refused},
		{name: "one refusal fails the whole set", providers: []orchestra.ProviderConfig{{Name: "claude", Binary: "claude"}, ompClaude}, want: `read-only provider policy: provider "claude" on backend "omp"` + refused},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := projectConfinedForTest(t, tc.providers...)

			require.Error(t, err)
			assert.Nil(t, got, "fail-close must not return a partially launchable provider set")
			assert.ErrorIs(t, err, errReadOnlyUnconfined)
			var violation *readOnlyPolicyViolation
			assert.False(t, errors.As(err, &violation), "a refusal is no argv policy violation")
			assert.Equal(t, tc.want, err.Error())
		})
	}
}

// S7 and Provider Contract item 3: the projection, not the user's argv, adds
// --verbose and --restricted, so a configured --verbose, --restricted, or
// --bare stays outside the claude allowlist: an argv policy violation that
// band reports as unavailable(provider_policy_rejected), not a refusal.
func TestReadOnlyProviderPolicy_ConfinedKeepsClaudeArgvAllowlist(t *testing.T) {
	t.Parallel()
	for _, flag := range []string{"--verbose", "--restricted", "--bare"} {
		t.Run(flag, func(t *testing.T) {
			t.Parallel()
			got, err := projectConfinedForTest(t, orchestra.ProviderConfig{Name: "claude", Binary: "claude", Args: []string{"--print", flag}})

			assert.Nil(t, got)
			var violation *readOnlyPolicyViolation
			require.ErrorAs(t, err, &violation)
			assert.Equal(t, readOnlyUnsupportedArgv, violation.Kind)
			assert.Equal(t, flag, violation.Item)
			assert.NotErrorIs(t, err, errReadOnlyUnconfined)
			assert.Equal(t, `read-only provider policy: provider "claude" contains unsupported argv "`+flag+`"`, err.Error())
		})
	}
}
