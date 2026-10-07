package orchestra

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteYieldOutput_JSONSchema(t *testing.T) {
	t.Parallel()

	out := YieldOutput{
		Strategy:  "debate",
		Rounds:    1,
		SessionID: "test-session-1",
		Panes:     map[string]string{"claude": "surface:1", "gemini": "surface:2"},
		RoundHistory: []YieldRound{
			{
				Round: 1,
				Responses: []YieldResponse{
					{Provider: "claude", Output: "idea A", DurationMs: 500, TimedOut: false},
					{Provider: "gemini", Output: "idea B", DurationMs: 800, TimedOut: false},
				},
			},
		},
	}

	var buf bytes.Buffer
	err := WriteYieldOutput(&buf, out)
	require.NoError(t, err)

	// Verify output is valid JSON
	var parsed YieldOutput
	err = json.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)

	assert.Equal(t, "debate", parsed.Strategy)
	assert.Equal(t, 1, parsed.Rounds)
	assert.Equal(t, "test-session-1", parsed.SessionID)
	assert.Len(t, parsed.Panes, 2)
	assert.Equal(t, "surface:1", parsed.Panes["claude"])
	assert.Len(t, parsed.RoundHistory, 1)
	assert.Len(t, parsed.RoundHistory[0].Responses, 2)
	assert.Equal(t, "idea A", parsed.RoundHistory[0].Responses[0].Output)
}

func TestWriteYieldOutput_EmptyRoundHistory(t *testing.T) {
	t.Parallel()

	out := YieldOutput{
		Strategy:     "consensus",
		Rounds:       0,
		RoundHistory: nil,
		SessionID:    "empty-session",
	}

	var buf bytes.Buffer
	err := WriteYieldOutput(&buf, out)
	require.NoError(t, err)

	var parsed YieldOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))
	assert.Equal(t, 0, parsed.Rounds)
	assert.Nil(t, parsed.RoundHistory)
}

func TestBuildYieldOutputFromResult_MultiRound(t *testing.T) {
	t.Parallel()

	orchResult := &OrchestraResult{
		Strategy: StrategyDebate,
		RoundHistory: [][]ProviderResponse{
			{{Provider: "claude", Output: "r1", Duration: 1 * time.Second}},
			{{Provider: "claude", Output: "r2", Duration: 2 * time.Second}},
		},
	}

	result := BuildYieldOutputFromResult(orchResult, "sess-multi")

	assert.Equal(t, "debate", result.Strategy)
	assert.Equal(t, 2, result.Rounds)
	assert.Len(t, result.RoundHistory, 2)
	assert.Equal(t, 1, result.RoundHistory[0].Round)
	assert.Equal(t, 2, result.RoundHistory[1].Round)
	assert.Equal(t, "r1", result.RoundHistory[0].Responses[0].Output)
	assert.Equal(t, "r2", result.RoundHistory[1].Responses[0].Output)
	assert.Nil(t, result.Panes) // no panes in FromResult variant
}
