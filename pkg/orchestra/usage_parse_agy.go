package orchestra

import (
	"github.com/insajin/autopus-adk/pkg/telemetry"
)

// parseAgyUsage reads the usage of agy's terminal result, from a stream-json
// result event or a json result object. Per-step usage is not summed: the
// result already totals the turns. Text output carries no usage.
func parseAgyUsage(raw string, binding usageBinding) []telemetry.UsageEnvelope {
	var counts *agyUsageCounts
	forEachJSONLine(raw, func(line []byte) {
		for _, kind := range []providerEnvelope{envelopeAgyStream, envelopeAgyJSON} {
			if result, _, ok := terminalEnvelopeLine(kind, line); ok && result.Usage != nil {
				counts = result.Usage
				return
			}
		}
	})
	if counts == nil {
		return nil
	}
	resolved := resolveUsageIdentity(providerUsageIdentity{}, binding)
	return []telemetry.UsageEnvelope{telemetry.NormalizeUsage(telemetry.UsageInput{
		RunID: resolved.RunID, CallID: resolved.CallID, TaskID: resolved.TaskID,
		Attempt: resolved.Attempt, Provider: resolved.Provider, Model: resolved.Model,
		Effort: resolved.Effort, Phase: resolved.Phase, Role: resolved.Role,
		Source: telemetry.UsageSourceProvider, SourceSchema: "agy.result.v1",
		InputTokensTotal: counts.InputTokens, CacheReadInputTokens: counts.CacheReadTokens,
		OutputTokensTotal: counts.OutputTokens, ReasoningTokens: counts.ThinkingTokens,
		ReasoningRelation: agyThinkingRelation(counts),
	})}
}

// agyThinkingRelation derives how thinking tokens relate to output from agy's
// own total: agy 1.5.0 reports total = input + output, so thinking is part of
// output. A total that also adds thinking makes it separate; any other total
// leaves the relation unknown, and normalization then refuses to guess.
func agyThinkingRelation(counts *agyUsageCounts) string {
	if counts.ThinkingTokens == nil {
		return ""
	}
	if counts.TotalTokens == nil || counts.InputTokens == nil || counts.OutputTokens == nil {
		return telemetry.ComponentSubsetOfOutput
	}
	base := *counts.InputTokens + *counts.OutputTokens
	switch *counts.TotalTokens {
	case base:
		return telemetry.ComponentSubsetOfOutput
	case base + *counts.ThinkingTokens:
		return telemetry.ComponentSeparate
	}
	return ""
}
