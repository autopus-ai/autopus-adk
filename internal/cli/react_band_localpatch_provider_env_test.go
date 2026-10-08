package cli

import (
	"context"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lpBakeShellVars puts a shell assignment of each of vars, in name order,
// right after the shebang line of a fake binary's script, so the fake finds
// its record directories without inheriting them through the confined
// environment's allowlist.
func lpBakeShellVars(script string, vars map[string]string) string {
	var assignments strings.Builder
	for _, name := range slices.Sorted(maps.Keys(vars)) {
		assignments.WriteString(name + "='" + strings.ReplaceAll(vars[name], "'", `'\''`) + "'\n")
	}
	head, body, _ := strings.Cut(script, "\n")
	return head + "\n" + assignments.String() + body
}

// Provider Contract item 6 (Phase 4 review, both reviewers): the confined
// claude starts from an allowlist, not from the inherited environment minus
// a deny list, so the variables of an agent session that runs band, of an
// IDE bridge, or of the user's shell never reach it.

// lpPollutedEnv are inherited variables that no confined request may see,
// each with a synthetic value: an outer claude session's markers, socket,
// and token, an IDE and MCP bridge, NODE_OPTIONS, an SSH agent, an API key
// with its base URL, and the credentials and GIT_* variables of 001's list.
var lpPollutedEnv = []string{
	"CLAUDECODE", "CLAUDE_CODE_ENTRYPOINT", "CLAUDE_CODE_SESSION_ID", "CLAUDE_CODE_CHILD_SESSION",
	"CLAUDE_CODE_MESSAGING_SOCKET", "CLAUDE_CODE_MESSAGING_TOKEN", "CLAUDE_CODE_SSE_PORT", "CLAUDE_PID", "CLAUDE_EFFORT",
	"ORCA_AGENT_HOOK_TOKEN", "ORCA_AGENT_HOOK_ENDPOINT", "ENABLE_IDE_INTEGRATION", "MCP_TIMEOUT", "NODE_OPTIONS",
	"SSH_AUTH_SOCK", "ANTHROPIC_API_KEY", "ANTHROPIC_BASE_URL", "GH_TOKEN", "AWS_SECRET_ACCESS_KEY", "GIT_DIR", "LP_UNLISTED",
}

// lpKeptEnv are allowlisted variables that reach the confined claude with
// their values: the claude configuration directory, the headless OAuth
// token, the proxy variables in both letter cases, a locale, an XDG
// directory, the time zone, and the TLS trust variables.
var lpKeptEnv = map[string]string{
	"CLAUDE_CONFIG_DIR": "/synthetic/claude-config", "CLAUDE_CODE_OAUTH_TOKEN": "synthetic-oauth",
	"HTTPS_PROXY": "http://proxy.invalid:3128", "https_proxy": "http://proxy.invalid:3128", "NO_PROXY": "localhost",
	"no_proxy": "localhost", "LC_ALL": "C", "XDG_CONFIG_HOME": "/synthetic/xdg", "TZ": "UTC",
	"SSL_CERT_FILE": "/synthetic/ca.pem", "SSL_CERT_DIR": "/synthetic/certs", "NODE_EXTRA_CA_CERTS": "/synthetic/extra.pem",
}

// lpAllowedEnvName is the SPEC's allowlist, plus the variables that the
// fake's own /bin/sh sets (PWD, SHLVL, _, OLDPWD).
func lpAllowedEnvName(name string) bool {
	exact := []string{
		"PATH", "HOME", "USER", "LOGNAME", "SHELL", "TMPDIR", "LANG", "TERM", "TZ", "CLAUDE_CONFIG_DIR", "CLAUDE_CODE_OAUTH_TOKEN",
		"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "no_proxy", "SSL_CERT_FILE", "SSL_CERT_DIR",
		"NODE_EXTRA_CA_CERTS", "PWD", "SHLVL", "_", "OLDPWD",
	}
	return slices.Contains(exact, name) || strings.HasPrefix(name, "LC_") || strings.HasPrefix(name, "XDG_")
}

// F3, Provider Contract item 4: a confined request runs with no fast-fail
// rule, so a tracked file whose text a Read returns in the stream, here one
// holding orchestra's RESOURCE_EXHAUSTED and ratelimitexceeded rules, does
// not end the provider as a capacity failure.
func TestReactBandLocalPatchProvider_RepositoryTextDoesNotFastFail(t *testing.T) {
	fake := installLPFakeClaude(t)
	file := `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":` +
		`"// status RESOURCE_EXHAUSTED: quota\n// grpc code ratelimitexceeded\nfunc Foo() int { return 1 }"}]}}`
	fake.setStream(t, lpStream(lpInit55, lpAssistant55, file, lpResult("### Summary\nquota text is repository content")))
	provider := newBandConfinedProvider(lpHarness("claude", "", "", nil))
	projected, reason := provider.resolve()
	require.Empty(t, reason)

	reply, reason := provider.request(context.Background(), projected, t.TempDir(), "p", lpRequestDiagnosis)

	require.Empty(t, reason, "the provider was not killed by a fast-fail rule")
	text, _, reason := reply.diagnosisText()
	assert.Empty(t, reason)
	assert.Equal(t, "### Summary\nquota text is repository content", text)
}

func TestReactBandLocalPatchProvider_EnvironmentIsAnAllowlist(t *testing.T) {
	fake := installLPFakeClaude(t)
	for _, name := range lpPollutedEnv {
		t.Setenv(name, "synthetic-"+strings.ToLower(name))
	}
	for name, value := range lpKeptEnv {
		t.Setenv(name, value)
	}
	fake.setStream(t, lpStream(lpInit55, lpAssistant55, lpResult("### Summary\nok")))
	provider := newBandConfinedProvider(lpHarness("claude", "", "", nil))
	projected, reason := provider.resolve()
	require.Empty(t, reason)

	_, reason = provider.request(context.Background(), projected, t.TempDir(), "p", lpRequestDiagnosis)
	require.Empty(t, reason)

	env := map[string]string{}
	for _, line := range strings.Split(strings.TrimSuffix(fake.record(t, "env"), "\n"), "\n") {
		name, value, _ := strings.Cut(line, "=")
		env[name] = value
	}
	for name := range env {
		assert.True(t, lpAllowedEnvName(name), "%s reached the confined claude", name)
	}
	for _, name := range lpPollutedEnv {
		assert.NotContains(t, env, name)
	}
	for name, value := range lpKeptEnv {
		assert.Equal(t, value, env[name], name)
	}
	assert.NotEmpty(t, env["PATH"])
	assert.NotEmpty(t, env["HOME"])
}
