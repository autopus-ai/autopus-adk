package config

import (
	"slices"
	"strings"
)

// The Claude review provider (orchestra.providers.claude) and the Claude Code
// quality preset are separate settings: the preset decides the agents'
// frontmatter models, the provider argv decides the model review, plan, and
// secure run with. These helpers let a surface that shows one name the other.

// ClaudeProviderModel returns the model a CLI Claude provider passes with
// `--model`, in either the `--model X` or the `--model=X` spelling. A
// backend-routed entry, an argv without the flag, or a dangling flag has none.
func ClaudeProviderModel(entry ProviderEntry) (string, bool) {
	if entry.Backend != "" {
		return "", false
	}
	for i, arg := range entry.Args {
		if value, found := strings.CutPrefix(arg, "--model="); found && value != "" {
			return value, true
		}
		if arg == "--model" && i+1 < len(entry.Args) && entry.Args[i+1] != "" {
			return entry.Args[i+1], true
		}
	}
	return "", false
}

// ClaudeModelTier folds a Claude model alias (`opus`, `opus[1m]`) or slug
// (`claude-opus-5-5`, `claude-fable-5-1`) onto its relative quality tier. A
// name outside the vocabulary has no tier.
func ClaudeModelTier(model string) (string, bool) {
	name := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(model)), "claude-")
	for _, tier := range qualityTiers {
		if name == tier || strings.HasPrefix(name, tier+"-") || strings.HasPrefix(name, tier+"[") {
			return tier, true
		}
	}
	return "", false
}

// ClaudeTopTier returns the strongest tier any canonical agent gets under the
// effective Claude Code preset, as the Claude generator projects it, or ""
// when no agent resolves to a known tier.
func (q QualityConf) ClaudeTopTier() string {
	top := -1
	for _, agent := range canonicalAgentNames {
		model, _ := q.ClaudeAgentProfile(agent, "sonnet")
		tier, ok := ClaudeModelTier(model)
		if !ok {
			continue
		}
		if rank := slices.Index(qualityTiers, tier); top < 0 || rank < top {
			top = rank
		}
	}
	if top < 0 {
		return ""
	}
	return qualityTiers[top]
}
