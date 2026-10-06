package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/orchestra"
)

const ompAnthropicModel = "anthropic/claude-opus-5-5:max"

// installFakeOMP puts a never-executed omp file first on PATH and returns its canonical path.
func installFakeOMP(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "omp")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755))
	t.Setenv("PATH", dir)
	canonical, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)
	return canonical
}

func ompUsageJSON(reports, withoutUsage, disabled string) string {
	return `{"generatedAt":1,"reports":` + reports + `,"accountsWithoutUsage":` + withoutUsage +
		`,"disabledCredentials":` + disabled + `,"capacity":{}}`
}

func TestProbeProviderReadiness_OMPUsageShapes_ClassifyAnthropicFamily(t *testing.T) {
	const expired = `[{"provider":"anthropic","account":"c3d4","reason":"Refresh token expired"}]`
	const usable = `[{"provider":"anthropic","account":"a1b2"}]`
	tests := []struct {
		name    string
		reply   readinessReply
		env     []string
		want    string
		remedy  string
		warning string
	}{
		{name: "O1 no accounts", reply: replyWith(0, ompUsageJSON("[]", "[]", "[]"), ""), want: "unknown(no_account_evidence)"},
		{name: "O2 expired only", reply: replyWith(0, ompUsageJSON("[]", "[]", expired), ""), want: "not_ready(auth_expired)", remedy: "omp login anthropic"},
		{name: "O3 disabled only", reply: replyWith(0, ompUsageJSON("[]", "[]", `[{"provider":"anthropic","account":"c3d4","reason":"Account disabled"}]`), ""), want: "not_ready(account_disabled)", remedy: "omp login anthropic"},
		{name: "O4 one usable one expired", reply: replyWith(0, ompUsageJSON(usable, "[]", expired), ""), want: "ready",
			warning: `omp anthropic: 1 of 2 accounts unusable (auth_expired); run "omp usage --redact" for details`},
		{name: "O5 account without usage", reply: replyWith(0, ompUsageJSON("[]", usable, "[]"), ""), want: "ready"},
		{name: "O6 other family only", reply: replyWith(0, ompUsageJSON(`[{"provider":"openai-codex","account":"e5f6"}]`, "[]", "[]"), ""), want: "unknown(no_account_evidence)"},
		{name: "O7 element without provider", reply: replyWith(0, ompUsageJSON("[]", "[]", `[{"account":"c3d4","reason":"Refresh token expired"}]`), ""), want: "unknown(no_account_evidence)"},
		{name: "O8 missing arrays", reply: replyWith(0, `{"reports":[]}`, ""), want: "unknown(unparsable)"},
		{name: "O9 exit 1", reply: replyWith(1, "", "error"), want: "unknown(probe_failed)"},
		{name: "mixed reasons are account_disabled", reply: replyWith(0, ompUsageJSON("[]", "[]",
			`[{"provider":"anthropic","reason":"Refresh token expired"},{"provider":"anthropic","reason":"Account disabled"}]`), ""),
			want: "not_ready(account_disabled)", remedy: "omp login anthropic"},
		{name: "expired match ignores case", reply: replyWith(0, ompUsageJSON("[]", "[]", `[{"provider":"anthropic","reason":"TOKEN EXPIRED"}]`), ""),
			want: "not_ready(auth_expired)", remedy: "omp login anthropic"},
		{name: "non-string reason is account_disabled", reply: replyWith(0, ompUsageJSON("[]", "[]", `[{"provider":"anthropic","reason":5}]`), ""),
			want: "not_ready(account_disabled)", remedy: "omp login anthropic"},
		{name: "non-string provider does not count", reply: replyWith(0, ompUsageJSON(`[{"provider":["anthropic"]}]`, `["anthropic"]`, `[{"Provider":"anthropic","reason":"expired"}]`), ""),
			want: "unknown(no_account_evidence)"},
		{name: "null array is unparsable", reply: replyWith(0, ompUsageJSON("null", "[]", "[]"), ""), want: "unknown(unparsable)"},
		{name: "object array is unparsable", reply: replyWith(0, ompUsageJSON("[]", "{}", "[]"), ""), want: "unknown(unparsable)"},
		{name: "top level array is unparsable", reply: replyWith(0, `[{"reports":[]}]`, ""), want: "unknown(unparsable)"},
		{name: "plain text is unparsable", reply: replyWith(0, "No accounts configured", ""), want: "unknown(unparsable)"},
		{name: "several usable and disabled are counted", reply: replyWith(0, ompUsageJSON(usable, `[{"provider":"anthropic","account":"a3"}]`,
			`[{"provider":"anthropic","reason":"Account disabled"},{"provider":"anthropic","reason":"Refresh token expired"},{"provider":"openai-codex","reason":"Account disabled"}]`), ""),
			want: "ready", warning: `omp anthropic: 2 of 4 accounts unusable (account_disabled); run "omp usage --redact" for details`},
		{name: "remedy names the review agent dir", reply: replyWith(0, ompUsageJSON("[]", "[]", expired), ""), env: []string{"PI_CODING_AGENT_DIR=/srv/agent"},
			want: "not_ready(auth_expired)", remedy: "omp login anthropic (same PI_CODING_AGENT_DIR as this review)"},
		{name: "empty agent dir adds no note", reply: replyWith(0, ompUsageJSON("[]", "[]", expired), ""), env: []string{"PI_CODING_AGENT_DIR="},
			want: "not_ready(auth_expired)", remedy: "omp login anthropic"},
	}
	canonical := installFakeOMP(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spy := installReadinessRunner(t, tt.reply)
			provider := orchestra.ProviderConfig{Name: "claude", Backend: "omp", Binary: "omp", Model: ompAnthropicModel}

			results := probeProviderReadiness(context.Background(), []orchestra.ProviderConfig{provider},
				providerReadinessOptions{Env: append([]string{"HOME=/nonexistent"}, tt.env...)})

			require.Len(t, results, 1)
			assert.Equal(t, tt.want, results[0].Token())
			assert.Equal(t, tt.remedy, results[0].Remedy)
			if tt.warning == "" {
				assert.Empty(t, results[0].Warnings)
			} else {
				assert.Equal(t, []string{tt.warning}, results[0].Warnings)
			}
			assert.Equal(t, [][]string{{canonical, "usage", "--json", "--redact"}}, spy.argvs())
		})
	}
}

func TestProbeProviderReadiness_OMPProviders_ShareOneProbeInReviewEnvironment(t *testing.T) {
	// Given: OMP claude and codex reviewers plus an OMP claude judge (S11)
	canonical := installFakeOMP(t)
	t.Setenv("NODE_OPTIONS", "--require /tmp/inject.js")
	spy := installReadinessRunner(t, replyWith(0, ompUsageJSON(`[{"provider":"openai-codex","account":"e5f6"}]`, "[]",
		`[{"provider":"anthropic","account":"c3d4","reason":"Refresh token expired"}]`), ""))
	providers := []orchestra.ProviderConfig{
		{Name: "claude", Backend: "omp", Model: ompAnthropicModel},
		{Name: "codex", Backend: "omp", Model: "openai-codex/gpt-5.6-sol:max"},
		{Name: "claude", Backend: "omp", Model: ompAnthropicModel},
	}

	// When: the probe inherits the process environment
	results := probeProviderReadiness(context.Background(), providers, providerReadinessOptions{})

	// Then
	require.Len(t, results, 3)
	assert.Equal(t, "not_ready(auth_expired)", results[0].Token())
	assert.Equal(t, "ready", results[1].Token())
	assert.Equal(t, "not_ready(auth_expired)", results[2].Token())
	require.Len(t, spy.calls, 1)
	assert.Equal(t, []string{canonical, "usage", "--json", "--redact"}, spy.calls[0].Argv)
	expectedEnv, err := normalizePipelineOMPEnvironment(os.Environ())
	require.NoError(t, err)
	assert.Equal(t, expectedEnv, spy.calls[0].Env)
	assert.Contains(t, spy.calls[0].Env, "AUTOPUS_OMP_MANAGED_INNER=1")
	assert.NotContains(t, spy.calls[0].Env, "NODE_OPTIONS=--require /tmp/inject.js")
}

func TestProbeProviderReadiness_OMPProbeCannotStart_IsProbeFailedWithoutRunning(t *testing.T) {
	provider := orchestra.ProviderConfig{Name: "claude", Backend: "omp", Model: ompAnthropicModel}
	t.Run("omp missing from PATH", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		spy := installReadinessRunner(t, replyWith(0, ompUsageJSON("[]", "[]", "[]"), ""))
		results := probeProviderReadiness(context.Background(), []orchestra.ProviderConfig{provider}, providerReadinessOptions{Env: []string{}})
		assert.Equal(t, "unknown(probe_failed)", results[0].Token())
		assert.Empty(t, spy.argvs())
	})
	t.Run("malformed environment", func(t *testing.T) {
		installFakeOMP(t)
		spy := installReadinessRunner(t, replyWith(0, ompUsageJSON("[]", "[]", "[]"), ""))
		results := probeProviderReadiness(context.Background(), []orchestra.ProviderConfig{provider},
			providerReadinessOptions{Env: []string{"HOME=/a", "HOME=/b"}})
		assert.Equal(t, "unknown(probe_failed)", results[0].Token())
		assert.Empty(t, spy.argvs())
	})
	t.Run("model without family", func(t *testing.T) {
		installFakeOMP(t)
		installReadinessRunner(t, replyWith(0, ompUsageJSON("[]", "[]", `[{"provider":"","reason":"expired"}]`), ""))
		noFamily := orchestra.ProviderConfig{Name: "claude", Backend: "omp", Model: "claude-opus-5-5"}
		results := probeProviderReadiness(context.Background(), []orchestra.ProviderConfig{noFamily}, providerReadinessOptions{Env: []string{"HOME=/a"}})
		assert.Equal(t, "unknown(no_account_evidence)", results[0].Token())
	})
}

func readReadinessFixture(t *testing.T, name string) string {
	t.Helper()
	if name == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join("testdata", "provider_readiness", name))
	require.NoError(t, err)
	return string(data)
}

// The fixtures keep the status shapes verified on 2026-10-06 (research.md) with placeholder identifiers.
func TestProbeProviderReadiness_VerifiedStatusFixtures_ClassifyAsContract(t *testing.T) {
	claude, codex := orchestra.ProviderConfig{Name: "claude"}, orchestra.ProviderConfig{Name: "codex"}
	tests := []struct {
		name           string
		provider       orchestra.ProviderConfig
		exit           int
		stdout, stderr string
		env            []string
		want           string
	}{
		{"claude OAuth login", claude, 0, "claude_auth_status_logged_in.json", "", nil, "ready"},
		{"claude logged out", claude, 1, "claude_auth_status_logged_out.json", "", nil, "not_ready(logged_out)"},
		{"claude logged out beside an API key", claude, 1, "claude_auth_status_logged_out.json", "", []string{"ANTHROPIC_API_KEY=x"}, "unknown(env_credentials)"},
		{"claude API key login", claude, 0, "claude_auth_status_api_key.json", "", []string{"ANTHROPIC_API_KEY=x"}, "ready"},
		{"codex ChatGPT login", codex, 0, "", "codex_login_status_logged_in.stderr", nil, "ready"},
		{"codex logged out", codex, 1, "", "codex_login_status_logged_out.stderr", nil, "not_ready(logged_out)"},
		{"codex logged out beside an API key", codex, 1, "", "codex_login_status_logged_out.stderr", []string{"OPENAI_API_KEY=x"}, "unknown(env_credentials)"},
		{"omp under an empty HOME", orchestra.ProviderConfig{Name: "claude", Backend: "omp", Model: ompAnthropicModel}, 0,
			"omp_usage_no_accounts.json", "", nil, "unknown(no_account_evidence)"},
	}
	installFakeOMP(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installReadinessRunner(t, replyWith(tt.exit, readReadinessFixture(t, tt.stdout), readReadinessFixture(t, tt.stderr)))
			results := probeProviderReadiness(context.Background(), []orchestra.ProviderConfig{tt.provider},
				providerReadinessOptions{Env: append([]string{"HOME=/nonexistent"}, tt.env...)})
			assert.Equal(t, tt.want, results[0].Token())
		})
	}
}

func TestProbeProviderReadiness_IncidentFixture_FlagsOnlyTheExpiredFamily(t *testing.T) {
	// Given: the S13 incident, an expired anthropic account beside usable codex and antigravity accounts
	installFakeOMP(t)
	spy := installReadinessRunner(t, replyWith(0, readReadinessFixture(t, "omp_usage_disabled_assumed.json"), ""))
	providers := []orchestra.ProviderConfig{
		{Name: "claude", Backend: "omp", Model: ompAnthropicModel},
		{Name: "codex", Backend: "omp", Model: "openai-codex/gpt-5.6-sol:max"},
		{Name: "gemini", Backend: "omp", Model: "google-antigravity/gemini-3-pro:high"},
		{Name: "claude", Backend: "omp", Model: ompAnthropicModel},
	}

	results := probeProviderReadiness(context.Background(), providers, providerReadinessOptions{Env: []string{"HOME=/a"}})

	assert.Equal(t, []string{"not_ready(auth_expired)", "ready", "ready", "not_ready(auth_expired)"},
		[]string{results[0].Token(), results[1].Token(), results[2].Token(), results[3].Token()})
	assert.Equal(t, `run "omp login anthropic"`, results[3].RunRemedy())
	assert.Len(t, spy.argvs(), 1)
}

func TestProbeProviderReadiness_DistinctProbes_RunConcurrently(t *testing.T) {
	// Given: three probes that each take 1 s
	installFakeOMP(t)
	installReadinessRunner(t, func(ctx context.Context, command providerReadinessCommand) (providerReadinessProcess, error) {
		select {
		case <-time.After(time.Second):
		case <-ctx.Done():
		}
		if command.Argv[0] == "claude" {
			return exitedReadinessProcess(0, `{"loggedIn":true}`, ""), nil
		}
		return exitedReadinessProcess(0, ompUsageJSON(`[{"provider":"anthropic"}]`, "[]", "[]"), ""), nil
	})
	providers := []orchestra.ProviderConfig{
		{Name: "claude"}, {Name: "codex"}, {Name: "gemini", Backend: "omp", Model: "anthropic/claude-sonnet-5:high"},
	}

	// When
	started := time.Now()
	results := probeProviderReadiness(context.Background(), providers, providerReadinessOptions{Env: []string{"HOME=/a"}})

	// Then
	assert.Less(t, time.Since(started), 2500*time.Millisecond)
	tokens := []string{results[0].Token(), results[1].Token(), results[2].Token()}
	assert.Equal(t, []string{"ready", "ready", "ready"}, tokens)
}
