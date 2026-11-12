package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/insajin/autopus-adk/pkg/telemetry"
	"github.com/insajin/autopus-adk/pkg/worker/stream"
)

// ClaudeAdapter implements ProviderAdapter for Claude Code CLI.
type ClaudeAdapter struct{}

// NewClaudeAdapter creates a new ClaudeAdapter.
func NewClaudeAdapter() *ClaudeAdapter {
	return &ClaudeAdapter{}
}

// Name returns "claude".
func (a *ClaudeAdapter) Name() string { return "claude" }

// @AX:ANCHOR [AUTO]: Claude CLI argv boundary; preserve separated flag/value ordering and the session-effort allowlist. @AX:SPEC SPEC-FABLE5-001
// @AX:REASON [AUTO]: ProviderAdapter consumers execute this command across agent-run, worker-loop, and pipeline paths.
// BuildCommand constructs the exec.Cmd for Claude Code with stream-json output.
func (a *ClaudeAdapter) BuildCommand(ctx context.Context, task TaskConfig) *exec.Cmd {
	sessionID := task.SessionID
	if sessionID == "" {
		sessionID = fmt.Sprintf("worker-sess-%s", task.TaskID)
	}

	args := []string{
		"--print",
		"--output-format", "stream-json",
		"--verbose",
		// No tool-call hook is installed: SecurityPolicy command fields do
		// not confine this task (see security.SecurityPolicy).
		"--dangerously-skip-permissions",
	}

	args = append(args, "--resume", sessionID)

	if task.MCPConfig != "" {
		args = append(args, "--mcp-config", task.MCPConfig)
	}

	if task.Model != "" {
		args = append(args, "--model", task.Model)
	}

	switch task.Effort {
	case "low", "medium", "high", "xhigh", "max", "ultracode":
		args = append(args, "--effort", task.Effort)
	}

	if task.ComputerUse {
		args = append(args, "--computer-use")
	}

	cmd := exec.CommandContext(ctx, ResolveBinary("claude"), args...)
	cmd.Dir = task.WorkDir

	// Build environment: inherit current env plus task-specific vars.
	env := cmd.Environ()
	env = append(env,
		fmt.Sprintf("AUTOPUS_TASK_ID=%s", task.TaskID),
		"CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1",
	)
	for k, v := range task.EnvVars {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = EnvironWithToolPath(env)

	return cmd
}

// ParseEvent parses a single line of stream-json output into a StreamEvent.
// Maps Claude's "tool_use" type to the canonical EventToolCall type.
func (a *ClaudeAdapter) ParseEvent(line []byte) (StreamEvent, error) {
	evt, err := stream.ParseLine(line)
	if err != nil {
		return StreamEvent{}, err
	}
	typ := evt.Type
	toolCalls := 0
	if typ == "tool_use" {
		typ = stream.EventToolCall
		toolCalls = 1
	}
	usage := parseClaudeUsage(evt.Raw)
	return StreamEvent{
		Type:      typ,
		Subtype:   evt.Subtype,
		Data:      evt.Raw,
		Usage:     usage,
		ToolCalls: toolCalls,
	}, nil
}

// ExtractResult extracts the final task result from a result event.
func (a *ClaudeAdapter) ExtractResult(event StreamEvent) TaskResult {
	var rd stream.ResultData
	if err := json.Unmarshal(event.Data, &rd); err != nil {
		return TaskResult{Output: string(event.Data)}
	}
	output := rd.Output
	if output == "" {
		output = rd.Result
	}
	var current struct {
		TotalCostUSD *float64 `json:"total_cost_usd"`
	}
	_ = json.Unmarshal(event.Data, &current)
	costUSD := rd.CostUSD
	if current.TotalCostUSD != nil {
		costUSD = *current.TotalCostUSD
	}
	result := TaskResult{
		CostUSD:    costUSD,
		DurationMS: rd.DurationMS,
		SessionID:  rd.SessionID,
		Output:     output,
		IsError:    rd.IsError,
		Usage:      append([]telemetry.UsageEnvelope(nil), event.Usage...),
		ToolCalls:  event.ToolCalls,
	}
	if rd.IsError {
		result.Error = strings.TrimSpace(output)
		if result.Error == "" {
			result.Error = "claude result marked as error"
		}
		if rd.APIErrorStatus > 0 {
			result.Error = fmt.Sprintf("claude api error %d: %s", rd.APIErrorStatus, result.Error)
		}
	}
	return result
}

func parseClaudeUsage(data []byte) []telemetry.UsageEnvelope {
	var receipt struct {
		providerUsageIdentity
		CostUSD      *float64 `json:"cost_usd"`
		TotalCostUSD *float64 `json:"total_cost_usd"`
		Usage        *struct {
			InputTokens         *int64 `json:"input_tokens"`
			CacheCreationTokens *int64 `json:"cache_creation_input_tokens"`
			CacheReadTokens     *int64 `json:"cache_read_input_tokens"`
			OutputTokens        *int64 `json:"output_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(data, &receipt) != nil {
		return nil
	}
	var input telemetry.UsageInput
	applyUsageIdentity(&input, receipt.providerUsageIdentity, "claude", "claude.stream-json.result.v1")
	input.ActualCostUSD = receipt.TotalCostUSD
	if input.ActualCostUSD == nil {
		input.ActualCostUSD = receipt.CostUSD
	}
	if receipt.Usage != nil {
		input.UncachedInputTokens = receipt.Usage.InputTokens
		input.CacheCreationInputTokens = receipt.Usage.CacheCreationTokens
		input.CacheReadInputTokens = receipt.Usage.CacheReadTokens
		input.OutputTokensTotal = receipt.Usage.OutputTokens
	}
	if receipt.Usage == nil && receipt.CostUSD == nil && receipt.TotalCostUSD == nil {
		return nil
	}
	return []telemetry.UsageEnvelope{normalizeProviderUsage(input)}
}
