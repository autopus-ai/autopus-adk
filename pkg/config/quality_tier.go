package config

import "strings"

// Canonical Claude slugs for the relative quality tiers. Every Claude-facing
// projection resolves through these so a tier promotion lands in one place.
const (
	ClaudeFableModel  = "claude-fable-5-1"
	ClaudeOpusModel   = "claude-opus-5"
	ClaudeSonnetModel = "claude-sonnet-5"
	ClaudeHaikuModel  = "claude-haiku-4-5"
)

// canonicalAgentNames lists the generated source agents in stable order. This
// is the authoritative role set: quality presets, Claude frontmatter, cost
// accounting, and the OMP role matrix all key off exactly these names.
var canonicalAgentNames = []string{
	"annotator", "architect", "debugger", "deep-worker", "devops", "executor",
	"explorer", "frontend-specialist", "perf-engineer", "planner", "reviewer",
	"security-auditor", "spec-writer", "tester", "ux-validator", "validator",
}

// agentNameAliases folds workflow phase-role spelling onto agent file names.
// Workflow schemas use underscores; agent files and quality presets use hyphens.
var agentNameAliases = map[string]string{
	"security_auditor":    "security-auditor",
	"spec_writer":         "spec-writer",
	"deep_worker":         "deep-worker",
	"frontend_specialist": "frontend-specialist",
	"perf_engineer":       "perf-engineer",
	"ux_validator":        "ux-validator",
}

// CanonicalAgentNames returns a detached copy of the authoritative role set.
func CanonicalAgentNames() []string {
	return append([]string(nil), canonicalAgentNames...)
}

// NormalizeAgentName maps a role spelling onto its canonical agent name.
// Unknown names are returned lowercased and otherwise unchanged so callers can
// still look them up in a custom preset.
func NormalizeAgentName(raw string) string {
	name := strings.ToLower(strings.TrimSpace(raw))
	if canonical, ok := agentNameAliases[name]; ok {
		return canonical
	}
	return name
}

// qualityTiers is the relative tier vocabulary every preset is written in,
// ordered strongest first. Each provider adapter projects it onto its own
// model ids; nothing outside this file decides what a tier name may be.
var qualityTiers = []string{"fable", "opus", "sonnet", "haiku"}

// QualityTiers returns a detached copy of the tier vocabulary, strongest first.
func QualityTiers() []string {
	return append([]string(nil), qualityTiers...)
}

// NormalizeQualityTier folds an operator-typed tier onto its canonical
// spelling. The second result is false for anything outside the vocabulary, so
// a caller persists a tier only after the name is known.
func NormalizeQualityTier(raw string) (string, bool) {
	tier := strings.ToLower(strings.TrimSpace(raw))
	for _, known := range qualityTiers {
		if tier == known {
			return tier, true
		}
	}
	return "", false
}

// AgentTier resolves the relative tier (fable, opus, sonnet, or haiku) for one
// agent under a provider's effective quality view. This is the single tier
// decision; provider adapters project it onto their own model vocabulary.
func (q QualityConf) AgentTier(provider, agentName, fallbackTier string) string {
	name := NormalizeAgentName(agentName)
	preset, ok := q.Presets[q.ForProvider(provider).Default]
	if !ok {
		preset, ok = q.Presets["balanced"]
	}
	if ok {
		if tier, valid := NormalizeQualityTier(preset.Agents[name]); valid {
			return tier
		}
	}
	if tier, valid := NormalizeQualityTier(fallbackTier); valid {
		return tier
	}
	return "sonnet"
}

// ClaudeAgentModel projects the resolved tier onto a Claude model slug.
func (q QualityConf) ClaudeAgentModel(agentName, fallbackTier string) string {
	if candidate, standard := q.NativeBalancedAgentCandidate(QualityProviderClaude, agentName); standard {
		return strings.TrimPrefix(candidate.Selector, "anthropic/")
	}
	return ClaudeModelForTier(q.AgentTier(QualityProviderClaude, agentName, fallbackTier))
}

// ClaudeModelForTier maps a relative tier onto its Claude slug. An unknown tier
// resolves to the mid tier, matching AgentTier's own fallback.
func ClaudeModelForTier(tier string) string {
	switch tier {
	case "fable":
		return ClaudeFableModel
	case "opus":
		return ClaudeOpusModel
	case "haiku":
		return ClaudeHaikuModel
	default:
		return ClaudeSonnetModel
	}
}
