package cli

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// A --no-judge run hands its round history to the caller as one JSON
// document instead of the merged text.
func TestWriteOrchestraPrimaryOutput_NoJudgeWritesRoundHistoryJSON(t *testing.T) {
	t.Parallel()

	result := &orchestra.OrchestraResult{
		Strategy: orchestra.StrategyDebate,
		Merged:   "must not be printed",
		RoundHistory: [][]orchestra.ProviderResponse{
			{{Provider: "claude", Output: "round one", Duration: 1500 * time.Millisecond}},
			{{Provider: "claude", Output: "round two"}},
		},
	}
	var output bytes.Buffer

	structured, err := writeOrchestraPrimaryOutput(&output, result, true, "")

	require.NoError(t, err)
	assert.True(t, structured)
	var got orchestra.YieldOutput
	require.NoError(t, json.Unmarshal(output.Bytes(), &got), "stdout must contain exactly one JSON document")
	assert.Equal(t, "debate", got.Strategy)
	assert.Equal(t, 2, got.Rounds)
	require.Len(t, got.RoundHistory, 2)
	require.Len(t, got.RoundHistory[0].Responses, 1)
	assert.Equal(t, "round one", got.RoundHistory[0].Responses[0].Output)
	assert.Equal(t, int64(1500), got.RoundHistory[0].Responses[0].DurationMs)
	assert.Empty(t, got.SessionID)
	assert.NotContains(t, output.String(), "must not be printed")
}

// A judged run, or a --no-judge run without rounds, leaves stdout to the
// merged text the caller prints.
func TestWriteOrchestraPrimaryOutput_WithoutNoJudgeRoundsWritesNothing(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		noJudge bool
		rounds  [][]orchestra.ProviderResponse
	}{
		"judged run":          {noJudge: false, rounds: [][]orchestra.ProviderResponse{{{Provider: "claude", Output: "r1"}}}},
		"no-judge, no rounds": {noJudge: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var output bytes.Buffer
			result := &orchestra.OrchestraResult{Merged: "merged", RoundHistory: tc.rounds}

			structured, err := writeOrchestraPrimaryOutput(&output, result, tc.noJudge, "")

			require.NoError(t, err)
			assert.False(t, structured)
			assert.Empty(t, output.String())
		})
	}
}
