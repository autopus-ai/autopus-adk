package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/insajin/autopus-adk/pkg/telemetry"
)

// CodexAdapter implements ProviderAdapter for OpenAI Codex CLI.
type CodexAdapter struct{}

// NewCodexAdapter creates a new CodexAdapter.
func NewCodexAdapter() *CodexAdapter {
	return &CodexAdapter{}
}

// Name returns "codex".
func (a *CodexAdapter) Name() string { return "codex" }

// BuildCommand constructs the exec.Cmd for Codex CLI.
func (a *CodexAdapter) BuildCommand(ctx context.Context, task TaskConfig) *exec.Cmd {
	args := []string{"exec"}
	if task.Codex.evidenceSafe() {
		args = append(args, "--sandbox", safeCodexSandbox(task.Codex.Sandbox))
		args = appendCodexEvidenceFlags(args, task.Codex)
	} else {
		args = append(args, codexWorkerSandboxArgs(task.WorkDir)...)
	}
	if task.Prompt != "" {
		// Read the sensitive task prompt from stdin instead of exposing it
		// in the process argv where other local processes can inspect it.
		args = append(args, "-")
	}
	args = append(args, "--json")
	if effort, ok := codexReasoningEffort(task.Effort); ok {
		args = append(args, "-c", `model_reasoning_effort="`+effort+`"`)
	}
	if task.Codex.RawTokenBudget > 0 {
		args = append(args, "-c", codexRolloutBudget(task.Codex.RawTokenBudget))
	}

	if model, ok := codexModelOverride(task.Model); ok {
		args = append(args, "-m", model)
	} else if strings.TrimSpace(task.Model) != "" {
		if isCodexAccountUnsupportedModel(task.Model) {
			slog.Info("codex model override is not supported by the current account, using codex default model",
				"task_id", task.TaskID,
				"model", task.Model)
		} else {
			slog.Warn("model is not supported by codex provider, omitting explicit model override",
				"task_id", task.TaskID,
				"model", task.Model)
		}
	}

	if task.ComputerUse {
		slog.Warn("computer_use not supported by codex provider, ignoring",
			"task_id", task.TaskID)
	}

	cmd := exec.CommandContext(ctx, ResolveBinary("codex"), args...)
	cmd.Dir = task.WorkDir

	// Build environment: inherit current env plus task-specific vars.
	env := cmd.Environ()
	env = append(env, fmt.Sprintf("AUTOPUS_TASK_ID=%s", task.TaskID))
	for k, v := range task.EnvVars {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = EnvironWithToolPath(env)

	return cmd
}

func (o CodexTaskOptions) evidenceSafe() bool {
	return o.Sandbox != "" || o.Ephemeral || o.IgnoreUserConfig || o.IgnoreRules ||
		o.SkipGitRepoCheck || o.OutputSchema != "" ||
		o.ZeroToolMode || o.RawTokenBudget != 0
}

func safeCodexSandbox(mode CodexSandboxMode) string {
	if mode == CodexSandboxWorkspaceWrite {
		return string(CodexSandboxWorkspaceWrite)
	}
	return string(CodexSandboxReadOnly)
}

func appendCodexEvidenceFlags(args []string, options CodexTaskOptions) []string {
	flags := []struct {
		enabled bool
		value   string
	}{
		{options.Ephemeral, "--ephemeral"},
		{options.IgnoreUserConfig, "--ignore-user-config"},
		{options.IgnoreRules, "--ignore-rules"},
		{options.SkipGitRepoCheck, "--skip-git-repo-check"},
	}
	for _, flag := range flags {
		if flag.enabled {
			args = append(args, flag.value)
		}
	}
	if schema := strings.TrimSpace(options.OutputSchema); schema != "" {
		args = append(args, "--output-schema", schema)
	}
	if options.ZeroToolMode {
		for _, feature := range []string{"multi_agent", "multi_agent_v2", "enable_fanout", "shell_tool", "apps", "hooks"} {
			args = append(args, "--disable", feature)
		}
	}
	return args
}

func codexReasoningEffort(effort string) (string, bool) {
	switch effort {
	case "low", "medium", "high", "xhigh", "max", "ultra":
		return effort, true
	default:
		return "", false
	}
}

func codexRolloutBudget(limit int64) string {
	return fmt.Sprintf("features.rollout_budget={enabled=true,limit_tokens=%d,reminder_at_remaining_tokens=[],sampling_token_weight=1.0,prefill_token_weight=1.0}", limit)
}

func codexModelOverride(model string) (string, bool) {
	model = strings.TrimSpace(model)
	if model == "" {
		return "", false
	}
	if provider, name, ok := strings.Cut(model, "/"); ok {
		if provider != "openai" || strings.TrimSpace(name) == "" || strings.Contains(name, "/") {
			return "", false
		}
		model = strings.TrimSpace(name)
	}
	if isCodexAccountUnsupportedModel(model) {
		return "", false
	}
	return model, true
}

func isCodexAccountUnsupportedModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if provider, name, ok := strings.Cut(model, "/"); ok && provider == "openai" {
		model = strings.TrimSpace(name)
	}
	return strings.Contains(model, "codex")
}

// codexResultEvent is the JSON structure of a Codex result line.
type codexResultEvent struct {
	Type      string  `json:"type"`
	Output    string  `json:"output,omitempty"`
	CostUSD   float64 `json:"cost_usd,omitempty"`
	SessionID string  `json:"session_id,omitempty"`
}

// ParseEvent parses a single line of Codex JSON output into a StreamEvent.
// Maps Codex's "tool_call" type to the canonical EventToolCall type.
func (a *CodexAdapter) ParseEvent(line []byte) (StreamEvent, error) {
	var raw struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(line, &raw); err != nil {
		return StreamEvent{}, fmt.Errorf("codex parse: %w", err)
	}
	if raw.Type == "" {
		return StreamEvent{}, fmt.Errorf("codex: missing type field")
	}

	if raw.Type == "item.completed" {
		var item struct {
			Item struct {
				Type string `json:"type"`
				Text string `json:"text,omitempty"`
			} `json:"item"`
		}
		if err := json.Unmarshal(line, &item); err != nil {
			return StreamEvent{}, fmt.Errorf("codex parse item.completed: %w", err)
		}
		if item.Item.Type == "agent_message" || item.Item.Type == "assistant_message" {
			synthetic, err := json.Marshal(codexResultEvent{
				Type:   "result",
				Output: item.Item.Text,
			})
			if err != nil {
				return StreamEvent{}, fmt.Errorf("codex synthesize result: %w", err)
			}
			return StreamEvent{
				Type: "result",
				Data: synthetic,
			}, nil
		}
		switch item.Item.Type {
		case "", "reasoning", "error":
		default:
			return StreamEvent{
				Type:      "tool_call",
				Subtype:   item.Item.Type,
				Data:      json.RawMessage(append([]byte(nil), line...)),
				ToolCalls: 1,
			}, nil
		}
	}

	typ, subtype := splitEventType(raw.Type)
	toolCalls := 0
	if typ == "tool_call" {
		toolCalls = 1
	}

	return StreamEvent{
		Type:      typ,
		Subtype:   subtype,
		Data:      json.RawMessage(append([]byte(nil), line...)),
		Usage:     parseCodexUsage(line, raw.Type),
		ToolCalls: toolCalls,
	}, nil
}

// ExtractResult extracts the final task result from a Codex result event.
func (a *CodexAdapter) ExtractResult(event StreamEvent) TaskResult {
	var re codexResultEvent
	if err := json.Unmarshal(event.Data, &re); err != nil {
		return TaskResult{Output: string(event.Data)}
	}
	return TaskResult{
		CostUSD:   re.CostUSD,
		SessionID: re.SessionID,
		Output:    re.Output,
		Usage:     append([]telemetry.UsageEnvelope(nil), event.Usage...),
		ToolCalls: event.ToolCalls,
	}
}

func parseCodexUsage(data []byte, eventType string) []telemetry.UsageEnvelope {
	var receipt struct {
		providerUsageIdentity
		CostUSD *float64 `json:"cost_usd"`
		Usage   *struct {
			InputTokens     *int64 `json:"input_tokens"`
			CachedTokens    *int64 `json:"cached_input_tokens"`
			OutputTokens    *int64 `json:"output_tokens"`
			ReasoningTokens *int64 `json:"reasoning_output_tokens"`
			ToolTokens      *int64 `json:"tool_tokens"`
			ToolRelation    string `json:"tool_tokens_relation"`
		} `json:"usage"`
	}
	if json.Unmarshal(data, &receipt) != nil {
		return nil
	}
	if receipt.Usage == nil && receipt.CostUSD == nil {
		return nil
	}
	if eventType != "turn.completed" && eventType != "result" {
		return nil
	}
	var input telemetry.UsageInput
	applyUsageIdentity(&input, receipt.providerUsageIdentity, "codex", "codex.exec-json.turn.completed.v1")
	input.ActualCostUSD = receipt.CostUSD
	if receipt.Usage != nil {
		input.InputTokensTotal = receipt.Usage.InputTokens
		input.CachedInputTokens = receipt.Usage.CachedTokens
		input.OutputTokensTotal = receipt.Usage.OutputTokens
		input.ReasoningTokens = receipt.Usage.ReasoningTokens
		if receipt.Usage.ReasoningTokens != nil {
			input.ReasoningRelation = telemetry.ComponentSubsetOfOutput
		}
		input.ToolTokens = receipt.Usage.ToolTokens
		input.ToolRelation = receipt.Usage.ToolRelation
	}
	return []telemetry.UsageEnvelope{normalizeProviderUsage(input)}
}
