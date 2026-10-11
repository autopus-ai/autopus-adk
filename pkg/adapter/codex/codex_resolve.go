// Pure Codex profile resolution over one catalog snapshot. The agent templates
// and the quality composition table both call these functions, so a table
// cell and the generated TOML cannot disagree when they share a snapshot.
package codex

import "github.com/insajin/autopus-adk/pkg/config"

// ResolveAgentProfile resolves the managed Codex profile one agent template
// renders. snapshot is the `codex debug models` JSON the run probed once; nil
// or malformed JSON is an unreadable catalog. fallbackTier and declaredEffort
// are the literal arguments the agent template passes.
//
// The standard native balanced placement is a routing decision rather than a
// preference: answering it with a lower model or effort would hand the agent a
// weaker rung than the policy names, so that path refuses substitution with
// ErrCodexNativePlacementUnavailable. An unreadable catalog keeps the placement
// verbatim and reports it as unverified: Fallback is false while Reason stays
// config.CodexResolutionCatalogUnknown and CatalogError says why. Every other
// profile — Ultra, a hand-edited tier, a non-canonical agent — keeps the
// ordered availability fallback of config.ResolveCodexProfile.
func ResolveAgentProfile(
	quality config.QualityConf,
	snapshot []byte,
	agentName, fallbackTier, declaredEffort string,
) (config.CodexProfileResolution, error) {
	requested := quality.CodexAgentProfile(agentName, fallbackTier, declaredEffort)
	if _, placed := quality.NativeBalancedAgentCandidate(config.QualityProviderCodex, agentName); placed {
		return resolveCodexPlacement(requested, snapshot)
	}
	return config.ResolveCodexProfile(requested, snapshot), nil
}

// ResolveSupervisorProfile resolves the managed root session profile against
// snapshot. The boolean reports whether quality manages the root model at all;
// when it does not, generation writes no root model and the resolution is zero.
func ResolveSupervisorProfile(
	quality config.QualityConf,
	snapshot []byte,
) (config.CodexProfileResolution, bool) {
	if !quality.ManagesSupervisorModel() {
		return config.CodexProfileResolution{}, false
	}
	return config.ResolveCodexProfile(quality.CodexSupervisorProfile(), snapshot), true
}

// codexPlacementUnverified reports a native placement kept without catalog
// confirmation. config.ResolveCodexProfile always marks an unreadable catalog
// as a fallback, so only resolveCodexPlacement produces this shape.
func codexPlacementUnverified(resolution config.CodexProfileResolution) bool {
	return !resolution.Fallback && resolution.Reason == config.CodexResolutionCatalogUnknown
}
