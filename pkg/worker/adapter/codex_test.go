package adapter

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/telemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodexAdapterName(t *testing.T) {
	a := NewCodexAdapter()
	assert.Equal(t, "codex", a.Name())
}

func TestCodexAdapterBuildCommand(t *testing.T) {
	a := NewCodexAdapter()
	task := TaskConfig{
		TaskID:  "task-c1",
		Prompt:  "fix the bug",
		WorkDir: "/tmp/codex-work",
		EnvVars: map[string]string{"FOO": "bar"},
	}
	cmd := a.BuildCommand(context.Background(), task)
	assert.Contains(t, cmd.Args, "exec")
	assertCodexWorkerSandboxed(t, cmd.Args)
	assert.NotContains(t, cmd.Args, "--full-auto")
	assert.Contains(t, cmd.Args, "-")
	assert.NotContains(t, cmd.Args, "fix the bug")
	assert.Contains(t, cmd.Args, "--json")
	assert.NotContains(t, cmd.Args, "resume")
	assert.Equal(t, "/tmp/codex-work", cmd.Dir)
	envContains(t, cmd.Env, "AUTOPUS_TASK_ID=task-c1")
	envContains(t, cmd.Env, "FOO=bar")
}

func TestCodexAdapterBuildCommandWithSession(t *testing.T) {
	a := NewCodexAdapter()
	task := TaskConfig{
		TaskID:    "task-c2",
		SessionID: "my-session",
	}
	cmd := a.BuildCommand(context.Background(), task)
	assert.NotContains(t, cmd.Args, "resume")
	assert.NotContains(t, cmd.Args, "my-session")
}

func TestCodexAdapterBuildCommandWithModel(t *testing.T) {
	a := NewCodexAdapter()
	task := TaskConfig{
		TaskID: "task-cm1",
		Model:  "o3",
	}
	cmd := a.BuildCommand(context.Background(), task)
	assertCodexWorkerSandboxed(t, cmd.Args)
	assert.Contains(t, cmd.Args, "-m")
	assert.Contains(t, cmd.Args, "o3")
}

func TestCodexAdapterBuildCommand_NormalizesOpenAIModelOverride(t *testing.T) {
	a := NewCodexAdapter()
	task := TaskConfig{
		TaskID: "task-cm2",
		Model:  "openai/gpt-5.4",
	}
	cmd := a.BuildCommand(context.Background(), task)
	assertCodexWorkerSandboxed(t, cmd.Args)
	assert.Contains(t, cmd.Args, "-m")
	assert.Contains(t, cmd.Args, "gpt-5.4")
	assert.NotContains(t, cmd.Args, "openai/gpt-5.4")
}

func TestCodexAdapterBuildCommand_OmitsCodexAccountUnsupportedModelOverride(t *testing.T) {
	a := NewCodexAdapter()
	for _, model := range []string{"openai/gpt-5.2-codex", "gpt-5.2-codex"} {
		t.Run(model, func(t *testing.T) {
			task := TaskConfig{
				TaskID: "task-cm-codex",
				Model:  model,
			}
			cmd := a.BuildCommand(context.Background(), task)
			assertCodexWorkerSandboxed(t, cmd.Args)
			assert.NotContains(t, cmd.Args, "-m")
			assert.NotContains(t, cmd.Args, model)
		})
	}
}

func TestCodexAdapterBuildCommand_OmitsNonOpenAIModelOverride(t *testing.T) {
	a := NewCodexAdapter()
	task := TaskConfig{
		TaskID: "task-cm3",
		Model:  "anthropic/claude-opus-4-6",
	}
	cmd := a.BuildCommand(context.Background(), task)
	assertCodexWorkerSandboxed(t, cmd.Args)
	assert.NotContains(t, cmd.Args, "-m")
	assert.NotContains(t, cmd.Args, "anthropic/claude-opus-4-6")
}

func TestCodexAdapterParseEvent(t *testing.T) {
	a := NewCodexAdapter()
	line := []byte(`{"type":"result","output":"done"}`)
	evt, err := a.ParseEvent(line)
	require.NoError(t, err)
	assert.Equal(t, "result", evt.Type)
	assert.NotEmpty(t, evt.Data)
}

func TestCodexAdapterParseEventSplitsDottedTypes(t *testing.T) {
	a := NewCodexAdapter()
	line := []byte(`{"type":"turn.completed"}`)
	evt, err := a.ParseEvent(line)
	require.NoError(t, err)
	assert.Equal(t, "turn", evt.Type)
	assert.Equal(t, "completed", evt.Subtype)
}

func TestCodexAdapterParseEventPromotesAgentMessageToResult(t *testing.T) {
	a := NewCodexAdapter()
	line := []byte(`{"type":"item.completed","item":{"id":"item_0","type":"agent_message","text":"done via codex"}}`)
	evt, err := a.ParseEvent(line)
	require.NoError(t, err)
	assert.Equal(t, "result", evt.Type)
	result := a.ExtractResult(evt)
	assert.Equal(t, "done via codex", result.Output)
}

func TestCodexAdapterParseEventToolCall(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, line, wantType, wantSubtype string
		wantCalls                         int
	}{
		{"legacy top-level", `{"type":"tool_call","name":"exec_cmd"}`, "tool_call", "", 1},
		{"command execution", `{"type":"item.completed","item":{"type":"command_execution"}}`, "tool_call", "command_execution", 1},
		{"file change", `{"type":"item.completed","item":{"type":"file_change"}}`, "tool_call", "file_change", 1},
		{"mcp", `{"type":"item.completed","item":{"type":"mcp_tool_call"}}`, "tool_call", "mcp_tool_call", 1},
		{"web search", `{"type":"item.completed","item":{"type":"web_search"}}`, "tool_call", "web_search", 1},
		{"future tool fail closed", `{"type":"item.completed","item":{"type":"future_tool"}}`, "tool_call", "future_tool", 1},
		{"reasoning", `{"type":"item.completed","item":{"type":"reasoning"}}`, "item", "completed", 0},
		{"started avoids double count", `{"type":"item.started","item":{"type":"command_execution"}}`, "item", "started", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evt, err := NewCodexAdapter().ParseEvent([]byte(tt.line))
			require.NoError(t, err)
			assert.Equal(t, tt.wantType, evt.Type)
			assert.Equal(t, tt.wantSubtype, evt.Subtype)
			assert.Equal(t, tt.wantCalls, evt.ToolCalls)
			assert.JSONEq(t, tt.line, string(evt.Data))
		})
	}
}

func TestCodexAdapterParseEventInvalid(t *testing.T) {
	a := NewCodexAdapter()
	_, err := a.ParseEvent([]byte("not json"))
	require.Error(t, err)
}

func TestCodexAdapterParseEventMissingType(t *testing.T) {
	a := NewCodexAdapter()
	_, err := a.ParseEvent([]byte(`{"output":"no type"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing type")
}

func TestCodexAdapterExtractResult(t *testing.T) {
	a := NewCodexAdapter()
	evt := StreamEvent{
		Type: "result",
		Data: []byte(`{"type":"result","output":"all done","cost_usd":0.12,"session_id":"s1"}`),
	}
	result := a.ExtractResult(evt)
	assert.InDelta(t, 0.12, result.CostUSD, 0.001)
	assert.Equal(t, "s1", result.SessionID)
	assert.Equal(t, "all done", result.Output)
}

func TestCodexAdapterExtractResultInvalidJSON(t *testing.T) {
	a := NewCodexAdapter()
	evt := StreamEvent{Data: []byte("bad")}
	result := a.ExtractResult(evt)
	assert.Equal(t, "bad", result.Output)
}

func TestCodexAdapterParseEvent_TurnCompletedNormalizesOpenAIUsage(t *testing.T) {
	a := NewCodexAdapter()
	evt, err := a.ParseEvent([]byte(`{"type":"turn.completed","run_id":"run-c","call_id":"call-c","task_id":"task-c","attempt":3,"phase":"review","role":"reviewer","effort":"xhigh","model":"gpt-5.4","usage":{"input_tokens":1000,"cached_input_tokens":400,"output_tokens":300,"reasoning_output_tokens":100}}`))
	require.NoError(t, err)
	require.Len(t, evt.Usage, 1)
	usage := evt.Usage[0]
	assert.Equal(t, telemetry.UsageStatusActual, usage.UsageStatus)
	assert.Equal(t, int64(1000), *usage.InputTokensTotal)
	assert.Equal(t, int64(600), *usage.UncachedInputTokens)
	assert.Equal(t, int64(400), *usage.CachedInputTokens)
	assert.Equal(t, int64(300), *usage.OutputTokensTotal)
	assert.Equal(t, int64(100), *usage.ReasoningTokens)
	assert.Equal(t, telemetry.ComponentSubsetOfOutput, usage.ReasoningRelation)
	assert.Equal(t, int64(1300), *usage.RawTotalTokens)
	payload, marshalErr := json.Marshal(usage)
	require.NoError(t, marshalErr)
	assert.NotContains(t, string(payload), "prompt")
	assert.NotContains(t, string(payload), "response")
}

func TestCodexAdapterParseEvent_UnboundUsagePreservesSemanticsUntilWorkerBinding(t *testing.T) {
	a := NewCodexAdapter()
	evt, err := a.ParseEvent([]byte(`{"type":"turn.completed","usage":{"input_tokens":10,"output_tokens":5}}`))
	require.NoError(t, err)
	require.Len(t, evt.Usage, 1)
	assert.Empty(t, evt.Usage[0].RunID)
	assert.Empty(t, evt.Usage[0].CallID)
	assert.Equal(t, telemetry.UsageStatusActual, evt.Usage[0].UsageStatus)
	assert.Equal(t, int64(15), *evt.Usage[0].RawTotalTokens)
	assert.True(t, telemetry.AggregateUsage(evt.Usage).PromotionBlocked)
}

func TestTaskConfig_UsageIdentityIsAdditive(t *testing.T) {
	config := TaskConfig{
		TaskID: "task", RunID: "run", CallID: "call", Attempt: 2,
		Phase: "implementation", Role: "executor", Effort: "high",
		ProviderVersion: "2.1.154", ModelVersion: "2026-07-01", RiskPolicy: "risk-v1",
		CacheStratum: "cold", ConfigHash: "config-hash",
	}
	assert.Equal(t, "run", config.RunID)
	assert.Equal(t, "call", config.CallID)
	assert.Equal(t, 2, config.Attempt)
	assert.Equal(t, "implementation", config.Phase)
	assert.Equal(t, "executor", config.Role)
	assert.Equal(t, "high", config.Effort)
	assert.Equal(t, "2.1.154", config.ProviderVersion)
	assert.Equal(t, "2026-07-01", config.ModelVersion)
	assert.Equal(t, "risk-v1", config.RiskPolicy)
	assert.Equal(t, "cold", config.CacheStratum)
	assert.Equal(t, "config-hash", config.ConfigHash)
}

func TestCodexAdapterBuildCommand_EvidenceSafeOptions(t *testing.T) {
	t.Parallel()
	task := TaskConfig{TaskID: "evidence", Prompt: "measure", Effort: "ultra", Codex: CodexTaskOptions{
		Sandbox: CodexSandboxWorkspaceWrite, Ephemeral: true, IgnoreUserConfig: true,
		IgnoreRules: true, SkipGitRepoCheck: true, OutputSchema: "/tmp/result.schema.json",
		ZeroToolMode: true, RawTokenBudget: 1_500_000,
	}}
	got := NewCodexAdapter().BuildCommand(context.Background(), task).Args[1:]
	want := []string{
		"exec", "--sandbox", "workspace-write", "--ephemeral", "--ignore-user-config", "--ignore-rules",
		"--skip-git-repo-check", "--output-schema", "/tmp/result.schema.json",
		"--disable", "multi_agent", "--disable", "multi_agent_v2", "--disable", "enable_fanout",
		"--disable", "shell_tool", "--disable", "apps", "--disable", "hooks", "-", "--json",
		"-c", `model_reasoning_effort="ultra"`, "-c", `features.rollout_budget={enabled=true,limit_tokens=1500000,reminder_at_remaining_tokens=[],sampling_token_weight=1.0,prefill_token_weight=1.0}`,
	}
	assert.Equal(t, want, got)
	assert.NotContains(t, got, "--dangerously-bypass-approvals-and-sandbox")
}

func TestCodexAdapterBuildCommand_ReasoningEffortAllowlist(t *testing.T) {
	t.Parallel()
	for _, effort := range []string{"low", "medium", "high", "xhigh", "max", "ultra", "invalid", " high "} {
		t.Run(effort, func(t *testing.T) {
			got := NewCodexAdapter().BuildCommand(context.Background(), TaskConfig{Effort: effort}).Args
			assignment := `model_reasoning_effort="` + effort + `"`
			if effort == "invalid" || effort == " high " {
				assert.NotContains(t, strings.Join(got, " "), "model_reasoning_effort=")
				return
			}
			assert.Equal(t, 1, strings.Count(strings.Join(got, " "), assignment))
		})
	}
}

func TestCodexAdapterBuildCommand_InvalidSafeValuesFailClosed(t *testing.T) {
	t.Parallel()
	tests := []CodexTaskOptions{
		{RawTokenBudget: -1},
		{OutputSchema: "   "},
		{Sandbox: "danger-full-access"},
	}
	for _, options := range tests {
		got := NewCodexAdapter().BuildCommand(context.Background(), TaskConfig{Codex: options}).Args
		assert.Contains(t, got, "read-only")
		assert.NotContains(t, got, "--dangerously-bypass-approvals-and-sandbox")
		assert.NotContains(t, got, "danger-full-access")
		assert.NotContains(t, strings.Join(got, " "), "rollout_budget")
		assert.NotContains(t, got, "--output-schema")
	}
}

func TestCodexAdapterBuildCommand_ReadOnlyEvidenceSandbox(t *testing.T) {
	t.Parallel()
	got := NewCodexAdapter().BuildCommand(context.Background(), TaskConfig{
		Codex: CodexTaskOptions{Sandbox: CodexSandboxReadOnly},
	}).Args[1:]
	assert.Equal(t, []string{"exec", "--sandbox", "read-only", "--json"}, got)
}
