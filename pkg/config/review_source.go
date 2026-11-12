package config

// Multi-CLI review has one source of truth (SPEC decision recorded in the
// model-selection redesign):
//
//   - models: orchestra.providers.<name> (args, model, backend) — the only
//     place a review CLI's model lives;
//   - which CLIs review: spec.review_gate.providers;
//   - who judges: spec.review_gate.judge (and judge_model/judge_effort).
//
// `auto orchestra review` and `auto spec review` both read the review gate's
// list and judge. orchestra.commands.review.providers and .judge stay valid
// as explicit overrides an operator writes, and orchestra.judge stays the
// judge of the other orchestra commands.

// ReviewDerivedProvidersKey is the key whose ADK-written value the update
// migration drops so review follows the review gate's list.
const ReviewDerivedProvidersKey = "orchestra.commands.review.providers"

// ReviewGateOrchestraView returns the orchestra config `auto orchestra review`
// runs with: when orchestra.commands.review names no providers or no judge
// and the review gate is enabled, they come from the review gate. The
// caller's config is not changed.
func ReviewGateOrchestraView(cfg *HarnessConfig) *OrchestraConf {
	view := cfg.Orchestra
	gate := cfg.Spec.ReviewGate
	if !gate.Enabled {
		return &view
	}
	command := view.Commands["review"]
	changed := false
	if len(command.Providers) == 0 && len(gate.Providers) > 0 {
		command.Providers = append([]string(nil), gate.Providers...)
		changed = true
	}
	if command.Judge == "" && gate.Judge != "" {
		command.Judge = gate.Judge
		changed = true
	}
	if changed {
		commands := make(map[string]CommandEntry, len(view.Commands)+1)
		for name, entry := range view.Commands {
			commands[name] = entry
		}
		commands["review"] = command
		view.Commands = commands
	}
	return &view
}

// ReviewProvidersDerivable reports an orchestra.commands.review.providers
// value ADK wrote, the shipped list or every configured provider, that the
// review gate's list can replace: the gate is enabled and names providers.
// A list the operator narrowed or reordered past that is theirs.
func ReviewProvidersDerivable(cfg *HarnessConfig) bool {
	if cfg == nil || !cfg.Spec.ReviewGate.Enabled || len(cfg.Spec.ReviewGate.Providers) == 0 {
		return false
	}
	command, ok := cfg.Orchestra.Commands["review"]
	if !ok || len(command.Providers) == 0 {
		return false
	}
	providers := make([]string, 0, len(cfg.Orchestra.Providers))
	for name := range cfg.Orchestra.Providers {
		providers = append(providers, name)
	}
	return sameProviderSet(command.Providers, shippedOrchestraCommandProviders) ||
		sameProviderSet(command.Providers, providers)
}

// DeriveReviewProviders clears an ADK-written review command provider list
// (ReviewProvidersDerivable) so review follows the review gate's list.
func DeriveReviewProviders(cfg *HarnessConfig) bool {
	if !ReviewProvidersDerivable(cfg) {
		return false
	}
	command := cfg.Orchestra.Commands["review"]
	command.Providers = nil
	cfg.Orchestra.Commands["review"] = command
	return true
}
