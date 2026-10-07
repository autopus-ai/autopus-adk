package orchestra

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recheck strategy must be reachable through the same registry every other
// strategy uses; a strategy that validates but has no handler fails at merge
// time instead of at config time.
func TestStrategyRecheck_IsValidAndRegistered(t *testing.T) {
	t.Parallel()
	assert.True(t, StrategyRecheck.IsValid())
	fn, err := GetStrategyFunc(StrategyRecheck)
	require.NoError(t, err)
	assert.NotNil(t, fn)
}

// Round 2 must restate the task and hand the model its own prior answer, and
// nothing else. Peer content is the part measurement found worthless.
func TestBuildRecheckPrompt_CarriesOwnAnswerOnce(t *testing.T) {
	t.Parallel()
	prompt := buildRecheckPrompt("TASK-BODY", "OWN-ANSWER")

	assert.Contains(t, prompt, "TASK-BODY")
	assert.Contains(t, prompt, "Your round 1 answer")
	assert.Contains(t, prompt, "Re-derive")
	// Exactly one copy: a second copy would mean the answer leaked into the
	// task slot as well, which is how peer text historically crept in.
	assert.Equal(t, 1, strings.Count(prompt, "OWN-ANSWER"))
}

// The re-derived answer is the answer. Returning the first response would
// silently spend a second provider call and throw the result away.
func TestHandleRecheck_ReturnsRederivedAnswer(t *testing.T) {
	t.Parallel()
	responses := []ProviderResponse{
		{Provider: "claude", Output: "first"},
		{Provider: "claude", Output: "second"},
	}
	merged, summary, err := handleRecheck(context.Background(), responses, OrchestraConfig{})
	require.NoError(t, err)
	assert.Equal(t, "second", merged)
	assert.Contains(t, summary, "수정")
	assert.Contains(t, summary, "2라운드")
}

// An unchanged answer is a real and common outcome; it must be reported as
// held rather than revised so callers can tell the two apart.
func TestHandleRecheck_ReportsHeldAnswer(t *testing.T) {
	t.Parallel()
	responses := []ProviderResponse{
		{Provider: "codex", Output: "same"},
		{Provider: "codex", Output: "same"},
	}
	merged, summary, err := handleRecheck(context.Background(), responses, OrchestraConfig{})
	require.NoError(t, err)
	assert.Equal(t, "same", merged)
	assert.Contains(t, summary, "유지")
}

func TestHandleRecheck_EmptyResponsesFail(t *testing.T) {
	t.Parallel()
	_, _, err := handleRecheck(context.Background(), nil, OrchestraConfig{})
	require.Error(t, err)
}

// recheck is a single-participant strategy by construction. Silently dropping
// extra providers would bill the caller for a fan-out they never got.
func TestRunRecheck_RejectsNonSingleProvider(t *testing.T) {
	t.Parallel()
	for name, providers := range map[string][]ProviderConfig{
		"two":  {{Name: "a", Binary: "cat"}, {Name: "b", Binary: "cat"}},
		"none": {},
	} {
		providers := providers
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, _, _, err := runRecheck(context.Background(), OrchestraConfig{
				Strategy: StrategyRecheck, Providers: providers, Prompt: "p", TimeoutSeconds: 10,
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "정확히 1개")
		})
	}
}

// End-to-end through the public runner with `cat` as the provider: stdin is
// echoed back, so each response is literally the prompt that produced it.
// That makes the round-2 input observable, which is the property that matters
// — a bug that forgets to rebuild the prompt still returns a plausible answer.
func TestRunOrchestraRecheck_SecondRoundSeesOwnFirstAnswer(t *testing.T) {
	t.Parallel()
	result, err := RunOrchestra(context.Background(), OrchestraConfig{
		Strategy:       StrategyRecheck,
		Providers:      []ProviderConfig{{Name: "cat", Binary: "cat", ExecutionTimeout: 20 * time.Second}},
		Prompt:         "ORIGINAL-TASK",
		TimeoutSeconds: 30,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Len(t, result.Responses, 2, "recheck must issue exactly two provider calls")
	require.Len(t, result.RoundHistory, 2, "both rounds must be retained as evidence")

	assert.Equal(t, "ORIGINAL-TASK", strings.TrimSpace(result.Responses[0].Output))
	// Round 2's echoed prompt proves it received the round-1 answer and the
	// re-derivation instruction, with the task restated.
	round2 := result.Responses[1].Output
	assert.Contains(t, round2, "Round 2: Reconsider")
	assert.Contains(t, round2, "Your round 1 answer")
	assert.Contains(t, round2, "ORIGINAL-TASK")

	assert.Equal(t, round2, result.Merged, "merged answer must be the re-derived one")
	assert.Equal(t, StrategyRecheck, result.Strategy)
	assert.False(t, result.Degraded)
	for i, resp := range result.Responses {
		assert.Equal(t, "subprocess", resp.ExecutedBackend,
			"round %d must report the subprocess transport", i+1)
	}
}
