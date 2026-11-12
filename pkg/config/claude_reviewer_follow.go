package config

import "slices"

// The Claude review provider (orchestra.providers.claude) defaults to the
// model the reviewer agent runs with under the effective Claude Code preset:
// balanced reviews on Opus, ultra on Fable. One preset choice then decides
// both, and the "review model differs" warning appears only when an operator
// diverges on purpose.

// ClaudeReviewerProfile returns the Claude model id and effort the reviewer
// agent runs with under q, as the Claude generator projects it. A relative
// tier the generator writes as an alias is spelled as its model id, since the
// provider argv names an exact model.
func (q QualityConf) ClaudeReviewerProfile() (model, effort string) {
	model, effort = q.ClaudeAgentProfile("reviewer", "opus")
	if tier, ok := NormalizeQualityTier(model); ok {
		model = ClaudeModelForTier(tier)
	}
	return model, effort
}

// ClaudeReviewFollow is one move of the claude review provider onto the
// reviewer's model: the model and effort before and after.
type ClaudeReviewFollow struct {
	FromModel, FromEffort string
	ToModel, ToEffort     string
}

// ClaudeReviewFollowFor reports the move that puts an ADK-written claude
// review provider on the reviewer's model under the effective quality, or
// false when there is none: the provider already follows the reviewer, is
// backend-routed, pinned, or wrapped, or names a model ADK never wrote, which
// is the operator's choice. ADK-written means the argv tail is the current
// shipped default, a former one, or what a built-in preset's reviewer gives.
func ClaudeReviewFollowFor(cfg *HarnessConfig) (ClaudeReviewFollow, bool) {
	if cfg == nil {
		return ClaudeReviewFollow{}, false
	}
	entry, ok := cfg.Orchestra.Providers["claude"]
	if !ok {
		return ClaudeReviewFollow{}, false
	}
	model, effort, ok := ownedClaudeReviewPair(entry, cfg.Quality)
	if !ok {
		return ClaudeReviewFollow{}, false
	}
	toModel, toEffort := cfg.Quality.ClaudeReviewerProfile()
	if toEffort == "" || model == toModel && effort == toEffort {
		return ClaudeReviewFollow{}, false
	}
	return ClaudeReviewFollow{FromModel: model, FromEffort: effort, ToModel: toModel, ToEffort: toEffort}, true
}

// ApplyClaudeReviewFollow moves the claude review provider in cfg onto the
// reviewer's model when ClaudeReviewFollowFor reports a move, keeping every
// other argument, and returns that move.
func ApplyClaudeReviewFollow(cfg *HarnessConfig) (ClaudeReviewFollow, bool) {
	follow, ok := ClaudeReviewFollowFor(cfg)
	if !ok {
		return follow, false
	}
	entry := cfg.Orchestra.Providers["claude"]
	args := slices.Clone(entry.Args)
	args[2], args[4] = follow.ToModel, follow.ToEffort
	entry.Args = args
	cfg.Orchestra.Providers["claude"] = entry
	return follow, true
}

// ownedClaudeReviewPair returns the model and effort of a claude CLI entry
// whose argv is `<print> --model M --effort E [--output-format json]` with a
// pair ADK wrote.
func ownedClaudeReviewPair(entry ProviderEntry, quality QualityConf) (string, string, bool) {
	if entry.Backend != "" || entry.ModelPolicy == ProviderModelPolicyPinned || entry.Binary != "claude" {
		return "", "", false
	}
	args := entry.Args
	if len(args) < 5 || !slices.Contains(claudePrintFlags, args[0]) ||
		args[1] != "--model" || args[3] != "--effort" ||
		len(withoutDefaultClaudeOutputFormat(args[1:])) != 4 {
		return "", "", false
	}
	pair := []string{"--model", args[2], "--effort", args[4]}
	owned := append([][]string{claudeDefaultModelArgs()}, historicalClaudeDefaultModelArgs...)
	for _, preset := range []string{"ultra", "balanced"} {
		model, effort := quality.WithGlobalOverride(preset).ClaudeReviewerProfile()
		owned = append(owned, []string{"--model", model, "--effort", effort})
	}
	if !slices.ContainsFunc(owned, func(shipped []string) bool { return slices.Equal(shipped, pair) }) {
		return "", "", false
	}
	return args[2], args[4], true
}
