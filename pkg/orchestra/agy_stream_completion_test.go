package orchestra

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/insajin/autopus-adk/pkg/telemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// agyStreamFixtureEvents is the stream-json NDJSON agy 1.5.0 prints for a
// one-turn answer, trimmed to the fields the pipeline reads.
const agyStreamFixtureEvents = `{"event":"init","conversation_id":"c1","init":{"cwd":"/tmp"}}
{"event":"step_update","step_update":{"conversation_id":"c1","step_index":1,"state":"ACTIVE","step_type":"agent_response","text_delta":"VERDICT: PASS"}}
{"event":"step_update","step_update":{"conversation_id":"c1","step_index":1,"state":"DONE","step_type":"agent_response","text_delta":"\n","usage":{"input_tokens":8609,"output_tokens":25,"thinking_tokens":20,"cache_read_tokens":0,"total_tokens":8634}}}
{"event":"result","result":{"conversation_id":"c1","status":"SUCCESS","response":"VERDICT: PASS\n","duration_seconds":5.4,"num_turns":1,"usage":{"input_tokens":8609,"output_tokens":25,"thinking_tokens":20,"cache_read_tokens":0,"total_tokens":8634}}}
`

// writeFakeAgy writes an executable named agy that prints stdout and then runs
// the tail commands, so a test can make it hang, exit, or fail after output.
func writeFakeAgy(t *testing.T, stdout, tail string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake agy fixture requires a POSIX shell")
	}
	path := filepath.Join(t.TempDir(), "agy")
	script := "#!/bin/sh\nprintf '%s' " + shellSingleQuote(stdout) + "\n" + tail + "\n"
	writeExecutableFixture(t, path, script, 0o755)
	return path
}

func agyStreamProvider(binary string) ProviderConfig {
	return ProviderConfig{
		Name: "gemini", Binary: binary, PromptViaArgs: true, OutputFormat: "text",
		Args: []string{"--print", "", "--output-format", "stream-json", "--model", "gemini-4-argon-high"},
	}
}

func TestSubprocessBackend_AgyResultThenHang_ReapsAndReturnsResult(t *testing.T) {
	t.Parallel()
	// Given: agy prints its terminal result event and then never exits,
	// like the language server that outlives a large review.
	binary := writeFakeAgy(t, agyStreamFixtureEvents, "sleep 120")

	// When
	start := time.Now()
	resp, err := NewSubprocessBackendImpl().Execute(context.Background(), ProviderRequest{
		Provider: "gemini", Prompt: "review", Role: "reviewer", Timeout: 2 * time.Minute,
		Config: agyStreamProvider(binary),
	})
	elapsed := time.Since(start)

	// Then: the call completes after the grace period, not the timeout.
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Less(t, elapsed, providerCompletionGrace+20*time.Second)
	assert.GreaterOrEqual(t, elapsed, providerCompletionGrace)
	assert.False(t, resp.TimedOut)
	assert.Equal(t, 0, resp.ExitCode)
	assert.Equal(t, "VERDICT: PASS\n", resp.Output)
	require.NotNil(t, resp.Execution)
	assert.True(t, resp.Execution.ReapedAfterCompletion)
	require.Len(t, resp.Usage, 1)
	assert.Equal(t, telemetry.UsageStatusActual, resp.Usage[0].UsageStatus)
	require.NotNil(t, resp.Usage[0].RawTotalTokens)
	assert.Equal(t, int64(8634), *resp.Usage[0].RawTotalTokens)
}

func TestSubprocessBackend_AgyResultThenExit_IsNotReaped(t *testing.T) {
	t.Parallel()
	binary := writeFakeAgy(t, agyStreamFixtureEvents, "exit 0")

	resp, err := NewSubprocessBackendImpl().Execute(context.Background(), ProviderRequest{
		Provider: "gemini", Prompt: "review", Timeout: time.Minute, Config: agyStreamProvider(binary),
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "VERDICT: PASS\n", resp.Output)
	assert.False(t, resp.Execution.ReapedAfterCompletion)
	assert.True(t, resp.UsageCapability.Observed)
}

func TestSubprocessBackend_AgyExitsWithoutResult_Fails(t *testing.T) {
	t.Parallel()
	// Given: agy streams a partial answer and dies without a result event.
	partial := `{"event":"init","conversation_id":"c1"}
{"event":"step_update","step_update":{"state":"ACTIVE","step_type":"agent_response","text_delta":"VERDICT"}}
`
	for name, tail := range map[string]string{"clean exit": "exit 0", "crash": "exit 3"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			binary := writeFakeAgy(t, partial, tail)

			resp, err := NewSubprocessBackendImpl().Execute(context.Background(), ProviderRequest{
				Provider: "gemini", Prompt: "review", Timeout: time.Minute, Config: agyStreamProvider(binary),
			})

			require.Error(t, err)
			assert.Contains(t, err.Error(), "result")
			require.NotNil(t, resp)
			assert.False(t, resp.TimedOut)
		})
	}
}

func TestSubprocessBackend_AgyMalformedLinesBeforeResult_StillParses(t *testing.T) {
	t.Parallel()
	noisy := "Warning: something on stdout\n{\"event\":\"step_update\",broken\n" + agyStreamFixtureEvents
	binary := writeFakeAgy(t, noisy, "exit 0")

	resp, err := NewSubprocessBackendImpl().Execute(context.Background(), ProviderRequest{
		Provider: "gemini", Prompt: "review", Timeout: time.Minute, Config: agyStreamProvider(binary),
	})

	require.NoError(t, err)
	assert.Equal(t, "VERDICT: PASS\n", resp.Output)
}

func TestSubprocessBackend_AgyNonSuccessResult_Fails(t *testing.T) {
	t.Parallel()
	failed := `{"event":"result","result":{"status":"ERROR","response":"","usage":{"input_tokens":10,"output_tokens":0,"total_tokens":10}}}` + "\n"
	binary := writeFakeAgy(t, failed, "exit 0")

	_, err := NewSubprocessBackendImpl().Execute(context.Background(), ProviderRequest{
		Provider: "gemini", Prompt: "review", Timeout: time.Minute, Config: agyStreamProvider(binary),
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "ERROR")
}

// agy 1.5.0 in --mode plan ends its turn on the first denied tool and reports
// SUCCESS with an empty response; the failure must name the denied action.
func TestSubprocessBackend_AgyDeniedActionEmptyResponse_NamesDenial(t *testing.T) {
	t.Parallel()
	denied := `{"event":"result","result":{"status":"SUCCESS","response":"","usage":{"input_tokens":17454,"output_tokens":399,"total_tokens":17853},"denied_actions":[{"action":"command","display_name":"RunCommand"}]}}` + "\n"
	binary := writeFakeAgy(t, denied, "exit 0")

	_, err := NewSubprocessBackendImpl().Execute(context.Background(), ProviderRequest{
		Provider: "gemini", Prompt: "review", Timeout: time.Minute, Config: agyStreamProvider(binary),
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "denied")
	assert.Contains(t, err.Error(), "RunCommand")
}

func TestDecodeProviderEnvelope_AgyDeniedActionWithAnswer_KeepsAnswer(t *testing.T) {
	t.Parallel()
	raw := `{"event":"result","result":{"status":"SUCCESS","response":"VERDICT: PASS\n","denied_actions":[{"action":"command","display_name":"RunCommand"}]}}` + "\n"

	text, err := decodeProviderEnvelope(envelopeAgyStream, raw)

	require.NoError(t, err)
	assert.Equal(t, "VERDICT: PASS\n", text)
}

func TestRunProvider_AgyResultThenHang_ReapsAndReturnsResult(t *testing.T) {
	t.Parallel()
	binary := writeFakeAgy(t, agyStreamFixtureEvents, "sleep 120")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	start := time.Now()
	resp, err := runProvider(ctx, agyStreamProvider(binary), "review")
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Less(t, elapsed, providerCompletionGrace+20*time.Second)
	assert.False(t, resp.TimedOut)
	assert.Equal(t, "VERDICT: PASS\n", resp.Output)
	assert.True(t, resp.Execution.ReapedAfterCompletion)
	assert.True(t, resp.UsageCapability.Observed)
}
