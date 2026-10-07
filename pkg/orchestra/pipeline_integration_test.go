package orchestra

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockFailBackend always returns an error for a specific provider.
type mockFailBackend struct {
	failProvider string
}

func (m *mockFailBackend) Execute(_ context.Context, req ProviderRequest) (*ProviderResponse, error) {
	if req.Provider == m.failProvider {
		return nil, fmt.Errorf("provider %s failed", req.Provider)
	}
	return &ProviderResponse{
		Provider: req.Provider,
		Output:   defaultOutput(req.Role),
	}, nil
}

func (m *mockFailBackend) Name() string { return "mock-fail" }

func (m *mockFailBackend) freshExecutionPerRequest() bool { return true }

func TestIntegration_SubprocessPipeline_FullDebate(t *testing.T) {
	t.Parallel()
	backend := &mockBackend{name: "integration"}
	cfg := SubprocessPipelineConfig{
		Backend: backend,
		Providers: []ProviderConfig{
			{Name: "claude", Binary: "echo"},
			{Name: "codex", Binary: "echo"},
			{Name: "gemini", Binary: "echo"},
		},
		Topic: "design a caching layer",
		PromptData: PromptData{
			ProjectName: "test", ProjectSummary: "test project", TechStack: "Go",
			MustReadFiles: []string{"go.mod"}, Topic: "design a caching layer", MaxTurns: 10,
		},
		Rounds: 1, // standard mode
		Judge:  ProviderConfig{Name: "claude", Binary: "echo"},
	}

	result, err := RunSubprocessPipeline(context.Background(), cfg)
	require.NoError(t, err)

	assert.Equal(t, StrategyDebate, result.Strategy)
	assert.Contains(t, result.Merged, "Orchestra Result")
	assert.Contains(t, result.Merged, "Judge Synthesis")
	assert.True(t, result.Duration > 0)
	assert.Empty(t, result.FailedProviders)
	// 3 providers + 1 judge
	assert.GreaterOrEqual(t, len(result.Responses), 4)
}

func TestIntegration_GracefulDegradation(t *testing.T) {
	t.Parallel()
	backend := &mockFailBackend{failProvider: "gemini"}
	cfg := SubprocessPipelineConfig{
		Backend: backend,
		Providers: []ProviderConfig{
			{Name: "claude", Binary: "echo"},
			{Name: "codex", Binary: "echo"},
			{Name: "gemini", Binary: "echo"},
		},
		Topic: "test degradation",
		PromptData: PromptData{
			ProjectName: "test", ProjectSummary: "s", TechStack: "Go",
			MustReadFiles: []string{"go.mod"}, Topic: "test degradation", MaxTurns: 5,
		},
		Rounds: 0, // fast mode
		Judge:  ProviderConfig{Name: "claude", Binary: "echo"},
	}

	result, err := RunSubprocessPipeline(context.Background(), cfg)
	require.NoError(t, err)

	// Gemini should be in failed providers
	assert.Len(t, result.FailedProviders, 1)
	assert.Equal(t, "gemini", result.FailedProviders[0].Name)
	// Pipeline should complete with 2 providers
	assert.Contains(t, result.Merged, "Orchestra Result")
}

func TestIntegration_AllProvidersFail(t *testing.T) {
	t.Parallel()
	backend := &mockBackend{name: "all-fail", err: fmt.Errorf("all down")}
	cfg := SubprocessPipelineConfig{
		Backend:   backend,
		Providers: []ProviderConfig{{Name: "p1", Binary: "echo"}},
		Topic:     "test",
		PromptData: PromptData{
			ProjectName: "t", ProjectSummary: "s", TechStack: "Go",
			MustReadFiles: []string{"go.mod"}, Topic: "test", MaxTurns: 5,
		},
		Rounds: 0,
		Judge:  ProviderConfig{Name: "judge", Binary: "echo"},
	}

	_, err := RunSubprocessPipeline(context.Background(), cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "all 1 providers failed")
}

func TestIntegration_DeepMode(t *testing.T) {
	t.Parallel()
	backend := &mockBackend{name: "deep"}
	cfg := SubprocessPipelineConfig{
		Backend:   backend,
		Providers: []ProviderConfig{{Name: "p1", Binary: "echo"}, {Name: "p2", Binary: "echo"}},
		Topic:     "deep analysis",
		PromptData: PromptData{
			ProjectName: "t", ProjectSummary: "s", TechStack: "Go",
			MustReadFiles: []string{"go.mod"}, Topic: "deep analysis", MaxTurns: 10,
		},
		Rounds: 2, // deep mode: independent + 2x cross-pollinate + judge
		Judge:  ProviderConfig{Name: "p1", Binary: "echo"},
	}

	result, err := RunSubprocessPipeline(context.Background(), cfg)
	require.NoError(t, err)
	assert.Contains(t, result.Summary, "3 rounds")
}

func TestIntegration_OutputParser_EndToEnd(t *testing.T) {
	t.Parallel()
	// Simulate: provider returns JSON, parser extracts, validator checks.
	r1 := DebaterR1Output{
		CurrentState: "good",
		Ideas:        []IdeaOutput{{Title: "cache", Description: "add redis", Rationale: "speed", Risks: "complexity", Category: "perf"}},
		Assumptions:  []AssumptionOut{{Type: "feasibility", Description: "redis avail", RiskLevel: "low"}},
		HMWQuestions: []string{"How might we reduce latency?"},
	}
	data, _ := json.Marshal(r1)

	parser := &OutputParser{}
	result, err := parser.ParseDebaterR1(string(data))
	require.NoError(t, err)
	assert.Equal(t, "cache", result.Ideas[0].Title)
}
