package config

import "strings"

// EffectiveAgentTier returns the relative tier the native generators project
// for one agent under the global quality view (provider overrides cleared, the
// same view as ForProvider("")): the balanced rung tier wherever the standard
// balanced placement applies, otherwise AgentTier with the fallback tier the
// agent's embedded source declares. A column view the caller already resolved
// with ForProvider keeps its own mode, because that view carries no overrides.
func (q QualityConf) EffectiveAgentTier(agent, fallbackTier string) string {
	view := q.ForProvider("")
	// Without overrides both native providers share one mode, so the Claude
	// probe answers for the rung decision alone; the family is irrelevant here.
	if _, standard := view.NativeBalancedAgentCandidate(QualityProviderClaude, agent); standard {
		return balancedTierForRung(balancedRungByAgent[NormalizeAgentName(agent)])
	}
	return view.AgentTier("", agent, fallbackTier)
}

// ClaudeAgentProfile resolves one agent's Claude frontmatter model and
// reasoning effort exactly as the Claude generator writes them. The standard
// balanced placement addresses exact model ids so Claude Code routes the top
// rung to the frontier model instead of whichever release the tier alias
// currently points at; every other preset — Ultra, a custom preset, or a
// per-agent tier the user moved off the standard rung — keeps projecting the
// relative tier alias.
func (q QualityConf) ClaudeAgentProfile(agent, fallbackTier string) (model, effort string) {
	if candidate, ok := q.NativeBalancedAgentCandidate(QualityProviderClaude, agent); ok {
		return nativeClaudeModelID(candidate.Selector), candidate.Thinking
	}
	tier := q.AgentTier(QualityProviderClaude, agent, fallbackTier)
	return tier, ClaudeAgentEffort(q.EffectiveMode(QualityProviderClaude), tier)
}

// nativeClaudeModelID drops the routing provider prefix from a role model
// selector, leaving the model id the Claude CLI itself accepts.
func nativeClaudeModelID(selector string) string {
	if _, model, found := strings.Cut(selector, "/"); found {
		return model
	}
	return selector
}

// ClaudeAgentEffort maps a quality mode and relative tier onto Claude's
// reasoning effort. Haiku takes no effort line, and only ultra raises the
// opus and sonnet rungs.
func ClaudeAgentEffort(mode, tier string) string {
	switch tier {
	case "haiku":
		return ""
	case "fable":
		return "max"
	case "opus":
		if mode == "ultra" {
			return "max"
		}
		return "high"
	default:
		if mode == "ultra" {
			return "high"
		}
		return "medium"
	}
}
