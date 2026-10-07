package orchestra

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Helper function tests (countNonEmpty, perRoundTimeout, buildDebateResult, mergeByStrategyWithRoundHistory) ---

// TestCountNonEmpty verifies counting of non-empty responses.
func TestCountNonEmpty(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		resps    []ProviderResponse
		expected int
	}{
		{"all non-empty", []ProviderResponse{{Output: "a"}, {Output: "b"}, {Output: "c"}}, 3},
		{"mixed", []ProviderResponse{{Output: "a"}, {Output: ""}, {Output: "c"}}, 2},
		{"all empty", []ProviderResponse{{Output: ""}, {Output: ""}}, 0},
		{"nil slice", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, countNonEmpty(tt.resps))
		})
	}
}

// TestPerRoundTimeout verifies per-round timeout calculation.
func TestPerRoundTimeout(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		total    int
		rounds   int
		expected time.Duration
	}{
		{"120s / 3 rounds", 120, 3, 45 * time.Second},        // debate=60, 60/3=20 < 45 -> floor
		{"60s / 1 round", 60, 1, 45 * time.Second},           // debate=0, 0/1=0 < 45 -> floor
		{"zero total defaults", 0, 2, 45 * time.Second},      // total=120, debate=60, 60/2=30 < 45 -> floor
		{"negative total defaults", -1, 4, 45 * time.Second}, // total=120, debate=60, 60/4=15 < 45 -> floor
		{"zero rounds defaults", 60, 0, 45 * time.Second},    // debate=0, 0/1=0 < 45 -> floor
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, perRoundTimeout(tt.total, tt.rounds, false))
		})
	}
}

// TestPerRoundTimeout_MinimumFloor verifies 45-second minimum floor per round.
func TestPerRoundTimeout_MinimumFloor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		total    int
		rounds   int
		expected time.Duration
	}{
		{"floor applied", 60, 3, 45 * time.Second},    // debate=0, 0/3=0 < 45 -> floor
		{"no floor needed", 120, 2, 45 * time.Second}, // debate=60, 60/2=30 < 45 -> floor
		{"default total", 0, 1, 60 * time.Second},     // total=120, debate=60, 60/1=60 > 45
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, perRoundTimeout(tt.total, tt.rounds, false))
		})
	}
}

// TestBuildDebateResult verifies result construction.
func TestBuildDebateResult(t *testing.T) {
	t.Parallel()
	responses := []ProviderResponse{
		{Provider: "claude", Output: "claude says"},
		{Provider: "gemini", Output: "gemini says"},
	}
	history := [][]ProviderResponse{responses}
	result := buildDebateResult(OrchestraConfig{Strategy: StrategyDebate}, responses, history, time.Now())
	assert.Equal(t, StrategyDebate, result.Strategy)
	assert.Len(t, result.Responses, 2)
	assert.Len(t, result.RoundHistory, 1)
	assert.NotEmpty(t, result.Merged)
}

// TestBuildDebateResult_NilResponses verifies nil responses fallback.
func TestBuildDebateResult_NilResponses(t *testing.T) {
	t.Parallel()
	result := buildDebateResult(OrchestraConfig{Strategy: StrategyDebate}, nil, nil, time.Now())
	assert.Contains(t, result.Merged, "0 rounds completed")
}

// TestBuildDebateResult_SingleRound verifies single round result.
func TestBuildDebateResult_SingleRound(t *testing.T) {
	t.Parallel()
	responses := []ProviderResponse{{Provider: "claude", Output: "only"}}
	history := [][]ProviderResponse{responses}
	result := buildDebateResult(OrchestraConfig{Strategy: StrategyDebate}, responses, history, time.Now())
	assert.Len(t, result.RoundHistory, 1)
	assert.NotEmpty(t, result.Merged)
}

// TestMergeByStrategyWithRoundHistory verifies round history merge.
func TestMergeByStrategyWithRoundHistory(t *testing.T) {
	t.Parallel()
	rounds := [][]ProviderResponse{
		{{Provider: "claude", Output: "r1"}, {Provider: "gemini", Output: "r1"}},
		{{Provider: "claude", Output: "r2"}, {Provider: "gemini", Output: "r2"}},
	}
	result := mergeByStrategyWithRoundHistory(rounds, OrchestraConfig{Strategy: StrategyDebate})
	require.NotNil(t, result)
	assert.Equal(t, StrategyDebate, result.Strategy)
	assert.Len(t, result.RoundHistory, 2)
	assert.Len(t, result.Responses, 2)
}

// TestMergeByStrategyWithRoundHistory_Empty verifies empty rounds.
func TestMergeByStrategyWithRoundHistory_Empty(t *testing.T) {
	t.Parallel()
	result := mergeByStrategyWithRoundHistory(nil, OrchestraConfig{Strategy: StrategyDebate})
	require.NotNil(t, result)
	assert.Nil(t, result.Responses)
}

// TestMergeByStrategyWithRoundHistory_SingleRound verifies single round.
func TestMergeByStrategyWithRoundHistory_SingleRound(t *testing.T) {
	t.Parallel()
	rounds := [][]ProviderResponse{{{Provider: "claude", Output: "single"}}}
	result := mergeByStrategyWithRoundHistory(rounds, OrchestraConfig{Strategy: StrategyDebate})
	assert.Len(t, result.Responses, 1)
	assert.Len(t, result.RoundHistory, 1)
}

// --- SPEC-ORCH-013 R1: Judge Timeout Separation ---

// TestPerRoundTimeout_JudgeBudgetSubtracted verifies perRoundTimeout subtracts
// judge budget (min 60s) from total before dividing by rounds.
// S2: total=120, rounds=3 -> judge=60, debate=60, per-round=max(60/3, 45)=45
func TestPerRoundTimeout_JudgeBudgetSubtracted(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		total    int
		rounds   int
		expected time.Duration
	}{
		{
			"total=120 rounds=3 -> judge=60 debate=60 perRound=45(floor)",
			120, 3, 45 * time.Second, // debate=60, 60/3=20 < 45 -> floor
		},
		{
			"total=300 rounds=2 -> judge=60 debate=240 perRound=120",
			300, 2, 120 * time.Second, // debate=240, 240/2=120
		},
		{
			"total=90 rounds=1 -> judge=60 debate=30 perRound=45(floor)",
			90, 1, 45 * time.Second, // debate=30, 30/1=30 < 45 -> floor
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := perRoundTimeout(tt.total, tt.rounds, false)
			assert.Equal(t, tt.expected, got,
				"perRoundTimeout must subtract judge budget (min 60s) before dividing")
		})
	}
}

// TestPerRoundTimeout_JudgeBudgetMinimum60s verifies judge budget is at least 60s.
func TestPerRoundTimeout_JudgeBudgetMinimum60s(t *testing.T) {
	t.Parallel()
	// total=80, rounds=2 -> judge=60, debate=20, per-round=max(10, 45)=45
	// Current code returns 80/2=40<45=45, but with judge subtraction: (80-60)/2=10<45=45
	// Both give 45 due to floor — add a case that distinguishes:
	// total=200, rounds=2 -> without judge: 200/2=100; with judge: (200-60)/2=70
	got := perRoundTimeout(200, 2, false)
	assert.Equal(t, 70*time.Second, got,
		"judge budget must be minimum 60s; total=200, rounds=2 -> debate=140, perRound=70")
}

// TestPerRoundTimeout_NoJudge verifies that noJudge=true skips judge budget reservation,
// giving the full timeout to debate rounds.
func TestPerRoundTimeout_NoJudge(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		total    int
		rounds   int
		noJudge  bool
		expected time.Duration
	}{
		{
			"noJudge=true total=120 rounds=2 -> no reserve, 120/2=60",
			120, 2, true, 60 * time.Second,
		},
		{
			"noJudge=true total=300 rounds=1 -> no reserve, 300/1=300",
			300, 1, true, 300 * time.Second,
		},
		{
			"noJudge=false total=300 rounds=1 -> reserve=60, 240/1=240",
			300, 1, false, 240 * time.Second,
		},
		{
			"noJudge=true total=120 rounds=1 -> no reserve, 120/1=120 (brainstorm yield)",
			120, 1, true, 120 * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := perRoundTimeout(tt.total, tt.rounds, tt.noJudge)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestJudgeTimeout_UsesParentContext verifies judge respects parent context
// cancellation and does not keep running independently.
func TestJudgeTimeout_UsesParentContext(t *testing.T) {
	t.Parallel()
	parentCtx, cancel := context.WithCancel(context.Background())
	cancel()

	cfg := OrchestraConfig{
		Providers:      []ProviderConfig{{Name: "claude", Binary: "echo"}},
		JudgeProvider:  "claude",
		TimeoutSeconds: 60,
	}
	resp := runJudgeRound(parentCtx, cfg, nil, nil, []ProviderResponse{
		{Provider: "claude", Output: "test output"},
	}, 1)
	require.NotNil(t, resp, "cancelled judge attempts must remain observable")
	assert.ErrorIs(t, context.Canceled, parentCtx.Err())
	assert.Equal(t, context.Canceled.Error(), resp.Error)
	assert.Equal(t, "judge", resp.Role)
	assert.Equal(t, 2, resp.Attempt)
	assert.Equal(t, TerminalBlocked, resp.TerminalState)
	assert.Equal(t, noneBackendMarker, resp.ExecutedBackend)
}
