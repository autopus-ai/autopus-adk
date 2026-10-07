package orchestra

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConsensus_ExplicitThreshold_PreservesEveryDissentingClaim(t *testing.T) {
	t.Parallel()

	responses := []ProviderResponse{
		{Provider: "claude", Output: "1. Rotate leaked signing key immediately\n2. Preserve the audit log"},
		{Provider: "codex", Output: "1. No action required\n2. Preserve the audit log"},
		{Provider: "gemini", Output: "1. No action required\n2. Preserve the audit log"},
	}
	cfg := OrchestraConfig{
		Strategy:           StrategyConsensus,
		ConsensusThreshold: 0.67,
	}

	merged, _, err := handleConsensus(context.Background(), responses, cfg)

	require.NoError(t, err)
	assert.Contains(t, merged, "Rotate leaked signing key immediately")
	assert.Contains(t, merged, "No action required")
	assert.Contains(t, merged, "[1/3]")
	assert.Contains(t, merged, "이견")
}

func TestConsensusReached_IdenticalStructuredClaims_ReturnsTrue(t *testing.T) {
	t.Parallel()

	output := "1. Preserve requested strategy\n2. Record provider quorum\n3. Keep dissent evidence"
	responses := []ProviderResponse{
		{Provider: "claude", Output: output},
		{Provider: "codex", Output: output},
	}

	assert.True(t, consensusReached(responses, OrchestraConfig{ConsensusThreshold: 0.67}))
}

type contractRoleBackend struct {
	mu     sync.Mutex
	counts map[string]int
}

func (b *contractRoleBackend) Execute(_ context.Context, req ProviderRequest) (*ProviderResponse, error) {
	b.mu.Lock()
	if b.counts == nil {
		b.counts = make(map[string]int)
	}
	b.counts[req.Role]++
	b.mu.Unlock()

	return &ProviderResponse{Provider: req.Provider, Output: contractRoleOutput(req.Role)}, nil
}

func (b *contractRoleBackend) Name() string { return "contract-recorder" }

// freshExecutionPerRequest models one independent built-in execution per recorded request.
func (b *contractRoleBackend) freshExecutionPerRequest() bool { return true }

func (b *contractRoleBackend) count(role string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.counts[role]
}

func contractRoleOutput(role string) string {
	if role == "judge" {
		body, _ := json.Marshal(JudgeOutput{Recommendation: "proceed"})
		return string(body)
	}
	body, _ := json.Marshal(DebaterR1Output{
		CurrentState: "state",
		Ideas: []IdeaOutput{{
			Title:       "contract",
			Description: "converge orchestration",
			Rationale:   "observable behavior",
			Risks:       "none",
			Category:    "runtime",
		}},
	})
	return string(body)
}

func TestRunSubprocessPipeline_StandardDebate_UsesExpectedRoleCounts(t *testing.T) {
	t.Parallel()

	backend := &contractRoleBackend{}
	cfg := SubprocessPipelineConfig{
		Backend: backend,
		Providers: []ProviderConfig{
			{Name: "claude", Binary: "claude"},
			{Name: "codex", Binary: "codex"},
			{Name: "gemini", Binary: "agy"},
		},
		Topic: "orchestration convergence",
		PromptData: PromptData{
			ProjectName:    "autopus-adk",
			ProjectSummary: "contract test",
			TechStack:      "Go",
			MustReadFiles:  []string{"go.mod"},
			Topic:          "orchestration convergence",
			MaxTurns:       5,
		},
		Rounds: 1,
		Judge:  ProviderConfig{Name: "judge", Binary: "judge"},
	}

	result, err := RunSubprocessPipeline(context.Background(), cfg)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 3, backend.count("debater_r1"))
	assert.Equal(t, 3, backend.count("debater_r2"))
	assert.Equal(t, 1, backend.count("judge"))
}

func TestConsensus_StableFindingIdentityAndCriticalVeto_AreTyped(t *testing.T) {
	t.Parallel()

	responses := []ProviderResponse{
		{Provider: "claude", Output: marshalReviewerOutput(t, []Finding{
			{Severity: "major", Category: "correctness", ScopeRef: "pkg/router.go", Location: "pkg/router.go:12", Description: "Preserve the configured provider denominator"},
			{Severity: "critical", Category: "security", ScopeRef: "pkg/keys.go", Location: "pkg/keys.go:41", Description: "Rotate the leaked signing key"},
		})},
		{Provider: "codex", Output: marshalReviewerOutput(t, []Finding{
			{Severity: "minor", Category: "style", ScopeRef: "README.md", Location: "README.md:7", Description: "Clarify the example"},
			{Severity: "major", Category: "correctness", Location: "pkg/router.go:99", Description: "Preserve the configured provider denominator"},
		})},
		{Provider: "gemini", Output: marshalReviewerOutput(t, []Finding{
			{Severity: "major", Category: "correctness", ScopeRef: "pkg/router.go#L44", Description: "Preserve the configured provider denominator"},
		})},
	}
	cfg := OrchestraConfig{
		Strategy:            StrategyConsensus,
		Providers:           []ProviderConfig{{Name: "claude"}, {Name: "codex"}, {Name: "gemini"}},
		ConfiguredProviders: []string{"claude", "codex", "gemini"},
		ConsensusThreshold:  0.67,
	}
	merged, summary := MergeConsensus(responses, cfg.ConsensusThreshold)
	result := FinalizeOrchestrationResult(&OrchestraResult{
		Strategy: StrategyConsensus, Responses: responses, Merged: merged, Summary: summary,
	}, cfg)

	require.NotNil(t, result.ConsensusMetrics)
	assert.Equal(t, 3, result.ConsensusMetrics.TotalClaims)
	assert.Equal(t, 1, result.ConsensusMetrics.AgreedClaims)
	assert.Equal(t, 2, result.ConsensusMetrics.DissentClaims)
	assert.True(t, result.ConsensusMetrics.CriticalVeto)
	assert.True(t, result.Veto)
	assert.Equal(t, TerminalBlocked, result.TerminalState)
	assert.Equal(t, "blocked", result.GateStatus)
	assert.Contains(t, result.DegradedReasons, "critical_dissent_veto")
	assert.Contains(t, merged, "[3/3]")
	assert.Contains(t, merged, "[1/3]")

	require.NotNil(t, result.RunReceipt)
	assert.Equal(t, OrchestrationReceiptSchema, result.RunReceipt.Schema)
	assert.True(t, result.RunReceipt.CriticalVeto)
	assert.Equal(t, TerminalBlocked, result.RunReceipt.TerminalState)
	assert.Equal(t, 3, result.RunReceipt.DispatchCount)
	data, err := json.Marshal(result.RunReceipt)
	require.NoError(t, err)
	var receipt map[string]any
	require.NoError(t, json.Unmarshal(data, &receipt))
	assert.Contains(t, receipt, "failed_providers")
	assert.Contains(t, receipt, "provider_receipts")
	assert.Contains(t, receipt, "worker_receipts")
	assert.Contains(t, receipt, "transitions")
}

func TestProviderIntegrity_ConfiguredDenominatorBlocksOneOfThree(t *testing.T) {
	t.Parallel()

	result := FinalizeOrchestrationResult(&OrchestraResult{
		Strategy:  StrategyConsensus,
		Responses: []ProviderResponse{{Provider: "claude", Output: "1. usable"}},
	}, OrchestraConfig{
		Strategy:            StrategyConsensus,
		Providers:           []ProviderConfig{{Name: "claude"}},
		RequestedProviders:  []string{"claude", "codex", "gemini"},
		ConfiguredProviders: []string{"claude", "codex", "gemini"},
	})

	assert.Equal(t, []string{"claude", "codex", "gemini"}, result.RequestedProviders)
	assert.Equal(t, []string{"claude", "codex", "gemini"}, result.ConfiguredProviders)
	assert.Equal(t, []string{"claude"}, result.ResolvedProviders)
	assert.Equal(t, []string{"claude"}, result.AttemptedProviders)
	assert.Equal(t, []string{"claude"}, result.UsableProviders)
	assert.Empty(t, result.FailedProviderNames)
	assert.Equal(t, 2, result.QuorumRequired)
	assert.False(t, result.QuorumMet)
	assert.True(t, result.Degraded)
	assert.Contains(t, result.DegradedReasons, "provider_quorum")
	assert.Equal(t, "blocked", result.GateStatus)
	require.NotNil(t, result.RunReceipt)
	assert.False(t, result.RunReceipt.QuorumMet)
	assert.Equal(t, "blocked", result.RunReceipt.GateStatus)
}

func TestProviderIntegrity_RiskFloorCanRequireTwoFromConfiguredOne(t *testing.T) {
	t.Parallel()

	result := FinalizeOrchestrationResult(&OrchestraResult{
		Strategy:  StrategyConsensus,
		Responses: []ProviderResponse{{Provider: "claude", Output: "1. usable"}},
	}, OrchestraConfig{
		Strategy: StrategyConsensus, Providers: []ProviderConfig{{Name: "claude"}},
		ConfiguredProviders: []string{"claude"}, MinimumProviders: 2,
	})

	assert.Equal(t, 2, result.QuorumRequired)
	assert.False(t, result.QuorumMet)
	assert.Contains(t, result.DegradedReasons, "provider_quorum")
	assert.Equal(t, "blocked", result.GateStatus)
}

func TestRunOrchestra_RequiredJudgeFailurePreservesParticipantsAndBlocks(t *testing.T) {
	result, err := RunOrchestra(context.Background(), OrchestraConfig{
		Providers: []ProviderConfig{echoProvider("claude")},
		Strategy:  StrategyDebate, Prompt: "judge contract", TimeoutSeconds: 5,
		JudgeProvider: "autopus-judge-that-does-not-exist",
	})

	require.Error(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, result.Responses)
	assert.Equal(t, "claude", result.Responses[0].Provider)
	assert.Equal(t, JudgeFailed, result.JudgeStatus)
	assert.Equal(t, TerminalBlocked, result.TerminalState)
	assert.Equal(t, "blocked", result.GateStatus)
	assert.Contains(t, result.DegradedReasons, "judge_failure")
	assert.Contains(t, result.Summary, "필수 판정 실패")
	judgeFailures := 0
	for _, failed := range result.FailedProviders {
		if failed.Role == "judge" {
			judgeFailures++
		}
	}
	assert.Equal(t, 1, judgeFailures)
	require.NotNil(t, result.RunReceipt)
	assert.Equal(t, JudgeFailed, result.RunReceipt.JudgeStatus)
	assert.Equal(t, TerminalBlocked, result.RunReceipt.TerminalState)
}

func marshalReviewerOutput(t *testing.T, findings []Finding) string {
	t.Helper()
	data, err := json.Marshal(ReviewerOutput{Findings: findings, Verdict: "REVISE"})
	require.NoError(t, err)
	return string(data)
}
