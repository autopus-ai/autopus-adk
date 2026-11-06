package orchestra

// claudeModelUsage is one entry of the result's modelUsage map. Claude Code
// totals every model the call used here, subagents included, which the
// top-level usage of the main model leaves out.
type claudeModelUsage struct {
	InputTokens              *int64 `json:"inputTokens"`
	OutputTokens             *int64 `json:"outputTokens"`
	CacheReadInputTokens     *int64 `json:"cacheReadInputTokens"`
	CacheCreationInputTokens *int64 `json:"cacheCreationInputTokens"`
	ThinkingTokens           *int64 `json:"thinkingTokens"`
}

// claudeUsageCounts are the token counters one claude result contributes.
type claudeUsageCounts struct {
	input, cacheCreation, cacheRead, output, thinking *int64
}

// sumClaudeModelUsage totals modelUsage across models. It reports false when
// the map is empty or any model lacks its input or output count, so a partial
// map never understates the call; the caller then reads the top-level usage.
func sumClaudeModelUsage(models map[string]claudeModelUsage) (claudeUsageCounts, bool) {
	if len(models) == 0 {
		return claudeUsageCounts{}, false
	}
	var counts claudeUsageCounts
	for _, model := range models {
		if model.InputTokens == nil || model.OutputTokens == nil {
			return claudeUsageCounts{}, false
		}
		counts.input = addCount(counts.input, model.InputTokens)
		counts.output = addCount(counts.output, model.OutputTokens)
		counts.cacheRead = addCount(counts.cacheRead, model.CacheReadInputTokens)
		counts.cacheCreation = addCount(counts.cacheCreation, model.CacheCreationInputTokens)
		counts.thinking = addCount(counts.thinking, model.ThinkingTokens)
	}
	return counts, true
}

func claudeTopLevelUsage(event claudeUsageEvent) claudeUsageCounts {
	if event.Usage == nil {
		return claudeUsageCounts{}
	}
	counts := claudeUsageCounts{
		input: event.Usage.InputTokens, cacheCreation: event.Usage.CacheCreationTokens,
		cacheRead: event.Usage.CacheReadTokens, output: event.Usage.OutputTokens,
	}
	if event.Usage.OutputDetails != nil {
		counts.thinking = event.Usage.OutputDetails.ThinkingTokens
	}
	return counts
}

// addCount adds an optional counter: absent on both sides stays absent.
func addCount(total, value *int64) *int64 {
	if value == nil {
		return total
	}
	sum := *value
	if total != nil {
		sum += *total
	}
	return &sum
}
