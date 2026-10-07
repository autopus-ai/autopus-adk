package orchestra

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunOrchestra_SubprocessDebateProjectsVerifiedFreshJudgeEvidence(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
	}{
		{name: "forced subprocess"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			judge := typedJudgeProvider(t, "judge")
			cfg := OrchestraConfig{
				Providers: []ProviderConfig{
					echoProvider("debater-a"),
					echoProvider("debater-b"),
				},
				Strategy:       StrategyDebate,
				Prompt:         "fresh subprocess judge evidence",
				TimeoutSeconds: 10,
				JudgeProvider:  judge.Name,
				JudgeConfig:    &judge,
			}

			result, err := RunOrchestra(context.Background(), cfg)

			require.NoError(t, err)
			require.NotNil(t, result)
			require.NotNil(t, result.FreshJudgeSession)
			assert.True(t, result.FreshJudgeSession.Required)
			assert.True(t, result.FreshJudgeSession.Isolated)
			assert.True(t, result.FreshJudgeSession.Verified)
			assert.True(t, result.FreshJudgeSession.ParticipantsTerminated)
			assert.Equal(t, "fresh_backend_execution", result.FreshJudgeSession.Mechanism)
			require.NotNil(t, result.RunReceipt)
			assert.Equal(t, result.FreshJudgeSession, result.RunReceipt.FreshJudgeSession)
		})
	}
}

func TestRunOrchestra_SubprocessJudgeResumeArgsFailClosedBeforeDispatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		provider string
		args     []string
	}{
		{name: "claude continue", provider: "claude", args: []string{"--continue"}},
		{name: "codex exec resume", provider: "codex", args: []string{"exec", "resume", "--last"}},
		{name: "gemini conversation", provider: "gemini", args: []string{"--conversation", "prior-id"}},
		{name: "opencode session", provider: "opencode", args: []string{"run", "--session", "prior-id"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			judge := typedJudgeProvider(t, tt.provider)
			marker := filepath.Join(t.TempDir(), "judge-dispatched")
			script := "#!/bin/sh\n: > " + shellSingleQuote(marker) +
				"\ncat >/dev/null\nprintf '%s\\n' '{\"recommendation\":\"resumed judge\"}'\n"
			require.NoError(t, os.WriteFile(judge.Binary, []byte(script), 0o700))
			judge.Args = tt.args
			cfg := OrchestraConfig{
				Providers:      []ProviderConfig{echoProvider("debater")},
				Strategy:       StrategyDebate,
				Prompt:         "resume arguments must not satisfy fresh evidence",
				TimeoutSeconds: 10,
				JudgeProvider:  judge.Name,
				JudgeConfig:    &judge,
			}

			result, err := RunOrchestra(context.Background(), cfg)

			require.Error(t, err)
			require.NotNil(t, result)
			assert.Equal(t, JudgeFailed, result.JudgeStatus)
			assert.Equal(t, TerminalBlocked, result.TerminalState)
			assert.Contains(t, result.DegradedReasons, "fresh_judge_session")
			require.NotNil(t, result.FreshJudgeSession)
			assert.True(t, result.FreshJudgeSession.Required)
			assert.False(t, result.FreshJudgeSession.Isolated)
			assert.False(t, result.FreshJudgeSession.Verified)
			assert.True(t, result.FreshJudgeSession.ParticipantsTerminated)
			assert.Contains(t, result.FreshJudgeSession.Reason, "resume")
			require.NotNil(t, result.RunReceipt)
			assert.Equal(t, result.FreshJudgeSession, result.RunReceipt.FreshJudgeSession)
			assert.Contains(t, result.RunReceipt.DegradedReasons, "fresh_judge_session")
			_, statErr := os.Stat(marker)
			assert.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}
}

func TestFinalizeDebateOutcome_InvalidFreshJudgeEvidenceBlocksPassedJudge(t *testing.T) {
	t.Parallel()
	validEvidence := func() *FreshJudgeSessionEvidence {
		return &FreshJudgeSessionEvidence{
			Required:               true,
			Isolated:               true,
			Verified:               true,
			ParticipantsTerminated: true,
			Mechanism:              "fresh_backend_execution",
		}
	}
	tests := []struct {
		name     string
		evidence func() *FreshJudgeSessionEvidence
	}{
		{name: "missing", evidence: func() *FreshJudgeSessionEvidence { return nil }},
		{name: "not required", evidence: func() *FreshJudgeSessionEvidence {
			evidence := validEvidence()
			evidence.Required = false
			return evidence
		}},
		{name: "not isolated", evidence: func() *FreshJudgeSessionEvidence {
			evidence := validEvidence()
			evidence.Isolated = false
			return evidence
		}},
		{name: "not verified", evidence: func() *FreshJudgeSessionEvidence {
			evidence := validEvidence()
			evidence.Verified = false
			return evidence
		}},
		{name: "participants not terminated", evidence: func() *FreshJudgeSessionEvidence {
			evidence := validEvidence()
			evidence.ParticipantsTerminated = false
			return evidence
		}},
	}
	cfg := OrchestraConfig{
		Providers:        []ProviderConfig{{Name: "debater"}},
		Strategy:         StrategyDebate,
		JudgeProvider:    "judge",
		MinimumProviders: 1,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := finalizeDebateOutcome(&OrchestraResult{
				Strategy: StrategyDebate,
				Responses: []ProviderResponse{{
					Provider:        "judge (judge)",
					Output:          `{"recommendation":"apparently successful"}`,
					Role:            "judge",
					Attempt:         2,
					ExecutedBackend: "subprocess",
					TerminalState:   TerminalCompleted,
				}},
				FreshJudgeSession: tt.evidence(),
			}, cfg)

			require.Error(t, err)
			require.NotNil(t, result)
			assert.Equal(t, JudgeFailed, result.JudgeStatus)
			assert.Equal(t, TerminalBlocked, result.TerminalState)
			assert.Equal(t, "blocked", result.GateStatus)
			assert.Contains(t, result.DegradedReasons, "fresh_judge_session")
			require.NotNil(t, result.RunReceipt)
			assert.Equal(t, result.FreshJudgeSession, result.RunReceipt.FreshJudgeSession)
		})
	}
}
