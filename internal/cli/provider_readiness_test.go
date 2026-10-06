package cli

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// readinessRunnerSpy records every command that reached the runner seam.
type readinessRunnerSpy struct {
	mu    sync.Mutex
	calls []providerReadinessCommand
}

func (spy *readinessRunnerSpy) argvs() [][]string {
	spy.mu.Lock()
	defer spy.mu.Unlock()
	argvs := make([][]string, 0, len(spy.calls))
	for _, call := range spy.calls {
		argvs = append(argvs, append([]string(nil), call.Argv...))
	}
	return argvs
}

type readinessReply func(ctx context.Context, command providerReadinessCommand) (providerReadinessProcess, error)

// installReadinessRunner swaps the runner seam for one test; such tests stay serial.
func installReadinessRunner(t *testing.T, reply readinessReply) *readinessRunnerSpy {
	t.Helper()
	spy := &readinessRunnerSpy{}
	previous := providerReadinessRunner
	providerReadinessRunner = func(ctx context.Context, command providerReadinessCommand) (providerReadinessProcess, error) {
		spy.mu.Lock()
		spy.calls = append(spy.calls, command)
		spy.mu.Unlock()
		return reply(ctx, command)
	}
	t.Cleanup(func() { providerReadinessRunner = previous })
	return spy
}

func exitedReadinessProcess(exitCode int, stdout, stderr string) providerReadinessProcess {
	return providerReadinessProcess{Stdout: strings.NewReader(stdout), Stderr: strings.NewReader(stderr),
		Wait: func() (int, error) { return exitCode, nil }}
}

func replyWith(exitCode int, stdout, stderr string) readinessReply {
	return func(context.Context, providerReadinessCommand) (providerReadinessProcess, error) {
		return exitedReadinessProcess(exitCode, stdout, stderr), nil
	}
}

const claudeLoggedInR1 = `{"loggedIn":true,"authMethod":"claude.ai","apiProvider":"firstParty","email":"dev@example.com","orgId":"org-123"}`

func TestProbeProviderReadiness_ClaudeStatusShapes_ClassifyExactly(t *testing.T) {
	const loggedOut = `{"loggedIn":false,"authMethod":"none","apiProvider":"firstParty","analyticsDisabled":false}`
	tests := []struct {
		name   string
		reply  readinessReply
		env    []string
		want   string
		remedy string
	}{
		{name: "R1 OAuth login", reply: replyWith(0, claudeLoggedInR1, ""), want: "ready"},
		{name: "R2 logged out", reply: replyWith(1, loggedOut, ""), want: "not_ready(logged_out)", remedy: "claude auth login"},
		{name: "R3 logged out with API key", reply: replyWith(1, loggedOut, ""), env: []string{"ANTHROPIC_API_KEY=sk-ant-test"}, want: "unknown(env_credentials)"},
		{name: "auth token env", reply: replyWith(1, loggedOut, ""), env: []string{"ANTHROPIC_AUTH_TOKEN=x"}, want: "unknown(env_credentials)"},
		{name: "oauth token env", reply: replyWith(1, loggedOut, ""), env: []string{"CLAUDE_CODE_OAUTH_TOKEN=x"}, want: "unknown(env_credentials)"},
		{name: "bedrock env", reply: replyWith(1, loggedOut, ""), env: []string{"CLAUDE_CODE_USE_BEDROCK=1"}, want: "unknown(env_credentials)"},
		{name: "vertex env", reply: replyWith(1, loggedOut, ""), env: []string{"CLAUDE_CODE_USE_VERTEX=1"}, want: "unknown(env_credentials)"},
		{name: "empty credential env is absent", reply: replyWith(1, loggedOut, ""), env: []string{"ANTHROPIC_API_KEY="}, want: "not_ready(logged_out)", remedy: "claude auth login"},
		{name: "unrelated env", reply: replyWith(1, loggedOut, ""), env: []string{"OPENAI_API_KEY=x"}, want: "not_ready(logged_out)", remedy: "claude auth login"},
		{name: "R4 api key login", reply: replyWith(0, `{"loggedIn":true,"authMethod":"api_key","apiProvider":"firstParty","apiKeySource":"ANTHROPIC_API_KEY"}`, ""), env: []string{"ANTHROPIC_API_KEY=x"}, want: "ready"},
		{name: "R5 plain text", reply: replyWith(0, "Logged in", ""), want: "unknown(unparsable)"},
		{name: "loggedIn null", reply: replyWith(0, `{"loggedIn":null}`, ""), want: "unknown(unparsable)"},
		{name: "loggedIn string", reply: replyWith(0, `{"loggedIn":"false"}`, ""), want: "unknown(unparsable)"},
		{name: "loggedIn key case differs", reply: replyWith(1, `{"LoggedIn":false}`, ""), want: "unknown(unparsable)"},
		{name: "loggedIn missing", reply: replyWith(1, `{"authMethod":"none"}`, ""), want: "unknown(unparsable)"},
		{name: "top level array", reply: replyWith(0, `[{"loggedIn":true}]`, ""), want: "unknown(unparsable)"},
		{name: "empty output", reply: replyWith(1, "", "error: unknown option '--json'"), want: "unknown(unparsable)"},
		{name: "R11 start failure", reply: func(context.Context, providerReadinessCommand) (providerReadinessProcess, error) {
			return providerReadinessProcess{}, errors.New(`exec: "claude": executable file not found in $PATH`)
		}, want: "unknown(probe_failed)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := probeSingleReadiness(t, orchestra.ProviderConfig{Name: "claude", Binary: "claude"}, tt.reply, tt.env)
			assert.Equal(t, tt.want, result.Token())
			assert.Equal(t, tt.remedy, result.Remedy)
		})
	}
}

func TestProbeProviderReadiness_CodexStatusShapes_ClassifyExactly(t *testing.T) {
	tests := []struct {
		name   string
		reply  readinessReply
		env    []string
		want   string
		remedy string
	}{
		{name: "R7 ChatGPT login on stderr", reply: replyWith(0, "", "Logged in using ChatGPT\n"), want: "ready"},
		{name: "exit 0 with any text", reply: replyWith(0, "Logged in using an API key\n", ""), want: "ready"},
		{name: "R8 not logged in on stderr", reply: replyWith(1, "", "Not logged in\n"), want: "not_ready(logged_out)", remedy: "codex login"},
		{name: "not logged in on stdout", reply: replyWith(1, "Not logged in\n", ""), want: "not_ready(logged_out)", remedy: "codex login"},
		{name: "R9 codex API key env", reply: replyWith(1, "", "Not logged in\n"), env: []string{"CODEX_API_KEY=x"}, want: "unknown(env_credentials)"},
		{name: "OpenAI API key env", reply: replyWith(1, "", "Not logged in\n"), env: []string{"OPENAI_API_KEY=x"}, want: "unknown(env_credentials)"},
		{name: "anthropic env does not count", reply: replyWith(1, "", "Not logged in\n"), env: []string{"ANTHROPIC_API_KEY=x"}, want: "not_ready(logged_out)", remedy: "codex login"},
		{name: "R10 old CLI rejects status", reply: replyWith(2, "", "error: unexpected argument 'status' found\n"), want: "unknown(probe_failed)"},
		{name: "exit 1 without the marker", reply: replyWith(1, "", "Error: failed to read auth.json\n"), want: "unknown(probe_failed)"},
		{name: "marker with other exit code", reply: replyWith(3, "", "Not logged in\n"), want: "unknown(probe_failed)"},
		{name: "marker case differs", reply: replyWith(1, "", "not logged in\n"), want: "unknown(probe_failed)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := probeSingleReadiness(t, orchestra.ProviderConfig{Name: "codex", Binary: "codex"}, tt.reply, tt.env)
			assert.Equal(t, tt.want, result.Token())
			assert.Equal(t, tt.remedy, result.Remedy)
		})
	}
}

func TestProbeProviderReadiness_ProviderWithoutStatusCommand_RunsNothing(t *testing.T) {
	// Given: R12 gemini (agy) and a provider outside the Readiness Contract
	spy := installReadinessRunner(t, replyWith(0, `{"loggedIn":true}`, ""))
	providers := []orchestra.ProviderConfig{{Name: "gemini", Binary: "agy"}, {Name: "opencode", Binary: "opencode"}}

	// When
	results := probeProviderReadiness(context.Background(), providers, providerReadinessOptions{Env: []string{}})

	// Then
	require.Len(t, results, 2)
	assert.Equal(t, "unknown(no_status_command)", results[0].Token())
	assert.Equal(t, "unknown(no_status_command)", results[1].Token())
	assert.Equal(t, "opencode", results[1].Provider)
	assert.Empty(t, spy.argvs())
}

func TestProbeProviderReadiness_Skip_RunsNoProbeAndReportsSkipped(t *testing.T) {
	spy := installReadinessRunner(t, replyWith(1, "", "Not logged in"))
	providers := []orchestra.ProviderConfig{
		{Name: "claude", Binary: "claude"}, {Name: "codex", Binary: "codex"}, {Name: "gemini", Binary: "agy"},
		{Name: "claude", Backend: "omp", Model: "anthropic/claude-opus-5-5:max"},
	}

	results := probeProviderReadiness(context.Background(), providers, providerReadinessOptions{Skip: true})

	require.Len(t, results, 4)
	for index, result := range results {
		assert.Equal(t, providers[index].Name, result.Provider)
		assert.Equal(t, "skipped", result.Token())
		assert.Empty(t, result.Remedy)
	}
	assert.Empty(t, spy.argvs())
}

func TestProbeProviderReadiness_SharedStatusCommand_RunsOnceAndKeepsInputOrder(t *testing.T) {
	// Given: claude as reviewer and as judge, plus codex and gemini
	spy := installReadinessRunner(t, func(_ context.Context, command providerReadinessCommand) (providerReadinessProcess, error) {
		if command.Argv[0] == "codex" {
			return exitedReadinessProcess(1, "", "Not logged in"), nil
		}
		return exitedReadinessProcess(0, `{"loggedIn":true}`, ""), nil
	})
	providers := []orchestra.ProviderConfig{
		{Name: "claude", Binary: "claude"}, {Name: "codex", Binary: "codex"},
		{Name: "gemini", Binary: "agy"}, {Name: "claude", Binary: "claude"},
	}

	// When
	results := probeProviderReadiness(context.Background(), providers, providerReadinessOptions{Env: []string{}})

	// Then
	tokens := make([]string, 0, len(results))
	for _, result := range results {
		tokens = append(tokens, result.Provider+" "+result.Token())
	}
	assert.Equal(t, []string{
		"claude ready", "codex not_ready(logged_out)", "gemini unknown(no_status_command)", "claude ready",
	}, tokens)
	assert.ElementsMatch(t, [][]string{{"claude", "auth", "status", "--json"}, {"codex", "login", "status"}}, spy.argvs())
}

// heldReader blocks like a pipe whose write end a grandchild still holds; Wait releases it.
type heldReader struct{ release <-chan struct{} }

func (reader heldReader) Read([]byte) (int, error) {
	<-reader.release
	return 0, io.EOF
}

func TestProbeProviderReadiness_ProbeThatNeverExits_TimesOutWithinBound(t *testing.T) {
	// Given: R6, a claude probe that neither exits nor closes its streams
	installReadinessRunner(t, func(ctx context.Context, _ providerReadinessCommand) (providerReadinessProcess, error) {
		release := make(chan struct{})
		return providerReadinessProcess{
			Stdout: heldReader{release}, Stderr: heldReader{release},
			Wait: func() (int, error) {
				<-ctx.Done()
				close(release)
				return -1, ctx.Err()
			},
		}, nil
	})

	// When
	started := time.Now()
	results := probeProviderReadiness(context.Background(),
		[]orchestra.ProviderConfig{{Name: "claude", Binary: "claude"}}, providerReadinessOptions{Env: []string{}})
	elapsed := time.Since(started)

	// Then
	require.Len(t, results, 1)
	assert.Equal(t, "unknown(timeout)", results[0].Token())
	assert.GreaterOrEqual(t, elapsed, 4900*time.Millisecond)
	assert.Less(t, elapsed, 5500*time.Millisecond)
}

func TestProviderReadinessResult_PreflightLine_RendersContractLine(t *testing.T) {
	t.Parallel()
	expired := providerReadinessResult{Provider: "claude", Status: providerReadinessNotReady, Reason: "auth_expired",
		Remedy: "omp login anthropic (same PI_CODING_AGENT_DIR as this review)"}
	tests := []struct {
		result providerReadinessResult
		want   string
	}{
		{providerReadinessResult{Provider: "claude", Status: providerReadinessReady}, "preflight: claude ready"},
		{providerReadinessResult{Provider: "gemini", Status: providerReadinessSkipped}, "preflight: gemini skipped"},
		{providerReadinessResult{Provider: "gemini", Status: providerReadinessUnknown, Reason: "no_status_command"},
			"preflight: gemini unknown(no_status_command)"},
		{providerReadinessResult{Provider: "codex", Status: providerReadinessNotReady, Reason: "logged_out", Remedy: "codex login"},
			`preflight: codex not_ready(logged_out) - run "codex login"`},
		{expired, `preflight: claude not_ready(auth_expired) - run "omp login anthropic" (same PI_CODING_AGENT_DIR as this review)`},
		{providerReadinessResult{Provider: "dev@example.com", Status: providerReadinessReady}, "preflight: [REDACTED] ready"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.result.PreflightLine())
	}
	expired.Remedy = "omp login anthropic"
	assert.Equal(t, `run "omp login anthropic"`, expired.RunRemedy())
	assert.Empty(t, providerReadinessResult{Provider: "claude", Status: providerReadinessReady}.RunRemedy())
}

func TestProbeProviderReadiness_AccountIdentifiersInOutput_NeverReachResults(t *testing.T) {
	// Given: S16, R1 and an O4 variant whose account fields hold an email and a token
	installFakeOMP(t)
	installReadinessRunner(t, func(_ context.Context, command providerReadinessCommand) (providerReadinessProcess, error) {
		if command.Argv[0] == "claude" {
			return exitedReadinessProcess(0, claudeLoggedInR1, ""), nil
		}
		return exitedReadinessProcess(0, ompUsageJSON(`[{"provider":"anthropic","account":"dev@example.com"}]`, "[]",
			`[{"provider":"anthropic","account":"sk-ant-oat01-abc123","reason":"Refresh token expired for dev@example.com"}]`), ""), nil
	})
	providers := []orchestra.ProviderConfig{{Name: "claude"}, {Name: "gemini", Backend: "omp", Model: ompAnthropicModel}}

	// When
	results := probeProviderReadiness(context.Background(), providers, providerReadinessOptions{Env: []string{"HOME=/a"}})

	// Then
	require.Len(t, results, 2)
	assert.Equal(t, []string{`omp anthropic: 1 of 2 accounts unusable (auth_expired); run "omp usage --redact" for details`},
		results[1].Warnings)
	for _, result := range results {
		rendered := strings.Join(append([]string{result.Token(), result.Remedy, result.PreflightLine()}, result.Warnings...), "\n")
		for _, secret := range []string{"dev@example.com", "org-123", "sk-ant-"} {
			assert.NotContains(t, rendered, secret)
		}
	}
}

// readinessContractArgv is the Readiness Contract probe argv of each CLI provider.
var readinessContractArgv = map[string][]string{
	"claude": {"claude", "auth", "status", "--json"},
	"codex":  {"codex", "login", "status"},
}

// probeSingleReadiness classifies one CLI provider in an environment free of host credentials.
func probeSingleReadiness(
	t *testing.T, provider orchestra.ProviderConfig, reply readinessReply, env []string,
) providerReadinessResult {
	t.Helper()
	spy := installReadinessRunner(t, reply)
	results := probeProviderReadiness(context.Background(), []orchestra.ProviderConfig{provider},
		providerReadinessOptions{Env: append([]string{"HOME=/nonexistent"}, env...)})
	require.Len(t, results, 1)
	assert.Equal(t, provider.Name, results[0].Provider)
	assert.Equal(t, [][]string{readinessContractArgv[provider.Name]}, spy.argvs())
	return results[0]
}
