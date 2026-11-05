package orchestra

import (
	"testing"

	"github.com/insajin/autopus-adk/pkg/telemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecorateProviderUsage_AgyResult_IsActual(t *testing.T) {
	t.Parallel()
	jsonResult := `{"conversation_id":"c1","status":"SUCCESS","response":"OK\n","duration_seconds":5.4,"num_turns":1,` +
		`"usage":{"input_tokens":8609,"output_tokens":25,"thinking_tokens":20,"cache_read_tokens":600,"total_tokens":8634}}`
	cases := map[string]string{
		"stream-json": agyStreamFixtureEvents,
		"json":        jsonResult + "\n",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			usage, capability := decorateProviderUsage(ProviderConfig{Name: "gemini", Binary: "/opt/bin/agy"}, "reviewer", 1, raw)

			require.Len(t, usage, 1)
			receipt := usage[0]
			assert.Equal(t, telemetry.UsageStatusActual, receipt.UsageStatus)
			assert.Equal(t, int64(8609), *receipt.InputTokensTotal)
			assert.Equal(t, int64(25), *receipt.OutputTokensTotal)
			assert.Equal(t, int64(20), *receipt.ReasoningTokens)
			assert.Equal(t, telemetry.ComponentSubsetOfOutput, receipt.ReasoningRelation)
			assert.Equal(t, int64(8634), *receipt.RawTotalTokens, "raw total must match agy's own total_tokens")
			assert.True(t, capability.Supported)
			assert.True(t, capability.Observed)
		})
	}
}

func TestDecorateProviderUsage_AgyCacheRead_IsSubsetOfInput(t *testing.T) {
	t.Parallel()
	raw := `{"event":"result","result":{"status":"SUCCESS","response":"x",` +
		`"usage":{"input_tokens":1000,"output_tokens":10,"thinking_tokens":0,"cache_read_tokens":600,"total_tokens":1010}}}`

	usage, _ := decorateProviderUsage(ProviderConfig{Name: "gemini", Binary: "agy"}, "", 0, raw)

	require.Len(t, usage, 1)
	assert.Equal(t, int64(600), *usage[0].CacheReadInputTokens)
	assert.Equal(t, int64(400), *usage[0].UncachedInputTokens)
	assert.Equal(t, int64(1010), *usage[0].RawTotalTokens)
}

func TestDecorateProviderUsage_AgyTotalAddsThinking_CountsItSeparately(t *testing.T) {
	t.Parallel()
	raw := `{"event":"result","result":{"status":"SUCCESS","response":"x",` +
		`"usage":{"input_tokens":100,"output_tokens":10,"thinking_tokens":5,"total_tokens":115}}}`

	usage, _ := decorateProviderUsage(ProviderConfig{Name: "gemini", Binary: "agy"}, "", 0, raw)

	require.Len(t, usage, 1)
	assert.Equal(t, telemetry.ComponentSeparate, usage[0].ReasoningRelation)
	assert.Equal(t, int64(115), *usage[0].RawTotalTokens)
}

func TestDecorateProviderUsage_AgyTextOutput_IsSupportedButAbsent(t *testing.T) {
	t.Parallel()
	usage, capability := decorateProviderUsage(ProviderConfig{Name: "gemini", Binary: "agy"}, "", 0, "plain answer\n")

	require.Len(t, usage, 1)
	assert.Equal(t, telemetry.UsageStatusUnavailable, usage[0].UsageStatus)
	assert.True(t, capability.Supported)
	assert.False(t, capability.Observed)
}

// claudeJSONFixture is `claude --print --output-format json` as Claude Code
// 2.1.314 prints it, trimmed. modelUsage covers a subagent model that the
// top-level usage leaves out.
const claudeJSONFixture = `{"type":"result","subtype":"success","is_error":false,"result":"VERDICT: PASS",` +
	`"total_cost_usd":0.25,` +
	`"usage":{"input_tokens":2,"cache_creation_input_tokens":12631,"cache_read_input_tokens":11491,"output_tokens":4,"output_tokens_details":{"thinking_tokens":0}},` +
	`"modelUsage":{"claude-opus-5-5":{"inputTokens":2,"outputTokens":4,"cacheReadInputTokens":11491,"cacheCreationInputTokens":12631,"thinkingTokens":0,"costUSD":0.2},` +
	`"claude-haiku-4-5":{"inputTokens":100,"outputTokens":50,"cacheReadInputTokens":0,"cacheCreationInputTokens":0,"thinkingTokens":10,"costUSD":0.05}}}`

func TestDecorateProviderUsage_ClaudeModelUsage_SumsEveryModel(t *testing.T) {
	t.Parallel()
	raw := "Warning: no stdin data received in 3s, proceeding without it.\n" + claudeJSONFixture + "\n"

	usage, capability := decorateProviderUsage(ProviderConfig{Name: "claude", Binary: "claude"}, "reviewer", 1, raw)

	require.Len(t, usage, 1)
	receipt := usage[0]
	assert.Equal(t, telemetry.UsageStatusActual, receipt.UsageStatus)
	assert.Equal(t, int64(102), *receipt.UncachedInputTokens)
	assert.Equal(t, int64(12631), *receipt.CacheCreationInputTokens)
	assert.Equal(t, int64(11491), *receipt.CacheReadInputTokens)
	assert.Equal(t, int64(102+12631+11491), *receipt.InputTokensTotal)
	assert.Equal(t, int64(54), *receipt.OutputTokensTotal)
	assert.Equal(t, int64(10), *receipt.ReasoningTokens)
	assert.Equal(t, telemetry.ComponentSubsetOfOutput, receipt.ReasoningRelation)
	assert.Equal(t, int64(102+12631+11491+54), *receipt.RawTotalTokens)
	assert.InDelta(t, 0.25, *receipt.ActualCostUSD, 1e-9)
	assert.True(t, capability.Observed)
}

func TestDecorateProviderUsage_ClaudeWithoutModelUsage_FallsBackToUsage(t *testing.T) {
	t.Parallel()
	raw := `{"type":"result","result":"x","total_cost_usd":0.1,` +
		`"usage":{"input_tokens":7,"cache_creation_input_tokens":3,"cache_read_input_tokens":0,"output_tokens":5}}`

	usage, _ := decorateProviderUsage(ProviderConfig{Name: "claude", Binary: "claude"}, "", 0, raw)

	require.Len(t, usage, 1)
	assert.Equal(t, int64(10), *usage[0].InputTokensTotal)
	assert.Equal(t, int64(5), *usage[0].OutputTokensTotal)
	assert.Equal(t, int64(15), *usage[0].RawTotalTokens)
}

func TestDecodeProviderEnvelope_ClaudeJSON(t *testing.T) {
	t.Parallel()
	text, err := decodeProviderEnvelope(envelopeClaudeJSON, "Warning: stdin\n"+claudeJSONFixture+"\n")
	require.NoError(t, err)
	assert.Equal(t, "VERDICT: PASS", text)

	_, err = decodeProviderEnvelope(envelopeClaudeJSON, `{"type":"result","subtype":"error_max_turns","is_error":true}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error_max_turns")

	_, err = decodeProviderEnvelope(envelopeClaudeJSON, "plain text answer")
	require.Error(t, err)
}

func TestProviderEnvelopeFor_ReadsTheExecutedArgv(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		binary string
		args   []string
		want   providerEnvelope
	}{
		{"agy stream", "agy", []string{"--print", "p", "--output-format", "stream-json"}, envelopeAgyStream},
		{"agy inline json", "/usr/local/bin/agy", []string{"--output-format=json", "--print", "p"}, envelopeAgyJSON},
		{"agy text", "agy", []string{"--print", "p"}, envelopeNone},
		{"agy last flag wins", "agy", []string{"--output-format", "json", "--output-format", "stream-json"}, envelopeAgyStream},
		{"claude json", "claude", []string{"--print", "--output-format", "json"}, envelopeClaudeJSON},
		{"claude confined stream stays raw", "claude", []string{"--print", "--output-format", "stream-json", "--verbose"}, envelopeNone},
		{"wrapper binary", "my-agy-wrapper", []string{"--output-format", "stream-json"}, envelopeNone},
		{"after separator", "agy", []string{"--", "--output-format", "json"}, envelopeNone},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, providerEnvelopeFor(tc.binary, tc.args), tc.name)
	}
}

func TestCompletionWatcher_FiresOnlyOnCompleteTerminalLine(t *testing.T) {
	t.Parallel()
	watcher := newCompletionWatcher(envelopeAgyStream)
	result := `{"event":"result","result":{"status":"SUCCESS","response":"ok"}}`

	_, _ = watcher.Write([]byte(`{"event":"step_update","step_update":{}}` + "\n" + result[:20]))
	assertNotDone(t, watcher)
	_, _ = watcher.Write([]byte(result[20:]))
	assertNotDone(t, watcher) // no newline yet: the line may still be growing
	_, _ = watcher.Write([]byte("\n"))

	select {
	case <-watcher.Done():
	default:
		t.Fatal("watcher must fire once the result line is complete")
	}
	assert.Nil(t, newCompletionWatcher(envelopeNone).Done())
}

func assertNotDone(t *testing.T, watcher *completionWatcher) {
	t.Helper()
	select {
	case <-watcher.Done():
		t.Fatal("watcher fired before a complete result line")
	default:
	}
}
