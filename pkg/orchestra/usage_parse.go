package orchestra

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"

	"github.com/insajin/autopus-adk/pkg/telemetry"
)

type providerUsageIdentity struct {
	Type    string `json:"type"`
	RunID   string `json:"run_id"`
	CallID  string `json:"call_id"`
	TaskID  string `json:"task_id"`
	Model   string `json:"model"`
	Effort  string `json:"effort"`
	Phase   string `json:"phase"`
	Role    string `json:"role"`
	Attempt int    `json:"attempt"`
}

type codexUsageEvent struct {
	providerUsageIdentity
	Usage *struct {
		InputTokens           *int64 `json:"input_tokens"`
		CachedInputTokens     *int64 `json:"cached_input_tokens"`
		OutputTokens          *int64 `json:"output_tokens"`
		ReasoningOutputTokens *int64 `json:"reasoning_output_tokens"`
		ReasoningTokens       *int64 `json:"reasoning_tokens"`
		ToolTokens            *int64 `json:"tool_tokens"`
		ToolRelation          string `json:"tool_relation"`
	} `json:"usage"`
}

type claudeUsageEvent struct {
	providerUsageIdentity
	CostUSD      *float64 `json:"cost_usd"`
	TotalCostUSD *float64 `json:"total_cost_usd"`
	Usage        *struct {
		InputTokens         *int64 `json:"input_tokens"`
		CacheCreationTokens *int64 `json:"cache_creation_input_tokens"`
		CacheReadTokens     *int64 `json:"cache_read_input_tokens"`
		OutputTokens        *int64 `json:"output_tokens"`
		OutputDetails       *struct {
			ThinkingTokens *int64 `json:"thinking_tokens"`
		} `json:"output_tokens_details"`
	} `json:"usage"`
	ModelUsage map[string]claudeModelUsage `json:"modelUsage"`
}

// parseCodexUsage extracts only allowlisted usage and identity fields. Provider
// prompts, response bodies, and arbitrary event metadata are never retained.
func parseCodexUsage(raw string, binding usageBinding) []telemetry.UsageEnvelope {
	var receipts []telemetry.UsageEnvelope
	forEachJSONLine(raw, func(line []byte) {
		var event codexUsageEvent
		if json.Unmarshal(line, &event) == nil && event.Type == "turn.completed" && event.Usage != nil {
			receipts = append(receipts, normalizeCodexUsage(event, binding))
		}
	})
	return receipts
}

func parseClaudeUsage(raw string, binding usageBinding) []telemetry.UsageEnvelope {
	var receipts []telemetry.UsageEnvelope
	forEachJSONLine(raw, func(line []byte) {
		var event claudeUsageEvent
		if json.Unmarshal(line, &event) != nil || event.Type != "result" ||
			(event.Usage == nil && len(event.ModelUsage) == 0 && event.TotalCostUSD == nil && event.CostUSD == nil) {
			return
		}
		resolved := resolveUsageIdentity(event.providerUsageIdentity, binding)
		actualCost := event.TotalCostUSD
		if actualCost == nil {
			actualCost = event.CostUSD
		}
		counts, ok := sumClaudeModelUsage(event.ModelUsage)
		if !ok {
			counts = claudeTopLevelUsage(event)
		}
		relation := ""
		if counts.thinking != nil {
			// Anthropic output_tokens already include thinking tokens.
			relation = telemetry.ComponentSubsetOfOutput
		}
		receipts = append(receipts, telemetry.NormalizeUsage(telemetry.UsageInput{
			RunID: resolved.RunID, CallID: resolved.CallID, TaskID: resolved.TaskID,
			Attempt: resolved.Attempt, Provider: resolved.Provider, Model: resolved.Model,
			Effort: resolved.Effort, Phase: resolved.Phase, Role: resolved.Role,
			Source: telemetry.UsageSourceProvider, SourceSchema: "claude.result.v1",
			UncachedInputTokens: counts.input, CacheCreationInputTokens: counts.cacheCreation,
			CacheReadInputTokens: counts.cacheRead, OutputTokensTotal: counts.output,
			ReasoningTokens: counts.thinking, ReasoningRelation: relation, ActualCostUSD: actualCost,
		}))
	})
	return receipts
}

func normalizeCodexUsage(event codexUsageEvent, binding usageBinding) telemetry.UsageEnvelope {
	resolved := resolveUsageIdentity(event.providerUsageIdentity, binding)
	reasoning := event.Usage.ReasoningOutputTokens
	reasoningRelation := ""
	if reasoning != nil {
		reasoningRelation = telemetry.ComponentSubsetOfOutput
	} else if event.Usage.ReasoningTokens != nil {
		reasoning = event.Usage.ReasoningTokens
	}
	return telemetry.NormalizeUsage(telemetry.UsageInput{
		RunID: resolved.RunID, CallID: resolved.CallID, TaskID: resolved.TaskID,
		Attempt: resolved.Attempt, Provider: resolved.Provider, Model: resolved.Model,
		Effort: resolved.Effort, Phase: resolved.Phase, Role: resolved.Role,
		Source: telemetry.UsageSourceProvider, SourceSchema: "codex.turn.completed.v1",
		InputTokensTotal: event.Usage.InputTokens, CachedInputTokens: event.Usage.CachedInputTokens,
		OutputTokensTotal: event.Usage.OutputTokens, ReasoningTokens: reasoning,
		ReasoningRelation: reasoningRelation, ToolTokens: event.Usage.ToolTokens,
		ToolRelation: safeRelation(event.Usage.ToolRelation),
	})
}

func resolveUsageIdentity(event providerUsageIdentity, fallback usageBinding) usageBinding {
	// Provider output is untrusted usage data. Supervisor-bound identity and
	// policy metadata are authoritative and must never be overwritten here.
	_ = event
	return fallback
}

// forEachJSONLine visits every JSON line of raw. A line that is a JSON array
// visits each element instead: `claude --print --output-format json
// --verbose` prints every message of the call as one array on one line.
func forEachJSONLine(raw string, visit func([]byte)) {
	reader := bufio.NewReader(strings.NewReader(raw))
	for {
		line, err := reader.ReadString('\n')
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			var events []json.RawMessage
			if strings.HasPrefix(trimmed, "[") && json.Unmarshal([]byte(trimmed), &events) == nil {
				for _, event := range events {
					visit(event)
				}
			} else {
				visit([]byte(line))
			}
		}
		if err != nil {
			if err != io.EOF {
				return
			}
			return
		}
	}
}

func safeRelation(value string) string {
	switch value {
	case telemetry.ComponentSubsetOfInput, telemetry.ComponentSubsetOfOutput, telemetry.ComponentSeparate:
		return value
	default:
		return ""
	}
}
